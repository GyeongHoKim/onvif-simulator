package event

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
)

func waitForExpiry(t *testing.T, b *Broker, id string, deadline time.Duration) {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		b.mu.Lock()
		_, err := b.requireSub(id)
		b.mu.Unlock()
		if errors.Is(err, eventsvc.ErrSubscriptionNotFound) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("subscription %s was not force-expired within %v", id, deadline)
}

func waitForCalls(t *testing.T, counter *int32, want int32, deadline time.Duration) {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if atomic.LoadInt32(counter) >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("only %d/%d calls observed within %v", atomic.LoadInt32(counter), want, deadline)
}

func TestBroker_NotifyThreshold_ForceExpiresOn5xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := defaultPushCfg()
	cfg.NotifyFailureThreshold = 3
	b := New(cfg)
	info, err := b.Subscribe(context.Background(), eventsvc.SubscribeParams{
		ConsumerAddress: srv.URL,
		Filter:          "tns1:VideoSource/MotionAlarm",
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	for range 3 {
		b.Publish("tns1:VideoSource/MotionAlarm", "<tt:Message/>")
	}
	waitForCalls(t, &hits, 3, 2*time.Second)
	waitForExpiry(t, b, info.SubscriptionID, time.Second)
}

func TestBroker_NotifyThreshold_ForceExpiresOnNetworkErrors(t *testing.T) {
	cfg := defaultPushCfg()
	cfg.NotifyFailureThreshold = 3
	cfg.NotifyTimeout = 100 * time.Millisecond
	b := New(cfg)
	info, err := b.Subscribe(context.Background(), eventsvc.SubscribeParams{
		ConsumerAddress: "http://127.0.0.1:1/sink", // closed port
		Filter:          "tns1:VideoSource/MotionAlarm",
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	for range 3 {
		b.Publish("tns1:VideoSource/MotionAlarm", "<tt:Message/>")
	}
	waitForExpiry(t, b, info.SubscriptionID, 3*time.Second)
}

func TestBroker_NotifyThreshold_SuccessResetsCounter(t *testing.T) {
	var resp atomic.Int32
	resp.Store(http.StatusInternalServerError) // start with failures
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(resp.Load()))
	}))
	defer srv.Close()

	cfg := defaultPushCfg()
	cfg.NotifyFailureThreshold = 3
	b := New(cfg)
	info, err := b.Subscribe(context.Background(), eventsvc.SubscribeParams{
		ConsumerAddress: srv.URL,
		Filter:          "tns1:VideoSource/MotionAlarm",
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// 2 failures, then success resets, then 2 more failures: should still be alive.
	publishAndWait := func() {
		b.Publish("tns1:VideoSource/MotionAlarm", "<tt:Message/>")
		// give the goroutine a moment to update the counter
		time.Sleep(80 * time.Millisecond)
	}
	publishAndWait()
	publishAndWait()
	resp.Store(http.StatusOK)
	publishAndWait()
	resp.Store(http.StatusInternalServerError)
	publishAndWait()
	publishAndWait()

	b.mu.Lock()
	_, err = b.requireSub(info.SubscriptionID)
	b.mu.Unlock()
	if errors.Is(err, eventsvc.ErrSubscriptionNotFound) {
		t.Errorf("subscription was force-expired even though success reset the counter")
	}
}

func TestBroker_NotifyThreshold_Configurable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := defaultPushCfg()
	cfg.NotifyFailureThreshold = 1
	b := New(cfg)
	info, err := b.Subscribe(context.Background(), eventsvc.SubscribeParams{
		ConsumerAddress: srv.URL,
		Filter:          "tns1:VideoSource/MotionAlarm",
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	b.Publish("tns1:VideoSource/MotionAlarm", "<tt:Message/>")
	waitForExpiry(t, b, info.SubscriptionID, time.Second)
}
