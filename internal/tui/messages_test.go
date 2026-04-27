package tui

import (
	"testing"
	"time"
)

func TestScreenIDString(t *testing.T) {
	cases := []struct {
		id   screenID
		want string
	}{
		{screenDashboard, "Dashboard"},
		{screenEvents, "Events"},
		{screenMedia, "Media"},
		{screenAuth, "Auth"},
		{screenDevice, "Device"},
		{screenLog, "Log"},
		{screenCount, ""},
		{screenID(99), ""},
	}
	for _, tc := range cases {
		if got := tc.id.String(); got != tc.want {
			t.Errorf("screenID(%d).String() = %q, want %q", tc.id, got, tc.want)
		}
	}
}

func TestTickStatusReturnsCmd(t *testing.T) {
	if cmd := tickStatus(); cmd == nil {
		t.Fatal("tickStatus should return a non-nil Cmd")
	}
}

func TestScheduleClearFlashReturnsCmd(t *testing.T) {
	if cmd := scheduleClearFlash(42); cmd == nil {
		t.Fatal("scheduleClearFlash should return a non-nil Cmd")
	}
}

func TestFlashKindConstants(t *testing.T) {
	if flashInfo != 0 {
		t.Fatal("flashInfo should be zero value")
	}
	if flashOK == flashErr {
		t.Fatal("flashOK and flashErr must be distinct")
	}
}

func TestMessageTypesInstantiate(t *testing.T) {
	now := time.Now()
	_ = tickStatusMsg(now)
	_ = statusMsg{s: Status{}}
	_ = eventMsg{Time: now, Topic: "tns1:X"}
	_ = mutationMsg{Time: now, Kind: "SetHostname"}
	_ = flashMsg{text: "ok", kind: flashOK}
	_ = clearFlashMsg{id: 1}
	_ = lifecycleMsg{action: "start"}
	_ = openModalMsg{}
	_ = closeModalMsg{}
}

func TestDashboardPollIntervalPositive(t *testing.T) {
	if dashboardPollInterval <= 0 {
		t.Fatal("dashboardPollInterval must be positive")
	}
}

func TestFlashTTLPositive(t *testing.T) {
	if flashTTL <= 0 {
		t.Fatal("flashTTL must be positive")
	}
}
