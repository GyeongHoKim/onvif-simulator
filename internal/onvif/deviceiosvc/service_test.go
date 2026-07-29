package deviceiosvc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockProvider implements Provider for unit tests.
type mockProvider struct {
	capabilities ServiceCapabilities
}

func (m *mockProvider) ServiceCapabilities(_ context.Context) (ServiceCapabilities, error) {
	return m.capabilities, nil
}

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	return NewHandler(&mockProvider{
		capabilities: ServiceCapabilities{
			RelayOutputs:  2,
			DigitalInputs: 4,
		},
	})
}

func deviceIOSoapRequest(operation, payload string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
  xmlns:tds="http://www.onvif.org/ver10/deviceio/wsdl">
  <s:Body>
    <tds:` + operation + `>` + payload + `</tds:` + operation + `>
  </s:Body>
</s:Envelope>`
}

func callDeviceIOService(t *testing.T, h *Handler, body string) string {
	t.Helper()
	ctx := context.Background()
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, DeviceIOServicePath, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestGetServiceCapabilities(t *testing.T) {
	h := newTestHandler(t)
	resp := callDeviceIOService(t, h, deviceIOSoapRequest("GetServiceCapabilities", ""))
	if !strings.Contains(resp, "GetServiceCapabilitiesResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	if !strings.Contains(resp, "RelayOutputs") {
		t.Fatalf("missing RelayOutputs capability: %s", resp)
	}
	if !strings.Contains(resp, "DigitalInputs") {
		t.Fatalf("missing DigitalInputs capability: %s", resp)
	}
}

func TestUnsupportedOperation(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		DeviceIOServicePath, strings.NewReader(deviceIOSoapRequest("BogusOperation", "")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("want 501, got %d", rec.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
		DeviceIOServicePath, http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}

func TestNewHandler_PanicsOnNilProvider(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil provider")
		}
	}()
	NewHandler(nil)
}
