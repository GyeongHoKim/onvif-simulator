package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// tickStatusMsg fires every dashboardPollInterval while the dashboard is mounted.
type tickStatusMsg time.Time

// statusMsg carries a Status snapshot to the dashboard and other screens.
type statusMsg struct{ s Status }

// eventMsg is routed to the Log screen when simulator.OnEvent fires.
type eventMsg EventRecord

// mutationMsg is routed to the Log screen when simulator.OnMutation fires.
type mutationMsg MutationRecord

// flashMsg renders a one-line status bar notification across all screens.
type flashMsg struct {
	text string
	kind flashKind
}

type flashKind int

const (
	flashInfo flashKind = iota
	flashOK
	flashErr
)

// clearFlashMsg clears the status bar after flashTTL.
type clearFlashMsg struct{ id int }

// lifecycleMsg reports the result of a Start/Stop invocation.
type lifecycleMsg struct {
	action string
	err    error
}

// openModalMsg is raised by a screen to request a full-screen modal overlay.
type openModalMsg struct{ modal modalModel }

// closeModalMsg tells the root model to dismiss the active modal.
type closeModalMsg struct{}

// stopTimeout bounds graceful shutdown on quit.
const stopTimeout = 3 * time.Second

// screenID identifies a top-level screen in the breadcrumb.
type screenID int

const (
	screenDashboard screenID = iota
	screenEvents
	screenMedia
	screenAuth
	screenDevice
	screenLog
	screenCount
)

func (s screenID) String() string {
	switch s {
	case screenDashboard:
		return "Dashboard"
	case screenEvents:
		return "Events"
	case screenMedia:
		return "Media"
	case screenAuth:
		return "Auth"
	case screenDevice:
		return "Device"
	case screenLog:
		return "Log"
	case screenCount:
		return ""
	default:
		return ""
	}
}

// dashboardPollInterval is the TUI polling interval per ui-ux.md (500ms).
const dashboardPollInterval = 500 * time.Millisecond

// flashTTL is how long a status-bar message stays up.
const flashTTL = 3 * time.Second

func tickStatus() tea.Cmd {
	return tea.Tick(dashboardPollInterval, func(t time.Time) tea.Msg {
		return tickStatusMsg(t)
	})
}

func scheduleClearFlash(id int) tea.Cmd {
	return tea.Tick(flashTTL, func(time.Time) tea.Msg {
		return clearFlashMsg{id: id}
	})
}
