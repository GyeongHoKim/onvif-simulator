package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	logRingCapacity   = 512
	logMaxVisibleRows = 40
	logTargetColWidth = 20
	logSearchWidth    = 40
	logSearchMaxLen   = 128
)

type logEntry struct {
	time    time.Time
	kind    string // "event" | "mutation"
	target  string
	detail  string
	topic   string
	source  string
	payload string
}

type logModel struct {
	entries    []logEntry
	showEvents bool
	showMuts   bool
	search     textinput.Model
	searching  bool
}

func newLogModel() *logModel {
	ti := textinput.New()
	ti.Placeholder = "search (substring)"
	ti.CharLimit = logSearchMaxLen
	ti.Width = logSearchWidth
	return &logModel{
		showEvents: true,
		showMuts:   true,
		search:     ti,
	}
}

func (*logModel) Init() tea.Cmd { return nil }
func (*logModel) Title() string { return "Log" }
func (*logModel) Help() string  { return "e: events · m: mutations · /: search · c: clear" }

func (m *logModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case eventMsg:
		m.append(&logEntry{
			time: msg.Time, kind: "event",
			target: msg.Source, topic: msg.Topic,
			source: msg.Source, payload: msg.Payload,
			detail: fmt.Sprintf("%s %s %s",
				shortTopic(msg.Topic), orDash(msg.Source), msg.Payload),
		})
	case mutationMsg:
		m.append(&logEntry{
			time: msg.Time, kind: "mutation",
			target: msg.Target,
			detail: fmt.Sprintf("%s %s %s",
				msg.Kind, orDash(msg.Target), msg.Detail),
		})
	case tea.KeyMsg:
		cmd := m.handleKey(msg)
		return m, cmd
	}
	return m, nil
}

func (m *logModel) append(e *logEntry) {
	m.entries = append(m.entries, *e)
	if len(m.entries) > logRingCapacity {
		m.entries = m.entries[len(m.entries)-logRingCapacity:]
	}
}

func (m *logModel) handleKey(msg tea.KeyMsg) tea.Cmd {
	if m.searching {
		return m.handleSearchKey(msg)
	}
	switch msg.String() {
	case "e":
		m.showEvents = !m.showEvents
	case "m":
		m.showMuts = !m.showMuts
	case "/":
		m.searching = true
		m.search.Focus()
		return textinput.Blink
	case "c":
		m.entries = m.entries[:0]
	}
	return nil
}

func (m *logModel) handleSearchKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case keyEsc:
		m.searching = false
		m.search.Blur()
		m.search.SetValue("")
		return nil
	case keyEnter:
		m.searching = false
		m.search.Blur()
		return nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	return cmd
}

func (m *logModel) View() string {
	var b strings.Builder
	b.WriteString(m.filterBar())
	b.WriteString("\n\n")
	b.WriteString(styleTableHeader.Render(
		fmt.Sprintf("%-8s  %-10s  %-*s  %s",
			"TIME", "KIND", logTargetColWidth, "TARGET", "DETAIL"),
	))
	b.WriteString("\n")
	needle := strings.ToLower(strings.TrimSpace(m.search.Value()))
	shown := 0
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]
		if e.kind == "event" && !m.showEvents {
			continue
		}
		if e.kind == "mutation" && !m.showMuts {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(e.detail), needle) {
			continue
		}
		fmt.Fprintf(&b, "%-8s  %-10s  %-*s  %s\n",
			e.time.Format("15:04:05"),
			e.kind,
			logTargetColWidth, truncate(orDash(e.target), logTargetColWidth),
			e.detail,
		)
		shown++
		if shown >= logMaxVisibleRows {
			break
		}
	}
	if len(m.entries) == 0 {
		b.WriteString(styleMuted.Render(
			"No entries yet — events and config mutations will appear here."))
	}
	return b.String()
}

func (m *logModel) filterBar() string {
	pill := func(label string, on bool) string {
		if on {
			return stylePillRunning.Render(label)
		}
		return stylePillStopped.Render(label)
	}
	search := "/: search"
	if m.searching || m.search.Value() != "" {
		search = "search: " + m.search.View()
	}
	return fmt.Sprintf("%s  %s  %s  %s",
		pill("Events", m.showEvents),
		pill("Mutations", m.showMuts),
		search,
		styleMuted.Render(fmt.Sprintf("(%d entries)", len(m.entries))),
	)
}
