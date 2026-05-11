package mediasvc

import (
	"context"
	"encoding/xml"
	"strings"
	"testing"

	"net/http"
)

// providerStub returns canned data so the tests can verify envelope
// shape without exercising the entire simulator.
type providerStub struct {
	options VideoEncoderConfigurationOptions
	guarN   int
}

func (*providerStub) ServiceCapabilities(context.Context) (ServiceCapabilities, error) {
	return ServiceCapabilities{}, nil
}
func (*providerStub) Profiles(context.Context) ([]Profile, error)      { return nil, nil }
func (*providerStub) Profile(context.Context, string) (Profile, error) { return Profile{}, nil }
func (*providerStub) CreateProfile(context.Context, string, string) (Profile, error) {
	return Profile{}, nil
}
func (*providerStub) DeleteProfile(context.Context, string) error { return nil }
func (*providerStub) VideoSources(context.Context) ([]VideoSource, error) {
	return nil, nil
}
func (*providerStub) VideoSourceConfigurations(context.Context) ([]VideoSourceConfiguration, error) {
	return nil, nil
}
func (*providerStub) VideoSourceConfiguration(context.Context, string) (VideoSourceConfiguration, error) {
	return VideoSourceConfiguration{}, nil
}
func (*providerStub) SetVideoSourceConfiguration(context.Context, VideoSourceConfiguration) error {
	return nil
}
func (*providerStub) AddVideoSourceConfiguration(context.Context, string, string) error {
	return nil
}
func (*providerStub) RemoveVideoSourceConfiguration(context.Context, string) error { return nil }
func (*providerStub) CompatibleVideoSourceConfigurations(
	context.Context, string,
) ([]VideoSourceConfiguration, error) {
	return nil, nil
}
func (*providerStub) VideoSourceConfigurationOptions(
	context.Context, string, string,
) (VideoSourceConfigurationOptions, error) {
	return VideoSourceConfigurationOptions{}, nil
}
func (*providerStub) VideoEncoderConfigurations(context.Context) ([]VideoEncoderConfiguration, error) {
	return nil, nil
}
func (*providerStub) VideoEncoderConfiguration(context.Context, string) (VideoEncoderConfiguration, error) {
	return VideoEncoderConfiguration{}, nil
}
func (*providerStub) SetVideoEncoderConfiguration(context.Context, VideoEncoderConfiguration) error {
	return nil
}
func (*providerStub) AddVideoEncoderConfiguration(context.Context, string, string) error {
	return nil
}
func (*providerStub) RemoveVideoEncoderConfiguration(context.Context, string) error { return nil }
func (*providerStub) CompatibleVideoEncoderConfigurations(
	context.Context, string,
) ([]VideoEncoderConfiguration, error) {
	return nil, nil
}
func (p *providerStub) VideoEncoderConfigurationOptions(
	context.Context, string, string,
) (VideoEncoderConfigurationOptions, error) {
	return p.options, nil
}
func (*providerStub) StreamURI(context.Context, string, StreamSetup) (MediaURI, error) {
	return MediaURI{}, nil
}
func (*providerStub) SnapshotURI(context.Context, string) (MediaURI, error) {
	return MediaURI{}, nil
}
func (p *providerStub) GuaranteedNumberOfVideoEncoderInstances(
	context.Context, string,
) (int, error) {
	return p.guarN, nil
}
func (*providerStub) MetadataConfigurations(context.Context) ([]MetadataConfiguration, error) {
	return nil, nil
}
func (*providerStub) MetadataConfiguration(context.Context, string) (MetadataConfiguration, error) {
	return MetadataConfiguration{}, nil
}
func (*providerStub) AddMetadataConfiguration(context.Context, string, string) error {
	return nil
}
func (*providerStub) RemoveMetadataConfiguration(context.Context, string) error { return nil }
func (*providerStub) SetMetadataConfiguration(context.Context, MetadataConfiguration) error {
	return nil
}
func (*providerStub) CompatibleMetadataConfigurations(
	context.Context, string,
) ([]MetadataConfiguration, error) {
	return nil, nil
}
func (*providerStub) MetadataConfigurationOptions(
	context.Context, string, string,
) (MetadataConfigurationOptions, error) {
	return MetadataConfigurationOptions{}, nil
}

// authStub allows every operation through so the test focuses on
// envelope encoding.
type authStub struct{}

func (authStub) Authorize(context.Context, string, *http.Request) error { return nil }

// TestGetVideoEncoderConfigurationOptionsIncludesJPEG verifies that when
// the provider advertises JPEG options, the envelope serializes a
// tt:JPEG block alongside tt:H264 — the Profile S §7.9.1 mandate.
func TestGetVideoEncoderConfigurationOptionsIncludesJPEG(t *testing.T) {
	t.Parallel()
	stub := &providerStub{
		options: VideoEncoderConfigurationOptions{
			QualityRange: IntRange{Min: 1, Max: 5},
			H264: H264Options{
				ResolutionsAvailable: []ResolutionOptions{{Width: 1920, Height: 1080}},
			},
			JPEG: JPEGOptions{
				ResolutionsAvailable: []ResolutionOptions{{Width: 320, Height: 240}},
				FrameRateRange:       IntRange{Min: 1, Max: 60},
			},
		},
	}
	h := NewHandler(stub, WithAuthHook(authStub{}))

	got, err := h.handleGetVideoEncoderConfigurationOptions(context.Background(),
		[]byte("<req/>"))
	if err != nil {
		t.Fatalf("handleGetVideoEncoderConfigurationOptions: %v", err)
	}
	s := string(got)
	if !strings.Contains(s, "<tt:JPEG>") {
		t.Errorf("response missing tt:JPEG element: %s", s)
	}
	if !strings.Contains(s, "<tt:H264>") {
		t.Errorf("response missing tt:H264 element: %s", s)
	}
	// Ensure XML is well-formed.
	var probe struct{ XMLName xml.Name }
	if err := xml.Unmarshal(append([]byte("<root xmlns:tt=\"x\">"), append(got, []byte("</root>")...)...), &probe); err != nil {
		t.Errorf("response is not well-formed XML: %v\n%s", err, s)
	}
}

// TestGetVideoEncoderConfigurationOptionsOmitsJPEGWhenEmpty verifies the
// envelope omits the tt:JPEG element entirely when no JPEG resolutions
// are advertised — keeps legacy clients that ignore unknown elements
// from seeing a stray empty tag.
func TestGetVideoEncoderConfigurationOptionsOmitsJPEGWhenEmpty(t *testing.T) {
	t.Parallel()
	stub := &providerStub{
		options: VideoEncoderConfigurationOptions{
			QualityRange: IntRange{Min: 1, Max: 5},
			H264:         H264Options{ResolutionsAvailable: []ResolutionOptions{{Width: 1280, Height: 720}}},
		},
	}
	h := NewHandler(stub, WithAuthHook(authStub{}))

	got, err := h.handleGetVideoEncoderConfigurationOptions(context.Background(),
		[]byte("<req/>"))
	if err != nil {
		t.Fatalf("handleGetVideoEncoderConfigurationOptions: %v", err)
	}
	if strings.Contains(string(got), "<tt:JPEG>") {
		t.Errorf("response should omit tt:JPEG when ResolutionsAvailable is empty; got: %s", got)
	}
}

// TestGetGuaranteedNumberOfVideoEncoderInstancesIncludesJPEG verifies the
// envelope advertises a JPEG instance count alongside H264 — Profile S
// clients consult both.
func TestGetGuaranteedNumberOfVideoEncoderInstancesIncludesJPEG(t *testing.T) {
	t.Parallel()
	stub := &providerStub{guarN: 2}
	h := NewHandler(stub, WithAuthHook(authStub{}))

	got, err := h.handleGetGuaranteedNumberOfVideoEncoderInstances(
		context.Background(), []byte("<req/>"))
	if err != nil {
		t.Fatalf("handleGetGuaranteedNumberOfVideoEncoderInstances: %v", err)
	}
	s := string(got)
	if !strings.Contains(s, "<JPEG>2</JPEG>") {
		t.Errorf("response missing <JPEG>2</JPEG>: %s", s)
	}
	if !strings.Contains(s, "<H264>2</H264>") {
		t.Errorf("response missing <H264>2</H264>: %s", s)
	}
}
