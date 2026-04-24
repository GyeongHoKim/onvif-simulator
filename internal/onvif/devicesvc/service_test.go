package devicesvc

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
)

type stubProvider struct{}

func (stubProvider) DeviceInfo(context.Context) (DeviceInfo, error) {
	return DeviceInfo{
		Manufacturer: "Acme",
		Model:        "SimCam",
		Firmware:     "1.0.0",
		Serial:       "ABC123",
		HardwareID:   "SimCam",
	}, nil
}

func (stubProvider) Services(context.Context, bool) ([]ServiceDescriptor, error) {
	return []ServiceDescriptor{{Namespace: DeviceNamespace, XAddr: "http://127.0.0.1:8080/onvif/device_service", Version: Version{Major: 2, Minor: 12}}}, nil
}

func (stubProvider) GetServiceCapabilities(context.Context) (DeviceServiceCapabilities, error) {
	return DeviceServiceCapabilities{
		Network:  NetworkCapabilities{ZeroConfiguration: true},
		Security: SecurityCapabilities{HTTPDigest: true},
		System:   SystemCapabilities{DiscoveryResolve: true, DiscoveryBye: true},
	}, nil
}

func (stubProvider) GetCapabilities(context.Context, string) (CapabilitySet, error) {
	return CapabilitySet{
		Device: DeviceCapability{
			XAddr:  "http://127.0.0.1:8080/onvif/device_service",
			System: CoreSystemCapabilities{DiscoveryResolve: true, DiscoveryBye: true, SupportedVersions: []Version{{Major: 2, Minor: 12}}},
		},
		Media: ServiceCapability{XAddr: "http://127.0.0.1:8080/onvif/media_service"},
	}, nil
}

func (stubProvider) WsdlURL(context.Context) (string, error) {
	return "http://www.onvif.org/ver10/device/wsdl/devicemgmt.wsdl", nil
}

// --- Discovery stubs ---

func (stubProvider) GetDiscoveryMode(context.Context) (DiscoveryInfo, error) {
	return DiscoveryInfo{DiscoveryMode: "Discoverable"}, nil
}
func (stubProvider) SetDiscoveryMode(context.Context, string) error { return nil }
func (stubProvider) GetScopes(context.Context) ([]ScopeEntry, error) {
	return []ScopeEntry{{ScopeDef: "Fixed", ScopeItem: "onvif://www.onvif.org/type/video_encoder"}}, nil
}
func (stubProvider) SetScopes(context.Context, []string) error                { return nil }
func (stubProvider) AddScopes(context.Context, []string) error                { return nil }
func (stubProvider) RemoveScopes(context.Context, []string) ([]string, error) { return nil, nil }

// --- Network stubs ---

func (stubProvider) GetHostname(context.Context) (HostnameInfo, error) {
	return HostnameInfo{Name: "simulator"}, nil
}
func (stubProvider) SetHostname(context.Context, string) error { return nil }
func (stubProvider) GetDNS(context.Context) (DNSInfo, error)   { return DNSInfo{}, nil }
func (stubProvider) SetDNS(context.Context, DNSInfo) error     { return nil }
func (stubProvider) GetNetworkInterfaces(context.Context) ([]NetworkInterfaceInfo, error) {
	return nil, nil
}
func (stubProvider) GetNetworkProtocols(context.Context) ([]NetworkProtocol, error) {
	return []NetworkProtocol{{Name: "HTTP", Enabled: true, Port: []int{80}}}, nil
}
func (stubProvider) SetNetworkProtocols(context.Context, []NetworkProtocol) error { return nil }
func (stubProvider) GetNetworkDefaultGateway(context.Context) (DefaultGatewayInfo, error) {
	return DefaultGatewayInfo{}, nil
}
func (stubProvider) SetNetworkDefaultGateway(context.Context, DefaultGatewayInfo) error { return nil }

// --- System stubs ---

func (stubProvider) GetSystemDateAndTime(context.Context) (SystemDateAndTimeInfo, error) {
	return SystemDateAndTimeInfo{DateTimeType: "Manual", TZ: "UTC"}, nil
}
func (stubProvider) SetSystemDateAndTime(context.Context, SetSystemDateAndTimeParams) error {
	return nil
}
func (stubProvider) SetSystemFactoryDefault(context.Context, string) error { return nil }
func (stubProvider) SystemReboot(context.Context) (string, error) {
	return "Rebooting simulator", nil
}

// --- User stubs ---

func (stubProvider) GetUsers(context.Context) ([]UserInfo, error) {
	return []UserInfo{{Username: "admin", UserLevel: "Administrator"}}, nil
}
func (stubProvider) CreateUsers(context.Context, []UserInfo) error { return nil }
func (stubProvider) SetUser(context.Context, []UserInfo) error     { return nil }
func (stubProvider) DeleteUsers(context.Context, []string) error   { return nil }
func (stubProvider) SetNetworkInterfaces(context.Context, []NetworkInterfaceInfo) error {
	return nil
}

var errProviderBoom = errors.New("provider boom")

type errProvider struct{}

func (errProvider) DeviceInfo(context.Context) (DeviceInfo, error) {
	return DeviceInfo{}, errProviderBoom
}

func (errProvider) Services(context.Context, bool) ([]ServiceDescriptor, error) {
	return nil, errProviderBoom
}

func (errProvider) GetServiceCapabilities(context.Context) (DeviceServiceCapabilities, error) {
	return DeviceServiceCapabilities{}, errProviderBoom
}

func (errProvider) GetCapabilities(context.Context, string) (CapabilitySet, error) {
	return CapabilitySet{}, errProviderBoom
}

func (errProvider) WsdlURL(context.Context) (string, error) {
	return "", errProviderBoom
}

// errProvider stubs for new operations — all return errProviderBoom.

func (errProvider) GetDiscoveryMode(context.Context) (DiscoveryInfo, error) {
	return DiscoveryInfo{}, errProviderBoom
}
func (errProvider) SetDiscoveryMode(context.Context, string) error { return errProviderBoom }
func (errProvider) GetScopes(context.Context) ([]ScopeEntry, error) {
	return nil, errProviderBoom
}
func (errProvider) SetScopes(context.Context, []string) error { return errProviderBoom }
func (errProvider) AddScopes(context.Context, []string) error { return errProviderBoom }
func (errProvider) RemoveScopes(context.Context, []string) ([]string, error) {
	return nil, errProviderBoom
}
func (errProvider) GetHostname(context.Context) (HostnameInfo, error) {
	return HostnameInfo{}, errProviderBoom
}
func (errProvider) SetHostname(context.Context, string) error { return errProviderBoom }
func (errProvider) GetDNS(context.Context) (DNSInfo, error)   { return DNSInfo{}, errProviderBoom }
func (errProvider) SetDNS(context.Context, DNSInfo) error     { return errProviderBoom }
func (errProvider) GetNetworkInterfaces(context.Context) ([]NetworkInterfaceInfo, error) {
	return nil, errProviderBoom
}
func (errProvider) GetNetworkProtocols(context.Context) ([]NetworkProtocol, error) {
	return nil, errProviderBoom
}
func (errProvider) SetNetworkProtocols(context.Context, []NetworkProtocol) error {
	return errProviderBoom
}
func (errProvider) GetNetworkDefaultGateway(context.Context) (DefaultGatewayInfo, error) {
	return DefaultGatewayInfo{}, errProviderBoom
}
func (errProvider) SetNetworkDefaultGateway(context.Context, DefaultGatewayInfo) error {
	return errProviderBoom
}
func (errProvider) GetSystemDateAndTime(context.Context) (SystemDateAndTimeInfo, error) {
	return SystemDateAndTimeInfo{}, errProviderBoom
}
func (errProvider) SetSystemDateAndTime(context.Context, SetSystemDateAndTimeParams) error {
	return errProviderBoom
}
func (errProvider) SetSystemFactoryDefault(context.Context, string) error { return errProviderBoom }
func (errProvider) SystemReboot(context.Context) (string, error)          { return "", errProviderBoom }
func (errProvider) GetUsers(context.Context) ([]UserInfo, error)          { return nil, errProviderBoom }
func (errProvider) CreateUsers(context.Context, []UserInfo) error         { return errProviderBoom }
func (errProvider) SetUser(context.Context, []UserInfo) error             { return errProviderBoom }
func (errProvider) DeleteUsers(context.Context, []string) error           { return errProviderBoom }
func (errProvider) SetNetworkInterfaces(context.Context, []NetworkInterfaceInfo) error {
	return errProviderBoom
}

type emptyServicesProvider struct{ stubProvider }

func (emptyServicesProvider) Services(context.Context, bool) ([]ServiceDescriptor, error) {
	return nil, nil
}

func TestServeHTTP_GetCapabilities(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetCapabilities", `<Category>All</Category>`)))
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<GetCapabilitiesResponse") {
		t.Fatalf("body missing GetCapabilitiesResponse: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<Media><XAddr>http://127.0.0.1:8080/onvif/media_service</XAddr></Media>") {
		t.Fatalf("body missing media xaddr: %s", rec.Body.String())
	}
}

func TestServeHTTP_GetServices(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetServices", `<IncludeCapability>false</IncludeCapability>`)))
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<Namespace>"+DeviceNamespace+"</Namespace>") {
		t.Fatalf("body missing namespace: %s", rec.Body.String())
	}
}

func TestServeHTTP_GetDeviceInformation(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetDeviceInformation", "")))
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env struct {
		Body struct {
			Inner []byte `xml:",innerxml"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal response envelope: %v", err)
	}
	if !bytes.Contains(env.Body.Inner, []byte("<Manufacturer>Acme</Manufacturer>")) {
		t.Fatalf("body missing manufacturer: %s", string(env.Body.Inner))
	}
}

func TestServeHTTP_UnsupportedOperationFault(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetUnknownOp", "")))
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
	if !strings.Contains(rec.Body.String(), "unsupported operation") {
		t.Fatalf("body missing fault reason: %s", rec.Body.String())
	}
}

func TestParseOperation(t *testing.T) {
	payload, op, err := parseOperation([]byte(soapRequest("GetWsdlUrl", "")))
	if err != nil {
		t.Fatalf("parseOperation: %v", err)
	}
	if op != "GetWsdlUrl" {
		t.Fatalf("operation = %q, want %q", op, "GetWsdlUrl")
	}
	if len(payload) == 0 {
		t.Fatal("payload must not be empty")
	}
}

func TestServeHTTP_AuthHook(t *testing.T) {
	svc := NewHandler(stubProvider{}, WithAuthHook(AuthFunc(func(context.Context, string, *http.Request) error {
		return io.EOF
	})))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetWsdlUrl", "")))
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestServeHTTP_MethodNotAllowed(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, DeviceServicePath, nil)
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestServeHTTP_InvalidEnvelopeFault(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString("not an xml envelope"))
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "env:Sender") {
		t.Fatalf("body missing Sender fault code: %s", rec.Body.String())
	}
}

func TestServeHTTP_EmptyBodyFault(t *testing.T) {
	svc := NewHandler(stubProvider{})
	envelope := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope">` +
		`<env:Body></env:Body></env:Envelope>`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(envelope))
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "empty soap body") {
		t.Fatalf("body missing empty body reason: %s", rec.Body.String())
	}
}

func TestServeHTTP_GetServiceCapabilities(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetServiceCapabilities", "")))
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<GetServiceCapabilitiesResponse") {
		t.Fatalf("body missing GetServiceCapabilitiesResponse: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `HttpDigest="true"`) {
		t.Fatalf("body missing HttpDigest attr: %s", rec.Body.String())
	}
}

func TestServeHTTP_GetWsdlUrl(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetWsdlUrl", "")))
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<WsdlUrl>http://www.onvif.org/ver10/device/wsdl/devicemgmt.wsdl</WsdlUrl>") {
		t.Fatalf("body missing wsdl url: %s", rec.Body.String())
	}
}

func TestServeHTTP_ProviderErrors(t *testing.T) {
	cases := []struct {
		op    string
		inner string
	}{
		{"GetDeviceInformation", ""},
		{"GetServices", "<IncludeCapability>false</IncludeCapability>"},
		{"GetServiceCapabilities", ""},
		{"GetCapabilities", "<Category>All</Category>"},
		{"GetWsdlUrl", ""},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			svc := NewHandler(errProvider{})
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest(tc.op, tc.inner)))
			rec := httptest.NewRecorder()

			svc.ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
			}
			if !strings.Contains(rec.Body.String(), "env:Receiver") {
				t.Fatalf("body missing Receiver fault code: %s", rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), errProviderBoom.Error()) {
				t.Fatalf("body missing provider error reason: %s", rec.Body.String())
			}
		})
	}
}

func TestServeHTTP_GetServicesNoServices(t *testing.T) {
	svc := NewHandler(emptyServicesProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetServices", "<IncludeCapability>false</IncludeCapability>")))
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "no services available") {
		t.Fatalf("body missing no services reason: %s", rec.Body.String())
	}
}

func TestServeHTTP_GetServicesInvalidPayload(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetServices", "<IncludeCapability>notabool</IncludeCapability>")))
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "env:Sender") {
		t.Fatalf("body missing Sender fault code: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "decode GetServices") {
		t.Fatalf("body missing decode error: %s", rec.Body.String())
	}
}

func TestServeHTTP_PayloadTooLarge(t *testing.T) {
	svc := NewHandler(stubProvider{})
	oversized := bytes.Repeat([]byte("a"), maxSOAPBodySize+1)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewReader(oversized))
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
	if !strings.Contains(rec.Body.String(), "env:Sender") {
		t.Fatalf("body missing Sender fault code: %s", rec.Body.String())
	}
}

func TestServeHTTP_AuthHookPreservesBody(t *testing.T) {
	var seen []byte
	svc := NewHandler(stubProvider{}, WithAuthHook(AuthFunc(func(_ context.Context, _ string, r *http.Request) error {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}
		seen = body
		return nil
	})))
	envelope := soapRequest("GetWsdlUrl", "")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(envelope))
	rec := httptest.NewRecorder()

	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if string(seen) != envelope {
		t.Fatalf("auth hook saw %q, want %q", string(seen), envelope)
	}
}

func TestParseOperation_Errors(t *testing.T) {
	t.Run("malformed xml", func(t *testing.T) {
		if _, _, err := parseOperation([]byte("not xml")); err == nil {
			t.Fatal("expected error for malformed xml")
		}
	})
	t.Run("empty body", func(t *testing.T) {
		env := `<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope"><env:Body></env:Body></env:Envelope>`
		_, _, err := parseOperation([]byte(env))
		if !errors.Is(err, errEmptySOAPBody) {
			t.Fatalf("err = %v, want errEmptySOAPBody", err)
		}
	})
}

func TestNewHandler_NilProviderPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil provider")
		}
	}()
	NewHandler(nil)
}

func TestWithAuthHook_NilHookIgnored(t *testing.T) {
	svc := NewHandler(stubProvider{}, WithAuthHook(nil))
	if svc.auth == nil {
		t.Fatal("auth must remain non-nil when WithAuthHook receives nil")
	}
}

func soapRequest(op, inner string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope" xmlns:tds="` + DeviceNamespace + `">` +
		`<env:Body><tds:` + op + `>` + inner + `</tds:` + op + `></env:Body></env:Envelope>`
}

// --- Discovery tests ---

func TestServeHTTP_GetDiscoveryMode(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetDiscoveryMode", "")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<DiscoveryMode>Discoverable</DiscoveryMode>") {
		t.Fatalf("body missing DiscoveryMode: %s", rec.Body.String())
	}
}

func TestServeHTTP_SetDiscoveryMode(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("SetDiscoveryMode", "<DiscoveryMode>NonDiscoverable</DiscoveryMode>")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "SetDiscoveryModeResponse") {
		t.Fatalf("body missing SetDiscoveryModeResponse: %s", rec.Body.String())
	}
}

func TestServeHTTP_SetDiscoveryModeInvalidPayload(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("SetDiscoveryMode", "<DiscoveryMode><bad</DiscoveryMode>")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestServeHTTP_GetScopes(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetScopes", "")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "GetScopesResponse") {
		t.Fatalf("body missing GetScopesResponse: %s", rec.Body.String())
	}
}

func TestServeHTTP_SetScopes(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("SetScopes", "<Scopes>onvif://www.onvif.org/type/video_encoder</Scopes>")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "SetScopesResponse") {
		t.Fatalf("body missing SetScopesResponse: %s", rec.Body.String())
	}
}

func TestServeHTTP_AddScopes(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("AddScopes", "<ScopeItem>onvif://www.onvif.org/type/ptz</ScopeItem>")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "AddScopesResponse") {
		t.Fatalf("body missing AddScopesResponse: %s", rec.Body.String())
	}
}

func TestServeHTTP_RemoveScopes(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("RemoveScopes", "<ScopeItem>onvif://www.onvif.org/type/ptz</ScopeItem>")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "RemoveScopesResponse") {
		t.Fatalf("body missing RemoveScopesResponse: %s", rec.Body.String())
	}
}

// --- Network tests ---

func TestServeHTTP_GetHostname(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetHostname", "")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<Name>simulator</Name>") {
		t.Fatalf("body missing hostname: %s", rec.Body.String())
	}
}

func TestServeHTTP_SetHostname(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("SetHostname", "<Name>mycamera</Name>")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "SetHostnameResponse") {
		t.Fatalf("body missing SetHostnameResponse: %s", rec.Body.String())
	}
}

func TestServeHTTP_GetDNS(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetDNS", "")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "GetDNSResponse") {
		t.Fatalf("body missing GetDNSResponse: %s", rec.Body.String())
	}
}

func TestServeHTTP_SetDNS(t *testing.T) {
	svc := NewHandler(stubProvider{})
	inner := `<FromDHCP>false</FromDHCP><DNSManual><IPv4Address>8.8.8.8</IPv4Address></DNSManual>`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("SetDNS", inner)))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "SetDNSResponse") {
		t.Fatalf("body missing SetDNSResponse: %s", rec.Body.String())
	}
}

func TestServeHTTP_GetNetworkInterfaces(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetNetworkInterfaces", "")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "GetNetworkInterfacesResponse") {
		t.Fatalf("body missing GetNetworkInterfacesResponse: %s", rec.Body.String())
	}
}

func TestServeHTTP_SetNetworkInterfaces(t *testing.T) {
	svc := NewHandler(stubProvider{})
	inner := `<InterfaceToken>eth0</InterfaceToken>` +
		`<NetworkInterface><Enabled>true</Enabled>` +
		`<IPv4><Enabled>true</Enabled><Config><DHCP>false</DHCP>` +
		`<Manual><Address>192.168.1.10</Address><PrefixLength>24</PrefixLength></Manual>` +
		`</Config></IPv4></NetworkInterface>`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("SetNetworkInterfaces", inner)))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "SetNetworkInterfacesResponse") {
		t.Fatalf("body missing SetNetworkInterfacesResponse: %s", body)
	}
	if !strings.Contains(body, "<RebootNeeded>false</RebootNeeded>") {
		t.Fatalf("body missing RebootNeeded: %s", body)
	}
}

func TestServeHTTP_SetNetworkInterfacesBadPayload(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("SetNetworkInterfaces", "<bad><xml")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestServeHTTP_GetNetworkProtocols(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetNetworkProtocols", "")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<Name>HTTP</Name>") {
		t.Fatalf("body missing HTTP protocol: %s", rec.Body.String())
	}
}

func TestServeHTTP_SetNetworkProtocols(t *testing.T) {
	svc := NewHandler(stubProvider{})
	inner := `<NetworkProtocols><Name>HTTP</Name><Enabled>true</Enabled><Port>80</Port></NetworkProtocols>`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("SetNetworkProtocols", inner)))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "SetNetworkProtocolsResponse") {
		t.Fatalf("body missing SetNetworkProtocolsResponse: %s", rec.Body.String())
	}
}

func TestServeHTTP_GetNetworkDefaultGateway(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetNetworkDefaultGateway", "")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "GetNetworkDefaultGatewayResponse") {
		t.Fatalf("body missing GetNetworkDefaultGatewayResponse: %s", rec.Body.String())
	}
}

func TestServeHTTP_SetNetworkDefaultGateway(t *testing.T) {
	svc := NewHandler(stubProvider{})
	inner := `<IPv4Address>192.168.1.1</IPv4Address>`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("SetNetworkDefaultGateway", inner)))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "SetNetworkDefaultGatewayResponse") {
		t.Fatalf("body missing SetNetworkDefaultGatewayResponse: %s", rec.Body.String())
	}
}

// --- System tests ---

func TestServeHTTP_GetSystemDateAndTime(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetSystemDateAndTime", "")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<DateTimeType>Manual</DateTimeType>") {
		t.Fatalf("body missing DateTimeType: %s", rec.Body.String())
	}
}

func TestServeHTTP_SetSystemDateAndTime(t *testing.T) {
	svc := NewHandler(stubProvider{})
	inner := `<DateTimeType>Manual</DateTimeType><DaylightSavings>false</DaylightSavings>` +
		`<TimeZone><TZ>UTC</TZ></TimeZone>` +
		`<UTCDateTime><Date><Year>2026</Year><Month>4</Month><Day>24</Day></Date>` +
		`<Time><Hour>12</Hour><Minute>0</Minute><Second>0</Second></Time></UTCDateTime>`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("SetSystemDateAndTime", inner)))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "SetSystemDateAndTimeResponse") {
		t.Fatalf("body missing SetSystemDateAndTimeResponse: %s", rec.Body.String())
	}
}

func TestServeHTTP_SetSystemFactoryDefault(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("SetSystemFactoryDefault", "<FactoryDefault>Soft</FactoryDefault>")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "SetSystemFactoryDefaultResponse") {
		t.Fatalf("body missing SetSystemFactoryDefaultResponse: %s", rec.Body.String())
	}
}

func TestServeHTTP_SystemReboot(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("SystemReboot", "")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "Rebooting simulator") {
		t.Fatalf("body missing reboot message: %s", rec.Body.String())
	}
}

// --- User tests ---

func TestServeHTTP_GetUsers(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetUsers", "")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<Username>admin</Username>") {
		t.Fatalf("body missing admin user: %s", rec.Body.String())
	}
}

func TestServeHTTP_CreateUsers(t *testing.T) {
	svc := NewHandler(stubProvider{})
	inner := `<User><Username>operator</Username><Password>pass</Password><UserLevel>Operator</UserLevel></User>`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("CreateUsers", inner)))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "CreateUsersResponse") {
		t.Fatalf("body missing CreateUsersResponse: %s", rec.Body.String())
	}
}

func TestServeHTTP_SetUser(t *testing.T) {
	svc := NewHandler(stubProvider{})
	inner := `<User><Username>admin</Username><Password>newpass</Password><UserLevel>Administrator</UserLevel></User>`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("SetUser", inner)))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "SetUserResponse") {
		t.Fatalf("body missing SetUserResponse: %s", rec.Body.String())
	}
}

func TestServeHTTP_DeleteUsers(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("DeleteUsers", "<Username>admin</Username>")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "DeleteUsersResponse") {
		t.Fatalf("body missing DeleteUsersResponse: %s", rec.Body.String())
	}
}

// --- Provider error tests for new operations ---

func TestServeHTTP_ProviderErrors_NewOps(t *testing.T) {
	cases := []struct {
		op    string
		inner string
	}{
		{"GetDiscoveryMode", ""},
		{"SetDiscoveryMode", "<DiscoveryMode>Discoverable</DiscoveryMode>"},
		{"GetScopes", ""},
		{"SetScopes", "<Scopes>onvif://www.onvif.org/type/video_encoder</Scopes>"},
		{"AddScopes", "<ScopeItem>onvif://www.onvif.org/type/ptz</ScopeItem>"},
		{"RemoveScopes", "<ScopeItem>onvif://www.onvif.org/type/ptz</ScopeItem>"},
		{"GetHostname", ""},
		{"SetHostname", "<Name>host</Name>"},
		{"GetDNS", ""},
		{"SetDNS", "<FromDHCP>false</FromDHCP>"},
		{"GetNetworkInterfaces", ""},
		{"SetNetworkInterfaces", `<InterfaceToken>eth0</InterfaceToken><NetworkInterface><Enabled>true</Enabled></NetworkInterface>`},
		{"GetNetworkProtocols", ""},
		{"SetNetworkProtocols", "<NetworkProtocols><Name>HTTP</Name><Enabled>true</Enabled></NetworkProtocols>"},
		{"GetNetworkDefaultGateway", ""},
		{"SetNetworkDefaultGateway", "<IPv4Address>192.168.1.1</IPv4Address>"},
		{"GetSystemDateAndTime", ""},
		{"SetSystemFactoryDefault", "<FactoryDefault>Soft</FactoryDefault>"},
		{"SystemReboot", ""},
		{"GetUsers", ""},
		{"CreateUsers", "<User><Username>u</Username><Password>p</Password><UserLevel>User</UserLevel></User>"},
		{"SetUser", "<User><Username>u</Username><Password>p</Password><UserLevel>User</UserLevel></User>"},
		{"DeleteUsers", "<Username>u</Username>"},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			svc := NewHandler(errProvider{})
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest(tc.op, tc.inner)))
			rec := httptest.NewRecorder()
			svc.ServeHTTP(rec, req)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
			}
			if !strings.Contains(rec.Body.String(), errProviderBoom.Error()) {
				t.Fatalf("body missing provider error: %s", rec.Body.String())
			}
		})
	}
}

func TestServeHTTP_SetSystemDateAndTimeProviderError(t *testing.T) {
	svc := NewHandler(errProvider{})
	inner := `<DateTimeType>Manual</DateTimeType><DaylightSavings>false</DaylightSavings>` +
		`<TimeZone><TZ>UTC</TZ></TimeZone>` +
		`<UTCDateTime><Date><Year>2026</Year><Month>4</Month><Day>24</Day></Date>` +
		`<Time><Hour>12</Hour><Minute>0</Minute><Second>0</Second></Time></UTCDateTime>`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("SetSystemDateAndTime", inner)))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

var errDigestChallenge = errors.New("digest challenge")
var errForbiddenChallenge = errors.New("forbidden challenge")

// --- Auth fault path tests ---

func TestServeHTTP_AuthFaultChallengeError(t *testing.T) {
	challenge := &auth.ChallengeError{
		Status:  http.StatusUnauthorized,
		Headers: http.Header{"WWW-Authenticate": []string{`Digest realm="onvif"`}},
		Err:     errDigestChallenge,
	}
	svc := NewHandler(stubProvider{}, WithAuthHook(AuthFunc(func(context.Context, string, *http.Request) error {
		return challenge
	})))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetWsdlUrl", "")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("WWW-Authenticate header missing from challenge response")
	}
}

func TestServeHTTP_AuthFaultForbidden(t *testing.T) {
	svc := NewHandler(stubProvider{}, WithAuthHook(AuthFunc(func(context.Context, string, *http.Request) error {
		return auth.ErrForbidden
	})))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetWsdlUrl", "")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestServeHTTP_AuthFaultChallengeErrorCustomStatus(t *testing.T) {
	challenge := &auth.ChallengeError{
		Status:  http.StatusForbidden,
		Headers: http.Header{},
		Err:     errForbiddenChallenge,
	}
	svc := NewHandler(stubProvider{}, WithAuthHook(AuthFunc(func(context.Context, string, *http.Request) error {
		return challenge
	})))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetWsdlUrl", "")))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
