package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Layout constants — magic-number linter prefers these named.
const (
	bodyReservedRows = 4
	minBodyHeight    = 1
	uiScreenCount    = int(screenCount)
	viewInitializing = "Starting…"
)

// ErrSaveRequiredField surfaces when a form field is empty on save.
var ErrSaveRequiredField = errors.New("field is required")

// screenModel is the contract a screen satisfies inside the root model. Each
// screen is a self-contained tea.Model that also reports a Title for the
// breadcrumb and a Help string for the bottom help strip. The root model
// dispatches updates to exactly one active screen at a time.
type screenModel interface {
	tea.Model
	Title() string
	Help() string
}

// modalModel is an optional overlay covering the active screen.
type modalModel interface {
	tea.Model
	Modal(width, height int) string
}

type rootModel struct {
	sim      SimulatorAPI
	screens  [screenCount]screenModel
	active   screenID
	modal    modalModel
	width    int
	height   int
	flash    flashState
	flashSeq int
	help     bool
}

type flashState struct {
	text string
	kind flashKind
}

func newRootModel(sim SimulatorAPI) *rootModel {
	r := &rootModel{sim: sim, active: screenDashboard}
	r.screens[screenDashboard] = newDashboardModel(sim)
	r.screens[screenEvents] = newEventsModel(sim)
	r.screens[screenMedia] = newMediaModel(sim)
	r.screens[screenAuth] = newAuthModel(sim)
	r.screens[screenDevice] = newDeviceModel(sim)
	r.screens[screenLog] = newLogModel()
	return r
}

func (m *rootModel) Init() tea.Cmd {
	cmds := []tea.Cmd{tickStatus()}
	for _, s := range m.screens {
		if c := s.Init(); c != nil {
			cmds = append(cmds, c)
		}
	}
	return tea.Batch(cmds...)
}

func (m *rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case tickStatusMsg:
		return m.handleTick()
	case eventMsg, mutationMsg:
		cmd := m.routeAll(msg)
		return m, cmd
	case lifecycleMsg:
		cmd := m.setFlashFromLifecycle(msg)
		return m, cmd
	case flashMsg:
		cmd := m.setFlash(msg)
		return m, cmd
	case clearFlashMsg:
		if msg.id == m.flashSeq {
			m.flash = flashState{}
		}
		return m, nil
	case openModalMsg:
		m.modal = msg.modal
		if m.modal != nil {
			return m, m.modal.Init()
		}
		return m, nil
	case closeModalMsg:
		m.modal = nil
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	if m.modal != nil {
		return m.forwardModal(msg)
	}
	return m.updateActive(msg)
}

func (m *rootModel) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	for i, s := range m.screens {
		updated, _ := s.Update(msg)
		screen, ok := updated.(screenModel)
		if !ok {
			continue
		}
		m.screens[i] = screen
	}
	return m, nil
}

func (m *rootModel) handleTick() (tea.Model, tea.Cmd) {
	st := m.sim.Status()
	cmd := m.routeAll(statusMsg{s: st})
	return m, tea.Batch(cmd, tickStatus())
}

func (m *rootModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modal != nil {
		return m.forwardModal(msg)
	}
	if cmd, handled := m.handleGlobalKey(msg); handled {
		return m, cmd
	}
	return m.updateActive(msg)
}

func (m *rootModel) forwardModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.modal.Update(msg)
	if updated == nil {
		m.modal = nil
		return m, cmd
	}
	modal, ok := updated.(modalModel)
	if !ok {
		m.modal = nil
		return m, cmd
	}
	m.modal = modal
	return m, cmd
}

func (m *rootModel) updateActive(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.screens[m.active].Update(msg)
	screen, ok := updated.(screenModel)
	if !ok {
		return m, cmd
	}
	m.screens[m.active] = screen
	return m, cmd
}

func (m *rootModel) routeAll(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.screens))
	for i, s := range m.screens {
		updated, cmd := s.Update(msg)
		screen, ok := updated.(screenModel)
		if ok {
			m.screens[i] = screen
		}
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

func (m *rootModel) handleGlobalKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	s := msg.String()
	switch s {
	case "ctrl+c", "q":
		return m.quit(), true
	case "?":
		m.help = !m.help
		return nil, true
	case keyTab:
		m.active = (m.active + 1) % screenCount
		return nil, true
	case keyShiftTab:
		m.active = (m.active + screenCount - 1) % screenCount
		return nil, true
	}
	if len(s) == 1 {
		c := s[0]
		if c >= '1' && c <= '6' {
			idx := screenID(c - '1')
			if idx < screenCount {
				m.active = idx
			}
			return nil, true
		}
	}
	return nil, false
}

func (m *rootModel) quit() tea.Cmd {
	if m.sim != nil && m.sim.Running() {
		ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
		defer cancel()
		_ = m.sim.Stop(ctx) //nolint:errcheck // best-effort cleanup on quit
	}
	return tea.Quit
}

func (m *rootModel) setFlash(msg flashMsg) tea.Cmd {
	m.flashSeq++
	m.flash = flashState(msg)
	return scheduleClearFlash(m.flashSeq)
}

func (m *rootModel) setFlashFromLifecycle(msg lifecycleMsg) tea.Cmd {
	if msg.err != nil {
		return m.setFlash(flashMsg{text: fmt.Sprintf("%s failed: %v", msg.action, msg.err), kind: flashErr})
	}
	return m.setFlash(flashMsg{text: msg.action + " ok", kind: flashOK})
}

func (m *rootModel) View() string {
	if m.width == 0 {
		return viewInitializing
	}
	var b strings.Builder
	b.WriteString(m.renderBreadcrumb())
	b.WriteString("\n")
	b.WriteString(m.renderBody())
	b.WriteString("\n")
	b.WriteString(m.renderStatusBar())
	b.WriteString("\n")
	b.WriteString(m.renderHelp())
	view := styleApp.Render(b.String())
	if m.modal != nil {
		overlay := m.modal.Modal(m.width, m.height)
		return overlayCentered(view, overlay, m.width, m.height)
	}
	return view
}

func (m *rootModel) renderBreadcrumb() string {
	parts := make([]string, 0, uiScreenCount+1)
	pill := stylePillStopped.Render("Stopped")
	if m.sim != nil && m.sim.Running() {
		st := m.sim.Status()
		pill = stylePillRunning.Render("Running · " + st.ListenAddr)
	}
	parts = append(parts, pill)
	for i := screenID(0); i < screenCount; i++ {
		label := fmt.Sprintf("%d %s", int(i)+1, i.String())
		if i == m.active {
			parts = append(parts, styleBreadcrumbActive.Render(label))
		} else {
			parts = append(parts, styleBreadcrumb.Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m *rootModel) renderBody() string {
	body := m.screens[m.active].View()
	height := m.height - bodyReservedRows
	if height < minBodyHeight {
		height = minBodyHeight
	}
	return lipgloss.NewStyle().Width(m.width).Height(height).Render(body)
}

func (m *rootModel) renderStatusBar() string {
	if m.flash.text == "" {
		return styleStatusBar.Render(" ")
	}
	style := styleStatusBar
	switch m.flash.kind {
	case flashOK:
		style = style.Foreground(colorSuccess)
	case flashErr:
		style = style.Foreground(colorError)
	case flashInfo:
		// default palette
	}
	return style.Render(m.flash.text)
}

func (m *rootModel) renderHelp() string {
	common := "tab/shift+tab: switch · 1-6: jump · ?: help · q: quit"
	return styleHelpBar.Render(common + " · " + m.screens[m.active].Help())
}

// overlayCentered draws the overlay on top of base, centered. Bubble Tea does
// not ship a composited renderer; we lean on lipgloss.Place which blanks the
// base layer for this frame, which is good enough for modal dialogs.
func overlayCentered(base, overlay string, w, h int) string {
	_ = base
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, overlay,
		lipgloss.WithWhitespaceChars(" "))
}
