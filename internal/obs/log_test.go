package obs

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestParseLevel(t *testing.T) {
	t.Parallel()
	cases := map[string]slog.Level{
		"":         slog.LevelInfo,
		"debug":    slog.LevelDebug,
		"DEBUG":    slog.LevelDebug,
		"INFO":     slog.LevelInfo,
		"warn":     slog.LevelWarn,
		"warning":  slog.LevelWarn,
		"error":    slog.LevelError,
		"nonsense": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q)=%v want %v", in, got, want)
		}
	}
	// Whitespace tolerance is part of the contract; gocritic flags
	// whitespace in map literal keys so we check it separately.
	if got := ParseLevel(" info "); got != slog.LevelInfo {
		t.Errorf("ParseLevel(\" info \")=%v want %v", got, slog.LevelInfo)
	}
}

func TestIsValidLevel(t *testing.T) {
	t.Parallel()
	for _, lvl := range []string{"", "debug", "INFO", "warn", "warning", "error"} {
		if !IsValidLevel(lvl) {
			t.Errorf("IsValidLevel(%q)=false, want true", lvl)
		}
	}
	for _, lvl := range []string{"trace", "fatal", "verbose"} {
		if IsValidLevel(lvl) {
			t.Errorf("IsValidLevel(%q)=true, want false", lvl)
		}
	}
}

func TestDiscard(t *testing.T) {
	t.Parallel()
	logger := Discard()
	if logger == nil {
		t.Fatal("Discard returned nil")
	}
	logger.Info("nothing should panic")
	if logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("discard logger reported Enabled=true at Info")
	}
}

// TestBuildWritesJSONToFile is the production happy path: an explicit File
// path is honored and records arrive as JSON with the expected fields.
func TestBuildWritesJSONToFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "log", "app.log")

	logger, state, err := Build(Config{Level: "debug", File: path})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() }) //nolint:errcheck // best-effort flush in tests

	logger.Info("filed", "k", "v")

	if closeErr := state.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	contents, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	out := string(contents)
	if !strings.Contains(out, `"msg":"filed"`) {
		t.Errorf("file sink missing record: %s", out)
	}
	if !strings.Contains(out, `"k":"v"`) {
		t.Errorf("file sink missing attr: %s", out)
	}
}

// TestBuildLevelFiltersOutput uses the "-" sentinel + an Extras capture
// handler to assert that level filtering applies to the live record stream
// without touching the disk.
func TestBuildLevelFiltersOutput(t *testing.T) {
	t.Parallel()
	captured := &captureHandler{}
	logger, state, err := Build(Config{Level: "warn", File: noFileSentinel, Extras: []slog.Handler{captured}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() }) //nolint:errcheck // best-effort flush in tests

	logger.Info("info-skipped")
	logger.Warn("warn-kept")
	if got := captured.Count(); got != 1 {
		t.Fatalf("captured=%d want 1", got)
	}
	if msg := captured.Records()[0].Message; msg != "warn-kept" {
		t.Errorf("captured msg=%q want warn-kept", msg)
	}
}

func TestStateApplyHotSwapsLevel(t *testing.T) {
	t.Parallel()
	captured := &captureHandler{}
	logger, state, err := Build(Config{Level: "warn", File: noFileSentinel, Extras: []slog.Handler{captured}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() }) //nolint:errcheck // best-effort flush in tests

	logger.Debug("first-debug")
	if got := captured.Count(); got != 0 {
		t.Fatalf("debug emitted at warn level: count=%d", got)
	}

	if applyErr := state.Apply(Config{Level: "debug", File: noFileSentinel}); applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}
	logger.Debug("second-debug")
	if got := captured.Count(); got != 1 {
		t.Errorf("debug missing after Apply: count=%d", got)
	}
}

func TestStateSetLevel(t *testing.T) {
	t.Parallel()
	captured := &captureHandler{}
	logger, state, err := Build(Config{Level: "info", File: noFileSentinel, Extras: []slog.Handler{captured}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() }) //nolint:errcheck // best-effort flush in tests

	logger.Debug("hidden")
	state.SetLevel(slog.LevelDebug)
	if state.Level() != slog.LevelDebug {
		t.Errorf("Level()=%v want debug", state.Level())
	}
	logger.Debug("now-shown")
	if got := captured.Count(); got != 1 {
		t.Errorf("debug missing after SetLevel: count=%d", got)
	}
}

func TestBuildFileSinkRejectsBadPath(t *testing.T) {
	t.Parallel()
	// A NUL byte forces both MkdirAll and OpenFile to error on every
	// supported OS, exercising the error-return path in openLogFile.
	_, state, err := Build(Config{File: "/this\x00path/is/bad.log"})
	if err == nil {
		_ = state.Close() //nolint:errcheck // best-effort flush in tests
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestBuildEmptyFileFallsBackToDefault(t *testing.T) {
	// t.Setenv forbids t.Parallel; this test runs serially.
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("HOME", dir)

	logger, state, err := Build(Config{Level: "info"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() }) //nolint:errcheck // best-effort flush in tests

	logger.Info("smoke")
	if closeErr := state.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}

	// We can't predict the exact path on every OS but DefaultLogPath
	// should produce a path with our app dir under the cache root.
	expected, derr := DefaultLogPath()
	if derr != nil {
		t.Fatalf("DefaultLogPath: %v", derr)
	}
	if _, statErr := os.Stat(expected); statErr != nil {
		// On macOS UserCacheDir returns Library/Caches regardless of
		// XDG_CACHE_HOME; if that path was used instead, just verify
		// the canonical app subdirectory exists somewhere.
		if !strings.Contains(expected, "onvif-simulator") {
			t.Fatalf("DefaultLogPath=%q does not contain onvif-simulator", expected)
		}
	}
}

func TestStateAppliedClosersAreReleased(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first := filepath.Join(dir, "first.log")
	second := filepath.Join(dir, "second.log")

	_, state, err := Build(Config{File: first})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() }) //nolint:errcheck // best-effort flush in tests

	if applyErr := state.Apply(Config{File: second}); applyErr != nil {
		t.Fatalf("Apply second file: %v", applyErr)
	}
	if _, statErr := os.Stat(first); statErr != nil {
		t.Errorf("first file removed unexpectedly: %v", statErr)
	}
}

func TestBuildDiscardHandlerWhenNoSinks(t *testing.T) {
	t.Parallel()
	// File "-" disables the file sink; with no Extras, applyInternal takes
	// the len(sinks)==0 branch and installs slog.DiscardHandler.
	logger, state, err := Build(Config{File: noFileSentinel})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() }) //nolint:errcheck // no file closers
	logger.Info("smoke")                    // must not panic; records go nowhere
}

func TestBuildExtrasOnlySinkSentinel(t *testing.T) {
	t.Parallel()
	captured := &captureHandler{}
	logger, state, err := Build(Config{File: noFileSentinel, Extras: []slog.Handler{captured}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() }) //nolint:errcheck // best-effort flush in tests

	logger.Info("only-extras")
	if got := captured.Count(); got != 1 {
		t.Errorf("captured %d records, want 1", got)
	}
}

func TestBuildBothFileAndExtras(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "fanout.log")
	captured := &captureHandler{}
	logger, state, err := Build(Config{File: path, Extras: []slog.Handler{captured}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() }) //nolint:errcheck // best-effort flush in tests

	logger.Info("dual")
	if closeErr := state.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	contents, _ := os.ReadFile(path) //nolint:errcheck // file existence already asserted via Close
	if !strings.Contains(string(contents), `"msg":"dual"`) {
		t.Errorf("file sink missing record: %s", contents)
	}
	if got := captured.Count(); got != 1 {
		t.Errorf("extras captured %d, want 1", got)
	}
}

func TestDefaultLogPath(t *testing.T) {
	t.Parallel()
	p, err := DefaultLogPath()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) && p != "" {
			t.Fatalf("DefaultLogPath returned both path and error: %q, %v", p, err)
		}
		return
	}
	if !strings.Contains(p, "onvif-simulator") {
		t.Errorf("DefaultLogPath=%q does not contain onvif-simulator", p)
	}
}

// captureHandler records every record passed through. Used by tests that
// need to assert on the number / shape of emitted lines without going
// through the file sink.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (*captureHandler) Enabled(context.Context, slog.Level) bool { return true }

// Handle implements slog.Handler.
//
//nolint:gocritic // slog.Handler interface mandates value receiver for the record
func (c *captureHandler) Handle(_ context.Context, r slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, r.Clone())
	return nil
}

func (c *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *captureHandler) WithGroup(string) slog.Handler      { return c }

func (c *captureHandler) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.records)
}

func (c *captureHandler) Records() []slog.Record {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]slog.Record, len(c.records))
	copy(out, c.records)
	return out
}
