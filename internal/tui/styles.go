package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorFg        = lipgloss.Color("#cdd6f4")
	colorMuted     = lipgloss.Color("#6c7086")
	colorAccent    = lipgloss.Color("#89b4fa")
	colorAccentAlt = lipgloss.Color("#cba6f7")
	colorSuccess   = lipgloss.Color("#a6e3a1")
	colorWarn      = lipgloss.Color("#f9e2af")
	colorError     = lipgloss.Color("#f38ba8")
	colorPanelBg   = lipgloss.Color("#1e1e2e")
	colorBorder    = lipgloss.Color("#45475a")
)

var (
	styleApp = lipgloss.NewStyle().
			Foreground(colorFg)

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	styleMuted = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleBreadcrumb = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	styleBreadcrumbActive = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true).
				Underline(true).
				Padding(0, 1)

	styleHelpBar = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorFg).
			Background(colorPanelBg).
			Padding(0, 1)

	stylePillRunning = lipgloss.NewStyle().
				Padding(0, 1).
				Bold(true).
				Foreground(lipgloss.Color("#1e1e2e")).
				Background(colorSuccess)

	stylePillStopped = lipgloss.NewStyle().
				Padding(0, 1).
				Bold(true).
				Foreground(colorFg).
				Background(colorBorder)

	stylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	stylePanelTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccentAlt)

	styleTableHeader = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent)

	styleTableRow = lipgloss.NewStyle().
			Foreground(colorFg)

	styleTableRowSel = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorPanelBg).
				Background(colorAccent)

	styleError = lipgloss.NewStyle().
			Foreground(colorError)

	styleSuccess = lipgloss.NewStyle().
			Foreground(colorSuccess)

	styleWarn = lipgloss.NewStyle().
			Foreground(colorWarn)

	styleModal = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2).
			Background(colorPanelBg)
)
