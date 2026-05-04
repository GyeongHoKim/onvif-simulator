package obs

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
)

// sinkGeneration is one installed handler stack plus its file descriptors.
// Ref counting ensures Apply does not close previous sinks while another
// goroutine is still inside Handle on the retired handler snapshot.
type sinkGeneration struct {
	handler slog.Handler
	closers []io.Closer
	refs    atomic.Int64
	retired atomic.Bool

	closeMu sync.Mutex
	closed  bool
}

func (g *sinkGeneration) closeOnce() error {
	g.closeMu.Lock()
	defer g.closeMu.Unlock()
	if g.closed {
		return nil
	}
	g.closed = true
	err := closeAll(g.closers)
	g.closers = nil
	return err
}

// atomicHandler indirects to a swappable underlying handler so a *slog.Logger
// returned by Build can outlive any individual sink configuration. WithAttrs
// and WithGroup record the call instead of applying it eagerly; the recorded
// stages are replayed against the current target on each Handle.
//
// Enabled checks the State's LevelVar so all sinks (file + Extras) honor a
// single source-of-truth level — Extras handlers don't get to bypass it.
type atomicHandler struct {
	gen      *atomic.Pointer[sinkGeneration]
	levelVar *slog.LevelVar
	stages   []handlerStage
}

type handlerStage struct {
	attrs []slog.Attr
	group string // when non-empty, this stage represents WithGroup(group); attrs is ignored
}

func newAtomicHandler(levelVar *slog.LevelVar) *atomicHandler {
	var gen atomic.Pointer[sinkGeneration]
	boot := &sinkGeneration{handler: slog.DiscardHandler}
	gen.Store(boot)
	return &atomicHandler{gen: &gen, levelVar: levelVar}
}

func (a *atomicHandler) swapGen(next *sinkGeneration) {
	old := a.gen.Swap(next)
	if old == nil {
		return
	}
	old.retired.Store(true)
	if old.refs.Load() == 0 {
		_ = old.closeOnce() //nolint:errcheck // best-effort close of retired idle generation
	}
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
	var g *sinkGeneration
	for {
		g = a.gen.Load()
		if g == nil {
			return nil
		}
		if g.retired.Load() {
			continue
		}
		g.refs.Add(1)
		if a.gen.Load() != g {
			if g.refs.Add(-1) == 0 && g.retired.Load() {
				_ = g.closeOnce() //nolint:errcheck // lost race: drain retired generation
			}
			continue
		}
		break
	}
	defer func() {
		if g.refs.Add(-1) != 0 {
			return
		}
		if g.retired.Load() {
			_ = g.closeOnce() //nolint:errcheck // best-effort close when last ref drops
		}
	}()

	h := g.handler
	for _, st := range a.stages {
		if st.group != "" {
			h = h.WithGroup(st.group)
			continue
		}
		if len(st.attrs) > 0 {
			h = h.WithAttrs(st.attrs)
		}
	}
	return h.Handle(ctx, r)
}

func (a *atomicHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return a
	}
	stages := make([]handlerStage, len(a.stages)+1)
	copy(stages, a.stages)
	stages[len(a.stages)] = handlerStage{attrs: append([]slog.Attr(nil), attrs...)}
	return &atomicHandler{gen: a.gen, levelVar: a.levelVar, stages: stages}
}

func (a *atomicHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return a
	}
	stages := make([]handlerStage, len(a.stages)+1)
	copy(stages, a.stages)
	stages[len(a.stages)] = handlerStage{group: name}
	return &atomicHandler{gen: a.gen, levelVar: a.levelVar, stages: stages}
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
