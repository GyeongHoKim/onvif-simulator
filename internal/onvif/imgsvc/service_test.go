package imgsvc

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockProvider implements Provider for unit tests.
type mockProvider struct {
	settings Settings
	presets  []Preset
	current  *Preset
}

func (m *mockProvider) ServiceCapabilities(_ context.Context) (ServiceCapabilities, error) {
	return ServiceCapabilities{}, nil
}
func (m *mockProvider) GetImagingSettings(_ context.Context, _ string) (Settings, error) {
	return m.settings, nil
}
func (m *mockProvider) SetImagingSettings(_ context.Context, _ string, s Settings, _ *bool) error {
	m.settings = s
	return nil
}
func (m *mockProvider) GetOptions(_ context.Context, _ string) (Options, error) {
	return Options{
		Brightness:        FloatRange{Min: 0, Max: 1},
		Contrast:          FloatRange{Min: 0, Max: 1},
		Sharpness:         FloatRange{Min: 0, Max: 1},
		ExposureModes:     []string{"AUTO", "MANUAL"},
		WhiteBalanceModes: []string{"AUTO", "MANUAL"},
		FocusModes:        []string{"AUTO", "MANUAL"},
	}, nil
}
func (m *mockProvider) GetStatus(_ context.Context, _ string) (Status, error) {
	return Status{FocusStatus: &FocusStatus{Position: "IDLE"}}, nil
}
func (m *mockProvider) GetPresets(_ context.Context, _ string) ([]Preset, error) {
	return m.presets, nil
}
func (m *mockProvider) GetCurrentPreset(_ context.Context, _ string) (*Preset, error) {
	return m.current, nil
}
func (m *mockProvider) SetCurrentPreset(_ context.Context, _, _ string) error { return nil }

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	brightness := 0.5
	contrast := 0.5
	return NewHandler(&mockProvider{
		settings: Settings{
			Brightness: &brightness,
			Contrast:   &contrast,
		},
		presets: []Preset{
			{Token: "clear", Name: "Clear Weather", Type: "ClearWeather"},
			{Token: "night", Name: "Night", Type: "Night"},
		},
	})
}

func imgSoapRequest(operation, payload string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
  xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl">
  <s:Body>
    <timg:` + operation + `>` + payload + `</timg:` + operation + `>
  </s:Body>
</s:Envelope>`
}

func callImagingService(t *testing.T, h *Handler, body string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, ImagingServicePath, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestGetServiceCapabilities(t *testing.T) {
	h := newTestHandler(t)
	resp := callImagingService(t, h, imgSoapRequest("GetServiceCapabilities", ""))
	if !strings.Contains(resp, "GetServiceCapabilitiesResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestGetImagingSettings(t *testing.T) {
	h := newTestHandler(t)
	resp := callImagingService(t, h, imgSoapRequest("GetImagingSettings",
		`<timg:VideoSourceToken>vs1</timg:VideoSourceToken>`))
	if !strings.Contains(resp, "GetImagingSettingsResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	if !strings.Contains(resp, "Brightness") {
		t.Fatalf("missing Brightness: %s", resp)
	}
}

func TestSetImagingSettings(t *testing.T) {
	h := newTestHandler(t)
	resp := callImagingService(t, h, imgSoapRequest("SetImagingSettings", `
    <timg:VideoSourceToken>vs1</timg:VideoSourceToken>
    <timg:ImagingSettings>
      <tt:Brightness>0.8</tt:Brightness>
      <tt:Contrast>0.3</tt:Contrast>
    </timg:ImagingSettings>`))
	if !strings.Contains(resp, "SetImagingSettingsResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestGetOptions(t *testing.T) {
	h := newTestHandler(t)
	resp := callImagingService(t, h, imgSoapRequest("GetOptions",
		`<timg:VideoSourceToken>vs1</timg:VideoSourceToken>`))
	if !strings.Contains(resp, "GetOptionsResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	if !strings.Contains(resp, "Brightness") {
		t.Fatalf("missing Brightness options: %s", resp)
	}
}

func TestGetStatus(t *testing.T) {
	h := newTestHandler(t)
	resp := callImagingService(t, h, imgSoapRequest("GetStatus",
		`<timg:VideoSourceToken>vs1</timg:VideoSourceToken>`))
	if !strings.Contains(resp, "GetStatusResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	if !strings.Contains(resp, "IDLE") {
		t.Fatalf("missing IDLE status: %s", resp)
	}
}

func TestGetPresets(t *testing.T) {
	h := newTestHandler(t)
	resp := callImagingService(t, h, imgSoapRequest("GetPresets",
		`<timg:VideoSourceToken>vs1</timg:VideoSourceToken>`))
	if !strings.Contains(resp, "GetPresetsResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	if !strings.Contains(resp, "clear") {
		t.Fatalf("missing preset token: %s", resp)
	}
}

func TestGetCurrentPreset(t *testing.T) {
	h := newTestHandler(t)
	resp := callImagingService(t, h, imgSoapRequest("GetCurrentPreset",
		`<timg:VideoSourceToken>vs1</timg:VideoSourceToken>`))
	if !strings.Contains(resp, "GetCurrentPresetResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestSetCurrentPreset(t *testing.T) {
	h := newTestHandler(t)
	resp := callImagingService(t, h, imgSoapRequest("SetCurrentPreset", `
    <timg:VideoSourceToken>vs1</timg:VideoSourceToken>
    <timg:PresetToken>clear</timg:PresetToken>`))
	if !strings.Contains(resp, "SetCurrentPresetResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestUnsupportedOperation(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, ImagingServicePath,
		strings.NewReader(imgSoapRequest("BogusOperation", "")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("want 501, got %d", rec.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, ImagingServicePath, nil)
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

func TestXMLRoundTrip_Settings(t *testing.T) {
	brightness := 0.7
	settings := Settings{Brightness: &brightness}
	env := settingsToEnvelope(&settings)
	data, err := xml.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), "0.7") {
		t.Fatalf("missing brightness value: %s", data)
	}
}
