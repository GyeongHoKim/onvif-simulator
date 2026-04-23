package devicesvc

import (
	"bytes"
	"context"
	"encoding/xml"
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

func TestReadOperation(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, DeviceServicePath, bytes.NewBufferString(soapRequest("GetWsdlUrl", "")))
	payload, op, err := readOperation(req)
	if err != nil {
		t.Fatalf("readOperation: %v", err)
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

func soapRequest(op, inner string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope" xmlns:tds="` + DeviceNamespace + `">` +
		`<env:Body><tds:` + op + `>` + inner + `</tds:` + op + `></env:Body></env:Envelope>`
}
