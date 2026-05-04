package event

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
)

// TestBroker_UpdateConfigPreservesLoggerOnNil locks the regression that
// hot-reload of topics / pull-point limits via UpdateConfig must not
// silently drop the logger the simulator wired in at New.
func TestBroker_UpdateConfigPreservesLoggerOnNil(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := defaultCfg()
	cfg.Logger = logger
	b := New(cfg)

	// Reload with no logger — broker should keep the original one.
	updated := defaultCfg()
	updated.Logger = nil
	b.UpdateConfig(updated)

	b.Publish("tns1:VideoSource/MotionAlarm", `<tt:Message/>`)
	if !strings.Contains(buf.String(), "publish") {
		t.Errorf("logger lost after UpdateConfig with nil Logger; output=%q", buf.String())
	}
}

func TestBroker_UpdateConfigSwapsLogger(t *testing.T) {
	t.Parallel()
	var first, second bytes.Buffer
	firstLogger := slog.New(slog.NewJSONHandler(&first, &slog.HandlerOptions{Level: slog.LevelDebug}))
	secondLogger := slog.New(slog.NewJSONHandler(&second, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := defaultCfg()
	cfg.Logger = firstLogger
	b := New(cfg)

	b.Publish("tns1:VideoSource/MotionAlarm", `<tt:Message/>`)
	if !strings.Contains(first.String(), "publish") {
		t.Fatalf("first logger missing initial record: %q", first.String())
	}

	swap := defaultCfg()
	swap.Logger = secondLogger
	b.UpdateConfig(swap)

	b.Publish("tns1:VideoSource/MotionAlarm", `<tt:Message/>`)
	if !strings.Contains(second.String(), "publish") {
		t.Errorf("second logger missing record after swap: %q", second.String())
	}
}

func TestBroker_PublishLogsOnDisabledTopic(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := defaultCfg()
	cfg.Logger = logger
	b := New(cfg)

	b.Publish("tns1:Device/Trigger/DigitalInput", `<tt:Message/>`) // disabled topic
	out := buf.String()
	if !strings.Contains(out, "drop publish") && !strings.Contains(out, "DigitalInput") {
		t.Errorf("disabled-topic drop not logged: %q", out)
	}
}

func TestBroker_CreatePullPointLogsOverflow(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := BrokerConfig{
		MaxPullPoints:       1,
		SubscriptionTimeout: time.Minute,
		Topics: []TopicConfig{
			{Name: "tns1:VideoSource/MotionAlarm", Enabled: true},
		},
		Logger: logger,
	}
	b := New(cfg)
	if _, err := b.CreatePullPointSubscription(context.Background(), eventsvc.CreatePullPointSubscriptionParams{}); err != nil {
		t.Fatalf("first sub: %v", err)
	}
	if _, err := b.CreatePullPointSubscription(context.Background(), eventsvc.CreatePullPointSubscriptionParams{}); err == nil {
		t.Fatal("second sub: expected error on overflow")
	}
	if !strings.Contains(buf.String(), "max pull points reached") {
		t.Errorf("overflow log missing: %q", buf.String())
	}
}
