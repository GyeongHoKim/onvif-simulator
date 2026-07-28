package ptzsvc

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
	status       Status
	presets      []Preset
	nodes        []PTZNode
	configurations []PTZConfiguration
	capabilities ServiceCapabilities
}

func (m *mockProvider) ServiceCapabilities(_ context.Context) (ServiceCapabilities, error) {
	return m.capabilities, nil
}
func (m *mockProvider) GetNodes(_ context.Context) ([]PTZNode, error) {
	if m.nodes == nil {
		m.nodes = []PTZNode{{Token: "node1", Name: "ePTZ"}}
	}
	return m.nodes, nil
}
func (m *mockProvider) GetNode(_ context.Context, token string) (PTZNode, error) {
	return PTZNode{Token: token, Name: "ePTZ"}, nil
}
func (m *mockProvider) GetConfigurations(_ context.Context) ([]PTZConfiguration, error) {
	if m.configurations == nil {
		m.configurations = []PTZConfiguration{{Token: "cfg1", Name: "ePTZ", NodeToken: "node1"}}
	}
	return m.configurations, nil
}
func (m *mockProvider) GetConfiguration(_ context.Context, token string) (PTZConfiguration, error) {
	return PTZConfiguration{Token: token, Name: "ePTZ", NodeToken: "node1"}, nil
}
func (m *mockProvider) GetConfigurationOptions(_ context.Context, _ string) (ConfigurationOptions, error) {
	return ConfigurationOptions{
		PanTiltPositionSpaceRange: []SpaceRange{{
			URI: "pos_space", XRange: IntRange{Min: -1, Max: 1}, YRange: IntRange{Min: -1, Max: 1},
		}},
		ZoomPositionSpaceRange: []SpaceRange{{
			URI: "pos_space", XRange: IntRange{Min: 0, Max: 1},
		}},
		PTZTimeout: IntRange{Min: 5, Max: 300},
	}, nil
}
func (m *mockProvider) GetStatus(_ context.Context, _ string) (Status, error) {
	return m.status, nil
}
func (m *mockProvider) ContinuousMove(_ context.Context, _ string, _ Speed, _ *string) error {
	return nil
}
func (m *mockProvider) AbsoluteMove(_ context.Context, _ string, _ Vector, _ *Speed) error {
	return nil
}
func (m *mockProvider) RelativeMove(_ context.Context, _ string, _ Vector, _ *Speed) error {
	return nil
}
func (m *mockProvider) Stop(_ context.Context, _ string, _, _ *bool) error {
	return nil
}
func (m *mockProvider) GetPresets(_ context.Context, _ string) ([]Preset, error) {
	return m.presets, nil
}
func (m *mockProvider) SetPreset(_ context.Context, _ string, name, token *string) (string, error) {
	t := "preset_1"
	if token != nil {
		t = *token
	}
	return t, nil
}
func (m *mockProvider) RemovePreset(_ context.Context, _, _ string) error { return nil }
func (m *mockProvider) GotoPreset(_ context.Context, _ string, _ string, _ *Speed) error {
	return nil
}
func (m *mockProvider) GotoHomePosition(_ context.Context, _ string, _ *Speed) error { return nil }
func (m *mockProvider) SetHomePosition(_ context.Context, _ string) error            { return nil }

func newHandler(t *testing.T) *Handler {
	t.Helper()
	return NewHandler(&mockProvider{
		status: Status{
			Position: &Vector{
				PanTilt: &PanTilt{X: 0, Y: 0},
				Zoom:    &Zoom{X: 0},
			},
			MoveStatus: &MoveStatus{PanTilt: "IDLE", Zoom: "IDLE"},
		},
		capabilities: ServiceCapabilities{MoveStatus: true, StatusPosition: true},
	})
}

func ptzSoapRequest(operation, payload string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
  xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl">
  <s:Body>
    <tptz:` + operation + `>` + payload + `</tptz:` + operation + `>
  </s:Body>
</s:Envelope>`
}

func callPTZService(t *testing.T, h *Handler, body string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, PTZServicePath, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestGetServiceCapabilities(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("GetServiceCapabilities", ""))
	if !strings.Contains(resp, "GetServiceCapabilitiesResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	if !strings.Contains(resp, "MoveStatus") {
		t.Fatalf("missing MoveStatus capability: %s", resp)
	}
}

func TestGetNodes(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("GetNodes", ""))
	if !strings.Contains(resp, "GetNodesResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	if !strings.Contains(resp, "node1") {
		t.Fatalf("missing node token: %s", resp)
	}
}

func TestGetNode(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("GetNode", `<tptz:NodeToken>node1</tptz:NodeToken>`))
	if !strings.Contains(resp, "GetNodeResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestGetConfigurations(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("GetConfigurations", ""))
	if !strings.Contains(resp, "GetConfigurationsResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	if !strings.Contains(resp, "cfg1") {
		t.Fatalf("missing config token: %s", resp)
	}
}

func TestGetConfiguration(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("GetConfiguration", `<tptz:PTZConfigurationToken>cfg1</tptz:PTZConfigurationToken>`))
	if !strings.Contains(resp, "GetConfigurationResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestGetConfigurationOptions(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("GetConfigurationOptions", `<tptz:ConfigurationToken>cfg1</tptz:ConfigurationToken>`))
	if !strings.Contains(resp, "GetConfigurationOptionsResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestGetStatus(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("GetStatus", `<tptz:ProfileToken>main</tptz:ProfileToken>`))
	if !strings.Contains(resp, "GetStatusResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	if !strings.Contains(resp, "IDLE") {
		t.Fatalf("missing IDLE status: %s", resp)
	}
}

func TestContinuousMove(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("ContinuousMove", `
    <tptz:ProfileToken>main</tptz:ProfileToken>
    <tptz:Velocity>
      <tt:PanTilt x="0.5" y="0.3"/>
      <tt:Zoom x="0.1"/>
    </tptz:Velocity>`))
	if !strings.Contains(resp, "ContinuousMoveResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestAbsoluteMove(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("AbsoluteMove", `
    <tptz:ProfileToken>main</tptz:ProfileToken>
    <tptz:Position>
      <tt:PanTilt x="0.8" y="-0.5"/>
      <tt:Zoom x="0.6"/>
    </tptz:Position>`))
	if !strings.Contains(resp, "AbsoluteMoveResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestRelativeMove(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("RelativeMove", `
    <tptz:ProfileToken>main</tptz:ProfileToken>
    <tptz:Translation>
      <tt:PanTilt x="0.1" y="-0.1"/>
    </tptz:Translation>`))
	if !strings.Contains(resp, "RelativeMoveResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestStop(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("Stop", `
    <tptz:ProfileToken>main</tptz:ProfileToken>
    <tptz:PanTilt>true</tptz:PanTilt>
    <tptz:Zoom>true</tptz:Zoom>`))
	if !strings.Contains(resp, "StopResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestGetPresets(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("GetPresets", `<tptz:ProfileToken>main</tptz:ProfileToken>`))
	if !strings.Contains(resp, "GetPresetsResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestSetPreset(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("SetPreset", `
    <tptz:ProfileToken>main</tptz:ProfileToken>
    <tptz:PresetName>Home</tptz:PresetName>
    <tptz:PresetToken>home1</tptz:PresetToken>`))
	if !strings.Contains(resp, "SetPresetResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	if !strings.Contains(resp, "home1") {
		t.Fatalf("missing preset token in response: %s", resp)
	}
}

func TestRemovePreset(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("RemovePreset", `
    <tptz:ProfileToken>main</tptz:ProfileToken>
    <tptz:PresetToken>home1</tptz:PresetToken>`))
	if !strings.Contains(resp, "RemovePresetResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestGotoPreset(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("GotoPreset", `
    <tptz:ProfileToken>main</tptz:ProfileToken>
    <tptz:PresetToken>home1</tptz:PresetToken>`))
	if !strings.Contains(resp, "GotoPresetResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestGotoHomePosition(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("GotoHomePosition", `<tptz:ProfileToken>main</tptz:ProfileToken>`))
	if !strings.Contains(resp, "GotoHomePositionResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestSetHomePosition(t *testing.T) {
	h := newHandler(t)
	resp := callPTZService(t, h, ptzSoapRequest("SetHomePosition", `<tptz:ProfileToken>main</tptz:ProfileToken>`))
	if !strings.Contains(resp, "SetHomePositionResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

func TestUnsupportedOperation(t *testing.T) {
	h := newHandler(t)
	req := httptest.NewRequest(http.MethodPost, PTZServicePath, strings.NewReader(ptzSoapRequest("BogusOperation", "")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("want 501, got %d", rec.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := newHandler(t)
	req := httptest.NewRequest(http.MethodGet, PTZServicePath, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}

func TestEmptyBody(t *testing.T) {
	h := newHandler(t)
	body := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body></s:Body>
</s:Envelope>`
	req := httptest.NewRequest(http.MethodPost, PTZServicePath, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestXMLRoundTrip_Vector(t *testing.T) {
	v := Vector{
		PanTilt: &PanTilt{X: 0.5, Y: -0.3},
		Zoom:    &Zoom{X: 0.8},
	}
	env := vectorToEnvelope(&v)
	data, err := xml.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `x="0.5"`) {
		t.Fatalf("missing x attr: %s", data)
	}
}

func TestParseOperation_InvalidNamespace(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
  xmlns:wrong="http://wrong.namespace">
  <s:Body>
    <wrong:Foo/>
  </s:Body>
</s:Envelope>`
	_, _, err := parseOperation([]byte(body))
	if err == nil {
		t.Fatal("expected error for invalid namespace")
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

func TestWriteSOAP(t *testing.T) {
	w := httptest.NewRecorder()
	writeSOAP(w, []byte("<test/>"))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/soap+xml") {
		t.Fatalf("want soap content type, got %s", ct)
	}
}
