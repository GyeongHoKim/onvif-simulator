package media2svc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockProvider implements Provider for unit tests.
type mockProvider struct {
	profiles            []Profile
	videoEncoderConfigs []VideoEncoderConfiguration
	streamURIs          map[string]string
	capabilities        ServiceCapabilities
	metadataConfigs     []MetadataConfiguration
	osdConfigs          []OSDConfiguration
}

func (m *mockProvider) ServiceCapabilities(_ context.Context) (ServiceCapabilities, error) {
	return m.capabilities, nil
}

func (m *mockProvider) Profiles(_ context.Context) ([]Profile, error) {
	return m.profiles, nil
}

func (m *mockProvider) VideoEncoderConfigurations(_ context.Context) ([]VideoEncoderConfiguration, error) {
	return m.videoEncoderConfigs, nil
}

func (m *mockProvider) VideoEncoderConfiguration(_ context.Context, token string) (VideoEncoderConfiguration, error) {
	for _, cfg := range m.videoEncoderConfigs {
		if cfg.Token == token {
			return cfg, nil
		}
	}
	return VideoEncoderConfiguration{}, nil
}

func (*mockProvider) VideoEncoderConfigurationOptions(_ context.Context, _ string) (VideoEncoderConfigurationOptions, error) {
	return VideoEncoderConfigurationOptions{
		H264: &H264Options{
			ResolutionsAvailable: []Resolution{{Width: 1920, Height: 1080}, {Width: 1280, Height: 720}},
			FrameRateRange:       IntRange{Min: 1, Max: 30},
			BitrateRange:         IntRange{Min: 100000, Max: 8000000},
		},
		H265: &H265Options{
			ResolutionsAvailable: []Resolution{{Width: 3840, Height: 2160}, {Width: 1920, Height: 1080}},
			FrameRateRange:       IntRange{Min: 1, Max: 60},
			BitrateRange:         IntRange{Min: 100000, Max: 16000000},
		},
		JPEG: &JPEGOptions{
			ResolutionsAvailable: []Resolution{{Width: 1920, Height: 1080}},
		},
	}, nil
}

func (*mockProvider) SetVideoEncoderConfiguration(_ context.Context, _ string, _ *VideoEncoderConfiguration) error {
	return nil
}

func (m *mockProvider) StreamURI(_ context.Context, profileToken, _ string) (string, error) {
	if m.streamURIs != nil {
		if uri, ok := m.streamURIs[profileToken]; ok {
			return uri, nil
		}
	}
	return "rtsp://localhost:8554/" + profileToken, nil
}

func (*mockProvider) SnapshotURI(_ context.Context, profileToken string) (string, error) {
	return "http://localhost:8080/snapshot/" + profileToken, nil
}

func (m *mockProvider) MetadataConfigurations(_ context.Context) ([]MetadataConfiguration, error) {
	return m.metadataConfigs, nil
}

func (*mockProvider) MetadataConfiguration(_ context.Context, _ string) (MetadataConfiguration, error) {
	return MetadataConfiguration{}, nil
}

func (*mockProvider) MetadataConfigurationOptions(_ context.Context, _ string) (MetadataConfigurationOptions, error) {
	return MetadataConfigurationOptions{
		PTZStatusFilter: true,
		AnalyticsFilter: true,
		EventFilter:     true,
	}, nil
}

func (m *mockProvider) OSDConfigurations(_ context.Context) ([]OSDConfiguration, error) {
	return m.osdConfigs, nil
}

func (*mockProvider) OSDConfiguration(_ context.Context, _ string) (OSDConfiguration, error) {
	return OSDConfiguration{}, nil
}

func (*mockProvider) CreateOSD(_ context.Context, _ OSDConfiguration) (string, error) {
	return "osd_1", nil
}

func (*mockProvider) DeleteOSD(_ context.Context, _ string) error {
	return nil
}

func (*mockProvider) OSDConfigurationOptions(_ context.Context) (OSDConfigurationOptions, error) {
	return OSDConfigurationOptions{
		Types:       []string{"Text", "Image", "Plain"},
		Positions:   []string{"UpperLeft", "UpperRight", "LowerLeft", "LowerRight"},
		TextFormats: []string{"Plain", "Date", "Time", "DateTime"},
	}, nil
}

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	return NewHandler(&mockProvider{
		profiles: []Profile{
			{
				Token: "profile_h265",
				Name:  "H265_Main",
				VideoEncoderConfigurations: []VideoEncoderConfiguration{
					{Token: "venc_h265", Encoding: "H265"},
				},
			},
			{
				Token: "profile_h264",
				Name:  "H264_Sub",
				VideoEncoderConfigurations: []VideoEncoderConfiguration{
					{Token: "venc_h264", Encoding: "H264"},
				},
			},
		},
		videoEncoderConfigs: []VideoEncoderConfiguration{
			{
				Token:     "venc_h265",
				Name:      "H265_Encoder",
				Encoding:  "H265",
				Width:     1920,
				Height:    1080,
				FrameRate: 30,
				Bitrate:   4000000,
			},
			{
				Token:     "venc_h264",
				Name:      "H264_Encoder",
				Encoding:  "H264",
				Width:     1280,
				Height:    720,
				FrameRate: 30,
				Bitrate:   2000000,
			},
		},
		capabilities: ServiceCapabilities{
			H264: true,
			H265: true,
			JPEG: true,
		},
	})
}

func media2SoapRequest(operation, payload string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
  xmlns:tr2="http://www.onvif.org/ver20/media/wsdl">
  <s:Body>
    <tr2:` + operation + `>` + payload + `</tr2:` + operation + `>
  </s:Body>
</s:Envelope>`
}

func callMedia2Service(t *testing.T, h *Handler, body string) string {
	t.Helper()
	ctx := context.Background()
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, Media2ServicePath, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// ---------- GetServiceCapabilities ----------

func TestGetServiceCapabilities(t *testing.T) {
	h := newTestHandler(t)
	resp := callMedia2Service(t, h, media2SoapRequest("GetServiceCapabilities", ""))
	if !strings.Contains(resp, "GetServiceCapabilitiesResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	// Profile T requires H.265 support
	if !strings.Contains(resp, "H265") {
		t.Fatalf("missing H265 capability in response: %s", resp)
	}
}

// ---------- GetProfiles ----------

func TestGetProfiles(t *testing.T) {
	h := newTestHandler(t)
	resp := callMedia2Service(t, h, media2SoapRequest("GetProfiles", ""))
	if !strings.Contains(resp, "GetProfilesResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	// Must contain H.265 profile
	if !strings.Contains(resp, "profile_h265") {
		t.Fatalf("missing H.265 profile token: %s", resp)
	}
	if !strings.Contains(resp, "H265_Main") {
		t.Fatalf("missing H.265 profile name: %s", resp)
	}
}

func TestGetProfiles_ContainsH265Encoding(t *testing.T) {
	h := newTestHandler(t)
	resp := callMedia2Service(t, h, media2SoapRequest("GetProfiles", ""))
	// Profile T mandates H.265 encoding support
	if !strings.Contains(resp, "H265") {
		t.Fatalf("expected H.265 encoding in profiles: %s", resp)
	}
}

// ---------- GetVideoEncoderConfigurations ----------

func TestGetVideoEncoderConfigurations(t *testing.T) {
	h := newTestHandler(t)
	resp := callMedia2Service(t, h, media2SoapRequest("GetVideoEncoderConfigurations", ""))
	if !strings.Contains(resp, "GetVideoEncoderConfigurationsResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	// Must include H.265 encoder configuration
	if !strings.Contains(resp, "venc_h265") {
		t.Fatalf("missing H.265 encoder config token: %s", resp)
	}
}

func TestGetVideoEncoderConfigurations_IncludesH265(t *testing.T) {
	h := newTestHandler(t)
	resp := callMedia2Service(t, h, media2SoapRequest("GetVideoEncoderConfigurations", ""))
	// Profile T requires H.265 encoding to be advertised
	if !strings.Contains(resp, "H265") {
		t.Fatalf("expected H.265 in encoder configurations: %s", resp)
	}
}

// ---------- GetVideoEncoderConfiguration ----------

func TestGetVideoEncoderConfiguration(t *testing.T) {
	h := newTestHandler(t)
	resp := callMedia2Service(t, h, media2SoapRequest("GetVideoEncoderConfiguration",
		`<tr2:ConfigurationToken>venc_h265</tr2:ConfigurationToken>`))
	if !strings.Contains(resp, "GetVideoEncoderConfigurationResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	if !strings.Contains(resp, "venc_h265") {
		t.Fatalf("missing encoder config token: %s", resp)
	}
}

// ---------- GetVideoEncoderConfigurationOptions ----------

func TestGetVideoEncoderConfigurationOptions(t *testing.T) {
	h := newTestHandler(t)
	resp := callMedia2Service(t, h, media2SoapRequest("GetVideoEncoderConfigurationOptions",
		`<tr2:ConfigurationToken>venc_h265</tr2:ConfigurationToken>`))
	if !strings.Contains(resp, "GetVideoEncoderConfigurationOptionsResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	// Profile T requires H.265 options
	if !strings.Contains(resp, "H265") {
		t.Fatalf("missing H.265 options in response: %s", resp)
	}
}

func TestGetVideoEncoderConfigurationOptions_IncludesH265(t *testing.T) {
	h := newTestHandler(t)
	resp := callMedia2Service(t, h, media2SoapRequest("GetVideoEncoderConfigurationOptions",
		`<tr2:ConfigurationToken>venc_h265</tr2:ConfigurationToken>`))
	// Must advertise H.265 resolution options
	if !strings.Contains(resp, "Resolution") || !strings.Contains(resp, "Width") {
		t.Fatalf("missing resolution options: %s", resp)
	}
}

// ---------- GetStreamUri ----------

func TestGetStreamUri(t *testing.T) {
	h := newTestHandler(t)
	resp := callMedia2Service(t, h, media2SoapRequest("GetStreamUri",
		`<tr2:ProfileToken>profile_h265</tr2:ProfileToken>
     <tr2:Protocol>RTSP</tr2:Protocol>`))
	if !strings.Contains(resp, "GetStreamUriResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	if !strings.Contains(resp, "rtsp://") {
		t.Fatalf("missing RTSP URI: %s", resp)
	}
}

// ---------- GetSnapshotUri ----------

func TestGetSnapshotUri(t *testing.T) {
	h := newTestHandler(t)
	resp := callMedia2Service(t, h, media2SoapRequest("GetSnapshotUri",
		`<tr2:ProfileToken>profile_h265</tr2:ProfileToken>`))
	if !strings.Contains(resp, "GetSnapshotUriResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
	if !strings.Contains(resp, "http://") {
		t.Fatalf("missing snapshot URI: %s", resp)
	}
}

// ---------- SetVideoEncoderConfiguration ----------

func TestSetVideoEncoderConfiguration(t *testing.T) {
	h := newTestHandler(t)
	resp := callMedia2Service(t, h, media2SoapRequest("SetVideoEncoderConfiguration", `
    <tr2:Configuration>
      <tr2:token>venc_h265</tr2:token>
      <tr2:Name>H265_Encoder</tr2:Name>
      <tr2:Encoding>H265</tr2:Encoding>
      <tr2:Width>3840</tr2:Width>
      <tr2:Height>2160</tr2:Height>
      <tr2:FrameRate>60</tr2:FrameRate>
      <tr2:Bitrate>8000000</tr2:Bitrate>
    </tr2:Configuration>`))
	if !strings.Contains(resp, "SetVideoEncoderConfigurationResponse") {
		t.Fatalf("missing response element: %s", resp)
	}
}

// ---------- Error handling ----------

func TestUnsupportedOperation(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		Media2ServicePath, strings.NewReader(media2SoapRequest("BogusOperation", "")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("want 501, got %d", rec.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
		Media2ServicePath, http.NoBody)
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
