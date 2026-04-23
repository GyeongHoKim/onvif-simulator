package mediasvc

import (
	"context"
	"net/http"
)

const (
	// MediaServicePath is the ONVIF media service endpoint path advertised
	// via Device.GetCapabilities.Media.XAddr.
	MediaServicePath = "/onvif/media_service"

	// MediaNamespace is the ONVIF media WSDL namespace (ver10).
	MediaNamespace = "http://www.onvif.org/ver10/media/wsdl"

	// SchemaNamespace is the ONVIF schema namespace used by Media payload
	// fragments (resolution, bounds, H264Configuration, etc.).
	SchemaNamespace = "http://www.onvif.org/ver10/schema"
)

// Resolution is a width×height pair used by VideoSource and VideoEncoder.
type Resolution struct {
	Width  int
	Height int
}

// Rectangle is an (x, y, width, height) viewport used by VideoSourceConfiguration.
type Rectangle struct {
	X, Y, Width, Height int
}

// VideoSource is a physical or virtual video input.
type VideoSource struct {
	Token      string
	Framerate  int
	Resolution Resolution
}

// VideoSourceConfiguration binds a VideoSource token to a Bounds rectangle.
type VideoSourceConfiguration struct {
	Token       string
	Name        string
	UseCount    int
	SourceToken string
	Bounds      Rectangle
}

// H264Configuration is the H.264-specific encoder parameter set.
type H264Configuration struct {
	GOVLength   int
	H264Profile string // Baseline | Main | Extended | High
}

// VideoRateControl is the shared rate-control parameter set.
type VideoRateControl struct {
	FrameRateLimit   int
	EncodingInterval int
	BitrateLimit     int
}

// VideoEncoderConfiguration describes the encoder used by a profile.
type VideoEncoderConfiguration struct {
	Token          string
	Name           string
	UseCount       int
	Encoding       string // JPEG | H264 | MPEG4
	Resolution     Resolution
	Quality        float64
	RateControl    VideoRateControl
	H264           H264Configuration
	SessionTimeout string // ISO 8601 duration (e.g. PT0S)
}

// Profile is an aggregated ONVIF media profile.
type Profile struct {
	Token        string
	Name         string
	Fixed        bool
	VideoSource  *VideoSourceConfiguration
	VideoEncoder *VideoEncoderConfiguration
}

// StreamSetup is the subset of the WSDL StreamSetup element that the
// simulator inspects to validate the request. The URI it returns is a
// verbatim copy of the configured pass-through URL regardless of Stream
// or Transport.
type StreamSetup struct {
	Stream    string // RTP-Unicast | RTP-Multicast
	Transport string // UDP | TCP | RTSP | HTTP
}

// MediaURI is the response payload of GetStreamUri / GetSnapshotUri.
type MediaURI struct {
	URI                 string
	InvalidAfterConnect bool
	InvalidAfterReboot  bool
	Timeout             string // ISO 8601 duration
}

// ResolutionOptions describes one advertised (width, height) pair in
// GetVideoEncoderConfigurationOptions.
type ResolutionOptions struct {
	Width, Height int
}

// IntRange is an inclusive numeric range used by *ConfigurationOptions.
type IntRange struct {
	Min, Max int
}

// VideoSourceConfigurationOptions advertises the valid Bounds area a
// client may assign via SetVideoSourceConfiguration.
type VideoSourceConfigurationOptions struct {
	MaximumNumberOfProfiles int
	BoundsRange             struct {
		XRange      IntRange
		YRange      IntRange
		WidthRange  IntRange
		HeightRange IntRange
	}
	VideoSourceTokensAvailable []string
}

// H264Options advertises the allowed H.264 values for the encoder.
type H264Options struct {
	ResolutionsAvailable  []ResolutionOptions
	GovLengthRange        IntRange
	FrameRateRange        IntRange
	EncodingIntervalRange IntRange
	H264ProfilesSupported []string
}

// VideoEncoderConfigurationOptions advertises encoder capabilities.
// The Media service exposes one entry per supported codec; the simulator
// currently advertises H.264 only.
type VideoEncoderConfigurationOptions struct {
	QualityRange IntRange
	H264         H264Options
}

// ServiceCapabilities is the GetServiceCapabilities payload for mediasvc.
// The simulator advertises conservative defaults that match its pass-through
// model: no RTP multicast, no audio, no metadata, no OSD.
type ServiceCapabilities struct {
	SnapshotURI             bool
	Rotation                bool
	VideoSourceMode         bool
	OSD                     bool
	TemporaryOSDText        bool
	EXICompression          bool
	RTPMulticast            bool
	RTPTCP                  bool // maps to the WSDL's RTP_TCP attribute
	RTPRTSPTCP              bool // maps to the WSDL's RTP_RTSP_TCP attribute
	NonAggregateControl     bool
	NoRTSPStreaming         bool
	MaximumNumberOfProfiles int
}

// Provider supplies operation data to the mediasvc Handler. Write
// operations must delegate to internal/config mutation helpers so the
// on-disk config remains the single source of truth.
type Provider interface {
	ServiceCapabilities(ctx context.Context) (ServiceCapabilities, error)

	Profiles(ctx context.Context) ([]Profile, error)
	Profile(ctx context.Context, token string) (Profile, error)
	CreateProfile(ctx context.Context, name, token string) (Profile, error)
	DeleteProfile(ctx context.Context, token string) error

	VideoSources(ctx context.Context) ([]VideoSource, error)
	VideoSourceConfigurations(ctx context.Context) ([]VideoSourceConfiguration, error)
	VideoSourceConfiguration(ctx context.Context, token string) (VideoSourceConfiguration, error)
	SetVideoSourceConfiguration(ctx context.Context, cfg VideoSourceConfiguration) error
	AddVideoSourceConfiguration(ctx context.Context, profileToken, configToken string) error
	RemoveVideoSourceConfiguration(ctx context.Context, profileToken string) error
	CompatibleVideoSourceConfigurations(
		ctx context.Context, profileToken string,
	) ([]VideoSourceConfiguration, error)
	VideoSourceConfigurationOptions(
		ctx context.Context, configToken, profileToken string,
	) (VideoSourceConfigurationOptions, error)

	VideoEncoderConfigurations(ctx context.Context) ([]VideoEncoderConfiguration, error)
	VideoEncoderConfiguration(ctx context.Context, token string) (VideoEncoderConfiguration, error)
	SetVideoEncoderConfiguration(ctx context.Context, cfg VideoEncoderConfiguration) error
	AddVideoEncoderConfiguration(ctx context.Context, profileToken, configToken string) error
	RemoveVideoEncoderConfiguration(ctx context.Context, profileToken string) error
	CompatibleVideoEncoderConfigurations(
		ctx context.Context, profileToken string,
	) ([]VideoEncoderConfiguration, error)
	VideoEncoderConfigurationOptions(
		ctx context.Context, configToken, profileToken string,
	) (VideoEncoderConfigurationOptions, error)

	StreamURI(ctx context.Context, profileToken string, setup StreamSetup) (MediaURI, error)
	SnapshotURI(ctx context.Context, profileToken string) (MediaURI, error)
}

// AuthHook authorizes one request before the operation is executed.
type AuthHook interface {
	Authorize(ctx context.Context, operation string, r *http.Request) error
}

// AuthFunc adapts a function into an AuthHook.
type AuthFunc func(ctx context.Context, operation string, r *http.Request) error

// Authorize executes the auth function.
func (f AuthFunc) Authorize(ctx context.Context, operation string, r *http.Request) error {
	return f(ctx, operation, r)
}
