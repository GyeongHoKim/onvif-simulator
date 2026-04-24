package eventsvc

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func soapRequestWSN(op, inner string) string {
	return `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Body><wsnt:` + op + ` xmlns:wsnt="` + WSNBaseNotificationNS + `">` + inner + `</wsnt:` + op + `></s:Body></s:Envelope>`
}

func soapRequestSubEvents(op, inner string) string {
	return `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Body><tev:` + op + ` xmlns:tev="` + EventsNamespace + `">` + inner + `</tev:` + op + `></s:Body></s:Envelope>`
}

func doSubRequest(
	t *testing.T, h *SubscriptionManagerHandler, rawBody, subscriptionID string,
) *httptest.ResponseRecorder {
	t.Helper()
	path := SubscriptionManagerPath
	if subscriptionID != "" {
		path += "?id=" + subscriptionID
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path,
		bytes.NewBufferString(rawBody))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// ---------- PullMessages ---------------------------------------------------------

func TestSubscriptionManager_PullMessages(t *testing.T) {
	h := NewSubscriptionManagerHandler(testStubProvider{})
	body := soapRequestSubEvents("PullMessages",
		`<tev:Timeout>PT5S</tev:Timeout><tev:MessageLimit>10</tev:MessageLimit>`)
	rec := doSubRequest(t, h, body, "sub-001")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "PullMessagesResponse") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "CurrentTime") {
		t.Fatalf("missing CurrentTime: %s", rec.Body.String())
	}
}

func TestSubscriptionManager_PullMessages_EmptyMessages(t *testing.T) {
	h := NewSubscriptionManagerHandler(testStubProvider{})
	body := soapRequestSubEvents("PullMessages", "")
	rec := doSubRequest(t, h, body, "sub-001")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSubscriptionManager_PullMessages_InvalidMessageLimit(t *testing.T) {
	h := NewSubscriptionManagerHandler(testStubProvider{})
	body := soapRequestSubEvents("PullMessages",
		`<tev:MessageLimit>notanumber</tev:MessageLimit>`)
	rec := doSubRequest(t, h, body, "sub-001")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400; body=%s", rec.Code, rec.Body.String())
	}
}

// ---------- SetSynchronizationPoint ----------------------------------------------

func TestSubscriptionManager_SetSynchronizationPoint(t *testing.T) {
	h := NewSubscriptionManagerHandler(testStubProvider{})
	body := soapRequestSubEvents("SetSynchronizationPoint", "")
	rec := doSubRequest(t, h, body, "sub-001")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "SetSynchronizationPointResponse") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

// ---------- Renew ----------------------------------------------------------------

func TestSubscriptionManager_Renew(t *testing.T) {
	h := NewSubscriptionManagerHandler(testStubProvider{})
	body := soapRequestWSN("Renew", `<wsnt:TerminationTime>PT1H</wsnt:TerminationTime>`)
	rec := doSubRequest(t, h, body, "sub-001")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	respBody := rec.Body.String()
	if !strings.Contains(respBody, "RenewResponse") {
		t.Fatalf("unexpected body: %s", respBody)
	}
	if !strings.Contains(respBody, "TerminationTime") {
		t.Fatalf("missing TerminationTime: %s", respBody)
	}
	// Renew response must declare WSN namespace, not events namespace.
	if !strings.Contains(respBody, WSNBaseNotificationNS) {
		t.Fatalf("RenewResponse should declare WSN namespace: %s", respBody)
	}
}

// ---------- Unsubscribe ----------------------------------------------------------

func TestSubscriptionManager_Unsubscribe(t *testing.T) {
	h := NewSubscriptionManagerHandler(testStubProvider{})
	body := soapRequestWSN("Unsubscribe", "")
	rec := doSubRequest(t, h, body, "sub-001")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	respBody := rec.Body.String()
	if !strings.Contains(respBody, "UnsubscribeResponse") {
		t.Fatalf("unexpected body: %s", respBody)
	}
	if !strings.Contains(respBody, WSNBaseNotificationNS) {
		t.Fatalf("UnsubscribeResponse should declare WSN namespace: %s", respBody)
	}
}

// ---------- Error paths ----------------------------------------------------------

func TestSubscriptionManager_UnsupportedOperation(t *testing.T) {
	h := NewSubscriptionManagerHandler(testStubProvider{})
	body := soapRequestSubEvents("UnknownOp", "")
	rec := doSubRequest(t, h, body, "sub-001")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d want 501; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSubscriptionManager_MethodNotAllowed(t *testing.T) {
	h := NewSubscriptionManagerHandler(testStubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, SubscriptionManagerPath, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d want 405", rec.Code)
	}
}

func TestSubscriptionManager_InvalidEnvelope(t *testing.T) {
	h := NewSubscriptionManagerHandler(testStubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, SubscriptionManagerPath,
		bytes.NewBufferString("not xml"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSubscriptionManager_ProviderError_PullMessages(t *testing.T) {
	h := NewSubscriptionManagerHandler(testErrProvider{})
	body := soapRequestSubEvents("PullMessages", "")
	rec := doSubRequest(t, h, body, "sub-001")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d want 500; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Receiver") {
		t.Fatalf("expected Receiver fault: %s", rec.Body.String())
	}
}

func TestSubscriptionManager_SubscriptionNotFound(t *testing.T) {
	h := NewSubscriptionManagerHandler(&notFoundProvider{})
	body := soapRequestSubEvents("PullMessages", "")
	rec := doSubRequest(t, h, body, "bad-id")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Sender") {
		t.Fatalf("expected Sender fault: %s", rec.Body.String())
	}
}

func TestSubscriptionManager_SizeCap(t *testing.T) {
	h := NewSubscriptionManagerHandler(testStubProvider{})
	big := bytes.Repeat([]byte("x"), maxSOAPBodySize+1)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, SubscriptionManagerPath,
		bytes.NewReader(big))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d want 413", rec.Code)
	}
}

func TestSubscriptionManagerHandler_PanicsOnNilProvider(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil provider")
		}
	}()
	NewSubscriptionManagerHandler(nil)
}

// ---------- namespace tests ------------------------------------------------------

func TestParseSubscriptionManagerOperation_AcceptsWSNNamespace(t *testing.T) {
	body := soapRequestWSN("Renew", "")
	_, op, err := parseSubscriptionManagerOperation([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op != "Renew" {
		t.Fatalf("operation = %q", op)
	}
}

func TestParseSubscriptionManagerOperation_AcceptsEventsNamespace(t *testing.T) {
	body := soapRequestSubEvents("PullMessages", "")
	_, op, err := parseSubscriptionManagerOperation([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op != "PullMessages" {
		t.Fatalf("operation = %q", op)
	}
}

func TestParseSubscriptionManagerOperation_RejectsOtherNamespace(t *testing.T) {
	body := `<?xml version="1.0"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Body><op xmlns="http://example.com/bad"/></s:Body></s:Envelope>`
	_, _, err := parseSubscriptionManagerOperation([]byte(body))
	if err == nil {
		t.Fatal("expected errInvalidNamespace, got nil")
	}
}

// ---------- helper providers for specific error paths ----------------------------

type notFoundProvider struct{ testStubProvider }

func (notFoundProvider) PullMessages(_ context.Context, _ string, _ PullMessagesParams) (PullMessagesResult, error) {
	return PullMessagesResult{}, ErrSubscriptionNotFound
}
