package obs

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestAtomicHandlerSwapAndDispatch(t *testing.T) {
	t.Parallel()
	// Zero (info) level: debug is filtered out, info passes.
	a := newAtomicHandler(testLevelVar(slog.LevelInfo))
	if a.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("atomicHandler should not be Enabled below configured level")
	}
	if !a.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("atomicHandler should be Enabled at configured level")
	}
	a = newAtomicHandler(testLevelVar(slog.LevelDebug))

	var buf bytes.Buffer
	target := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	a.swapGen(&sinkGeneration{handler: target})

	logger := slog.New(a)
	logger.Info("hello")
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("dispatch failed: %q", buf.String())
	}
}

func TestAtomicHandlerWithAttrsAndGroups(t *testing.T) {
	t.Parallel()
	a := newAtomicHandler(testLevelVar(slog.LevelDebug))
	var buf bytes.Buffer
	a.swapGen(&sinkGeneration{handler: slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})})

	logger := slog.New(a).With("component", "test").WithGroup("scope").With("k", "v")
	logger.Info("payload")
	out := buf.String()
	if !strings.Contains(out, "component=test") {
		t.Errorf("missing component attr: %q", out)
	}
	if !strings.Contains(out, "scope.k=v") {
		t.Errorf("missing grouped attr: %q", out)
	}
}

func TestAtomicHandlerSurvivesSwap(t *testing.T) {
	t.Parallel()
	a := newAtomicHandler(testLevelVar(slog.LevelDebug))
	var first, second bytes.Buffer
	a.swapGen(&sinkGeneration{handler: slog.NewTextHandler(&first, &slog.HandlerOptions{Level: slog.LevelDebug})})

	logger := slog.New(a).With("component", "x")
	logger.Info("before")

	a.swapGen(&sinkGeneration{handler: slog.NewTextHandler(&second, &slog.HandlerOptions{Level: slog.LevelDebug})})
	logger.Info("after")

	if !strings.Contains(first.String(), "before") {
		t.Errorf("first sink missing record: %q", first.String())
	}
	if !strings.Contains(second.String(), "after") {
		t.Errorf("second sink missing record: %q", second.String())
	}
	if strings.Contains(first.String(), "after") {
		t.Error("post-swap record leaked to old sink")
	}
}

func TestAtomicHandlerWithEmptyAttrsReturnsSelf(t *testing.T) {
	t.Parallel()
	a := newAtomicHandler(testLevelVar(slog.LevelDebug))
	if got := a.WithAttrs(nil); got != a {
		t.Error("WithAttrs(nil) should return receiver")
	}
	if got := a.WithGroup(""); got != a {
		t.Error("WithGroup(\"\") should return receiver")
	}
}

func TestMultiHandlerFanout(t *testing.T) {
	t.Parallel()
	var a, b bytes.Buffer
	mh := newMultiHandler(
		slog.NewTextHandler(&a, &slog.HandlerOptions{Level: slog.LevelDebug}),
		slog.NewJSONHandler(&b, &slog.HandlerOptions{Level: slog.LevelDebug}),
	)
	logger := slog.New(mh).With("k", "v")
	logger.Info("ping")
	if !strings.Contains(a.String(), "ping") {
		t.Errorf("first sink missing: %q", a.String())
	}
	if !strings.Contains(b.String(), `"msg":"ping"`) {
		t.Errorf("json sink missing: %q", b.String())
	}
}

func TestMultiHandlerEnabledIfAny(t *testing.T) {
	t.Parallel()
	mh := newMultiHandler(
		stubHandler{enabled: false},
		stubHandler{enabled: true},
	)
	if !mh.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("multi-handler should be Enabled if any child is")
	}
	mh = newMultiHandler(stubHandler{enabled: false})
	if mh.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("multi-handler should be disabled when all children disabled")
	}
}

func TestMultiHandlerSkipsDisabledChildren(t *testing.T) {
	t.Parallel()
	on := &captureHandler{}
	mh := newMultiHandler(stubHandler{enabled: false}, on)
	logger := slog.New(mh)
	logger.Info("only-on")
	if on.Count() != 1 {
		t.Errorf("captured %d, want 1", on.Count())
	}
}

var errBoom = errors.New("boom")

func TestMultiHandlerReturnsFirstError(t *testing.T) {
	t.Parallel()
	mh := newMultiHandler(
		stubHandler{enabled: true, err: errBoom},
		stubHandler{enabled: true},
	)
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "x", 0)
	if err := mh.Handle(context.Background(), rec); !errors.Is(err, errBoom) {
		t.Errorf("Handle err=%v want %v", err, errBoom)
	}
}

// testLevelVar returns a LevelVar pre-set to lvl.
func testLevelVar(lvl slog.Level) *slog.LevelVar {
	v := new(slog.LevelVar)
	v.Set(lvl)
	return v
}

type stubHandler struct {
	enabled bool
	err     error
}

func (s stubHandler) Enabled(context.Context, slog.Level) bool  { return s.enabled }
func (s stubHandler) Handle(context.Context, slog.Record) error { return s.err }
func (s stubHandler) WithAttrs(attrs []slog.Attr) slog.Handler  { _ = attrs; return s }
func (s stubHandler) WithGroup(name string) slog.Handler        { _ = name; return s }
