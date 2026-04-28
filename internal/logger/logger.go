// Package logger persists ONVIF event and config-mutation records to a
// rotating JSON-lines file on disk and exposes a paged reader for the GUI.
//
// Each Entry is one JSON object per line. Time is encoded as RFC3339Nano so
// the GUI's `new Date(iso)` parses it without conversion. Writes are
// non-blocking — they enqueue onto a buffered channel drained by a single
// goroutine, so a slow disk never back-pressures ONVIF SOAP handlers. On
// overflow, entries are dropped (counted in DroppedCount) rather than
// blocking the publisher.
//
// The file location follows the same convention as internal/config:
//
//	macOS:   ~/Library/Application Support/onvif-simulator/logs/onvif-simulator.log
//	Linux:   $XDG_CONFIG_HOME/onvif-simulator/logs/onvif-simulator.log
//	Windows: %AppData%\onvif-simulator\logs\onvif-simulator.log
//
// Rotation is delegated to lumberjack; backups are kept (compressed) for
// audit. Clear() rotates the active file rather than truncating, so the
// previous content is preserved as onvif-simulator.log.<n>(.gz).
package logger

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	defaultFilename   = "onvif-simulator.log"
	defaultDirSubpath = "logs"
	defaultMaxSizeMB  = 5
	defaultMaxBackups = 3
	defaultMaxAgeDays = 7
	defaultBufferSize = 1024
	logsDirMode       = 0o700
)

// Options configures a Logger. Zero values fall back to sensible defaults.
type Options struct {
	Dir        string // default: <UserConfigDir>/onvif-simulator/logs
	Filename   string // default: "onvif-simulator.log"
	MaxSizeMB  int    // default: 5
	MaxBackups int    // default: 3
	MaxAgeDays int    // default: 7
	Compress   *bool  // default: true (pointer so the zero value can mean "default")
	BufferSize int    // drain channel cap; default: 1024
}

// Entry is one record in the on-disk log. Kind discriminates the populated
// fields: "event" uses Topic/Source/Payload, "mutation" uses Op/Target/Detail.
type Entry struct {
	Time    time.Time `json:"time"`
	Kind    string    `json:"kind"`
	Topic   string    `json:"topic,omitempty"`
	Source  string    `json:"source,omitempty"`
	Payload string    `json:"payload,omitempty"`
	Op      string    `json:"op,omitempty"`
	Target  string    `json:"target,omitempty"`
	Detail  string    `json:"detail,omitempty"`
}

// Logger is the file-backed log sink.
type Logger struct {
	rotator *lumberjack.Logger
	core    zapcore.Core
	path    string

	ch        chan Entry
	dropped   uint64
	wg        sync.WaitGroup
	closeOnce sync.Once

	noop bool
}

// New constructs a Logger that writes to <Dir>/<Filename>. If the directory
// cannot be created the caller should fall back to NoOp.
func New(opts Options) (*Logger, error) {
	if opts.Dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("logger: resolve user config dir: %w", err)
		}
		opts.Dir = filepath.Join(base, "onvif-simulator", defaultDirSubpath)
	}
	if opts.Filename == "" {
		opts.Filename = defaultFilename
	}
	if opts.MaxSizeMB <= 0 {
		opts.MaxSizeMB = defaultMaxSizeMB
	}
	if opts.MaxBackups <= 0 {
		opts.MaxBackups = defaultMaxBackups
	}
	if opts.MaxAgeDays <= 0 {
		opts.MaxAgeDays = defaultMaxAgeDays
	}
	compress := true
	if opts.Compress != nil {
		compress = *opts.Compress
	}
	if opts.BufferSize <= 0 {
		opts.BufferSize = defaultBufferSize
	}

	if err := os.MkdirAll(opts.Dir, logsDirMode); err != nil {
		return nil, fmt.Errorf("logger: create log dir: %w", err)
	}

	path := filepath.Join(opts.Dir, opts.Filename)
	rotator := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    opts.MaxSizeMB,
		MaxBackups: opts.MaxBackups,
		MaxAge:     opts.MaxAgeDays,
		Compress:   compress,
	}

	encCfg := zapcore.EncoderConfig{
		TimeKey:    "time",
		EncodeTime: zapcore.RFC3339NanoTimeEncoder,
	}
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encCfg),
		zapcore.AddSync(rotator),
		zapcore.InfoLevel,
	)

	l := &Logger{
		rotator: rotator,
		core:    core,
		path:    path,
		ch:      make(chan Entry, opts.BufferSize),
	}
	l.wg.Add(1)
	go l.drain()
	return l, nil
}

// NoOp returns a Logger that silently discards every Write. Used when the
// log directory is unwriteable so the GUI can still boot.
func NoOp() *Logger {
	return &Logger{noop: true}
}

// Write enqueues e. Returns immediately. If the buffer is full the entry is
// dropped and DroppedCount is incremented.
//
//nolint:gocritic // Entry is a public value type; callers build literals at the call site.
func (l *Logger) Write(e Entry) {
	if l == nil || l.noop {
		return
	}
	select {
	case l.ch <- e:
	default:
		atomic.AddUint64(&l.dropped, 1)
	}
}

// DroppedCount returns the number of entries dropped due to buffer overflow.
func (l *Logger) DroppedCount() uint64 {
	if l == nil {
		return 0
	}
	return atomic.LoadUint64(&l.dropped)
}

// Read returns up to limit entries starting at offset, newest-first, plus
// the total number of entries on disk. Active file and all backups (incl.
// .gz) are included. Pass limit=0 to return everything from offset onward.
func (l *Logger) Read(offset, limit int) ([]Entry, int, error) {
	if l == nil || l.noop {
		return []Entry{}, 0, nil
	}
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}

	files, err := l.listFiles()
	if err != nil {
		return nil, 0, err
	}

	var all []Entry
	for _, f := range files {
		entries, err := readFile(f)
		if err != nil {
			return nil, 0, err
		}
		all = append(all, entries...)
	}

	sort.SliceStable(all, func(i, j int) bool {
		return all[i].Time.After(all[j].Time)
	})

	total := len(all)
	if offset >= total {
		return []Entry{}, total, nil
	}
	end := offset + limit
	if limit == 0 || end > total {
		end = total
	}
	out := make([]Entry, end-offset)
	copy(out, all[offset:end])
	return out, total, nil
}

// Clear rotates the active log file. The previous content is preserved as
// the next backup; subsequent Reads will not return it once it ages out.
func (l *Logger) Clear() error {
	if l == nil || l.noop {
		return nil
	}
	return l.rotator.Rotate()
}

// Close drains pending writes, syncs the file, and shuts the drain
// goroutine down. Safe to call multiple times.
func (l *Logger) Close() error {
	if l == nil || l.noop {
		return nil
	}
	var err error
	l.closeOnce.Do(func() {
		close(l.ch)
		l.wg.Wait()
		// Sync errors during shutdown are best-effort: the file is about to
		// be closed and any remaining buffered bytes were already flushed by
		// the drain goroutine. Reporting only the rotator close error keeps
		// the contract simple.
		if syncErr := l.core.Sync(); syncErr != nil {
			err = syncErr
		}
		if rotErr := l.rotator.Close(); rotErr != nil {
			err = rotErr
		}
	})
	return err
}

func (l *Logger) drain() {
	defer l.wg.Done()
	for e := range l.ch {
		fields := entryFields(&e)
		// core.Write returns an error only when the encoder/sink fails (full
		// disk, locked file). We can't surface it from a background goroutine
		// without a sink — the dropped counter covers buffer-side losses.
		if err := l.core.Write(zapcore.Entry{
			Level: zapcore.InfoLevel,
			Time:  e.Time,
		}, fields); err != nil {
			atomic.AddUint64(&l.dropped, 1)
		}
	}
}

const maxEntryFields = 7

func entryFields(e *Entry) []zapcore.Field {
	fields := make([]zapcore.Field, 0, maxEntryFields)
	fields = append(fields, zap.String("kind", e.Kind))
	if e.Topic != "" {
		fields = append(fields, zap.String("topic", e.Topic))
	}
	if e.Source != "" {
		fields = append(fields, zap.String("source", e.Source))
	}
	if e.Payload != "" {
		fields = append(fields, zap.String("payload", e.Payload))
	}
	if e.Op != "" {
		fields = append(fields, zap.String("op", e.Op))
	}
	if e.Target != "" {
		fields = append(fields, zap.String("target", e.Target))
	}
	if e.Detail != "" {
		fields = append(fields, zap.String("detail", e.Detail))
	}
	return fields
}

func (l *Logger) listFiles() ([]string, error) {
	dir := filepath.Dir(l.path)
	base := filepath.Base(l.path)
	prefix := strings.TrimSuffix(base, filepath.Ext(base))

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("logger: read dir: %w", err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == base || strings.HasPrefix(name, prefix+"-") || strings.HasPrefix(name, prefix+".") {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out, nil
}

func readFile(path string) ([]Entry, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from listFiles which scans only the log dir
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("logger: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // read-only file; close error is non-actionable

	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, fmt.Errorf("logger: gzip open %s: %w", path, err)
		}
		defer func() { _ = gz.Close() }() //nolint:errcheck // read-only stream
		r = gz
	}

	dec := json.NewDecoder(r)
	var out []Entry
	for {
		var e Entry
		if err := dec.Decode(&e); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			// Skip malformed lines rather than fail the whole read.
			continue
		}
		out = append(out, e)
	}
	return out, nil
}
