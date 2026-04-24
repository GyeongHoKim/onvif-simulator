package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

// Known ONVIF topic strings handled by typed helpers on *Simulator. Any topic
// outside this set falls back to PublishRaw.
const (
	topicMotion    = "tns1:VideoSource/MotionAlarm"
	topicImgBlurry = "tns1:VideoSource/ImageTooBlurry"
	topicImgDark   = "tns1:VideoSource/ImageTooDark"
	topicImgBright = "tns1:VideoSource/ImageTooBright"
	topicDigitalIn = "tns1:Device/Trigger/DigitalInput"
)

const (
	eventsTopicColWidth = 48
	eventsInputWidth    = 32
	eventsSyncFields    = 4
)

type eventsModel struct {
	sim           SimulatorAPI
	topics        []config.TopicConfig
	selected      int
	defaultSource string
	width         int
	height        int
}

func newEventsModel(sim SimulatorAPI) *eventsModel {
	return &eventsModel{sim: sim}
}

func (*eventsModel) Init() tea.Cmd { return nil }
func (*eventsModel) Title() string { return "Events" }
func (*eventsModel) Help() string {
	return "↑/↓: select · space: toggle · enter: trigger · y: syncproperty"
}

func (m *eventsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case statusMsg:
		m.refreshFromSnapshot()
	case tea.KeyMsg:
		cmd := m.handleKey(msg)
		return m, cmd
	}
	return m, nil
}

func (m *eventsModel) refreshFromSnapshot() {
	snap := m.sim.ConfigSnapshot()
	m.topics = append(m.topics[:0], snap.Events.Topics...)
	if m.selected >= len(m.topics) {
		m.selected = len(m.topics) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	m.defaultSource = ""
	for i := range snap.Media.Profiles {
		if snap.Media.Profiles[i].VideoSourceToken != "" {
			m.defaultSource = snap.Media.Profiles[i].VideoSourceToken
			break
		}
	}
	if m.defaultSource == "" {
		m.defaultSource = config.DefaultVideoSourceToken
	}
}

func (m *eventsModel) handleKey(msg tea.KeyMsg) tea.Cmd {
	if len(m.topics) == 0 {
		return nil
	}
	switch msg.String() {
	case keyUp, "k":
		if m.selected > 0 {
			m.selected--
		}
	case keyDown, "j":
		if m.selected < len(m.topics)-1 {
			m.selected++
		}
	case " ":
		return m.toggleTopic()
	case keyEnter:
		return m.openTrigger()
	case "y":
		return m.openSyncProperty()
	}
	return nil
}

func (m *eventsModel) toggleTopic() tea.Cmd {
	t := m.topics[m.selected]
	next := !t.Enabled
	sim := m.sim
	name := t.Name
	return func() tea.Msg {
		if err := sim.SetTopicEnabled(name, next); err != nil {
			return flashMsg{text: fmt.Sprintf("toggle %s: %v", name, err), kind: flashErr}
		}
		return flashMsg{text: fmt.Sprintf("topic %s %s", name, enabledWord(next)), kind: flashOK}
	}
}

func enabledWord(v bool) string {
	if v {
		return "enabled"
	}
	return "disabled"
}

func (m *eventsModel) openTrigger() tea.Cmd {
	t := m.topics[m.selected]
	if !t.Enabled {
		return func() tea.Msg {
			return flashMsg{text: "topic disabled — enable it first", kind: flashErr}
		}
	}
	var modal modalModel
	switch t.Name {
	case topicMotion, topicImgBlurry, topicImgDark, topicImgBright:
		modal = newEventTriggerModal(m.sim, t.Name, "Source token", m.defaultSource)
	case topicDigitalIn:
		modal = newEventTriggerModal(m.sim, t.Name, "Input token", "DI_0")
	default:
		modal = newRawPublishModal(m.sim, t.Name)
	}
	return func() tea.Msg { return openModalMsg{modal: modal} }
}

func (m *eventsModel) openSyncProperty() tea.Cmd {
	t := m.topics[m.selected]
	modal := newSyncPropertyModal(m.sim, t.Name, m.defaultSource)
	return func() tea.Msg { return openModalMsg{modal: modal} }
}

func (m *eventsModel) View() string {
	if len(m.topics) == 0 {
		return styleMuted.Render(
			"No topics configured. Edit onvif-simulator.json to declare topics.")
	}
	var b strings.Builder
	b.WriteString(stylePanelTitle.Render(
		fmt.Sprintf("Topics advertised by this device (%d)", len(m.topics)),
	))
	b.WriteString("\n\n")
	b.WriteString(styleTableHeader.Render(
		fmt.Sprintf("  %-4s  %-*s  %s", "EN", eventsTopicColWidth, "TOPIC", "TRIGGER"),
	))
	b.WriteString("\n")
	for i := range m.topics {
		t := m.topics[i]
		line := fmt.Sprintf("  [%s]  %-*s  %s",
			checkbox(t.Enabled),
			eventsTopicColWidth, truncate(t.Name, eventsTopicColWidth),
			triggerHint(t.Name, t.Enabled),
		)
		if i == m.selected {
			b.WriteString(styleTableRowSel.Render(line))
		} else {
			b.WriteString(styleTableRow.Render(line))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(styleMuted.Render("space: toggle · enter: trigger modal · y: SyncProperty"))
	return b.String()
}

func checkbox(v bool) string {
	if v {
		return "x"
	}
	return " "
}

func triggerHint(topic string, enabled bool) string {
	if !enabled {
		return lipgloss.NewStyle().Foreground(colorMuted).Render("(disabled)")
	}
	switch topic {
	case topicMotion, topicImgBlurry, topicImgDark, topicImgBright:
		return "source-token · on/off"
	case topicDigitalIn:
		return "input-token · on/off"
	}
	return "raw XML"
}

// ---------------------------------------------------------------------------
// Trigger modal (typed helpers — MotionAlarm / ImageToo* / DigitalInput)
// ---------------------------------------------------------------------------

type eventTriggerModal struct {
	sim   SimulatorAPI
	topic string
	label string
	input textinput.Model
	focus int // 0 = input, 1 = On, 2 = Off
}

func newEventTriggerModal(sim SimulatorAPI, topic, label, defaultToken string) *eventTriggerModal {
	ti := textinput.New()
	ti.Placeholder = defaultToken
	ti.SetValue(defaultToken)
	ti.Focus()
	ti.CharLimit = 64
	ti.Width = eventsInputWidth
	return &eventTriggerModal{sim: sim, topic: topic, label: label, input: ti}
}

func (*eventTriggerModal) Init() tea.Cmd { return textinput.Blink }

func (e *eventTriggerModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if model, cmd, handled := e.handleKey(km); handled {
			return model, cmd
		}
	}
	if e.focus == 0 {
		var cmd tea.Cmd
		e.input, cmd = e.input.Update(msg)
		return e, cmd
	}
	return e, nil
}

func (e *eventTriggerModal) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case keyEsc:
		return nil, closeModal(), true
	case keyTab:
		e.focus = (e.focus + 1) % 3
		e.syncFocus()
		return e, nil, true
	case keyShiftTab:
		e.focus = (e.focus + 2) % 3
		e.syncFocus()
		return e, nil, true
	case keyEnter:
		if e.focus == 0 {
			e.focus = 1
			e.syncFocus()
			return e, nil, true
		}
		return nil, tea.Batch(e.fire(e.focus == 1), closeModal()), true
	case keyLeft:
		if e.focus == 2 {
			e.focus = 1
		}
		return e, nil, true
	case keyRight:
		if e.focus == 1 {
			e.focus = 2
		}
		return e, nil, true
	}
	return nil, nil, false
}

func (e *eventTriggerModal) syncFocus() {
	if e.focus == 0 {
		e.input.Focus()
	} else {
		e.input.Blur()
	}
}

func (e *eventTriggerModal) fire(on bool) tea.Cmd {
	token := strings.TrimSpace(e.input.Value())
	topic := e.topic
	sim := e.sim
	return func() tea.Msg {
		switch topic {
		case topicMotion:
			sim.Motion(token, on)
		case topicImgBlurry:
			sim.ImageTooBlurry(token, on)
		case topicImgDark:
			sim.ImageTooDark(token, on)
		case topicImgBright:
			sim.ImageTooBright(token, on)
		case topicDigitalIn:
			sim.DigitalInput(token, on)
		}
		return flashMsg{
			text: fmt.Sprintf("%s sent to %s: %s", shortTopic(topic), token, onOffWord(on)),
			kind: flashOK,
		}
	}
}

func onOffWord(v bool) string {
	if v {
		return "ON"
	}
	return "OFF"
}

func (*eventTriggerModal) View() string { return "" }

func (e *eventTriggerModal) Modal(_, _ int) string {
	title := stylePanelTitle.Render("Trigger · " + shortTopic(e.topic))
	inputLine := e.label + ": " + e.input.View()
	onBtn := button("On", e.focus == 1)
	offBtn := button("Off", e.focus == 2)
	buttons := lipgloss.JoinHorizontal(lipgloss.Top, onBtn, " ", offBtn)
	help := styleMuted.Render("tab: next · enter: fire · esc: cancel")
	body := lipgloss.JoinVertical(lipgloss.Left, title, "", inputLine, "", buttons, "", help)
	return styleModal.Render(body)
}

func button(label string, focused bool) string {
	style := lipgloss.NewStyle().Padding(0, 2).Background(colorBorder).Foreground(colorFg)
	if focused {
		style = style.Background(colorAccent).Foreground(colorPanelBg).Bold(true)
	}
	return style.Render(label)
}

// closeModal returns a tea.Cmd that closes the active modal overlay.
func closeModal() tea.Cmd {
	return func() tea.Msg { return closeModalMsg{} }
}

// ---------------------------------------------------------------------------
// SyncProperty modal
// ---------------------------------------------------------------------------

type syncPropertyModal struct {
	sim    SimulatorAPI
	topic  string
	fields [eventsSyncFields]textinput.Model
	focus  int
}

func newSyncPropertyModal(sim SimulatorAPI, topic, defaultSource string) *syncPropertyModal {
	presets := []struct{ placeholder, val string }{
		{"VideoSourceConfigurationToken", "VideoSourceConfigurationToken"},
		{"source token", defaultSource},
		{"State", "State"},
		{"on|off", "on"},
	}
	m := &syncPropertyModal{sim: sim, topic: topic}
	for i, p := range presets {
		ti := textinput.New()
		ti.Placeholder = p.placeholder
		ti.SetValue(p.val)
		ti.CharLimit = 64
		ti.Width = eventsInputWidth
		m.fields[i] = ti
	}
	m.fields[0].Focus()
	return m
}

func (*syncPropertyModal) Init() tea.Cmd { return textinput.Blink }

func (s *syncPropertyModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case keyEsc:
			return nil, closeModal()
		case keyTab:
			s.fields[s.focus].Blur()
			s.focus = (s.focus + 1) % len(s.fields)
			s.fields[s.focus].Focus()
			return s, nil
		case keyShiftTab:
			s.fields[s.focus].Blur()
			s.focus = (s.focus + len(s.fields) - 1) % len(s.fields)
			s.fields[s.focus].Focus()
			return s, nil
		case keyEnter:
			return nil, tea.Batch(s.fire(), closeModal())
		}
	}
	var cmd tea.Cmd
	s.fields[s.focus], cmd = s.fields[s.focus].Update(msg)
	return s, cmd
}

func (s *syncPropertyModal) fire() tea.Cmd {
	sim := s.sim
	topic := s.topic
	src := strings.TrimSpace(s.fields[0].Value())
	tok := strings.TrimSpace(s.fields[1].Value())
	data := strings.TrimSpace(s.fields[2].Value())
	state := strings.EqualFold(strings.TrimSpace(s.fields[3].Value()), "on")
	return func() tea.Msg {
		sim.SyncProperty(topic, src, tok, data, state)
		return flashMsg{text: "SyncProperty sent for " + shortTopic(topic), kind: flashOK}
	}
}

func (*syncPropertyModal) View() string { return "" }

func (s *syncPropertyModal) Modal(_, _ int) string {
	labels := []string{"Source item", "Source token", "Data item", "State (on/off)"}
	var body strings.Builder
	body.WriteString(stylePanelTitle.Render("SyncProperty · " + shortTopic(s.topic)))
	body.WriteString("\n\n")
	for i, l := range labels {
		prefix := prefixUnsel
		if i == s.focus {
			prefix = prefixSel
		}
		fmt.Fprintf(&body, "%s%-14s %s\n", prefix, l+":", s.fields[i].View())
	}
	body.WriteString("\n")
	body.WriteString(styleMuted.Render("tab: next · enter: send · esc: cancel"))
	return styleModal.Render(body.String())
}

// ---------------------------------------------------------------------------
// Raw publish modal — fallback for unknown topics
// ---------------------------------------------------------------------------

const rawPublishInputWidth = 60
const rawPublishCharLimit = 4096

type rawPublishModal struct {
	sim   SimulatorAPI
	topic string
	input textinput.Model
}

func newRawPublishModal(sim SimulatorAPI, topic string) *rawPublishModal {
	ti := textinput.New()
	ti.Placeholder = `<tt:Message xmlns:tt="...">…</tt:Message>`
	ti.Focus()
	ti.CharLimit = rawPublishCharLimit
	ti.Width = rawPublishInputWidth
	return &rawPublishModal{sim: sim, topic: topic, input: ti}
}

func (*rawPublishModal) Init() tea.Cmd { return textinput.Blink }

func (r *rawPublishModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case keyEsc:
			return nil, closeModal()
		case keyEnter:
			sim := r.sim
			topic := r.topic
			payload := r.input.Value()
			return nil, tea.Batch(
				func() tea.Msg {
					sim.PublishRaw(topic, payload)
					return flashMsg{text: "PublishRaw sent to " + shortTopic(topic), kind: flashOK}
				},
				closeModal(),
			)
		}
	}
	var cmd tea.Cmd
	r.input, cmd = r.input.Update(msg)
	return r, cmd
}

func (*rawPublishModal) View() string { return "" }

func (r *rawPublishModal) Modal(_, _ int) string {
	body := lipgloss.JoinVertical(lipgloss.Left,
		stylePanelTitle.Render("PublishRaw · "+shortTopic(r.topic)),
		"",
		styleWarn.Render("Raw XML — no validation performed."),
		"",
		r.input.View(),
		"",
		styleMuted.Render("enter: publish · esc: cancel"),
	)
	return styleModal.Render(body)
}
