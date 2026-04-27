package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

const (
	authUsernameColWidth = 20
	authRoleColWidth     = 16
	authFormWidth        = 40
	authCharLimit        = 128
)

// ErrUserFormValidate is wrapped by user form validation errors.
var ErrUserFormValidate = errors.New("user form")

type authModel struct {
	sim      SimulatorAPI
	enabled  bool
	users    []config.UserConfig
	views    []UserView
	selected int
}

func newAuthModel(sim SimulatorAPI) *authModel {
	return &authModel{sim: sim}
}

func (*authModel) Init() tea.Cmd { return nil }
func (*authModel) Title() string { return "Auth" }
func (*authModel) Help() string  { return "t: toggle auth · a: add user · d: delete" }

func (m *authModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case statusMsg:
		m.refreshFromSnapshot()
	case tea.KeyMsg:
		cmd := m.handleKey(msg)
		return m, cmd
	}
	return m, nil
}

func (m *authModel) refreshFromSnapshot() {
	snap := m.sim.ConfigSnapshot()
	m.enabled = snap.Auth.Enabled
	m.users = append(m.users[:0], snap.Auth.Users...)
	m.views = append(m.views[:0], m.sim.Users()...)
	if m.selected >= len(m.users) {
		m.selected = len(m.users) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m *authModel) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "t":
		next := !m.enabled
		sim := m.sim
		return func() tea.Msg {
			if err := sim.SetAuthEnabled(next); err != nil {
				return flashMsg{text: "auth toggle: " + err.Error(), kind: flashErr}
			}
			return flashMsg{text: "auth " + enabledWord(next), kind: flashOK}
		}
	case keyUp, "k":
		if m.selected > 0 {
			m.selected--
		}
	case keyDown, "j":
		if m.selected < len(m.users)-1 {
			m.selected++
		}
	case "a":
		sim := m.sim
		return func() tea.Msg {
			return openModalMsg{modal: newUserFormModal(sim)}
		}
	case "d":
		if len(m.users) == 0 {
			return nil
		}
		u := m.users[m.selected]
		sim := m.sim
		return func() tea.Msg {
			return openModalMsg{modal: newConfirmModal(
				fmt.Sprintf("Delete user %q?", u.Username),
				deleteUserCmd(sim, u.Username),
			)}
		}
	}
	return nil
}

func deleteUserCmd(sim SimulatorAPI, username string) func() tea.Cmd {
	return func() tea.Cmd {
		return func() tea.Msg {
			if err := sim.RemoveUser(username); err != nil {
				return flashMsg{text: "delete user: " + err.Error(), kind: flashErr}
			}
			return flashMsg{text: "user " + username + " deleted", kind: flashOK}
		}
	}
}

func (m *authModel) View() string {
	var b strings.Builder
	banner := "Auth OFF — all operations succeed without credentials."
	style := styleWarn
	if m.enabled {
		banner = "Auth ON — operations require Digest or WS-UsernameToken."
		style = styleSuccess
	}
	b.WriteString(style.Render(banner))
	b.WriteString("\n\n")
	b.WriteString(stylePanelTitle.Render(fmt.Sprintf("Users (%d)", len(m.users))))
	b.WriteString("\n")
	if len(m.users) == 0 {
		b.WriteString(styleMuted.Render(
			"No users — enable auth and add one to secure the device."))
	} else {
		b.WriteString(styleTableHeader.Render(
			fmt.Sprintf("  %-*s  %-*s  %s",
				authUsernameColWidth, "USERNAME",
				authRoleColWidth, "ROLE", "SCHEMES"),
		))
		b.WriteString("\n")
		for i := range m.users {
			u := m.users[i]
			line := fmt.Sprintf("  %-*s  %-*s  %s",
				authUsernameColWidth, truncate(u.Username, authUsernameColWidth),
				authRoleColWidth, truncate(u.Role, authRoleColWidth),
				m.schemesFor(u.Username),
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
	b.WriteString(stylePanelTitle.Render("Digest / JWT tuning"))
	b.WriteString("\n")
	b.WriteString(styleMuted.Render(
		"Read-only for MVP — edit onvif-simulator.json and restart."))
	return b.String()
}

func (m *authModel) schemesFor(username string) string {
	for i := range m.views {
		if m.views[i].Username == username {
			if len(m.views[i].Roles) == 0 {
				return "(no schemes reported)"
			}
			return strings.Join(m.views[i].Roles, ", ")
		}
	}
	return "(not loaded)"
}

// ---------------------------------------------------------------------------
// User form modal — add only (MVP). Edit is deferred; operators delete+re-add.
// ---------------------------------------------------------------------------

const (
	userFldUsername int = iota
	userFldPassword
	userFldRole
	userFldCount
)

type userFormModal struct {
	sim     SimulatorAPI
	fields  [userFldCount]textinput.Model
	focus   int
	showPwd bool
	err     string
}

func newUserFormModal(sim SimulatorAPI) *userFormModal {
	m := &userFormModal{sim: sim}
	placeholders := []string{
		"username",
		"password",
		"Administrator | Operator | User | Extended",
	}
	for i, p := range placeholders {
		ti := textinput.New()
		ti.Placeholder = p
		ti.CharLimit = authCharLimit
		ti.Width = authFormWidth
		m.fields[i] = ti
	}
	m.fields[userFldPassword].EchoMode = textinput.EchoPassword
	m.fields[userFldPassword].EchoCharacter = '•'
	m.fields[userFldRole].SetValue(config.RoleOperator)
	m.fields[userFldUsername].Focus()
	return m
}

func (*userFormModal) Init() tea.Cmd { return textinput.Blink }

func (u *userFormModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if model, cmd, handled := u.handleKey(km); handled {
			return model, cmd
		}
	}
	var cmd tea.Cmd
	u.fields[u.focus], cmd = u.fields[u.focus].Update(msg)
	return u, cmd
}

func (u *userFormModal) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case keyEsc:
		return nil, closeModal(), true
	case keyTab, keyDown:
		u.fields[u.focus].Blur()
		u.focus = (u.focus + 1) % userFldCount
		u.fields[u.focus].Focus()
		return u, nil, true
	case keyShiftTab, keyUp:
		u.fields[u.focus].Blur()
		u.focus = (u.focus + userFldCount - 1) % userFldCount
		u.fields[u.focus].Focus()
		return u, nil, true
	case "ctrl+p":
		u.showPwd = !u.showPwd
		if u.showPwd {
			u.fields[userFldPassword].EchoMode = textinput.EchoNormal
		} else {
			u.fields[userFldPassword].EchoMode = textinput.EchoPassword
		}
		return u, nil, true
	case keyEnter, keyCtrlS:
		cmd, err := u.save()
		if err != nil {
			u.err = err.Error()
			return u, nil, true
		}
		return nil, tea.Batch(cmd, closeModal()), true
	}
	return nil, nil, false
}

func (u *userFormModal) save() (tea.Cmd, error) {
	user := strings.TrimSpace(u.fields[userFldUsername].Value())
	pass := u.fields[userFldPassword].Value()
	role := strings.TrimSpace(u.fields[userFldRole].Value())
	if user == "" || pass == "" || role == "" {
		return nil, fmt.Errorf("%w: username, password and role are required", ErrUserFormValidate)
	}
	cfg := config.UserConfig{Username: user, Password: pass, Role: role}
	sim := u.sim
	return func() tea.Msg {
		if err := sim.AddUser(cfg); err != nil {
			return flashMsg{text: "add user: " + err.Error(), kind: flashErr}
		}
		return flashMsg{text: "user " + user + " added", kind: flashOK}
	}, nil
}

func (*userFormModal) View() string { return "" }

func (u *userFormModal) Modal(_, _ int) string {
	labels := []string{"Username", "Password", "Role"}
	var body strings.Builder
	body.WriteString(stylePanelTitle.Render("Add user"))
	body.WriteString("\n\n")
	for i, l := range labels {
		prefix := prefixUnsel
		if i == u.focus {
			prefix = prefixSel
		}
		line := fmt.Sprintf("%s%-10s %s", prefix, l+":", u.fields[i].View())
		if i == userFldPassword {
			line += "  " + styleMuted.Render("(ctrl+p: show/hide)")
		}
		body.WriteString(line)
		body.WriteString("\n")
	}
	if u.err != "" {
		body.WriteString("\n")
		body.WriteString(styleError.Render(u.err))
		body.WriteString("\n")
	}
	body.WriteString("\n")
	body.WriteString(styleMuted.Render("tab: next · enter: save · esc: cancel"))
	return styleModal.Render(body.String())
}
