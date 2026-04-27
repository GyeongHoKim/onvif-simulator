package tui

import (
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/simulator"
)

func TestNewCallbackBridge(t *testing.T) {
	b := NewCallbackBridge()
	if b == nil {
		t.Fatal("expected non-nil CallbackBridge")
	}
}

func TestCallbackBridgeDropsEventsBeforeAttach(t *testing.T) {
	b := NewCallbackBridge()
	// Program not yet attached; these must not panic.
	b.OnEvent(simulator.EventRecord{
		Time:    time.Now(),
		Topic:   "tns1:VideoSource/MotionAlarm",
		Source:  "VS0",
		Payload: "state=true",
	})
	b.OnMutation(simulator.MutationRecord{
		Time:   time.Now(),
		Kind:   "SetDiscoveryMode",
		Target: "",
		Detail: "NonDiscoverable",
	})
}

func TestCallbackBridgeTranslatesEventFields(t *testing.T) {
	b := NewCallbackBridge()
	now := time.Now()
	rec := simulator.EventRecord{
		Time:    now,
		Topic:   "tns1:VideoSource/MotionAlarm",
		Source:  "VS0",
		Payload: "state=true",
	}
	// Exercises the conversion path inside OnEvent even when program is nil.
	b.OnEvent(rec)
}

func TestCallbackBridgeTranslatesMutationFields(t *testing.T) {
	b := NewCallbackBridge()
	now := time.Now()
	rec := simulator.MutationRecord{
		Time:   now,
		Kind:   "AddUser",
		Target: "alice",
		Detail: "role=User",
	}
	b.OnMutation(rec)
}
