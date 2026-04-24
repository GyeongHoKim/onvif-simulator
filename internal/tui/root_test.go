package tui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

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
