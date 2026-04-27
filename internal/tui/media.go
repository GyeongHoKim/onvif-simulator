package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

const (
	mediaNameColWidth   = 16
	mediaTokenColWidth  = 16
	mediaFileColWidth   = 40
	profileFormWidth    = 40
	profileCharLimit    = 256
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
			blank := config.ProfileConfig{}
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
		m.appendVideoSources(&b)
		return b.String()
	}
	b.WriteString(styleTableHeader.Render(
		fmt.Sprintf("  %-*s  %-*s  %-6s  %-12s  %s",
			mediaNameColWidth, "NAME",
			mediaTokenColWidth, "TOKEN",
			"CODEC", "RES@FPS", "FILE"),
	))
	b.WriteString("\n")
	for i := range m.profiles {
		p := m.profiles[i]
		codec := p.Encoding
		if codec == "" {
			codec = "auto"
		}
		res := "auto"
		if p.Width > 0 && p.Height > 0 && p.FPS > 0 {
			res = fmt.Sprintf("%dx%d@%d", p.Width, p.Height, p.FPS)
		}
		line := fmt.Sprintf("  %-*s  %-*s  %-6s  %-12s  %s",
			mediaNameColWidth, truncate(p.Name, mediaNameColWidth),
			mediaTokenColWidth, truncate(p.Token, mediaTokenColWidth),
			codec, res,
			truncate(p.MediaFilePath, mediaFileColWidth),
		)
		if i == m.selected {
			b.WriteString(styleTableRowSel.Render(line))
		} else {
			b.WriteString(styleTableRow.Render(line))
		}
		b.WriteString("\n")
	}
	m.appendVideoSources(&b)
	return b.String()
}

func (m *mediaModel) appendVideoSources(b *strings.Builder) {
	b.WriteString("\n")
	b.WriteString(stylePanelTitle.Render("Video sources (deduplicated)"))
	b.WriteString("\n")
	if len(m.sources) == 0 {
		b.WriteString(styleMuted.Render("(none)"))
		return
	}
	b.WriteString(strings.Join(m.sources, ", "))
}

// ---------------------------------------------------------------------------
// Profile form modal — add + edit share the same layout.
// ---------------------------------------------------------------------------

const (
	fldName int = iota
	fldToken
	fldMediaFile
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
		{"/absolute/path/to/video.mp4", p.MediaFilePath},
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
		m.focus = fldMediaFile
		m.fields[fldMediaFile].Focus()
	} else {
		m.fields[fldName].Focus()
	}
	return m
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
		return nil, tea.Batch(p.save(), closeModal()), true
	}
	return nil, nil, false
}

func (p *profileFormModal) advanceFocus(delta int) {
	p.fields[p.focus].Blur()
	step := (delta%fldCount + fldCount) % fldCount
	p.focus = (p.focus + step) % fldCount
	if p.edit && (p.focus == fldName || p.focus == fldToken || p.focus == fldVideoSource) {
		p.focus = fldMediaFile
		if delta < 0 {
			p.focus = fldSnapshot
		}
	}
	p.fields[p.focus].Focus()
}

func (p *profileFormModal) save() tea.Cmd {
	v := profileFormValues{
		name:      strings.TrimSpace(p.fields[fldName].Value()),
		token:     strings.TrimSpace(p.fields[fldToken].Value()),
		mediaFile: strings.TrimSpace(p.fields[fldMediaFile].Value()),
		snap:      strings.TrimSpace(p.fields[fldSnapshot].Value()),
		src:       strings.TrimSpace(p.fields[fldVideoSource].Value()),
	}
	sim := p.sim
	if p.edit {
		return editProfileCmd(sim, &v)
	}
	profile := config.ProfileConfig{
		Name:             v.name,
		Token:            v.token,
		MediaFilePath:    v.mediaFile,
		SnapshotURI:      v.snap,
		VideoSourceToken: v.src,
	}
	return addProfileCmd(sim, &profile)
}

type profileFormValues struct {
	name, token, mediaFile, snap, src string
}

func editProfileCmd(sim SimulatorAPI, v *profileFormValues) tea.Cmd {
	return func() tea.Msg {
		if err := sim.SetProfileMediaFilePath(v.token, v.mediaFile); err != nil {
			return flashMsg{text: "media file: " + err.Error(), kind: flashErr}
		}
		if err := sim.SetProfileSnapshotURI(v.token, v.snap); err != nil {
			return flashMsg{text: "snapshot: " + err.Error(), kind: flashErr}
		}
		return flashMsg{text: "profile " + v.token + " saved", kind: flashOK}
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

func (*profileFormModal) View() string { return "" }

func (p *profileFormModal) Modal(_, _ int) string {
	title := "Add profile"
	if p.edit {
		title = "Edit profile"
	}
	labels := []string{
		"Name", "Token", "Media file", "Snapshot URI", "Video source",
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
		if p.edit && (i == fldName || i == fldToken || i == fldVideoSource) {
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
