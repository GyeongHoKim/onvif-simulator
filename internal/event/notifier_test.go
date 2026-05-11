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
)

func TestBuildNotifyEnvelope_Basic(t *testing.T) {
	body := buildNotifyEnvelope(&pushDispatch{
		subscriptionAddr: "http://device/sm?id=sub-1",
		consumer: consumerEPR{
			address: "http://consumer/sink",
		},
		topic:   "tns1:VideoSource/MotionAlarm",
		message: `<tt:Message UtcTime="2026-01-01T00:00:00Z"></tt:Message>`,
	})
	got := string(body)
	for _, want := range []string{
		"<env:Envelope",
		"xmlns:wsnt=",
		"xmlns:wsa=",
		"<wsnt:Notify>",
		"<wsnt:NotificationMessage>",
		"<wsnt:SubscriptionReference><wsa:Address>http://device/sm?id=sub-1</wsa:Address></wsnt:SubscriptionReference>",
		"tns1:VideoSource/MotionAlarm",
		`<tt:Message UtcTime="2026-01-01T00:00:00Z">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("envelope missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestBuildNotifyEnvelope_EchoesReferenceParametersInHeader(t *testing.T) {
	body := buildNotifyEnvelope(&pushDispatch{
		subscriptionAddr: "http://device/sm?id=sub-1",
		consumer: consumerEPR{
			address:         "http://consumer/sink",
			referenceParams: "<wsa:MyId>abc</wsa:MyId>",
		},
		topic:   "tns1:Foo",
		message: "<tt:Message/>",
	})
	got := string(body)
	if !strings.Contains(got, "<env:Header>") {
		t.Fatalf("expected env:Header, got\n%s", got)
	}
	// The ReferenceParameters inner XML must appear inside the Header element,
	// before the Body, so consumers identify the subscription.
	headerStart := strings.Index(got, "<env:Header>")
	headerEnd := strings.Index(got, "</env:Header>")
	if headerStart < 0 || headerEnd < 0 {
		t.Fatalf("malformed Header in\n%s", got)
	}
	hdr := got[headerStart:headerEnd]
	if !strings.Contains(hdr, "<wsa:MyId>abc</wsa:MyId>") {
		t.Errorf("ReferenceParameters not echoed inside Header: %q", hdr)
	}
}

func TestBuildNotifyEnvelope_NoHeaderWhenNoReferenceParameters(t *testing.T) {
	body := buildNotifyEnvelope(&pushDispatch{
		subscriptionAddr: "http://device/sm?id=sub-1",
		consumer:         consumerEPR{address: "http://consumer/sink"},
		topic:            "tns1:Foo",
		message:          "<tt:Message/>",
	})
	got := string(body)
	if strings.Contains(got, "<env:Header>") {
		t.Errorf("env:Header must be omitted when ReferenceParameters is empty; got\n%s", got)
	}
}

func TestNotifier_DeliverPOSTsSOAP(t *testing.T) {
	var hits int32
	var capturedCT string
	var capturedSOAPAction string
	var capturedBody []byte
	done := make(chan struct{}, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		capturedCT = r.Header.Get("Content-Type")
		capturedSOAPAction = r.Header.Get("SOAPAction")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		capturedBody = body
		w.WriteHeader(http.StatusOK)
		done <- struct{}{}
	}))
	defer srv.Close()

	n := newNotifier(0, nil) // default timeout & discard logger
	var resultOK atomic.Bool
	n.deliver(context.Background(), &pushDispatch{
		subscriptionAddr: "http://device/sm?id=sub-1",
		consumer:         consumerEPR{address: srv.URL},
		topic:            "tns1:VideoSource/MotionAlarm",
		message:          "<tt:Message/>",
	}, func(success bool) { resultOK.Store(success) })

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("notifier did not POST within 1s")
	}
	if !strings.Contains(capturedCT, "application/soap+xml") {
		t.Errorf("Content-Type = %q, want application/soap+xml", capturedCT)
	}
	if capturedSOAPAction == "" {
		t.Error("SOAPAction header must be set")
	}
	if !strings.Contains(string(capturedBody), "<wsnt:Notify>") {
		t.Errorf("captured body missing wsnt:Notify: %s", capturedBody)
	}
	if !resultOK.Load() {
		t.Error("onResult must be called with success=true on 2xx")
	}
}

func TestNotifier_DeliverReportsFailureOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := newNotifier(0, nil)
	resultCh := make(chan bool, 1)
	n.deliver(context.Background(), &pushDispatch{
		subscriptionAddr: "http://device/sm?id=sub-1",
		consumer:         consumerEPR{address: srv.URL},
		topic:            "tns1:Foo",
		message:          "<tt:Message/>",
	}, func(success bool) { resultCh <- success })

	select {
	case ok := <-resultCh:
		if ok {
			t.Error("onResult must be called with success=false on 5xx")
		}
	case <-time.After(time.Second):
		t.Fatal("notifier did not report result within 1s")
	}
}

func TestNotifier_DeliverReportsFailureOnNetworkError(t *testing.T) {
	n := newNotifier(200*time.Millisecond, nil)
	resultCh := make(chan bool, 1)
	n.deliver(context.Background(), &pushDispatch{
		subscriptionAddr: "http://device/sm?id=sub-1",
		consumer:         consumerEPR{address: "http://127.0.0.1:1/sink"},
		topic:            "tns1:Foo",
		message:          "<tt:Message/>",
	}, func(success bool) { resultCh <- success })

	select {
	case ok := <-resultCh:
		if ok {
			t.Error("onResult must be called with success=false on connect refused")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("notifier did not report result within 2s")
	}
}
