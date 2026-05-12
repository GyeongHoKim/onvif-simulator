package event

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
)

// recvServer is a minimal consumer that captures the first POST body it sees.
type recvServer struct {
	srv  *httptest.Server
	body chan []byte
}

func newRecvServer(t *testing.T, handler func(http.ResponseWriter, *http.Request) []byte) *recvServer {
	t.Helper()
	rs := &recvServer{body: make(chan []byte, 8)}
	rs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if handler != nil {
			_ = handler(w, r)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		select {
		case rs.body <- b:
		default:
		}
	}))
	t.Cleanup(rs.srv.Close)
	return rs
}

func (rs *recvServer) URL() string { return rs.srv.URL }

func (rs *recvServer) Await(t *testing.T, d time.Duration) []byte {
	t.Helper()
	select {
	case b := <-rs.body:
		return b
	case <-time.After(d):
		t.Fatalf("consumer did not receive Notify within %v", d)
		return nil
	}
}

func (rs *recvServer) MustNotReceive(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case b := <-rs.body:
		t.Fatalf("consumer received unexpected Notify: %s", b)
	case <-time.After(d):
	}
}

func TestBroker_Publish_DeliversToPushSubscriber(t *testing.T) {
	rs := newRecvServer(t, nil)
	b := New(defaultPushCfg())
	info, err := b.Subscribe(context.Background(), eventsvc.SubscribeParams{
		ConsumerAddress: rs.URL(),
		Filter:          "tns1:VideoSource/MotionAlarm",
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	b.Publish("tns1:VideoSource/MotionAlarm",
		`<tt:Message UtcTime="2026-01-01T00:00:00Z"></tt:Message>`)

	body := string(rs.Await(t, 2*time.Second))
	for _, want := range []string{
		"<wsnt:Notify>",
		"tns1:VideoSource/MotionAlarm",
		"sub-", // subscription id in EPR
		info.SubscriptionID,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n%s", want, body)
		}
	}
}

func TestBroker_Publish_PushSkipsWhenFilterMisses(t *testing.T) {
	rs := newRecvServer(t, nil)
	b := New(defaultPushCfg())
	if _, err := b.Subscribe(context.Background(), eventsvc.SubscribeParams{
		ConsumerAddress: rs.URL(),
		Filter:          "tns1:Device/Trigger/DigitalInput",
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	// Disabled topic — Publish is a no-op anyway, so use a topic that is
	// enabled in cfg but does not match the filter.
	b.Publish("tns1:VideoSource/MotionAlarm", "<tt:Message/>")
	rs.MustNotReceive(t, 150*time.Millisecond)
}

func TestBroker_Publish_PushDoesNotBlockOnSlowConsumer(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(800 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := New(defaultPushCfg())
	if _, err := b.Subscribe(context.Background(), eventsvc.SubscribeParams{
		ConsumerAddress: srv.URL,
		Filter:          "tns1:VideoSource/MotionAlarm",
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	start := time.Now()
	b.Publish("tns1:VideoSource/MotionAlarm", "<tt:Message/>")
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Publish blocked %v on slow consumer; must return immediately", elapsed)
	}

	// Sanity check that the call eventually completes.
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&calls) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if atomic.LoadInt32(&calls) == 0 {
		t.Error("consumer was never called")
	}
}

func TestBroker_Publish_PullPathStillEnqueues(t *testing.T) {
	b := New(defaultPushCfg())
	id := mustCreateSub(t, b)

	b.Publish("tns1:VideoSource/MotionAlarm", "<tt:Message/>")

	res, err := b.PullMessages(context.Background(), id, eventsvc.PullMessagesParams{MessageLimit: 10})
	if err != nil {
		t.Fatalf("PullMessages: %v", err)
	}
	if len(res.Messages) != 1 {
		t.Errorf("PullMessages returned %d messages, want 1", len(res.Messages))
	}
}
