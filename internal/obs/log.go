package obs

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	logDirMode  os.FileMode = 0o700
	logFileMode os.FileMode = 0o600

	// noFileSentinel disables the file sink. Reserved for tests that prefer
	// to verify records via Extras (in-memory capture handler) instead of
	// reading from disk. Production callers leave Config.File empty so
	// Build falls back to DefaultLogPath.
	noFileSentinel = "-"
)

// Config configures Build. Zero value yields a logger that writes JSON to
// the OS default cache file (DefaultLogPath) at info level.
type Config struct {
	// Level is the minimum log level: "debug", "info", "warn", or "error".
	// Empty or unrecognized values fall back to "info".
	Level string
	// File is the path to the JSON log file. Empty falls back to
	// DefaultLogPath. The sentinel "-" disables the file sink and is
	// intended for tests that observe records via Extras.
	File string
	// Extras is a list of additional handlers fanned alongside the file
	// sink. The simulator does not use this in production today; tests
	// pass a capture handler to assert on emitted records, and a future
	// GUI Wails-event bridge can plug in here without touching frontends.
	Extras []slog.Handler
}

// State owns the live handler stack so callers can hot-reload Level / File
// without re-injecting the *slog.Logger into every component.
type State struct {
	levelVar *slog.LevelVar
	proxy    *atomicHandler

	// base records the Extras list installed at Build time so Apply can
	// preserve them across reloads without callers re-supplying.
	base Config

	mu      sync.Mutex
	closers []io.Closer
}

// Build constructs a *slog.Logger that writes JSON to the configured file
// (default: DefaultLogPath) plus any Extras. The returned logger is stable
// across State.Apply calls — components hold it once and the State swaps
// the underlying sink stack transparently on hot-reload.
//
// Callers must call State.Close on shutdown to flush the open file.
func Build(cfg Config) (*slog.Logger, *State, error) {
	levelVar := new(slog.LevelVar)
	s := &State{
		levelVar: levelVar,
		proxy:    newAtomicHandler(levelVar),
		base:     Config{Extras: cfg.Extras},
	}
	s.levelVar.Set(ParseLevel(cfg.Level))
	if err := s.applyInternal(cfg); err != nil {
		return nil, nil, err
	}
	return slog.New(s.proxy), s, nil
}

// Apply rebuilds the sink stack with new Level / File values. Extras
// installed at Build time are preserved.
func (s *State) Apply(cfg Config) error {
	cfg.Extras = s.base.Extras
	return s.applyInternal(cfg)
}

// applyInternal is the shared implementation behind Build and Apply.
func (s *State) applyInternal(cfg Config) error {
	s.levelVar.Set(ParseLevel(cfg.Level))

	var sinks []slog.Handler
	var newClosers []io.Closer

	file := strings.TrimSpace(cfg.File)
	if file == "" {
		defaultPath, err := DefaultLogPath()
		if err != nil {
			return fmt.Errorf("obs: resolve default log path: %w", err)
		}
		file = defaultPath
	}
	if file != noFileSentinel {
		f, err := openLogFile(file)
		if err != nil {
			return err
		}
		sinks = append(sinks, slog.NewJSONHandler(f, &slog.HandlerOptions{Level: s.levelVar}))
		newClosers = append(newClosers, f)
	}

	sinks = append(sinks, cfg.Extras...)

	var combined slog.Handler
	switch len(sinks) {
	case 0:
		combined = slog.DiscardHandler
	case 1:
		combined = sinks[0]
	default:
		combined = newMultiHandler(sinks...)
	}

	s.proxy.swap(combined)

	s.mu.Lock()
	prev := s.closers
	s.closers = newClosers
	s.mu.Unlock()

	return closeAll(prev)
}

// SetLevel updates the live log level without rebuilding the sink stack.
func (s *State) SetLevel(level slog.Level) {
	s.levelVar.Set(level)
}

// Level returns the current log level.
func (s *State) Level() slog.Level {
	return s.levelVar.Level()
}

// Close releases any resources owned by the active sinks (open log file).
// Safe to call multiple times.
func (s *State) Close() error {
	s.mu.Lock()
	cs := s.closers
	s.closers = nil
	s.mu.Unlock()
	return closeAll(cs)
}

// ParseLevel maps a textual level name to slog.Level, returning slog.LevelInfo
// for empty / unrecognized input. Accepts canonical and common variants
// ("warning", "WARN", " Debug ").
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// IsValidLevel reports whether s is a recognized level name. Empty is treated
// as valid (means "use default"). Used by config validation.
func IsValidLevel(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "debug", "info", "warn", "warning", "error":
		return true
	default:
		return false
	}
}

// Discard returns a logger whose output is silently dropped. Use as a default
// for components that accept a logger but were constructed without one.
func Discard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// DefaultLogPath returns the OS-standard cache location for the simulator's
// log file:
//
//	macOS:   ~/Library/Caches/onvif-simulator/onvif-simulator.log
//	Linux:   $XDG_CACHE_HOME/onvif-simulator/onvif-simulator.log
//	Windows: %LocalAppData%\onvif-simulator\onvif-simulator.log
//
// Used as the default destination when Config.File is empty.
func DefaultLogPath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("obs: resolve user cache dir: %w", err)
	}
	return filepath.Join(base, "onvif-simulator", "onvif-simulator.log"), nil
}

func openLogFile(path string) (*os.File, error) {
	cleaned := filepath.Clean(path)
	if dir := filepath.Dir(cleaned); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, logDirMode); err != nil {
			return nil, fmt.Errorf("obs: mkdir %s: %w", dir, err)
		}
	}
	// path is operator-supplied via config; this matches config package
	// conventions for trusted on-disk paths.
	f, err := os.OpenFile(cleaned, os.O_APPEND|os.O_CREATE|os.O_WRONLY, logFileMode)
	if err != nil {
		return nil, fmt.Errorf("obs: open %s: %w", cleaned, err)
	}
	return f, nil
}

func closeAll(cs []io.Closer) error {
	var firstErr error
	for _, c := range cs {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
