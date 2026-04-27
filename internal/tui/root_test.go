package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

var errBindFailed = errors.New("bind failed")

// sendKey is a tiny helper that feeds a single key through the root model
// and returns the cmds the root produced for inspection / batch draining.
func sendKey(t *testing.T, m *rootModel, key string) tea.Cmd {
	t.Helper()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	if key == keyTab {
		msg = tea.KeyMsg{Type: tea.KeyTab}
	}
	updated, cmd := m.Update(msg)
	if updated != m {
		t.Fatalf("root model identity changed after key %q", key)
	}
	return cmd
}

func TestRootModel_ScreenNavigation(t *testing.T) {
	sim := newMockSim()
	root := newRootModel(sim)
	// WindowSizeMsg is required before View works, but not for Update.
	if root.active != screenDashboard {
		t.Fatalf("expected to start on dashboard, got %v", root.active)
	}
	_ = sendKey(t, root, keyTab)
	if root.active != screenEvents {
		t.Fatalf("tab should advance to Events, got %v", root.active)
	}
	_ = sendKey(t, root, "3")
	if root.active != screenMedia {
		t.Fatalf("'3' should jump to Media, got %v", root.active)
	}
	_ = sendKey(t, root, "?")
	if !root.help {
		t.Fatal("? should toggle help")
	}
}

func TestDashboard_ToggleStartStop(t *testing.T) {
	sim := newMockSim()
	dash := newDashboardModel(sim)
	// Start
	_, cmd := dash.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatal("s key should produce a command")
	}
	msg := cmd()
	life, ok := msg.(lifecycleMsg)
	if !ok {
		t.Fatalf("expected lifecycleMsg, got %T", msg)
	}
	if life.action != "start" || life.err != nil {
		t.Fatalf("unexpected lifecycle msg: %+v", life)
	}
	// Stop
	_, cmd = dash.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatal("second s key should produce a command")
	}
	life, ok = cmd().(lifecycleMsg)
	if !ok {
		t.Fatal("second cmd did not produce lifecycleMsg")
	}
	if life.action != "stop" {
		t.Fatalf("expected stop action, got %+v", life)
	}
	if !slices.Contains(sim.callsCopy(), "Start") {
		t.Fatal("Start was not called")
	}
	if !slices.Contains(sim.callsCopy(), "Stop") {
		t.Fatal("Stop was not called")
	}
}

func TestEvents_ToggleTopic(t *testing.T) {
	sim := newMockSim()
	ev := newEventsModel(sim)
	ev.Update(statusMsg{s: sim.Status()})
	if len(ev.topics) == 0 {
		t.Fatal("events model should have loaded topics from snapshot")
	}
	// Toggle first topic off
	_, cmd := ev.Update(tea.KeyMsg{Type: tea.KeySpace})
	if cmd == nil {
		t.Fatal("space should produce a toggle command")
	}
	flash, ok := cmd().(flashMsg)
	if !ok {
		t.Fatalf("expected flashMsg, got %T", cmd())
	}
	if flash.kind == flashErr {
		t.Fatalf("toggle failed: %s", flash.text)
	}
	calls := sim.callsCopy()
	if !slices.Contains(calls, "SetTopicEnabled") {
		t.Fatal("SetTopicEnabled was not called")
	}
}

func TestMedia_AddProfileRequiresWidth(t *testing.T) {
	sim := newMockSim()
	p := config.ProfileConfig{Encoding: "H264"}
	form := newProfileFormModal(sim, &p, false)
	// Clear the width default so save() rejects the form.
	form.fields[fldWidth].SetValue("")
	_, err := form.save()
	if err == nil {
		t.Fatal("save should fail when width is blank")
	}
	if !strings.Contains(err.Error(), "width") {
		t.Fatalf("error should mention width: %v", err)
	}
}

// TestLog_RingBufferTruncation exercises the fixed-capacity ring.
func TestLog_RingBufferTruncation(t *testing.T) {
	m := newLogModel()
	for i := range logRingCapacity + 50 {
		m.append(&logEntry{kind: "event", detail: "e"})
		_ = i
	}
	if len(m.entries) != logRingCapacity {
		t.Fatalf("ring capacity not enforced: got %d, want %d",
			len(m.entries), logRingCapacity)
	}
}

// TestShortTopic strips the tns1: prefix for dashboard/log rendering.
func TestShortTopic(t *testing.T) {
	if got := shortTopic("tns1:VideoSource/MotionAlarm"); got != "VideoSource/MotionAlarm" {
		t.Fatalf("shortTopic: %q", got)
	}
	if got := shortTopic("custom"); got != "custom" {
		t.Fatalf("shortTopic should not mangle non-tns1 topics: %q", got)
	}
}

// ----- root model extra coverage -----------------------------------------------

func TestRootModel_FlashMessage(t *testing.T) {
	sim := newMockSim()
	root := newRootModel(sim)

	_, cmd := root.Update(flashMsg{text: "hello", kind: flashOK})
	if cmd == nil {
		t.Fatal("expected clearFlash cmd from flashMsg")
	}
	if root.flash.text != "hello" {
		t.Fatalf("expected flash text 'hello', got %q", root.flash.text)
	}
	if root.flash.kind != flashOK {
		t.Fatalf("expected flashOK, got %v", root.flash.kind)
	}
}

func TestRootModel_ClearFlashMatchingSeq(t *testing.T) {
	sim := newMockSim()
	root := newRootModel(sim)

	root.Update(flashMsg{text: "msg", kind: flashInfo})
	seq := root.flashSeq

	root.Update(clearFlashMsg{id: seq})
	if root.flash.text != "" {
		t.Fatal("matching clearFlashMsg should clear the flash")
	}
}

func TestRootModel_ClearFlashStaleSeq(t *testing.T) {
	sim := newMockSim()
	root := newRootModel(sim)

	root.Update(flashMsg{text: "first", kind: flashInfo})
	oldSeq := root.flashSeq - 1
	root.Update(flashMsg{text: "second", kind: flashOK})

	root.Update(clearFlashMsg{id: oldSeq})
	if root.flash.text == "" {
		t.Fatal("stale clearFlashMsg must not clear the current flash")
	}
}

func TestRootModel_LifecycleMsgOK(t *testing.T) {
	sim := newMockSim()
	root := newRootModel(sim)

	_, cmd := root.Update(lifecycleMsg{action: "start", err: nil})
	if cmd == nil {
		t.Fatal("expected cmd from lifecycleMsg")
	}
	if root.flash.kind != flashOK {
		t.Fatalf("expected flashOK for successful lifecycle, got %v", root.flash.kind)
	}
}

func TestRootModel_LifecycleMsgError(t *testing.T) {
	sim := newMockSim()
	root := newRootModel(sim)

	root.Update(lifecycleMsg{action: "start", err: errBindFailed})
	if root.flash.kind != flashErr {
		t.Fatalf("expected flashErr for failed lifecycle, got %v", root.flash.kind)
	}
}

func TestRootModel_ShiftTab(t *testing.T) {
	sim := newMockSim()
	root := newRootModel(sim)

	root.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if root.active != screenCount-1 {
		t.Fatalf("shift+tab from dashboard should wrap to last screen, got %v", root.active)
	}
}

func TestRootModel_NumberKeys(t *testing.T) {
	sim := newMockSim()
	root := newRootModel(sim)

	for i := 1; i <= int(screenCount); i++ {
		sendKey(t, root, string(rune('0'+i)))
		if root.active != screenID(i-1) {
			t.Errorf("key '%d' should navigate to screen %d, got %v", i, i-1, root.active)
		}
	}
}

func TestRootModel_WindowSizeUpdatesScreens(t *testing.T) {
	sim := newMockSim()
	root := newRootModel(sim)

	root.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if root.width != 120 || root.height != 40 {
		t.Fatalf("expected 120x40, got %dx%d", root.width, root.height)
	}
}

func TestRootModel_ViewBeforeWindowSize(t *testing.T) {
	sim := newMockSim()
	root := newRootModel(sim)

	if v := root.View(); v != viewInitializing {
		t.Fatalf("expected %q before WindowSizeMsg, got %q", viewInitializing, v)
	}
}

func TestRootModel_ViewAfterWindowSize(t *testing.T) {
	sim := newMockSim()
	root := newRootModel(sim)

	root.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	v := root.View()
	if v == "" || v == viewInitializing {
		t.Fatalf("unexpected view after WindowSizeMsg: %q", v)
	}
}

func TestRootModel_EventAndMutationMsgRouted(_ *testing.T) {
	sim := newMockSim()
	root := newRootModel(sim)

	root.Update(eventMsg{Time: time.Now(), Topic: "tns1:VideoSource/MotionAlarm", Source: "VS0"})
	root.Update(mutationMsg{Kind: "SetHostname", Target: "", Detail: "myhost"})
}

func TestRootModel_OpenCloseModal(_ *testing.T) {
	sim := newMockSim()
	root := newRootModel(sim)

	root.Update(openModalMsg{modal: nil})
	root.Update(closeModalMsg{})
}

// ----- dashboard extra coverage -------------------------------------------------

func TestDashboardModel_StatusTransitionCleared(t *testing.T) {
	sim := newMockSim()
	dash := newDashboardModel(sim)

	dash.transition = transitionStarting
	dash.Update(statusMsg{s: Status{Running: true}})
	if dash.transition != "" {
		t.Fatal("transition should clear when simulator reports Running=true")
	}

	dash.transition = transitionStopping
	dash.Update(statusMsg{s: Status{Running: false}})
	if dash.transition != "" {
		t.Fatal("transition should clear when simulator reports Running=false")
	}
}

func TestDashboardModel_ForceRefresh(t *testing.T) {
	sim := newMockSim()
	dash := newDashboardModel(sim)

	_, cmd := dash.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("r key should produce a refresh cmd")
	}
	if _, ok := cmd().(statusMsg); !ok {
		t.Fatalf("expected statusMsg from r key refresh")
	}
}

func TestDashboardModel_View(t *testing.T) {
	sim := newMockSim()
	dash := newDashboardModel(sim)
	dash.Update(statusMsg{s: sim.Status()})

	v := dash.View()
	if v == "" {
		t.Fatal("expected non-empty dashboard view")
	}
}

func TestDashboardModel_ViewWithEvents(t *testing.T) {
	sim := newMockSim()
	dash := newDashboardModel(sim)
	dash.Update(statusMsg{s: Status{
		Running: true,
		RecentEvents: []EventRecord{
			{Time: time.Now(), Topic: "tns1:VideoSource/MotionAlarm", Source: "VS0", Payload: "state=true"},
		},
	}})
	v := dash.View()
	if !strings.Contains(v, "VideoSource") {
		t.Fatalf("view should contain topic substring, got: %q", v)
	}
}

func TestUptimeHelper(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "-"},
		{-time.Second, "-"},
		{time.Hour + 2*time.Minute + 3*time.Second, "01:02:03"},
		{25*time.Hour + 5*time.Minute, "25:05:00"},
	}
	for _, tc := range cases {
		if got := uptime(tc.d); got != tc.want {
			t.Errorf("uptime(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestTruncateHelper(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate short string: got %q", got)
	}
	if got := truncate("hello world", 5); len([]rune(got)) != 5 {
		t.Errorf("truncate long string: got %q (len %d)", got, len(got))
	}
	if got := truncate("x", 1); got != "x" {
		t.Errorf("truncate n=1: %q", got)
	}
}

func TestTitleCaseHelper(t *testing.T) {
	if got := titleCase(""); got != "" {
		t.Errorf("titleCase('') = %q", got)
	}
	if got := titleCase("starting"); got != "Starting" {
		t.Errorf("titleCase('starting') = %q", got)
	}
}

func TestOrDashHelper(t *testing.T) {
	if got := orDash(""); got != "-" {
		t.Errorf("orDash('') = %q, want '-'", got)
	}
	if got := orDash("foo"); got != "foo" {
		t.Errorf("orDash('foo') = %q, want 'foo'", got)
	}
}

// ----- log screen extra coverage ------------------------------------------------

func TestLogModel_EventMsg(t *testing.T) {
	m := newLogModel()

	m.Update(eventMsg{Time: time.Now(), Topic: "tns1:X", Source: "VS0", Payload: "state=true"})
	if len(m.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(m.entries))
	}
	if m.entries[0].kind != "event" {
		t.Fatalf("expected kind 'event', got %q", m.entries[0].kind)
	}
}

func TestLogModel_MutationMsg(t *testing.T) {
	m := newLogModel()

	m.Update(mutationMsg{Time: time.Now(), Kind: "SetHostname", Target: "host", Detail: "myhost"})
	if len(m.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(m.entries))
	}
	if m.entries[0].kind != "mutation" {
		t.Fatalf("expected kind 'mutation', got %q", m.entries[0].kind)
	}
}

func TestLogModel_FilterToggleKeys(t *testing.T) {
	m := newLogModel()
	for i := range 3 {
		_ = i
		m.append(&logEntry{kind: "event"})
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.showEvents {
		t.Fatal("e key should toggle showEvents to false")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if m.showMuts {
		t.Fatal("m key should toggle showMuts to false")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if len(m.entries) != 0 {
		t.Fatalf("c key should clear entries, got %d", len(m.entries))
	}
}

func TestLogModel_SearchMode(t *testing.T) {
	m := newLogModel()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.searching {
		t.Fatal("/ key should enter search mode")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.searching {
		t.Fatal("esc key should exit search mode")
	}
}

// ----- auth screen extra coverage -----------------------------------------------

func TestAuthModel_NavigationKeys(t *testing.T) {
	sim := newMockSim()
	sim.snapshot.Auth.Users = []config.UserConfig{
		{Username: "a", Role: config.RoleUser},
		{Username: "b", Role: config.RoleOperator},
	}
	a := newAuthModel(sim)
	a.Update(statusMsg{s: sim.Status()})

	a.Update(tea.KeyMsg{Type: tea.KeyDown})
	if a.selected != 1 {
		t.Fatalf("down key should advance selection to 1, got %d", a.selected)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyUp})
	if a.selected != 0 {
		t.Fatalf("up key should retreat selection to 0, got %d", a.selected)
	}
}

func TestAuthModel_DeleteUserCmd(t *testing.T) {
	sim := newMockSim()
	sim.snapshot.Auth.Users = []config.UserConfig{
		{Username: "user1", Role: config.RoleUser},
	}
	a := newAuthModel(sim)
	a.Update(statusMsg{s: sim.Status()})

	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmd == nil {
		t.Fatal("d key on non-empty user list should produce a cmd")
	}
}

func TestAuthModel_DeleteUserEmptyList(t *testing.T) {
	sim := newMockSim()
	a := newAuthModel(sim)

	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmd != nil {
		t.Fatal("d key on empty list should produce no cmd")
	}
}

// TestAuthToggle flips auth via the `t` hotkey.
func TestAuthToggle(t *testing.T) {
	sim := newMockSim()
	a := newAuthModel(sim)
	a.Update(statusMsg{s: sim.Status()})
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if cmd == nil {
		t.Fatal("t key should produce a cmd")
	}
	msg := cmd()
	flash, ok := msg.(flashMsg)
	if !ok {
		t.Fatalf("expected flashMsg, got %T", msg)
	}
	if flash.kind == flashErr {
		t.Fatalf("auth toggle failed: %s", flash.text)
	}
	if !strings.Contains(strings.ToLower(flash.text), "auth") {
		t.Fatalf("flash should mention auth: %q", flash.text)
	}
}
