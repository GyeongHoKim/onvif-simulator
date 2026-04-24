package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

const (
	deviceHostnameWidth  = 32
	deviceHostnameMaxLen = 253
	deviceFocusCount     = 3
	deviceKVPadding      = 18
)

type deviceModel struct {
	sim       SimulatorAPI
	snapshot  config.Config
	mode      string // Discoverable | NonDiscoverable
	hostname  textinput.Model
	focus     int // 0 segmented control, 1 hostname input, 2 save button
	hostDirty bool
}

func newDeviceModel(sim SimulatorAPI) *deviceModel {
	ti := textinput.New()
	ti.Placeholder = "hostname"
	ti.CharLimit = deviceHostnameMaxLen
	ti.Width = deviceHostnameWidth
	return &deviceModel{sim: sim, hostname: ti}
}

func (*deviceModel) Init() tea.Cmd { return nil }
func (*deviceModel) Title() string { return "Device" }
func (*deviceModel) Help() string {
	return "tab: next · ←/→: discovery · enter: save"
}

func (m *deviceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case statusMsg:
		m.refreshFromSnapshot()
	case tea.KeyMsg:
		cmd := m.handleKey(msg)
		return m, cmd
	}
	return m, nil
}

func (m *deviceModel) refreshFromSnapshot() {
	m.snapshot = m.sim.ConfigSnapshot()
	if !m.hostDirty {
		m.hostname.SetValue(m.snapshot.Runtime.Hostname)
	}
	if m.mode == "" {
		m.mode = m.snapshot.Runtime.DiscoveryMode
		if m.mode == "" {
			m.mode = "Discoverable"
		}
	}
}

func (m *deviceModel) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case keyTab:
		m.focus = (m.focus + 1) % deviceFocusCount
		m.syncFocus()
	case keyShiftTab:
		m.focus = (m.focus + deviceFocusCount - 1) % deviceFocusCount
		m.syncFocus()
	case keyLeft, "h":
		if m.focus == 0 {
			return m.changeMode("Discoverable")
		}
	case keyRight, "l":
		if m.focus == 0 {
			return m.changeMode("NonDiscoverable")
		}
	case keyEnter:
		if m.focus == 1 {
			m.focus = 2
			m.syncFocus()
			return nil
		}
		if m.focus == 2 {
			return m.saveHostname()
		}
	}
	if m.focus == 1 {
		var cmd tea.Cmd
		m.hostname, cmd = m.hostname.Update(msg)
		m.hostDirty = true
		return cmd
	}
	return nil
}

func (m *deviceModel) syncFocus() {
	if m.focus == 1 {
		m.hostname.Focus()
	} else {
		m.hostname.Blur()
	}
}

func (m *deviceModel) changeMode(mode string) tea.Cmd {
	if m.mode == mode {
		return nil
	}
	m.mode = mode
	sim := m.sim
	return func() tea.Msg {
		if err := sim.SetDiscoveryMode(mode); err != nil {
			return flashMsg{text: "discovery: " + err.Error(), kind: flashErr}
		}
		return flashMsg{text: "discovery mode → " + mode, kind: flashOK}
	}
}

func (m *deviceModel) saveHostname() tea.Cmd {
	name := strings.TrimSpace(m.hostname.Value())
	sim := m.sim
	m.hostDirty = false
	return func() tea.Msg {
		if err := sim.SetHostname(name); err != nil {
			return flashMsg{text: "hostname: " + err.Error(), kind: flashErr}
		}
		return flashMsg{text: "hostname → " + name, kind: flashOK}
	}
}

func (m *deviceModel) View() string {
	var b strings.Builder
	b.WriteString(m.identityCard())
	b.WriteString("\n")
	b.WriteString(m.networkCard())
	b.WriteString("\n")
	b.WriteString(m.runtimeCard())
	return b.String()
}

func (m *deviceModel) identityCard() string {
	d := m.snapshot.Device
	rows := [][2]string{
		{"UUID", d.UUID},
		{"Manufacturer", d.Manufacturer},
		{"Model", d.Model},
		{"Serial", d.Serial},
		{"Firmware", orDash(d.Firmware)},
		{"Scopes", strings.Join(d.Scopes, " ")},
	}
	return stylePanel.Render(renderKV("Identity (read-only)", rows))
}

func (m *deviceModel) networkCard() string {
	segment := segmentedControl(
		m.focus == 0,
		[]string{"Discoverable", "NonDiscoverable"},
		m.mode,
	)
	hostLine := "Hostname: " + m.hostname.View()
	if m.focus == 1 {
		hostLine = prefixSel + hostLine
	} else {
		hostLine = prefixUnsel + hostLine
	}
	saveBtn := button("Save", m.focus == 2)
	port := strconv.Itoa(m.snapshot.Network.HTTPPort)
	body := lipgloss.JoinVertical(lipgloss.Left,
		stylePanelTitle.Render("Network"),
		"",
		"HTTP port: "+port+"  "+styleMuted.Render("(restart to change)"),
		"",
		"Discovery: "+segment,
		"",
		hostLine,
		"  "+saveBtn,
	)
	return stylePanel.Render(body)
}

func (m *deviceModel) runtimeCard() string {
	r := m.snapshot.Runtime
	dns := strings.Join(r.DNS.DNSManual, ", ")
	if dns == "" {
		dns = "(none)"
	}
	gw := strings.Join(r.DefaultGateway.IPv4Address, ", ")
	if gw == "" {
		gw = "(none)"
	}
	protocols := make([]string, 0, len(r.NetworkProtocols))
	for i := range r.NetworkProtocols {
		protocols = append(protocols,
			r.NetworkProtocols[i].Name+"="+enabledWord(r.NetworkProtocols[i].Enabled))
	}
	ifaces := make([]string, 0, len(r.NetworkInterfaces))
	for i := range r.NetworkInterfaces {
		ifaces = append(ifaces, r.NetworkInterfaces[i].Token)
	}
	rows := [][2]string{
		{"DNS", dns},
		{"Default gateway", gw},
		{"Network protocols", orDashStrList(protocols)},
		{"Interfaces", orDashStrList(ifaces)},
		{"System date/time", orDash(r.SystemDateAndTime.ManualDateTimeUTC)},
	}
	body := renderKV("Runtime (read-only for MVP)", rows)
	body += "\n" + styleMuted.Render(
		"These fields are set by ONVIF Set* operations from clients "+
			"and mirrored here for visibility.")
	return stylePanel.Render(body)
}

func orDashStrList(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ", ")
}

func renderKV(title string, rows [][2]string) string {
	var b strings.Builder
	b.WriteString(stylePanelTitle.Render(title))
	b.WriteString("\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "  %-*s %s\n", deviceKVPadding, r[0]+":", orDash(r[1]))
	}
	return b.String()
}

func segmentedControl(focused bool, options []string, value string) string {
	parts := make([]string, 0, len(options))
	for _, opt := range options {
		style := lipgloss.NewStyle().Padding(0, 2).Background(colorBorder).Foreground(colorFg)
		if opt == value {
			style = style.Background(colorAccent).Foreground(colorPanelBg).Bold(true)
		}
		if focused && opt == value {
			style = style.Background(colorAccentAlt).Foreground(colorPanelBg).Bold(true)
		}
		parts = append(parts, style.Render(opt))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}
