package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	dashCardWidth      = 24
	dashMaxRecent      = 20
	dashTopicColWidth  = 32
	dashSourceColWidth = 16
	dashStateColWidth  = 40

	transitionStarting = "starting"
	transitionStopping = "stopping"
)

type dashboardModel struct {
	sim         SimulatorAPI
	status      Status
	width       int
	height      int
	transition  string // "starting" | "stopping" | ""
	lastUpdated time.Time
}

func newDashboardModel(sim SimulatorAPI) *dashboardModel {
	return &dashboardModel{sim: sim}
}

func (*dashboardModel) Init() tea.Cmd { return nil }
func (*dashboardModel) Title() string { return "Dashboard" }
func (*dashboardModel) Help() string  { return "s: start/stop · r: reload" }

func (m *dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case statusMsg:
		m.status = msg.s
		m.lastUpdated = time.Now()
		if m.transition == transitionStarting && m.status.Running {
			m.transition = ""
		}
		if m.transition == transitionStopping && !m.status.Running {
			m.transition = ""
		}
	case tea.KeyMsg:
		cmd := m.handleKey(msg)
		return m, cmd
	}
	return m, nil
}

func (m *dashboardModel) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "s":
		return m.toggleLifecycle()
	case "r":
		return m.forceRefresh()
	}
	return nil
}

func (m *dashboardModel) toggleLifecycle() tea.Cmd {
	if m.sim == nil {
		return nil
	}
	if m.sim.Running() {
		m.transition = transitionStopping
		sim := m.sim
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
			defer cancel()
			return lifecycleMsg{action: "stop", err: sim.Stop(ctx)}
		}
	}
	m.transition = transitionStarting
	sim := m.sim
	return func() tea.Msg {
		return lifecycleMsg{action: "start", err: sim.Start(context.Background())}
	}
}

func (m *dashboardModel) forceRefresh() tea.Cmd {
	sim := m.sim
	return func() tea.Msg { return statusMsg{s: sim.Status()} }
}

func (m *dashboardModel) View() string {
	rowA := m.rowA()
	rowB := m.rowB()
	rowC := m.rowC()
	return lipgloss.JoinVertical(lipgloss.Left, rowA, rowB, rowC)
}

func (m *dashboardModel) rowA() string {
	running := "Stopped"
	if m.status.Running {
		running = "Running"
	}
	if m.transition != "" {
		running = titleCase(m.transition) + "…"
	}
	cards := []string{
		card("State", running),
		card("Listen", orDash(m.status.ListenAddr)),
		card("Uptime", uptime(m.status.Uptime)),
		card("Discovery", orDash(m.status.DiscoveryMode)),
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cards...)
}

func (m *dashboardModel) rowB() string {
	cards := []string{
		card("Profiles", strconv.Itoa(m.status.ProfileCount)),
		card("Topics", strconv.Itoa(m.status.TopicCount)+" enabled"),
		card("Users", strconv.Itoa(m.status.UserCount)),
		card("Subs", strconv.Itoa(m.status.ActivePullSubs)+" active"),
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cards...)
}

func (m *dashboardModel) rowC() string {
	var body strings.Builder
	body.WriteString(stylePanelTitle.Render("Recent events"))
	body.WriteString("\n")
	if len(m.status.RecentEvents) == 0 {
		body.WriteString(styleMuted.Render("No recent events."))
		return stylePanel.Render(body.String())
	}
	upto := len(m.status.RecentEvents)
	if upto > dashMaxRecent {
		upto = dashMaxRecent
	}
	body.WriteString(styleTableHeader.Render(
		fmt.Sprintf("%-8s  %-*s  %-*s  %s", "TIME",
			dashTopicColWidth, "TOPIC",
			dashSourceColWidth, "SOURCE", "STATE"),
	))
	body.WriteString("\n")
	for i := range upto {
		ev := m.status.RecentEvents[i]
		fmt.Fprintf(&body, "%-8s  %-*s  %-*s  %s\n",
			ev.Time.Format("15:04:05"),
			dashTopicColWidth, truncate(shortTopic(ev.Topic), dashTopicColWidth),
			dashSourceColWidth, truncate(orDash(ev.Source), dashSourceColWidth),
			truncate(ev.Payload, dashStateColWidth),
		)
	}
	return stylePanel.Render(body.String())
}

func card(title, value string) string {
	body := lipgloss.JoinVertical(lipgloss.Left,
		stylePanelTitle.Render(title),
		styleTitle.Render(value),
	)
	return stylePanel.Width(dashCardWidth).Render(body)
}

func uptime(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	d = d.Round(time.Second)
	h := int(d / time.Hour)
	d -= time.Duration(h) * time.Hour
	mi := int(d / time.Minute)
	d -= time.Duration(mi) * time.Minute
	s := int(d / time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", h, mi, s)
}

func shortTopic(t string) string {
	return strings.TrimPrefix(t, "tns1:")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// titleCase uppercases the first rune. Replaces strings.Title (deprecated).
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
