package gui

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// LogRecord is a JSON-serializable slog record for the GUI log panel.
type LogRecord struct {
	Time      time.Time      `json:"time"`
	Level     string         `json:"level"` // DEBUG|INFO|WARN|ERROR
	Message   string         `json:"message"`
	Component string         `json:"component"`
	Attrs     map[string]any `json:"attrs"`
}

const ringCap = 256

// logHandlerShared holds ring-buffer state and the Wails emitter; all
// slog.Handler clones from WithAttrs share one instance.
type logHandlerShared struct {
	mu sync.Mutex

	buf   [ringCap]LogRecord
	head  int // next write index
	count int // valid entries (<= ringCap)

	emit atomic.Pointer[func(LogRecord)]
}

// loghandler buffers recent slog records for Wails IPC and RecentLogs backfill.
type loghandler struct {
	shared *logHandlerShared

	attrs  []slog.Attr
	groups []string // reserved for future prefixing; Handle ignores groups today.
}

func newLogHandler() *loghandler {
	return &loghandler{shared: &logHandlerShared{}}
}

func (h *loghandler) setEmitter(fn func(LogRecord)) {
	h.shared.emit.Store(&fn)
}

// Snapshot returns up to the last ringCap records in oldest-first order.
func (h *loghandler) Snapshot() []LogRecord {
	s := h.shared
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.count == 0 {
		return nil
	}
	start := (s.head - s.count + ringCap) % ringCap
	out := make([]LogRecord, s.count)
	for i := range s.count {
		out[i] = s.buf[(start+i)%ringCap]
	}
	return out
}

func (*loghandler) Enabled(context.Context, slog.Level) bool {
	// Global level filter lives in obs.atomicHandler.
	return true
}

//nolint:gocritic // slog.Handler interface mandates value receiver for the record
func (h *loghandler) Handle(_ context.Context, r slog.Record) error {
	rec := h.recordFromSlog(r)

	s := h.shared
	s.mu.Lock()
	s.buf[s.head] = rec
	s.head = (s.head + 1) % ringCap
	if s.count < ringCap {
		s.count++
	}
	s.mu.Unlock()

	if p := s.emit.Load(); p != nil {
		fn := *p
		func() {
			defer func() {
				recover() //nolint:errcheck // panic isolation for Wails emit
			}()
			fn(rec)
		}()
	}
	return nil
}

//nolint:gocritic // slog.Record is passed by value from slog.Handler.Handle
func (h *loghandler) recordFromSlog(r slog.Record) LogRecord {
	attrs := make(map[string]any)
	var component string

	mergeAttr := func(a slog.Attr) {
		switch a.Key {
		case "component":
			component = stringifyAttr(a)
		default:
			attrs[a.Key] = a.Value.Resolve().Any()
		}
	}

	for _, a := range h.attrs {
		mergeAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		mergeAttr(a)
		return true
	})

	// Groups are not prefixed into attrs until WithGroup is used on GUI-facing loggers.
	return LogRecord{
		Time:      r.Time,
		Level:     r.Level.String(),
		Message:   r.Message,
		Component: component,
		Attrs:     attrs,
	}
}

func stringifyAttr(a slog.Attr) string {
	v := a.Value.Resolve().Any()
	switch s := v.(type) {
	case string:
		return s
	default:
		return fmt.Sprint(v)
	}
}

func (h *loghandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	merged := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(merged, h.attrs)
	copy(merged[len(h.attrs):], attrs)
	return &loghandler{
		shared: h.shared,
		attrs:  merged,
		groups: append([]string(nil), h.groups...),
	}
}

func (h *loghandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &loghandler{
		shared: h.shared,
		attrs:  append([]slog.Attr(nil), h.attrs...),
		groups: append(append([]string(nil), h.groups...), name),
	}
}
