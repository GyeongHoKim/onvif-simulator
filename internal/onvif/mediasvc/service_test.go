package mediasvc

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

// stubProvider returns minimal deterministic data for every operation.
type stubProvider struct{}

func (stubProvider) ServiceCapabilities(context.Context) (ServiceCapabilities, error) {
	return ServiceCapabilities{
		SnapshotURI:             true,
		RTPTCP:                  true,
		RTPRTSPTCP:              true,
		MaximumNumberOfProfiles: 2,
	}, nil
}

func (stubProvider) Profiles(context.Context) ([]Profile, error) {
	return []Profile{{
		Token: "profile_main",
		Name:  "main",
		Fixed: true,
		VideoSource: &VideoSourceConfiguration{
			Token: "VSConfig_main", Name: "main", UseCount: 1, SourceToken: "VS_MAIN",
			Bounds: Rectangle{Width: 1920, Height: 1080},
		},
		VideoEncoder: &VideoEncoderConfiguration{
			Token: "VEConfig_main", Name: "main", UseCount: 1,
			Encoding: "H264", Resolution: Resolution{Width: 1920, Height: 1080},
			Quality:        5,
			RateControl:    VideoRateControl{FrameRateLimit: 30, EncodingInterval: 1, BitrateLimit: 4096},
			H264:           H264Configuration{GOVLength: 60, H264Profile: "Main"},
			SessionTimeout: "PT0S",
		},
	}}, nil
}

func (s stubProvider) Profile(ctx context.Context, token string) (Profile, error) {
	ps, err := s.Profiles(ctx)
	if err != nil {
		return Profile{}, err
	}
	for _, p := range ps {
		if p.Token == token {
			return p, nil
		}
	}
	return Profile{}, ErrProfileNotFound
}

func (stubProvider) CreateProfile(_ context.Context, name, token string) (Profile, error) {
	return Profile{Token: token, Name: name, Fixed: false}, nil
}

func (stubProvider) DeleteProfile(context.Context, string) error { return nil }

func (stubProvider) VideoSources(context.Context) ([]VideoSource, error) {
	return []VideoSource{{Token: "VS_MAIN", Framerate: 30, Resolution: Resolution{Width: 1920, Height: 1080}}}, nil
}

func (stubProvider) VideoSourceConfigurations(context.Context) ([]VideoSourceConfiguration, error) {
	return []VideoSourceConfiguration{{
		Token: "VSConfig_main", Name: "main", UseCount: 1, SourceToken: "VS_MAIN",
		Bounds: Rectangle{Width: 1920, Height: 1080},
	}}, nil
}

func (stubProvider) VideoSourceConfiguration(_ context.Context, token string) (VideoSourceConfiguration, error) {
	if token != "VSConfig_main" {
		return VideoSourceConfiguration{}, ErrConfigNotFound
	}
	return VideoSourceConfiguration{
		Token: "VSConfig_main", Name: "main", UseCount: 1, SourceToken: "VS_MAIN",
		Bounds: Rectangle{Width: 1920, Height: 1080},
	}, nil
}

func (stubProvider) SetVideoSourceConfiguration(context.Context, VideoSourceConfiguration) error {
	return nil
}

func (stubProvider) AddVideoSourceConfiguration(context.Context, string, string) error { return nil }

func (stubProvider) RemoveVideoSourceConfiguration(context.Context, string) error { return nil }

func (stubProvider) CompatibleVideoSourceConfigurations(context.Context, string) ([]VideoSourceConfiguration, error) {
	return []VideoSourceConfiguration{{Token: "VSConfig_main", Name: "main"}}, nil
}

func (stubProvider) VideoSourceConfigurationOptions(context.Context, string, string) (VideoSourceConfigurationOptions, error) {
	var opt VideoSourceConfigurationOptions
	opt.MaximumNumberOfProfiles = 2
	opt.BoundsRange.XRange = IntRange{Min: 0, Max: 0}
	opt.BoundsRange.YRange = IntRange{Min: 0, Max: 0}
	opt.BoundsRange.WidthRange = IntRange{Min: 320, Max: 1920}
	opt.BoundsRange.HeightRange = IntRange{Min: 240, Max: 1080}
	opt.VideoSourceTokensAvailable = []string{"VS_MAIN"}
	return opt, nil
}

func (stubProvider) VideoEncoderConfigurations(context.Context) ([]VideoEncoderConfiguration, error) {
	return []VideoEncoderConfiguration{{
		Token: "VEConfig_main", Name: "main", UseCount: 1, Encoding: "H264",
		Resolution: Resolution{Width: 1920, Height: 1080}, Quality: 5,
		RateControl: VideoRateControl{FrameRateLimit: 30, BitrateLimit: 4096, EncodingInterval: 1},
		H264:        H264Configuration{GOVLength: 60, H264Profile: "Main"},
	}}, nil
}

func (stubProvider) VideoEncoderConfiguration(_ context.Context, token string) (VideoEncoderConfiguration, error) {
	if token != "VEConfig_main" {
		return VideoEncoderConfiguration{}, ErrConfigNotFound
	}
	return VideoEncoderConfiguration{
		Token: token, Name: "main", Encoding: "H264",
		Resolution: Resolution{Width: 1280, Height: 720}, Quality: 6,
	}, nil
}

func (stubProvider) SetVideoEncoderConfiguration(context.Context, VideoEncoderConfiguration) error {
	return nil
}

func (stubProvider) AddVideoEncoderConfiguration(context.Context, string, string) error { return nil }

func (stubProvider) RemoveVideoEncoderConfiguration(context.Context, string) error { return nil }

func (stubProvider) CompatibleVideoEncoderConfigurations(context.Context, string) ([]VideoEncoderConfiguration, error) {
	return []VideoEncoderConfiguration{{Token: "VEConfig_main", Name: "main", Encoding: "H264"}}, nil
}

func (stubProvider) VideoEncoderConfigurationOptions(context.Context, string, string) (VideoEncoderConfigurationOptions, error) {
	opt := VideoEncoderConfigurationOptions{
		QualityRange: IntRange{Min: 0, Max: 10},
	}
	opt.H264.ResolutionsAvailable = []ResolutionOptions{{Width: 640, Height: 480}, {Width: 1920, Height: 1080}}
	opt.H264.GovLengthRange = IntRange{Min: 1, Max: 120}
	opt.H264.FrameRateRange = IntRange{Min: 1, Max: 60}
	opt.H264.EncodingIntervalRange = IntRange{Min: 1, Max: 1}
	opt.H264.H264ProfilesSupported = []string{"Baseline", "Main", "High"}
	return opt, nil
}

func (stubProvider) StreamURI(_ context.Context, token string, _ StreamSetup) (MediaURI, error) {
	if token != "profile_main" {
		return MediaURI{}, ErrProfileNotFound
	}
	return MediaURI{URI: "rtsp://127.0.0.1:8554/main", Timeout: "PT0S"}, nil
}

func (stubProvider) SnapshotURI(_ context.Context, token string) (MediaURI, error) {
	if token != "profile_main" {
		return MediaURI{}, ErrProfileNotFound
	}
	return MediaURI{URI: "http://127.0.0.1:8080/snapshot/main.jpg", Timeout: "PT0S"}, nil
}

// errProvider always returns errProviderBoom; used to prove the 500/Receiver mapping.
type errProvider struct{ stubProvider }

var errProviderBoom = errors.New("provider boom")

func (errProvider) Profiles(context.Context) ([]Profile, error) { return nil, errProviderBoom }

// ---------- helpers ----------

func soapRequest(op, inner string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope" xmlns:trt="` + MediaNamespace + `">` +
		`<env:Body><trt:` + op + `>` + inner + `</trt:` + op + `></env:Body></env:Envelope>`
}

func TestVEConfigToEnvelope_OmitsClearedOptionalFields(t *testing.T) {
	env := veConfigToEnvelope(&VideoEncoderConfiguration{
		Token:      "VEConfig_main",
		Name:       "main",
		UseCount:   1,
		Encoding:   "H264",
		Resolution: Resolution{Width: 1920, Height: 1080},
		Quality:    5,
		RateControl: VideoRateControl{
			FrameRateLimit:   30,
			EncodingInterval: 1,
		},
		H264: H264Configuration{
			H264Profile: "Main",
		},
	})

	got, err := xml.Marshal(env)
	if err != nil {
		t.Fatalf("xml.Marshal: %v", err)
	}
	xmlText := string(got)

	if strings.Contains(xmlText, "<tt:BitrateLimit>") {
		t.Fatalf("BitrateLimit should be omitted when cleared: %s", xmlText)
	}
	if strings.Contains(xmlText, "<tt:GovLength>") {
		t.Fatalf("GovLength should be omitted when cleared: %s", xmlText)
	}
	if !strings.Contains(xmlText, "<tt:RateControl>") {
		t.Fatalf("RateControl should be present when other limits are set: %s", xmlText)
	}
	if !strings.Contains(xmlText, "<tt:H264>") {
		t.Fatalf("H264 should be present when H264Profile is set: %s", xmlText)
	}

	env = veConfigToEnvelope(&VideoEncoderConfiguration{
		Token:      "VEConfig_main",
		Name:       "main",
		UseCount:   1,
		Encoding:   "H264",
		Resolution: Resolution{Width: 1920, Height: 1080},
		Quality:    5,
	})

	got, err = xml.Marshal(env)
	if err != nil {
		t.Fatalf("xml.Marshal cleared: %v", err)
	}
	xmlText = string(got)

	if strings.Contains(xmlText, "<tt:RateControl>") {
		t.Fatalf("RateControl should be omitted when fully cleared: %s", xmlText)
	}
	if strings.Contains(xmlText, "<tt:H264>") {
		t.Fatalf("H264 should be omitted when fully cleared: %s", xmlText)
	}
}

func doRequest(t *testing.T, h *Handler, op, inner string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, MediaServicePath,
		bytes.NewBufferString(soapRequest(op, inner)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func soapBodyInnerXML(t *testing.T, body []byte) []byte {
	t.Helper()

	var env struct {
		Body struct {
			Inner []byte `xml:",innerxml"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal response envelope: %v", err)
	}
	if len(env.Body.Inner) == 0 {
		t.Fatal("response envelope body must not be empty")
	}
	return env.Body.Inner
}

func soapBodyRootName(t *testing.T, body []byte) xml.Name {
	t.Helper()

	decoder := xml.NewDecoder(bytes.NewReader(soapBodyInnerXML(t, body)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			t.Fatalf("read response body token: %v", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start.Name
		}
	}
}

func unmarshalSOAPBody(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := xml.Unmarshal(soapBodyInnerXML(t, body), v); err != nil {
		t.Fatalf("unmarshal soap body: %v", err)
	}
}

type serviceCapabilitiesResponseView struct {
	XMLName      xml.Name `xml:"GetServiceCapabilitiesResponse"`
	Capabilities struct {
		SnapshotURI         bool `xml:"SnapshotUri,attr"`
		ProfileCapabilities struct {
			MaximumNumberOfProfiles int `xml:"MaximumNumberOfProfiles,attr"`
		} `xml:"ProfileCapabilities"`
	} `xml:"Capabilities"`
}

type profilesResponseView struct {
	XMLName  xml.Name `xml:"GetProfilesResponse"`
	Profiles []struct {
		Token                     string `xml:"token,attr"`
		VideoEncoderConfiguration *struct {
			Encoding string `xml:"Encoding"`
		} `xml:"VideoEncoderConfiguration"`
	} `xml:"Profiles"`
}

type profileResponseView struct {
	XMLName xml.Name `xml:"GetProfileResponse"`
	Profile struct {
		Token string `xml:"token,attr"`
	} `xml:"Profile"`
}

type mediaURIResponseView struct {
	MediaURI struct {
		URI string `xml:"Uri"`
	} `xml:"MediaUri"`
}

// ---------- tests ----------

func TestServeHTTP_GetServiceCapabilities(t *testing.T) {
	svc := NewHandler(stubProvider{})
	rec := doRequest(t, svc, "GetServiceCapabilities", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp serviceCapabilitiesResponseView
	unmarshalSOAPBody(t, rec.Body.Bytes(), &resp)
	if resp.Capabilities.SnapshotURI != true {
		t.Fatalf("SnapshotUri = %t, want true", resp.Capabilities.SnapshotURI)
	}
	if resp.Capabilities.ProfileCapabilities.MaximumNumberOfProfiles != 2 {
		t.Fatalf(
			"MaximumNumberOfProfiles = %d, want 2",
			resp.Capabilities.ProfileCapabilities.MaximumNumberOfProfiles,
		)
	}
}

func TestServeHTTP_GetProfiles(t *testing.T) {
	svc := NewHandler(stubProvider{})
	rec := doRequest(t, svc, "GetProfiles", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp profilesResponseView
	unmarshalSOAPBody(t, rec.Body.Bytes(), &resp)
	if len(resp.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(resp.Profiles))
	}
	if resp.Profiles[0].Token != "profile_main" {
		t.Fatalf("profile token = %q, want profile_main", resp.Profiles[0].Token)
	}
	if resp.Profiles[0].VideoEncoderConfiguration == nil {
		t.Fatal("VideoEncoderConfiguration must be present")
	}
	if resp.Profiles[0].VideoEncoderConfiguration.Encoding != "H264" {
		t.Fatalf(
			"encoder encoding = %q, want H264",
			resp.Profiles[0].VideoEncoderConfiguration.Encoding,
		)
	}
}

func TestServeHTTP_GetProfile(t *testing.T) {
	svc := NewHandler(stubProvider{})
	rec := doRequest(t, svc, "GetProfile", "<ProfileToken>profile_main</ProfileToken>")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp profileResponseView
	unmarshalSOAPBody(t, rec.Body.Bytes(), &resp)
	if resp.Profile.Token != "profile_main" {
		t.Fatalf("profile token = %q, want profile_main", resp.Profile.Token)
	}
}

func TestServeHTTP_GetProfile_NotFound(t *testing.T) {
	svc := NewHandler(stubProvider{})
	rec := doRequest(t, svc, "GetProfile", "<ProfileToken>ghost</ProfileToken>")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "env:Sender") {
		t.Fatalf("body missing Sender fault code: %s", rec.Body.String())
	}
}

func TestServeHTTP_GetStreamUri(t *testing.T) {
	svc := NewHandler(stubProvider{})
	body := `<StreamSetup><Stream>RTP-Unicast</Stream><Transport><Protocol>RTSP</Protocol></Transport></StreamSetup>` +
		`<ProfileToken>profile_main</ProfileToken>`
	rec := doRequest(t, svc, "GetStreamUri", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp mediaURIResponseView
	unmarshalSOAPBody(t, rec.Body.Bytes(), &resp)
	if resp.MediaURI.URI != "rtsp://127.0.0.1:8554/main" {
		t.Fatalf("MediaUri.Uri = %q, want rtsp://127.0.0.1:8554/main", resp.MediaURI.URI)
	}
}

func TestServeHTTP_GetSnapshotUri(t *testing.T) {
	svc := NewHandler(stubProvider{})
	rec := doRequest(t, svc, "GetSnapshotUri", "<ProfileToken>profile_main</ProfileToken>")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp mediaURIResponseView
	unmarshalSOAPBody(t, rec.Body.Bytes(), &resp)
	if resp.MediaURI.URI != "http://127.0.0.1:8080/snapshot/main.jpg" {
		t.Fatalf(
			"MediaUri.Uri = %q, want http://127.0.0.1:8080/snapshot/main.jpg",
			resp.MediaURI.URI,
		)
	}
}

func TestServeHTTP_UnsupportedOperation(t *testing.T) {
	svc := NewHandler(stubProvider{})
	rec := doRequest(t, svc, "StartMulticastStreaming", "<ProfileToken>profile_main</ProfileToken>")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
	if !strings.Contains(rec.Body.String(), "unsupported operation") {
		t.Fatalf("body missing unsupported reason: %s", rec.Body.String())
	}
}

func TestServeHTTP_MethodNotAllowed(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, MediaServicePath, nil)
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestServeHTTP_InvalidEnvelope(t *testing.T) {
	svc := NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, MediaServicePath,
		bytes.NewBufferString("garbage"))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestServeHTTP_ProviderError(t *testing.T) {
	svc := NewHandler(errProvider{})
	rec := doRequest(t, svc, "GetProfiles", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "env:Receiver") {
		t.Fatalf("body missing Receiver fault: %s", rec.Body.String())
	}
}

func TestServeHTTP_SizeCap(t *testing.T) {
	svc := NewHandler(stubProvider{})
	big := strings.Repeat("x", maxSOAPBodySize+1)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, MediaServicePath,
		bytes.NewBufferString(big))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestServeHTTP_AuthHookRejects(t *testing.T) {
	svc := NewHandler(stubProvider{}, WithAuthHook(AuthFunc(func(context.Context, string, *http.Request) error {
		return io.EOF
	})))
	rec := doRequest(t, svc, "GetProfiles", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestParseOperation(t *testing.T) {
	payload, op, err := parseOperation([]byte(soapRequest("GetProfiles", "")))
	if err != nil {
		t.Fatalf("parseOperation: %v", err)
	}
	if op != "GetProfiles" {
		t.Fatalf("operation = %q, want GetProfiles", op)
	}
	if len(payload) == 0 {
		t.Fatal("payload must not be empty")
	}
}

func TestParseOperation_RejectsUnexpectedNamespace(t *testing.T) {
	req := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope" xmlns:bad="http://example.com/other">` +
		`<env:Body><bad:GetProfiles/></env:Body></env:Envelope>`

	_, _, err := parseOperation([]byte(req))
	if !errors.Is(err, errInvalidNamespace) {
		t.Fatalf("parseOperation error = %v, want %v", err, errInvalidNamespace)
	}
	if !strings.Contains(err.Error(), "http://example.com/other") {
		t.Fatalf("error = %v, want namespace in message", err)
	}
}

func TestResponseEnvelopeValid(t *testing.T) {
	// Sanity: response body must unmarshal as a soap envelope and contain
	// the expected response element in the Body.
	svc := NewHandler(stubProvider{})
	rec := doRequest(t, svc, "GetProfiles", "")
	root := soapBodyRootName(t, rec.Body.Bytes())
	if root.Local != "GetProfilesResponse" {
		t.Fatalf("body root = %q, want GetProfilesResponse", root.Local)
	}
}

func TestWithAuthHookNilIsNoop(t *testing.T) {
	svc := NewHandler(stubProvider{}, WithAuthHook(nil))
	if svc.auth == nil {
		t.Fatal("auth must remain non-nil when WithAuthHook receives nil")
	}
}

func TestNewHandlerPanicsOnNilProvider(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewHandler(nil) must panic")
		}
	}()
	NewHandler(nil)
}

// TestServeHTTP_DispatchHappyPath hits every in-scope operation with a
// minimal valid body and asserts the handler returns 200 + the expected
// response element. This covers the dispatch branches that the focused
// tests above don't touch (Delete, VideoSource/VideoEncoder CRUD,
// Compatible/Options, Add/Remove).
func TestServeHTTP_DispatchHappyPath(t *testing.T) {
	svc := NewHandler(stubProvider{})

	vsConfigInner := `<Configuration token="VSConfig_main">` +
		`<tt:Name xmlns:tt="http://www.onvif.org/ver10/schema">main</tt:Name>` +
		`<tt:UseCount xmlns:tt="http://www.onvif.org/ver10/schema">1</tt:UseCount>` +
		`<tt:SourceToken xmlns:tt="http://www.onvif.org/ver10/schema">VS_MAIN</tt:SourceToken>` +
		`<tt:Bounds xmlns:tt="http://www.onvif.org/ver10/schema" x="0" y="0" width="1920" height="1080"/>` +
		`</Configuration>`
	veConfigInner := `<Configuration token="VEConfig_main">` +
		`<tt:Name xmlns:tt="http://www.onvif.org/ver10/schema">main</tt:Name>` +
		`<tt:UseCount xmlns:tt="http://www.onvif.org/ver10/schema">1</tt:UseCount>` +
		`<tt:Encoding xmlns:tt="http://www.onvif.org/ver10/schema">H264</tt:Encoding>` +
		`<tt:Resolution xmlns:tt="http://www.onvif.org/ver10/schema">` +
		`<tt:Width>1920</tt:Width><tt:Height>1080</tt:Height></tt:Resolution>` +
		`<tt:Quality xmlns:tt="http://www.onvif.org/ver10/schema">5</tt:Quality>` +
		`<tt:RateControl xmlns:tt="http://www.onvif.org/ver10/schema">` +
		`<tt:FrameRateLimit>30</tt:FrameRateLimit>` +
		`<tt:EncodingInterval>1</tt:EncodingInterval>` +
		`<tt:BitrateLimit>4096</tt:BitrateLimit></tt:RateControl>` +
		`<tt:H264 xmlns:tt="http://www.onvif.org/ver10/schema">` +
		`<tt:GovLength>60</tt:GovLength><tt:H264Profile>Main</tt:H264Profile></tt:H264>` +
		`</Configuration>`

	cases := []struct {
		op       string
		inner    string
		respElem string
	}{
		{"DeleteProfile", "<ProfileToken>profile_main</ProfileToken>", "DeleteProfileResponse"},
		{"CreateProfile", "<Name>sub</Name><Token>profile_sub</Token>", "CreateProfileResponse"},
		{"GetVideoSources", "", "GetVideoSourcesResponse"},
		{"GetVideoSourceConfigurations", "", "GetVideoSourceConfigurationsResponse"},
		{"GetVideoSourceConfiguration",
			"<ConfigurationToken>VSConfig_main</ConfigurationToken>",
			"GetVideoSourceConfigurationResponse"},
		{"SetVideoSourceConfiguration", vsConfigInner, "SetVideoSourceConfigurationResponse"},
		{"AddVideoSourceConfiguration",
			"<ProfileToken>profile_main</ProfileToken><ConfigurationToken>VSConfig_main</ConfigurationToken>",
			"AddVideoSourceConfigurationResponse"},
		{"RemoveVideoSourceConfiguration",
			"<ProfileToken>profile_main</ProfileToken>",
			"RemoveVideoSourceConfigurationResponse"},
		{"GetCompatibleVideoSourceConfigurations",
			"<ProfileToken>profile_main</ProfileToken>",
			"GetCompatibleVideoSourceConfigurationsResponse"},
		{"GetVideoSourceConfigurationOptions",
			"<ConfigurationToken>VSConfig_main</ConfigurationToken><ProfileToken>profile_main</ProfileToken>",
			"GetVideoSourceConfigurationOptionsResponse"},
		{"GetVideoEncoderConfigurations", "", "GetVideoEncoderConfigurationsResponse"},
		{"GetVideoEncoderConfiguration",
			"<ConfigurationToken>VEConfig_main</ConfigurationToken>",
			"GetVideoEncoderConfigurationResponse"},
		{"SetVideoEncoderConfiguration", veConfigInner, "SetVideoEncoderConfigurationResponse"},
		{"AddVideoEncoderConfiguration",
			"<ProfileToken>profile_main</ProfileToken><ConfigurationToken>VEConfig_main</ConfigurationToken>",
			"AddVideoEncoderConfigurationResponse"},
		{"RemoveVideoEncoderConfiguration",
			"<ProfileToken>profile_main</ProfileToken>",
			"RemoveVideoEncoderConfigurationResponse"},
		{"GetCompatibleVideoEncoderConfigurations",
			"<ProfileToken>profile_main</ProfileToken>",
			"GetCompatibleVideoEncoderConfigurationsResponse"},
		{"GetVideoEncoderConfigurationOptions",
			"<ConfigurationToken>VEConfig_main</ConfigurationToken><ProfileToken>profile_main</ProfileToken>",
			"GetVideoEncoderConfigurationOptionsResponse"},
	}

	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			rec := doRequest(t, svc, tc.op, tc.inner)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
			}
			root := soapBodyRootName(t, rec.Body.Bytes())
			if root.Local != tc.respElem {
				t.Fatalf("response root = %q, want %s", root.Local, tc.respElem)
			}
		})
	}
}

// TestServeHTTP_DecodeErrorPaths covers the errDecodePayload branch in
// every handler that unmarshals a typed request struct.
func TestServeHTTP_DecodeErrorPaths(t *testing.T) {
	svc := NewHandler(stubProvider{})
	malformed := "<ProfileToken><bad></ProfileToken>"

	ops := []string{
		"GetProfile", "CreateProfile", "DeleteProfile",
		"GetVideoSourceConfiguration", "SetVideoSourceConfiguration",
		"AddVideoSourceConfiguration", "RemoveVideoSourceConfiguration",
		"GetCompatibleVideoSourceConfigurations", "GetVideoSourceConfigurationOptions",
		"GetVideoEncoderConfiguration", "SetVideoEncoderConfiguration",
		"AddVideoEncoderConfiguration", "RemoveVideoEncoderConfiguration",
		"GetCompatibleVideoEncoderConfigurations", "GetVideoEncoderConfigurationOptions",
		"GetStreamUri", "GetSnapshotUri",
	}
	for _, op := range ops {
		t.Run(op, func(t *testing.T) {
			rec := doRequest(t, svc, op, malformed)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "env:Sender") {
				t.Fatalf("body missing Sender fault: %s", rec.Body.String())
			}
		})
	}
}

// TestServeHTTP_NoSnapshotURI verifies that a profile without a snapshot
// URI surfaces ErrNoSnapshot as HTTP 400 + Sender fault.
func TestServeHTTP_NoSnapshotURI(t *testing.T) {
	svc := NewHandler(noSnapshotProvider{})
	rec := doRequest(t, svc, "GetSnapshotUri", "<ProfileToken>profile_main</ProfileToken>")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "snapshot uri not available") {
		t.Fatalf("body missing ErrNoSnapshot reason: %s", rec.Body.String())
	}
}

type noSnapshotProvider struct{ stubProvider }

func (noSnapshotProvider) SnapshotURI(context.Context, string) (MediaURI, error) {
	return MediaURI{}, ErrNoSnapshot
}
