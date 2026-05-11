package event

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
)

func defaultPushCfg() BrokerConfig {
	cfg := defaultCfg()
	cfg.MaxNotificationProducers = 5
	return cfg
}

func TestBroker_Subscribe_ReturnsIDAndTerminationTime(t *testing.T) {
	b := New(defaultPushCfg())
	info, err := b.Subscribe(context.Background(), eventsvc.SubscribeParams{
		ConsumerAddress:        "http://consumer.example/sink",
		InitialTerminationTime: "PT30S",
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if info.SubscriptionID == "" {
		t.Error("SubscriptionID must not be empty")
	}
	if info.CurrentTime.IsZero() {
		t.Error("CurrentTime must be set")
	}
	got := time.Until(info.TerminationTime)
	if got < 29*time.Second || got > 31*time.Second {
		t.Errorf("TerminationTime ~30s expected, got %v from now", got)
	}
}

func TestBroker_Subscribe_RejectsEmptyConsumerAddress(t *testing.T) {
	b := New(defaultPushCfg())
	_, err := b.Subscribe(context.Background(), eventsvc.SubscribeParams{
		ConsumerAddress: "",
	})
	if !errors.Is(err, eventsvc.ErrInvalidArgs) {
		t.Errorf("expected ErrInvalidArgs, got %v", err)
	}
}

func TestBroker_Subscribe_MaxNotificationProducersApplies(t *testing.T) {
	cfg := defaultPushCfg()
	cfg.MaxNotificationProducers = 1
	b := New(cfg)
	if _, err := b.Subscribe(context.Background(), eventsvc.SubscribeParams{
		ConsumerAddress: "http://c/1",
	}); err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}
	if _, err := b.Subscribe(context.Background(), eventsvc.SubscribeParams{
		ConsumerAddress: "http://c/2",
	}); err == nil {
		t.Error("second Subscribe must fail when cap reached")
	}
}

func TestBroker_Subscribe_DoesNotConsumePullPointCapacity(t *testing.T) {
	cfg := defaultPushCfg()
	cfg.MaxPullPoints = 1
	cfg.MaxNotificationProducers = 1
	b := New(cfg)
	// Fill the pull-point cap.
	mustCreateSub(t, b)
	// Push subscription must still succeed — separate capacity.
	if _, err := b.Subscribe(context.Background(), eventsvc.SubscribeParams{
		ConsumerAddress: "http://c/1",
	}); err != nil {
		t.Fatalf("push Subscribe must not be blocked by full pull-point cap: %v", err)
	}
}

func TestBroker_Subscribe_RejectedWhenCapZero(t *testing.T) {
	cfg := defaultCfg()
	cfg.MaxNotificationProducers = 0 // default; NotificationProducer disabled per ONVIF Core §9.3.2
	b := New(cfg)
	_, err := b.Subscribe(context.Background(), eventsvc.SubscribeParams{
		ConsumerAddress: "http://c/1",
	})
	if err == nil {
		t.Error("Subscribe must fail when MaxNotificationProducers is 0")
	}
}
