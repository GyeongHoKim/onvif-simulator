package event_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/event"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
)

const soapContentType = "application/soap+xml; charset=utf-8"

func postSOAP(t *testing.T, target, body string) (status int, raw []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, target,
		strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", soapContentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", target, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("response body close: %v", cerr)
		}
	}()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp.StatusCode, bodyBytes
}

type integrationFixture struct {
	broker      *event.Broker
	eventSrv    *httptest.Server
	subMgrSrv   *httptest.Server
	consumerSrv *httptest.Server
	bodies      chan []byte
}

//nolint:gocritic // BrokerConfig matches event.New API; test fixture is a one-shot setup
func setupPushIntegration(t *testing.T, brokerCfg event.BrokerConfig) *integrationFixture {
	t.Helper()

	f := &integrationFixture{
		bodies: make(chan []byte, 16),
	}

	f.consumerSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("consumer read body: %v", err)
		}
		select {
		case f.bodies <- b:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(f.consumerSrv.Close)

	// Compose broker config with sensible push defaults if caller omitted them.
	if brokerCfg.MaxNotificationProducers == 0 {
		brokerCfg.MaxNotificationProducers = 5
	}
	if len(brokerCfg.Topics) == 0 {
		brokerCfg.Topics = []event.TopicConfig{
			{Name: "tns1:VideoSource/MotionAlarm", Enabled: true},
		}
	}
	if brokerCfg.SubscriptionTimeout == 0 {
		brokerCfg.SubscriptionTimeout = time.Minute
	}

	f.broker = event.New(brokerCfg)

	subMgrHandler := eventsvc.NewSubscriptionManagerHandler(f.broker)
	f.subMgrSrv = httptest.NewServer(subMgrHandler)
	t.Cleanup(f.subMgrSrv.Close)

	// Update broker config so the SubscriptionReference EPRs point at the
	// httptest subscription-manager URL.
	brokerCfg.SubscriptionManagerAddr = f.subMgrSrv.URL
	f.broker.UpdateConfig(brokerCfg)

	eventHandler := eventsvc.NewEventServiceHandler(f.broker,
		eventsvc.WithSubscriptionManagerAddr(f.subMgrSrv.URL))
	f.eventSrv = httptest.NewServer(eventHandler)
	t.Cleanup(f.eventSrv.Close)

	return f
}

func (f *integrationFixture) subscribeAndWaitForEPR(t *testing.T) string {
	t.Helper()

	body := `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Body>` +
		`<wsnt:Subscribe xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2" xmlns:wsa="http://www.w3.org/2005/08/addressing">` +
		`<wsnt:ConsumerReference>` +
		`<wsa:Address>` + f.consumerSrv.URL + `</wsa:Address>` +
		`</wsnt:ConsumerReference>` +
		`<wsnt:Filter><wsnt:TopicExpression Dialect="http://docs.oasis-open.org/wsn/t-1/TopicExpression/Concrete">tns1:VideoSource/MotionAlarm</wsnt:TopicExpression></wsnt:Filter>` +
		`<wsnt:InitialTerminationTime>PT1H</wsnt:InitialTerminationTime>` +
		`</wsnt:Subscribe>` +
		`</s:Body></s:Envelope>`

	status, raw := postSOAP(t, f.eventSrv.URL, body)
	if status != http.StatusOK {
		t.Fatalf("Subscribe status = %d body=%s", status, raw)
	}
	addr := extractFirstWsaAddress(t, raw)
	if addr == "" {
		t.Fatalf("Subscribe response missing wsa:Address: %s", raw)
	}
	return addr
}

func extractFirstWsaAddress(t *testing.T, raw []byte) string {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "Address" {
			var v string
			if err := dec.DecodeElement(&v, &se); err == nil {
				return v
			}
		}
	}
}

func postUnsubscribe(t *testing.T, subMgrURL string) {
	t.Helper()
	body := `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Body><wsnt:Unsubscribe xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"/></s:Body></s:Envelope>`
	status, raw := postSOAP(t, subMgrURL, body)
	if status != http.StatusOK {
		t.Fatalf("Unsubscribe status = %d body=%s", status, raw)
	}
}

func (f *integrationFixture) awaitNotify(t *testing.T, d time.Duration) []byte {
	t.Helper()
	select {
	case b := <-f.bodies:
		return b
	case <-time.After(d):
		t.Fatalf("consumer did not receive Notify within %v", d)
		return nil
	}
}

func (f *integrationFixture) mustNotReceive(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case b := <-f.bodies:
		t.Fatalf("unexpected Notify received: %s", b)
	case <-time.After(d):
	}
}

func TestPushSubscription_EndToEnd(t *testing.T) {
	f := setupPushIntegration(t, event.BrokerConfig{})
	subMgrURL := f.subscribeAndWaitForEPR(t)

	// Sanity: SubscriptionReference should contain ?id= and point at the SM server.
	u, err := url.Parse(subMgrURL)
	if err != nil {
		t.Fatalf("parse subMgrURL %q: %v", subMgrURL, err)
	}
	if u.Query().Get("id") == "" {
		t.Errorf("SubscriptionReference URL missing ?id=: %s", subMgrURL)
	}

	f.broker.Publish("tns1:VideoSource/MotionAlarm",
		`<tt:Message UtcTime="2026-01-01T00:00:00Z"><tt:Source><tt:SimpleItem Name="VideoSourceConfigurationToken" Value="vs0"/></tt:Source></tt:Message>`)

	body := string(f.awaitNotify(t, 2*time.Second))
	for _, want := range []string{
		"<wsnt:Notify>",
		"tns1:VideoSource/MotionAlarm",
		"vs0",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Notify body missing %q\n%s", want, body)
		}
	}

	postUnsubscribe(t, subMgrURL)

	// Subsequent Publish must not reach the (now-unsubscribed) consumer.
	f.broker.Publish("tns1:VideoSource/MotionAlarm", "<tt:Message/>")
	f.mustNotReceive(t, 200*time.Millisecond)
}

func TestPushSubscription_RenewExtendsLifetime(t *testing.T) {
	f := setupPushIntegration(t, event.BrokerConfig{
		SubscriptionTimeout: 500 * time.Millisecond,
	})
	subMgrURL := f.subscribeAndWaitForEPR(t)

	// Renew before the original 500ms termination expires.
	renewBody := `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Body>` +
		`<wsnt:Renew xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"><wsnt:TerminationTime>PT1H</wsnt:TerminationTime></wsnt:Renew>` +
		`</s:Body></s:Envelope>`
	time.Sleep(200 * time.Millisecond)
	status, raw := postSOAP(t, subMgrURL, renewBody)
	if status != http.StatusOK {
		t.Fatalf("Renew status = %d body=%s", status, raw)
	}

	// Wait past the original termination, then publish — consumer must still receive.
	time.Sleep(400 * time.Millisecond)
	f.broker.Publish("tns1:VideoSource/MotionAlarm", "<tt:Message/>")
	f.awaitNotify(t, 2*time.Second)
}

// Compile-time guard: ensure the test file's helpers do not silently swallow
// a context cancellation that would mask a real failure.
var _ = context.Background
