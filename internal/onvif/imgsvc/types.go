package imgsvc

import (
	"context"
	"errors"
	"net/http"
)

const (
	// ImagingServicePath is the ONVIF Imaging service endpoint path.
	ImagingServicePath = "/onvif/imaging_service"

	// ImagingNamespace is the ONVIF Imaging WSDL namespace (ver20).
	ImagingNamespace = "http://www.onvif.org/ver20/imaging/wsdl"

	// SchemaNamespace is the ONVIF schema namespace.
	SchemaNamespace = "http://www.onvif.org/ver10/schema"
)

// Sentinel errors returned by Provider implementations.
var (
	ErrInvalidArgs      = errors.New("imgsvc: invalid argument value")
	ErrSourceNotFound   = errors.New("imgsvc: video source not found")
	ErrPresetNotFound   = errors.New("imgsvc: imaging preset not found")
)

// ---------- Service Capabilities ----------

// ServiceCapabilities is the GetServiceCapabilities response payload.
type ServiceCapabilities struct {
	ImageStabilization bool
}

// ---------- Imaging Settings ----------

// Settings holds the imaging configuration for a video source.
type Settings struct {
	Brightness            *float64
	Contrast              *float64
	Sharpness             *float64
	ExposureMode          *string  // "AUTO" | "MANUAL"
	ExposurePriority      *string  // "LowNoise" | "FrameRate"
	WhiteBalanceMode      *string  // "AUTO" | "MANUAL"
	IrCutFilter           *string  // "ON" | "OFF" | "AUTO"
	BacklightCompMode     *string  // "OFF" | "ON"
	BacklightCompLevel    *float64
	WideDynamicRangeMode  *string  // "OFF" | "ON"
	WideDynamicRangeLevel *float64
	FocusMode             *string  // "AUTO" | "MANUAL"
}

// ---------- Imaging Options ----------

// Options advertises the valid ranges for imaging parameters.
type Options struct {
	Brightness            FloatRange
	Contrast              FloatRange
	Sharpness             FloatRange
	ExposureModes         []string
	ExposurePriorities    []string
	WhiteBalanceModes     []string
	IrCutFilterModes      []string
	BacklightCompModes    []string
	WideDynamicRangeModes []string
	FocusModes            []string
}

// FloatRange is an inclusive numeric range for float values.
type FloatRange struct {
	Min float64
	Max float64
}

// ---------- Imaging Status ----------

// Status represents the current imaging status.
type Status struct {
	FocusStatus *FocusStatus
}

// FocusStatus indicates the current focus state.
type FocusStatus struct {
	Position string // "MOVING" | "IDLE" | "ERROR"
	Error    string
}

// ---------- Imaging Presets ----------

// Preset is an imaging preset (e.g. ClearWeather, Night, etc.).
type Preset struct {
	Token string
	Name  string
	Type  string // ImagingPresetType: "ClearWeather", "Night", etc.
}

// ---------- Provider interface ----------

// Provider supplies imaging operation data to the Handler.
type Provider interface {
	ServiceCapabilities(ctx context.Context) (ServiceCapabilities, error)
	GetImagingSettings(ctx context.Context, videoSourceToken string) (Settings, error)
	SetImagingSettings(ctx context.Context, videoSourceToken string, settings Settings, forcePersistence *bool) error
	GetOptions(ctx context.Context, videoSourceToken string) (Options, error)
	GetStatus(ctx context.Context, videoSourceToken string) (Status, error)
	GetPresets(ctx context.Context, videoSourceToken string) ([]Preset, error)
	GetCurrentPreset(ctx context.Context, videoSourceToken string) (*Preset, error)
	SetCurrentPreset(ctx context.Context, videoSourceToken string, presetToken string) error
}

// AuthHook authorizes an Imaging operation.
type AuthHook interface {
	Authorize(ctx context.Context, operation string, r *http.Request) error
}

// AuthFunc is a function adapter for AuthHook.
type AuthFunc func(ctx context.Context, operation string, r *http.Request) error

func (f AuthFunc) Authorize(ctx context.Context, operation string, r *http.Request) error {
	return f(ctx, operation, r)
}
