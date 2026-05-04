package gui

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestLogHandlerHandlePopulatesFields(t *testing.T) {
	t.Parallel()
	h := newLogHandler()
	logger := slog.New(h)
	logger.Info("hello", "k", "v", "component", "auth")

	snap := h.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("len(snap) = %d, want 1", len(snap))
	}
	r := snap[0]
	if r.Message != "hello" {
		t.Errorf("Message = %q, want hello", r.Message)
	}
	if r.Component != "auth" {
		t.Errorf("Component = %q, want auth", r.Component)
	}
	if r.Level != "INFO" {
		t.Errorf("Level = %q, want INFO", r.Level)
	}
	if r.Attrs["k"] != "v" {
		t.Errorf("attrs[k] = %v, want v", r.Attrs["k"])
	}
	if _, ok := r.Attrs["component"]; ok {
		t.Error("component should not appear in attrs map")
	}
}

func TestLogHandlerNilEmitterSafe(t *testing.T) {
	t.Parallel()
	h := newLogHandler()
	logger := slog.New(h)
	logger.Info("x")
	if len(h.Snapshot()) != 1 {
		t.Fatal("expected record in ring")
	}
}

func TestLogHandlerEmitsAfterSetEmitter(t *testing.T) {
	t.Parallel()
	h := newLogHandler()
	var got LogRecord
	var wg sync.WaitGroup
	wg.Add(1)
	h.setEmitter(func(r LogRecord) {
		got = r
		wg.Done()
	})
	logger := slog.New(h)
	logger.Warn("emit me")
	wg.Wait()
	if got.Message != "emit me" {
		t.Errorf("callback message = %q", got.Message)
	}
	if got.Level != "WARN" {
		t.Errorf("callback level = %q", got.Level)
	}
}

func TestLogHandlerRingEviction(t *testing.T) {
	t.Parallel()
	h := newLogHandler()
	logger := slog.New(h)
	for i := range 300 {
		logger.Info("msg", "n", i)
	}
	snap := h.Snapshot()
	if len(snap) != ringCap {
		t.Fatalf("len(snap) = %d, want %d", len(snap), ringCap)
	}
	// Oldest retained should be index 44 (300-256).
	first := intFromAny(t, snap[0].Attrs["n"])
	if first != 44 {
		t.Errorf("oldest n = %d, want 44", first)
	}
	last := intFromAny(t, snap[len(snap)-1].Attrs["n"])
	if last != 299 {
		t.Errorf("newest n = %d, want 299", last)
	}
}

func intFromAny(t *testing.T, v any) int {
	t.Helper()
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case uint64:
		return int(x)
	default:
		t.Fatalf("unexpected attr type %T (%v)", v, v)
		return 0
	}
}

func TestLogHandlerConcurrentAppendSnapshot(t *testing.T) {
	t.Parallel()
	h := newLogHandler()
	logger := slog.New(h)
	const (
		goroutines = 10
		per        = 50
	)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for m := range per {
				logger.Info("concurrent", "g", id, "m", m)
			}
		}(g)
	}
	wg.Wait()
	snap := h.Snapshot()
	if len(snap) != ringCap {
		t.Fatalf("len(snap) = %d, want ring capacity", len(snap))
	}
	// Spot-check invariants: no panics, fixed length.
	for _, r := range snap {
		if r.Message != "concurrent" {
			t.Errorf("unexpected message %q", r.Message)
		}
	}
}

func TestLogHandlerWithAttrsMerged(t *testing.T) {
	t.Parallel()
	h := newLogHandler()
	base := slog.New(h).With("component", "media")
	base.Info("shot")

	snap := h.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("len = %d", len(snap))
	}
	if snap[0].Component != "media" {
		t.Errorf("Component = %q, want media", snap[0].Component)
	}
}

func TestLogHandlerWithGroupChains(t *testing.T) {
	t.Parallel()
	h := newLogHandler()
	lg := slog.New(h).WithGroup("scope")
	lg.Info("grouped")
	snap := h.Snapshot()
	if len(snap) != 1 || snap[0].Message != "grouped" {
		t.Fatalf("snap = %#v", snap)
	}
}

func TestLogHandlerNonStringComponent(t *testing.T) {
	t.Parallel()
	h := newLogHandler()
	slog.New(h).Info("x", "component", 99)
	if got := h.Snapshot()[0].Component; got != "99" {
		t.Errorf("Component = %q, want 99", got)
	}
}

func TestLogHandlerWithAttrsNoOp(t *testing.T) {
	t.Parallel()
	root := newLogHandler()
	h := root.WithAttrs(nil)
	slog.New(h).Info("y")
	if len(root.Snapshot()) != 1 {
		t.Fatal("expected one record")
	}
}

func TestSnapshotOldestFirstOrder(t *testing.T) {
	t.Parallel()
	h := newLogHandler()
	logger := slog.New(h)
	logger.Info("a")
	logger.Info("b")
	snap := h.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("len = %d", len(snap))
	}
	if snap[0].Message != "a" || snap[1].Message != "b" {
		t.Fatalf("order = %#v, %#v", snap[0].Message, snap[1].Message)
	}
}

func TestLogRecordTimePreserved(t *testing.T) {
	t.Parallel()
	h := newLogHandler()
	ts := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	rec := slog.NewRecord(ts, slog.LevelError, "boom", 0)
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	snap := h.Snapshot()
	if !snap[0].Time.Equal(ts) {
		t.Errorf("Time = %v, want %v", snap[0].Time, ts)
	}
}
