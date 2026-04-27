package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

const (
	mediaNameColWidth   = 16
	mediaTokenColWidth  = 16
	mediaRTSPColWidth   = 40
	profileFormWidth    = 40
	profileCharLimit    = 128
	profileLabelPadding = 18
)

// ErrProfileFormValidate is wrapped by form-validation errors.
var ErrProfileFormValidate = errors.New("profile form")

type mediaModel struct {
	sim      SimulatorAPI
	profiles []config.ProfileConfig
	sources  []string
	selected int
}

func newMediaModel(sim SimulatorAPI) *mediaModel {
	return &mediaModel{sim: sim}
}

func (*mediaModel) Init() tea.Cmd { return nil }
func (*mediaModel) Title() string { return "Media" }
func (*mediaModel) Help() string  { return "a: add · e: edit · d: delete" }

func (m *mediaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case statusMsg:
		m.refreshFromSnapshot()
	case tea.KeyMsg:
		cmd := m.handleKey(msg)
		return m, cmd
	}
	return m, nil
}

func (m *mediaModel) refreshFromSnapshot() {
	snap := m.sim.ConfigSnapshot()
	m.profiles = append(m.profiles[:0], snap.Media.Profiles...)
	seen := map[string]bool{}
	m.sources = m.sources[:0]
	for i := range m.profiles {
		tok := m.profiles[i].VideoSourceToken
		if tok == "" {
			tok = config.DefaultVideoSourceToken
		}
		if !seen[tok] {
			seen[tok] = true
			m.sources = append(m.sources, tok)
		}
	}
	if m.selected >= len(m.profiles) {
		m.selected = len(m.profiles) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m *mediaModel) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case keyUp, "k":
		if m.selected > 0 {
			m.selected--
		}
	case keyDown, "j":
		if m.selected < len(m.profiles)-1 {
			m.selected++
		}
	case "a":
		sim := m.sim
		return func() tea.Msg {
			blank := config.ProfileConfig{Encoding: "H264"}
			return openModalMsg{modal: newProfileFormModal(sim, &blank, false)}
		}
	case "e":
		if len(m.profiles) == 0 {
			return nil
		}
		p := m.profiles[m.selected]
		sim := m.sim
		return func() tea.Msg {
			return openModalMsg{modal: newProfileFormModal(sim, &p, true)}
		}
	case "d":
		if len(m.profiles) == 0 {
			return nil
		}
		p := m.profiles[m.selected]
		sim := m.sim
		return func() tea.Msg {
			return openModalMsg{modal: newConfirmModal(
				fmt.Sprintf("Delete profile %q?", p.Token),
				deleteProfileCmd(sim, p.Token),
			)}
		}
	}
	return nil
}

func deleteProfileCmd(sim SimulatorAPI, token string) func() tea.Cmd {
	return func() tea.Cmd {
		return func() tea.Msg {
			if err := sim.RemoveProfile(token); err != nil {
				return flashMsg{text: "delete: " + err.Error(), kind: flashErr}
			}
			return flashMsg{text: "profile " + token + " deleted", kind: flashOK}
		}
	}
}

func (m *mediaModel) View() string {
	var b strings.Builder
	b.WriteString(stylePanelTitle.Render(fmt.Sprintf("Media profiles (%d)", len(m.profiles))))
	b.WriteString("\n\n")
	if len(m.profiles) == 0 {
		b.WriteString(styleMuted.Render(
			"No profiles yet — press `a` to add one so the simulator answers Media traffic."))
		b.WriteString("\n")
	} else {
		b.WriteString(styleTableHeader.Render(
			fmt.Sprintf("  %-*s  %-*s  %-6s  %-12s  %s",
				mediaNameColWidth, "NAME",
				mediaTokenColWidth, "TOKEN",
				"CODEC", "RES@FPS", "RTSP"),
		))
		b.WriteString("\n")
		for i := range m.profiles {
			p := m.profiles[i]
			line := fmt.Sprintf("  %-*s  %-*s  %-6s  %-12s  %s",
				mediaNameColWidth, truncate(p.Name, mediaNameColWidth),
				mediaTokenColWidth, truncate(p.Token, mediaTokenColWidth),
				p.Encoding,
				fmt.Sprintf("%dx%d@%d", p.Width, p.Height, p.FPS),
				truncate(p.RTSP, mediaRTSPColWidth),
			)
			if i == m.selected {
				b.WriteString(styleTableRowSel.Render(line))
			} else {
				b.WriteString(styleTableRow.Render(line))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(stylePanelTitle.Render("Video sources (deduplicated)"))
	b.WriteString("\n")
	if len(m.sources) == 0 {
		b.WriteString(styleMuted.Render("(none)"))
	} else {
		b.WriteString(strings.Join(m.sources, ", "))
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Profile form modal — add + edit share the same layout.
// ---------------------------------------------------------------------------

const (
	fldName int = iota
	fldToken
	fldRTSP
	fldEncoding
	fldWidth
	fldHeight
	fldFPS
	fldBitrate
	fldGOP
	fldSnapshot
	fldVideoSource
	fldCount
)

type profileFormModal struct {
	sim    SimulatorAPI
	edit   bool
	fields [fldCount]textinput.Model
	focus  int
	err    string
}

func newProfileFormModal(sim SimulatorAPI, p *config.ProfileConfig, edit bool) *profileFormModal {
	presets := []struct{ placeholder, val string }{
		{"human-readable name", p.Name},
		{"stable token (key)", p.Token},
		{"rtsp://host:554/stream", p.RTSP},
		{"H264 | H265 | MJPEG", p.Encoding},
		{"width (pixels)", intOr(p.Width, "1920")},
		{"height (pixels)", intOr(p.Height, "1080")},
		{"fps", intOr(p.FPS, "30")},
		{"bitrate (kbps, optional)", intOrEmpty(p.Bitrate)},
		{"gop length (optional)", intOrEmpty(p.GOPLength)},
		{"http(s) snapshot URL (optional)", p.SnapshotURI},
		{"video source token (optional)", p.VideoSourceToken},
	}
	m := &profileFormModal{sim: sim, edit: edit}
	for i, l := range presets {
		ti := textinput.New()
		ti.Placeholder = l.placeholder
		ti.SetValue(l.val)
		ti.CharLimit = profileCharLimit
		ti.Width = profileFormWidth
		m.fields[i] = ti
	}
	// Name and Token are immutable on edit — token is the key.
	if edit {
		m.fields[fldName].SetValue(p.Name)
		m.fields[fldToken].SetValue(p.Token)
		m.focus = fldRTSP
		m.fields[fldRTSP].Focus()
	} else {
		m.fields[fldName].Focus()
	}
	return m
}

func intOr(v int, def string) string {
	if v != 0 {
		return strconv.Itoa(v)
	}
	return def
}

func intOrEmpty(v int) string {
	if v == 0 {
		return ""
	}
	return strconv.Itoa(v)
}

func (*profileFormModal) Init() tea.Cmd { return textinput.Blink }

func (p *profileFormModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if model, cmd, handled := p.handleKey(km); handled {
			return model, cmd
		}
	}
	var cmd tea.Cmd
	p.fields[p.focus], cmd = p.fields[p.focus].Update(msg)
	return p, cmd
}

func (p *profileFormModal) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case keyEsc:
		return nil, closeModal(), true
	case keyTab, keyDown:
		p.advanceFocus(1)
		return p, nil, true
	case keyShiftTab, keyUp:
		p.advanceFocus(-1)
		return p, nil, true
	case keyCtrlS, keyEnter:
		cmd, err := p.save()
		if err != nil {
			p.err = err.Error()
			return p, nil, true
		}
		return nil, tea.Batch(cmd, closeModal()), true
	}
	return nil, nil, false
}

func (p *profileFormModal) advanceFocus(delta int) {
	p.fields[p.focus].Blur()
	step := (delta%fldCount + fldCount) % fldCount
	p.focus = (p.focus + step) % fldCount
	if p.edit && (p.focus == fldName || p.focus == fldToken) {
		p.focus = fldRTSP
		if delta < 0 {
			p.focus = fldVideoSource
		}
	}
	p.fields[p.focus].Focus()
}

func (p *profileFormModal) save() (tea.Cmd, error) {
	name := strings.TrimSpace(p.fields[fldName].Value())
	token := strings.TrimSpace(p.fields[fldToken].Value())
	rtsp := strings.TrimSpace(p.fields[fldRTSP].Value())
	enc := strings.TrimSpace(p.fields[fldEncoding].Value())
	width, err := parseInt(p.fields[fldWidth].Value(), "width")
	if err != nil {
		return nil, err
	}
	height, err := parseInt(p.fields[fldHeight].Value(), "height")
	if err != nil {
		return nil, err
	}
	fps, err := parseInt(p.fields[fldFPS].Value(), "fps")
	if err != nil {
		return nil, err
	}
	bitrate, err := parseIntOrZero(p.fields[fldBitrate].Value(), "bitrate")
	if err != nil {
		return nil, err
	}
	gop, err := parseIntOrZero(p.fields[fldGOP].Value(), "gop")
	if err != nil {
		return nil, err
	}
	snap := strings.TrimSpace(p.fields[fldSnapshot].Value())
	src := strings.TrimSpace(p.fields[fldVideoSource].Value())

	sim := p.sim
	if p.edit {
		return editProfileCmd(sim, token, rtsp, snap, enc, width, height, fps, bitrate, gop), nil
	}
	profile := config.ProfileConfig{
		Name:             name,
		Token:            token,
		RTSP:             rtsp,
		Encoding:         enc,
		Width:            width,
		Height:           height,
		FPS:              fps,
		Bitrate:          bitrate,
		GOPLength:        gop,
		SnapshotURI:      snap,
		VideoSourceToken: src,
	}
	return addProfileCmd(sim, &profile), nil
}

func editProfileCmd(
	sim SimulatorAPI,
	token, rtsp, snap, enc string,
	width, height, fps, bitrate, gop int,
) tea.Cmd {
	return func() tea.Msg {
		if err := sim.SetProfileRTSP(token, rtsp); err != nil {
			return flashMsg{text: "rtsp: " + err.Error(), kind: flashErr}
		}
		if err := sim.SetProfileSnapshotURI(token, snap); err != nil {
			return flashMsg{text: "snapshot: " + err.Error(), kind: flashErr}
		}
		if err := sim.SetProfileEncoder(token, enc, width, height, fps, bitrate, gop); err != nil {
			return flashMsg{text: "encoder: " + err.Error(), kind: flashErr}
		}
		return flashMsg{text: "profile " + token + " saved", kind: flashOK}
	}
}

func addProfileCmd(sim SimulatorAPI, profile *config.ProfileConfig) tea.Cmd {
	p := *profile
	return func() tea.Msg {
		if err := sim.AddProfile(p); err != nil {
			return flashMsg{text: "add profile: " + err.Error(), kind: flashErr}
		}
		return flashMsg{text: "profile " + p.Token + " added", kind: flashOK}
	}
}

func parseInt(raw, field string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("%w: %s is required", ErrProfileFormValidate, field)
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %w", ErrProfileFormValidate, field, err)
	}
	return v, nil
}

func parseIntOrZero(raw, field string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %w", ErrProfileFormValidate, field, err)
	}
	return v, nil
}

func (*profileFormModal) View() string { return "" }

func (p *profileFormModal) Modal(_, _ int) string {
	title := "Add profile"
	if p.edit {
		title = "Edit profile"
	}
	labels := []string{
		"Name", "Token", "RTSP", "Encoding", "Width", "Height",
		"FPS", "Bitrate", "GOP", "Snapshot URI", "Video source",
	}
	var body strings.Builder
	body.WriteString(stylePanelTitle.Render(title))
	body.WriteString("\n\n")
	for i := range fldCount {
		prefix := prefixUnsel
		if i == p.focus {
			prefix = prefixSel
		}
		label := labels[i]
		if p.edit && (i == fldName || i == fldToken) {
			label += " (readonly)"
		}
		fmt.Fprintf(&body, "%s%-*s %s\n", prefix, profileLabelPadding, label+":", p.fields[i].View())
	}
	if p.err != "" {
		body.WriteString("\n")
		body.WriteString(styleError.Render(p.err))
		body.WriteString("\n")
	}
	body.WriteString("\n")
	body.WriteString(styleMuted.Render("tab: next · enter/ctrl+s: save · esc: cancel"))
	return styleModal.Render(body.String())
}

// ---------------------------------------------------------------------------
// Confirm modal
// ---------------------------------------------------------------------------

type confirmModal struct {
	prompt  string
	onYes   func() tea.Cmd
	focused int // 0 yes, 1 no
}

func newConfirmModal(prompt string, onYes func() tea.Cmd) *confirmModal {
	return &confirmModal{prompt: prompt, onYes: onYes, focused: 1}
}

func (*confirmModal) Init() tea.Cmd { return nil }

func (c *confirmModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil
	}
	switch km.String() {
	case keyEsc, "n":
		return nil, closeModal()
	case keyLeft, keyRight, keyTab:
		c.focused = 1 - c.focused
		return c, nil
	case "y":
		return nil, tea.Batch(c.onYes(), closeModal())
	case keyEnter:
		if c.focused == 0 {
			return nil, tea.Batch(c.onYes(), closeModal())
		}
		return nil, closeModal()
	}
	return c, nil
}

func (*confirmModal) View() string { return "" }

func (c *confirmModal) Modal(_, _ int) string {
	yes := button("Yes", c.focused == 0)
	no := button("No", c.focused == 1)
	body := lipgloss.JoinVertical(lipgloss.Left,
		stylePanelTitle.Render("Confirm"),
		"",
		c.prompt,
		"",
		lipgloss.JoinHorizontal(lipgloss.Top, yes, " ", no),
		"",
		styleMuted.Render("y: yes · n/esc: no · tab: switch · enter: activate"),
	)
	return styleModal.Render(body)
}
