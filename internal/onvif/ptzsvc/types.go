package ptzsvc

import (
	"context"
	"errors"
	"net/http"
)

// Sentinel errors returned by Provider implementations.
var (
	ErrPresetNotFound = errors.New("ptzsvc: preset not found")
	ErrInvalidArgs    = errors.New("ptzsvc: invalid argument value")
)

const (
	// PTZServicePath is the ONVIF PTZ service endpoint path advertised
	// via Device.GetCapabilities.PTZ.XAddr.
	PTZServicePath = "/onvif/ptz_service"

	// PTZNamespace is the ONVIF PTZ WSDL namespace (ver20).
	PTZNamespace = "http://www.onvif.org/ver20/ptz/wsdl"

	// SchemaNamespace is the ONVIF schema namespace shared with mediasvc.
	SchemaNamespace = "http://www.onvif.org/ver10/schema"
)

// ---------- Service Capabilities ----------

// ServiceCapabilities is the GetServiceCapabilities response payload.
type ServiceCapabilities struct {
	EFlip                       bool
	Reverse                     bool
	GetCompatibleConfigurations bool
	MoveStatus                  bool
	StatusPosition              bool
}

// ---------- PTZ Node ----------

// PTZNode describes a PTZ driver on the device.
type PTZNode struct {
	Token string
	Name  string
	// SupportedPTZSpaces lists the coordinate spaces this node supports.
	SupportedPTZSpaces map[string]Space
}

// Space defines a PTZ coordinate space with its range.
type Space struct {
	URI    string
	XRange *IntRange
	YRange *IntRange
}

// IntRange is an inclusive numeric range.
type IntRange struct {
	Min float64
	Max float64
}

// ---------- PTZ Configuration ----------

// PTZConfiguration binds a PTZ node to a media profile and defines
// default coordinate spaces and speed settings.
type PTZConfiguration struct {
	Token     string
	Name      string
	UseCount  int
	NodeToken string

	DefaultAbsolutePantTiltPositionSpace   string
	DefaultAbsoluteZoomPositionSpace       string
	DefaultRelativePanTiltTranslationSpace string
	DefaultRelativeZoomTranslationSpace    string
	DefaultContinuousPanTiltVelocitySpace  string
	DefaultContinuousZoomVelocitySpace     string

	PanTiltLimits *PanTiltLimits
	ZoomLimits    *ZoomLimits
}

// PanTiltLimits defines the allowed pan/tilt range.
type PanTiltLimits struct {
	Range Range
}

// ZoomLimits defines the allowed zoom range.
type ZoomLimits struct {
	Range Range
}

// Range is a two-axis range used by limits.
type Range struct {
	XRange IntRange
	YRange IntRange
}

// ---------- PTZ Vector / Speed ----------

// Vector is a PTZ position or translation vector.
type Vector struct {
	PanTilt *PanTilt
	Zoom    *Zoom
}

// PanTilt represents pan and tilt coordinates.
type PanTilt struct {
	X float64 `xml:"x,attr"`
	Y float64 `xml:"y,attr"`
}

// Zoom represents zoom level.
type Zoom struct {
	X float64 `xml:"x,attr"`
}

// Speed represents PTZ movement speed.
type Speed struct {
	PanTilt *PanTilt
	Zoom    *Zoom
}

// ---------- PTZ Status ----------

// Status represents the current PTZ position and movement state.
type Status struct {
	Position   *Vector
	MoveStatus *MoveStatus
}

// MoveStatus indicates whether pan/tilt and zoom are currently moving.
type MoveStatus struct {
	PanTilt string // "IDLE" | "MOVING"
	Zoom    string // "IDLE" | "MOVING"
}

// ---------- PTZ Preset ----------

// Preset is a saved PTZ position.
type Preset struct {
	Token    string
	Name     string
	Position *Vector
}

// ---------- PTZ Configuration Options ----------

// ConfigurationOptions advertises the valid ranges for PTZ parameters.
type ConfigurationOptions struct {
	PanTiltPositionSpaceRange    []SpaceRange
	ZoomPositionSpaceRange       []SpaceRange
	PanTiltTranslationSpaceRange []SpaceRange
	ZoomTranslationSpaceRange    []SpaceRange
	PanTiltVelocitySpaceRange    []SpaceRange
	ZoomVelocitySpaceRange       []SpaceRange
	PTZTimeout                   IntRange
}

// SpaceRange describes the valid range for a specific space.
type SpaceRange struct {
	URI    string
	XRange IntRange
	YRange IntRange
}

// ---------- Provider interface ----------

// Provider supplies PTZ operation data to the Handler.
type Provider interface {
	ServiceCapabilities(ctx context.Context) (ServiceCapabilities, error)

	GetNodes(ctx context.Context) ([]PTZNode, error)
	GetNode(ctx context.Context, nodeToken string) (PTZNode, error)
	GetConfigurations(ctx context.Context) ([]PTZConfiguration, error)
	GetConfiguration(ctx context.Context, configToken string) (PTZConfiguration, error)
	GetConfigurationOptions(ctx context.Context, configToken string) (ConfigurationOptions, error)

	GetStatus(ctx context.Context, profileToken string) (Status, error)
	ContinuousMove(ctx context.Context, profileToken string, velocity Speed, timeout *string) error
	AbsoluteMove(ctx context.Context, profileToken string, position Vector, speed *Speed) error
	RelativeMove(ctx context.Context, profileToken string, translation Vector, speed *Speed) error
	Stop(ctx context.Context, profileToken string, panTilt *bool, zoom *bool) error

	GetPresets(ctx context.Context, profileToken string) ([]Preset, error)
	SetPreset(ctx context.Context, profileToken string, presetName *string, presetToken *string) (string, error)
	RemovePreset(ctx context.Context, profileToken string, presetToken string) error
	GotoPreset(ctx context.Context, profileToken string, presetToken string, speed *Speed) error
	GotoHomePosition(ctx context.Context, profileToken string, speed *Speed) error
	SetHomePosition(ctx context.Context, profileToken string) error
}

// AuthHook authorizes a PTZ operation. It returns a *auth.ChallengeError
// when the caller is not permitted.
type AuthHook interface {
	Authorize(ctx context.Context, operation string, r *http.Request) error
}

// AuthFunc is a function adapter for AuthHook.
type AuthFunc func(ctx context.Context, operation string, r *http.Request) error

// Authorize delegates to the underlying function.
func (f AuthFunc) Authorize(ctx context.Context, operation string, r *http.Request) error {
	return f(ctx, operation, r)
}
