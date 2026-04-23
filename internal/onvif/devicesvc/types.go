package devicesvc

import (
	"context"
	"net/http"
)

const (
	// DeviceServicePath is the ONVIF device management endpoint path.
	DeviceServicePath = "/onvif/device_service"
	// MediaServicePath is advertised via GetCapabilities.
	MediaServicePath = "/onvif/media_service"

	// DeviceNamespace is the ONVIF device management namespace.
	DeviceNamespace = "http://www.onvif.org/ver10/device/wsdl"
	// MediaNamespace is the ONVIF media namespace used in GetCapabilities.
	MediaNamespace = "http://www.onvif.org/ver10/media/wsdl"
)

// Version reports an ONVIF service version.
type Version struct {
	Major int
	Minor int
}

// DeviceInfo is returned by GetDeviceInformation.
type DeviceInfo struct {
	Manufacturer string
	Model        string
	Firmware     string
	Serial       string
	HardwareID   string
}

// ServiceDescriptor advertises one ONVIF service endpoint.
type ServiceDescriptor struct {
	Namespace string
	XAddr     string
	Version   Version
}

// NetworkCapabilities is returned by GetServiceCapabilities.
type NetworkCapabilities struct {
	IPFilter           bool
	ZeroConfiguration  bool
	IPVersion6         bool
	DynDNS             bool
	Dot11Configuration bool
	HostnameFromDHCP   bool
	NTP                int
}

// SecurityCapabilities is returned by GetServiceCapabilities.
type SecurityCapabilities struct {
	UsernameToken bool
	HTTPDigest    bool
	JSONWebToken  bool
}

// SystemCapabilities is returned by GetServiceCapabilities.
type SystemCapabilities struct {
	DiscoveryResolve  bool
	DiscoveryBye      bool
	HTTPSystemLogging bool
}

// DeviceServiceCapabilities is the device service capability payload.
type DeviceServiceCapabilities struct {
	Network  NetworkCapabilities
	Security SecurityCapabilities
	System   SystemCapabilities
}

// CoreNetworkCapabilities is returned via GetCapabilities.Device.Network.
type CoreNetworkCapabilities struct {
	IPFilter          bool
	ZeroConfiguration bool
	IPVersion6        bool
	DynDNS            bool
}

// CoreSecurityCapabilities is returned via GetCapabilities.Device.Security.
type CoreSecurityCapabilities struct {
	UsernameToken bool
	HTTPDigest    bool
	JSONWebToken  bool
}

// CoreSystemCapabilities is returned via GetCapabilities.Device.System.
type CoreSystemCapabilities struct {
	DiscoveryResolve  bool
	DiscoveryBye      bool
	SupportedVersions []Version
}

// DeviceCapability is the device capability section in GetCapabilities.
type DeviceCapability struct {
	XAddr    string
	Network  CoreNetworkCapabilities
	System   CoreSystemCapabilities
	Security CoreSecurityCapabilities
}

// ServiceCapability is the minimal XAddr-based capability section.
type ServiceCapability struct {
	XAddr string
}

// CapabilitySet is the GetCapabilities result set.
type CapabilitySet struct {
	Device  DeviceCapability
	Media   ServiceCapability
	Events  ServiceCapability
	PTZ     ServiceCapability
	Imaging ServiceCapability
}

// Provider supplies operation data for a Device Service.
type Provider interface {
	DeviceInfo(ctx context.Context) (DeviceInfo, error)
	Services(ctx context.Context, includeCapability bool) ([]ServiceDescriptor, error)
	GetServiceCapabilities(ctx context.Context) (DeviceServiceCapabilities, error)
	GetCapabilities(ctx context.Context, category string) (CapabilitySet, error)
	WsdlURL(ctx context.Context) (string, error)
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
