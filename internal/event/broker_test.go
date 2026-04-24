package event

import (
	"context"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
)

func defaultCfg() BrokerConfig {
	return BrokerConfig{
		MaxPullPoints:       5,
		SubscriptionTimeout: time.Minute,
		Topics: []TopicConfig{
			{Name: "tns1:VideoSource/MotionAlarm", Enabled: true},
			{Name: "tns1:Device/Trigger/DigitalInput", Enabled: false},
		},
	}
}

func mustCreateSub(t *testing.T, b *Broker) string {
	t.Helper()
	info, err := b.CreatePullPointSubscription(context.Background(), eventsvc.CreatePullPointSubscriptionParams{})
	if err != nil {
		t.Fatalf("CreatePullPointSubscription: %v", err)
	}
	return info.SubscriptionID
}

// ---------- EventServiceCapabilities -------------------------------------------

func TestBroker_EventServiceCapabilities(t *testing.T) {
	b := New(defaultCfg())
	caps, err := b.EventServiceCapabilities(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !caps.WSPullPointSupport {
		t.Error("WSPullPointSupport must be true")
	}
	if caps.MaxPullPoints != 5 {
		t.Errorf("MaxPullPoints = %d, want 5", caps.MaxPullPoints)
	}
}

// ---------- EventProperties -----------------------------------------------------

func TestBroker_EventProperties_EnabledTopicsAppear(t *testing.T) {
	b := New(defaultCfg())
	props, err := b.EventProperties(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !props.FixedTopicSet {
		t.Error("FixedTopicSet must be true")
	}
	// Enabled topic must appear in TopicSet XML.
	if props.TopicSet == "" {
		t.Fatal("TopicSet must not be empty when enabled topics exist")
	}
	if !containsStr(props.TopicSet, "tns1:VideoSource/MotionAlarm") {
		t.Errorf("enabled topic not in TopicSet: %s", props.TopicSet)
	}
	// Disabled topic must not appear.
	if containsStr(props.TopicSet, "tns1:Device/Trigger/DigitalInput") {
		t.Errorf("disabled topic must not be in TopicSet: %s", props.TopicSet)
	}
}

// ---------- CreatePullPointSubscription ----------------------------------------

func TestBroker_CreatePullPointSubscription_ReturnsID(t *testing.T) {
	b := New(defaultCfg())
	info, err := b.CreatePullPointSubscription(context.Background(), eventsvc.CreatePullPointSubscriptionParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.SubscriptionID == "" {
		t.Error("SubscriptionID must not be empty")
	}
	if info.TerminationTime.IsZero() {
		t.Error("TerminationTime must not be zero")
	}
}

func TestBroker_CreatePullPointSubscription_MaxReached(t *testing.T) {
	b := New(BrokerConfig{MaxPullPoints: 2, SubscriptionTimeout: time.Minute})
	mustCreateSub(t, b)
	mustCreateSub(t, b)
	_, err := b.CreatePullPointSubscription(context.Background(), eventsvc.CreatePullPointSubscriptionParams{})
	if err == nil {
		t.Fatal("expected error when max pull points reached")
	}
}

func TestBroker_CreatePullPointSubscription_ISO8601Timeout(t *testing.T) {
	b := New(defaultCfg())
	info, err := b.CreatePullPointSubscription(context.Background(), eventsvc.CreatePullPointSubscriptionParams{
		InitialTerminationTime: "PT30S",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	remaining := time.Until(info.TerminationTime)
	if remaining < 25*time.Second || remaining > 31*time.Second {
		t.Errorf("unexpected termination time window: %v remaining", remaining)
	}
}

// ---------- Publish + PullMessages ---------------------------------------------

func TestBroker_Publish_DeliveredToPullMessages(t *testing.T) {
	b := New(defaultCfg())
	id := mustCreateSub(t, b)

	b.Publish("tns1:VideoSource/MotionAlarm", "<tt:Message/>")

	result, err := b.PullMessages(context.Background(), id, eventsvc.PullMessagesParams{})
	if err != nil {
		t.Fatalf("PullMessages: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	if result.Messages[0].Topic != "tns1:VideoSource/MotionAlarm" {
		t.Errorf("topic = %q", result.Messages[0].Topic)
	}
}

func TestBroker_Publish_MessageLimit(t *testing.T) {
	b := New(defaultCfg())
	id := mustCreateSub(t, b)

	for range 5 {
		b.Publish("tns1:VideoSource/MotionAlarm", "<tt:Message/>")
	}

	result, err := b.PullMessages(context.Background(), id, eventsvc.PullMessagesParams{MessageLimit: 3})
	if err != nil {
		t.Fatalf("PullMessages: %v", err)
	}
	if len(result.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result.Messages))
	}

	// Second pull gets the remaining 2.
	result2, err := b.PullMessages(context.Background(), id, eventsvc.PullMessagesParams{})
	if err != nil {
		t.Fatalf("PullMessages second: %v", err)
	}
	if len(result2.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result2.Messages))
	}
}

func TestBroker_Publish_TopicFilter(t *testing.T) {
	b := New(defaultCfg())

	motionID, err := b.CreatePullPointSubscription(context.Background(), eventsvc.CreatePullPointSubscriptionParams{
		Filter: "tns1:VideoSource/MotionAlarm",
	})
	if err != nil {
		t.Fatalf("CreatePullPointSubscription: %v", err)
	}

	b.Publish("tns1:VideoSource/MotionAlarm", "<tt:Message/>")
	b.Publish("tns1:Device/Trigger/DigitalInput", "<tt:Message/>")

	result, err := b.PullMessages(context.Background(), motionID.SubscriptionID, eventsvc.PullMessagesParams{})
	if err != nil {
		t.Fatalf("PullMessages: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message for filtered sub, got %d", len(result.Messages))
	}
}

func TestBroker_PullMessages_EmptyQueue(t *testing.T) {
	b := New(defaultCfg())
	id := mustCreateSub(t, b)

	result, err := b.PullMessages(context.Background(), id, eventsvc.PullMessagesParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Messages) != 0 {
		t.Fatalf("expected empty messages, got %d", len(result.Messages))
	}
}

func TestBroker_PullMessages_UnknownSubscription(t *testing.T) {
	b := New(defaultCfg())
	_, err := b.PullMessages(context.Background(), "nonexistent", eventsvc.PullMessagesParams{})
	if err == nil {
		t.Fatal("expected error for unknown subscription ID")
	}
}

// ---------- Renew ---------------------------------------------------------------

func TestBroker_Renew(t *testing.T) {
	b := New(defaultCfg())
	id := mustCreateSub(t, b)

	result, err := b.Renew(context.Background(), id, eventsvc.RenewParams{TerminationTime: "PT2H"})
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	remaining := time.Until(result.TerminationTime)
	if remaining < time.Hour || remaining > 3*time.Hour {
		t.Errorf("unexpected remaining time after Renew: %v", remaining)
	}
}

func TestBroker_Renew_UnknownSubscription(t *testing.T) {
	b := New(defaultCfg())
	_, err := b.Renew(context.Background(), "nonexistent", eventsvc.RenewParams{})
	if err == nil {
		t.Fatal("expected error for unknown subscription ID")
	}
}

// ---------- Unsubscribe ---------------------------------------------------------

func TestBroker_Unsubscribe(t *testing.T) {
	b := New(defaultCfg())
	id := mustCreateSub(t, b)

	if err := b.Unsubscribe(context.Background(), id); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	_, err := b.PullMessages(context.Background(), id, eventsvc.PullMessagesParams{})
	if err == nil {
		t.Fatal("expected error after Unsubscribe")
	}
}

func TestBroker_Unsubscribe_UnknownSubscription(t *testing.T) {
	b := New(defaultCfg())
	if err := b.Unsubscribe(context.Background(), "nonexistent"); err == nil {
		t.Fatal("expected error for unknown subscription ID")
	}
}

// ---------- SetSynchronizationPoint --------------------------------------------

func TestBroker_SetSynchronizationPoint(t *testing.T) {
	b := New(defaultCfg())
	id := mustCreateSub(t, b)
	if err := b.SetSynchronizationPoint(context.Background(), id); err != nil {
		t.Fatalf("SetSynchronizationPoint: %v", err)
	}
}

func TestBroker_SetSynchronizationPoint_Unknown(t *testing.T) {
	b := New(defaultCfg())
	if err := b.SetSynchronizationPoint(context.Background(), "none"); err == nil {
		t.Fatal("expected error for unknown subscription ID")
	}
}

// ---------- UpdateConfig --------------------------------------------------------

func TestBroker_UpdateConfig(t *testing.T) {
	b := New(defaultCfg())
	b.UpdateConfig(BrokerConfig{
		MaxPullPoints:       2,
		SubscriptionTimeout: 30 * time.Second,
		Topics:              []TopicConfig{{Name: "tns1:VideoSource/MotionAlarm", Enabled: true}},
	})
	caps, err := b.EventServiceCapabilities(context.Background())
	if err != nil {
		t.Fatalf("EventServiceCapabilities: %v", err)
	}
	if caps.MaxPullPoints != 2 {
		t.Errorf("MaxPullPoints after UpdateConfig = %d, want 2", caps.MaxPullPoints)
	}
}

// ---------- helpers -------------------------------------------------------------

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (s != "" && containsSubstr(s, sub)))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
