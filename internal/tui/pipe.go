package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// eventPipe is the fan-in the root model owns. Run wires simulator callbacks
// into the Send* methods, which convert records into Bubble Tea messages and
// push them onto the program. The channels are buffered so a slow UI cannot
// block a publisher goroutine — overflow is dropped rather than blocking the
// simulator, matching the Options.OnEvent / OnMutation contract that forbids
// blocking inside callbacks.
type eventPipe struct {
	program *tea.Program
}

func newEventPipe() *eventPipe { return &eventPipe{} }

func (p *eventPipe) attach(prog *tea.Program) { p.program = prog }

func (p *eventPipe) sendEvent(rec EventRecord) {
	if p.program == nil {
		return
	}
	p.program.Send(eventMsg(rec))
}

func (p *eventPipe) sendMutation(rec MutationRecord) {
	if p.program == nil {
		return
	}
	p.program.Send(mutationMsg(rec))
}
