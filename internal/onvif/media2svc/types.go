package media2svc

import (
	"context"
	"errors"
	"net/http"
)

const (
	// Media2ServicePath is the ONVIF Media2 service endpoint path.
	Media2ServicePath = "/onvif/media2_service"

	// Media2Namespace is the ONVIF Media2 WSDL namespace.
	Media2Namespace = "http://www.onvif.org/ver20/media/wsdl"

	// SchemaNamespace is the ONVIF schema namespace.
	SchemaNamespace = "http://www.onvif.org/ver10/schema"
)

// Sentinel errors.
var (
	ErrProviderRequired = errors.New("media2svc: provider is required")
	ErrInvalidArgs      = errors.New("media2svc: invalid argument value")
)

// ---------- Service Capabilities ----------

// ServiceCapabilities is the GetServiceCapabilities response payload.
type ServiceCapabilities struct {
	H264                    bool
	H265                    bool
	JPEG                    bool
	SnapshotURI             bool
	Rotation                bool
	VideoSourceMode         bool
	OSD                     bool
	TemporaryOSDText        bool
	MaximumNumberOfProfiles int
	RTPMulticast            bool
	RTPTCP                  bool
	RTPRTSPTCP              bool
	NonAggregateControl     bool
	NoRTSPStreaming         bool
}

// ---------- Profile ----------

// Profile represents a Media2 profile.
type Profile struct {
	Token                            string
	Name                             string
	VideoSourceConfigurations        []VideoSourceConfiguration
	AudioSourceConfigurations        []AudioSourceConfiguration
	VideoEncoderConfigurations       []VideoEncoderConfiguration
	AudioEncoderConfigurations       []AudioEncoderConfiguration
	PTZConfigurationToken            string
	VideoAnalyticsConfigurationToken string
	MetadataConfigurationToken       string
}

// ---------- Video Source Configuration ----------

// VideoSourceConfiguration describes a video source input.
type VideoSourceConfiguration struct {
	Token            string
	Name             string
	UseCount         int
	VideoSourceToken string
	Bounds           *Bounds
}

// Bounds defines a rectangular region.
type Bounds struct {
	X      int
	Y      int
	Width  int
	Height int
}

// ---------- Audio Source Configuration ----------

// AudioSourceConfiguration describes an audio source input.
type AudioSourceConfiguration struct {
	Token            string
	Name             string
	UseCount         int
	AudioSourceToken string
}

// ---------- Video Encoder Configuration ----------

// VideoEncoderConfiguration describes a video encoder.
type VideoEncoderConfiguration struct {
	Token     string
	Name      string
	UseCount  int
	Encoding  string // "H264" | "H265" | "JPEG"
	Width     int
	Height    int
	FrameRate int
	Bitrate   int
	Quality   float64
}

// ---------- Audio Encoder Configuration ----------

// AudioEncoderConfiguration describes an audio encoder.
type AudioEncoderConfiguration struct {
	Token      string
	Name       string
	UseCount   int
	Encoding   string
	Bitrate    int
	SampleRate int
}

// ---------- Video Encoder Configuration Options ----------

// VideoEncoderConfigurationOptions describes valid encoder settings.
type VideoEncoderConfigurationOptions struct {
	H264 *H264Options
	H265 *H265Options
	JPEG *JPEGOptions
}

// H264Options describes valid H.264 encoder settings.
type H264Options struct {
	ResolutionsAvailable []Resolution
	FrameRateRange       IntRange
	BitrateRange         IntRange
	ProfilesAvailable    []string
}

// H265Options describes valid H.265 encoder settings.
type H265Options struct {
	ResolutionsAvailable []Resolution
	FrameRateRange       IntRange
	BitrateRange         IntRange
	ProfilesAvailable    []string
}

// JPEGOptions describes valid JPEG encoder settings.
type JPEGOptions struct {
	ResolutionsAvailable []Resolution
	FrameRateRange       IntRange
}

// Resolution represents a video resolution.
type Resolution struct {
	Width  int
	Height int
}

// IntRange is an inclusive numeric range.
type IntRange struct {
	Min int
	Max int
}

// ---------- Provider interface ----------

// Provider supplies Media2 operation data to the Handler.
type Provider interface {
	ServiceCapabilities(ctx context.Context) (ServiceCapabilities, error)
	Profiles(ctx context.Context) ([]Profile, error)
	VideoEncoderConfigurations(ctx context.Context) ([]VideoEncoderConfiguration, error)
	VideoEncoderConfiguration(ctx context.Context, token string) (VideoEncoderConfiguration, error)
	VideoEncoderConfigurationOptions(ctx context.Context, token string) (VideoEncoderConfigurationOptions, error)
	SetVideoEncoderConfiguration(ctx context.Context, token string, config *VideoEncoderConfiguration) error
	StreamURI(ctx context.Context, profileToken, protocol string) (string, error)
	SnapshotURI(ctx context.Context, profileToken string) (string, error)
}

// AuthHook authorizes a Media2 operation.
type AuthHook interface {
	Authorize(ctx context.Context, operation string, r *http.Request) error
}

// AuthFunc is a function adapter for AuthHook.
type AuthFunc func(ctx context.Context, operation string, r *http.Request) error

// Authorize delegates to the underlying function.
func (f AuthFunc) Authorize(ctx context.Context, operation string, r *http.Request) error {
	return f(ctx, operation, r)
}
