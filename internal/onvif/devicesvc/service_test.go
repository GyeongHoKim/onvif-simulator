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
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetUsers", "")))
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

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "decode GetServices") {
		t.Fatalf("body missing decode error: %s", rec.Body.String())
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
