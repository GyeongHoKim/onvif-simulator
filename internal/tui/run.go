package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/GyeongHoKim/onvif-simulator/internal/simulator"
)

// Run starts the TUI bound to sim. It blocks until the user quits or the
// terminal program exits. On quit it attempts a graceful Stop on the
// simulator inside the root model.
//
// The variadic bridge argument is optional: pass nothing for a TUI that
// only reflects Status polling, or a single *CallbackBridge to stream
// simulator.OnEvent / OnMutation records into the Log screen. The bridge
// must be constructed *before* simulator.New so the Options closures can
// reference it.
//
//	bridge := tui.NewCallbackBridge()
//	sim, _ := simulator.New(simulator.Options{
//	    OnEvent:    bridge.OnEvent,
//	    OnMutation: bridge.OnMutation,
//	})
//	return tui.Run(sim, bridge)
func Run(sim *simulator.Simulator, bridge ...*CallbackBridge) error {
	adapter := newSimulatorAdapter(sim)
	root := newRootModel(adapter)
	prog := tea.NewProgram(root, tea.WithAltScreen(), tea.WithMouseCellMotion())
	for _, b := range bridge {
		if b != nil {
			b.attach(prog)
		}
	}
	_, err := prog.Run()
	return err
}

// CallbackBridge adapts simulator.EventRecord / MutationRecord records from
// Options.OnEvent and Options.OnMutation into Bubble Tea messages posted onto
// the running tea.Program. The bridge is created before the simulator so the
// simulator's Options closures can reference it; attach happens inside Run
// once the program is built.
type CallbackBridge struct {
	pipe *eventPipe
}

// NewCallbackBridge returns a bridge ready to be passed to Options.OnEvent
// and Options.OnMutation. Callbacks fired before Run attaches a program are
// silently dropped — event delivery starts once the TUI is up.
func NewCallbackBridge() *CallbackBridge {
	return &CallbackBridge{pipe: newEventPipe()}
}

// OnEvent satisfies simulator.Options.OnEvent.
func (b *CallbackBridge) OnEvent(rec simulator.EventRecord) {
	b.pipe.sendEvent(EventRecord{
		Time:    rec.Time,
		Topic:   rec.Topic,
		Source:  rec.Source,
		Payload: rec.Payload,
	})
}

// OnMutation satisfies simulator.Options.OnMutation.
func (b *CallbackBridge) OnMutation(rec simulator.MutationRecord) {
	b.pipe.sendMutation(MutationRecord{
		Time:   rec.Time,
		Kind:   rec.Kind,
		Target: rec.Target,
		Detail: rec.Detail,
	})
}

func (b *CallbackBridge) attach(prog *tea.Program) { b.pipe.attach(prog) }
