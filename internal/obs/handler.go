package obs

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// atomicHandler indirects to a swappable underlying handler so a *slog.Logger
// returned by Build can outlive any individual sink configuration. WithAttrs
// and WithGroup record the call instead of applying it eagerly; the recorded
// stages are replayed against the current target on each Handle.
//
// Enabled checks the State's LevelVar so all sinks (file + Extras) honor a
// single source-of-truth level — Extras handlers don't get to bypass it.
type atomicHandler struct {
	target   *atomic.Pointer[slog.Handler]
	levelVar *slog.LevelVar
	stages   []handlerStage
}

type handlerStage struct {
	attrs []slog.Attr
	group string // when non-empty, this stage represents WithGroup(group); attrs is ignored
}

func newAtomicHandler(levelVar *slog.LevelVar) *atomicHandler {
	p := &atomic.Pointer[slog.Handler]{}
	initial := slog.DiscardHandler
	p.Store(&initial)
	return &atomicHandler{target: p, levelVar: levelVar}
}

func (a *atomicHandler) swap(h slog.Handler) {
	a.target.Store(&h)
}

func (a *atomicHandler) currentForRecord() slog.Handler {
	h := *a.target.Load()
	for _, st := range a.stages {
		if st.group != "" {
			h = h.WithGroup(st.group)
			continue
		}
		if len(st.attrs) > 0 {
			h = h.WithAttrs(st.attrs)
		}
	}
	return h
}

// Enabled checks the live LevelVar so the level filter applies to every sink,
// including Extras whose own Enabled is unaware of the State's level.
func (a *atomicHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= a.levelVar.Level()
}

// Handle implements slog.Handler. The slog.Record is heavy (~288 bytes) but
// the standard slog.Handler interface mandates it by value, so we cannot
// change the receiver shape; gocritic's hugeParam warning is suppressed.
//
//nolint:gocritic // slog.Handler interface mandates value receiver for the record
func (a *atomicHandler) Handle(ctx context.Context, r slog.Record) error {
	return a.currentForRecord().Handle(ctx, r)
}

func (a *atomicHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return a
	}
	stages := make([]handlerStage, len(a.stages)+1)
	copy(stages, a.stages)
	stages[len(a.stages)] = handlerStage{attrs: append([]slog.Attr(nil), attrs...)}
	return &atomicHandler{target: a.target, levelVar: a.levelVar, stages: stages}
}

func (a *atomicHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return a
	}
	stages := make([]handlerStage, len(a.stages)+1)
	copy(stages, a.stages)
	stages[len(a.stages)] = handlerStage{group: name}
	return &atomicHandler{target: a.target, levelVar: a.levelVar, stages: stages}
}

// multiHandler fans every record out to multiple sinks. Used when more than
// one sink is active (e.g. stderr + file).
type multiHandler struct {
	handlers []slog.Handler
}

func newMultiHandler(h ...slog.Handler) *multiHandler {
	return &multiHandler{handlers: append([]slog.Handler(nil), h...)}
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle implements slog.Handler. See atomicHandler.Handle for the same
// hugeParam rationale.
//
//nolint:gocritic // slog.Handler interface mandates value receiver for the record
func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, h := range m.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		if err := h.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		out[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: out}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	out := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		out[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: out}
}
