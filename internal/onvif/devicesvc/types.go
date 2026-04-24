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

// DiscoveryInfo is returned by GetDiscoveryMode and consumed by SetDiscoveryMode.
type DiscoveryInfo struct {
	// DiscoveryMode is "Discoverable" or "NonDiscoverable".
	DiscoveryMode string
}

// ScopeEntry represents one scope item returned by GetScopes.
type ScopeEntry struct {
	// ScopeDef is "Fixed" or "Configurable".
	ScopeDef string
	// ScopeItem is the absolute URI, e.g. "onvif://www.onvif.org/name/…".
	ScopeItem string
}

// HostnameInfo is returned by GetHostname.
type HostnameInfo struct {
	// FromDHCP indicates whether the hostname was obtained via DHCP.
	FromDHCP bool
	// Name is the current hostname. May be empty if not configured.
	Name string
}

// DNSInfo is returned by GetDNS.
type DNSInfo struct {
	FromDHCP     bool
	SearchDomain []string
	// DNSManual holds manually configured DNS server addresses.
	DNSManual []string
}

// NetworkInterfaceInfo represents one network interface returned by
// GetNetworkInterfaces.
type NetworkInterfaceInfo struct {
	Token   string
	Enabled bool
	// HwAddress is the MAC address, e.g. "aa:bb:cc:dd:ee:ff".
	HwAddress string
	// MTU is the interface MTU. 0 means not reported.
	MTU int
	// IPv4 holds IPv4 configuration. Nil if not configured.
	IPv4 *IPv4Config
}

// IPv4Config holds the IPv4 configuration for one interface.
type IPv4Config struct {
	Enabled bool
	DHCP    bool
	// Manual holds manually assigned addresses (CIDR notation, e.g. "192.168.1.10/24").
	Manual []string
}

// NetworkProtocol mirrors the ONVIF NetworkProtocol type for one protocol entry.
type NetworkProtocol struct {
	Name    string
	Enabled bool
	Port    []int
}

// DefaultGatewayInfo is returned by GetNetworkDefaultGateway.
type DefaultGatewayInfo struct {
	IPv4Address []string
	IPv6Address []string
}

// SystemDateAndTimeInfo is returned by GetSystemDateAndTime.
type SystemDateAndTimeInfo struct {
	// DateTimeType is "Manual" or "NTP".
	DateTimeType    string
	DaylightSavings bool
	// TZ is the POSIX timezone string, e.g. "UTC".
	TZ string
	// UTCDateTime is the current UTC time. Zero value is acceptable for
	// implementations that defer to the host clock.
	UTCDateTime SystemDateTime
}

// SystemDateTime carries year/month/day and hour/minute/second.
type SystemDateTime struct {
	Year, Month, Day     int
	Hour, Minute, Second int
}

// SetSystemDateAndTimeParams carries the parameters for SetSystemDateAndTime.
type SetSystemDateAndTimeParams struct {
	DateTimeType    string
	DaylightSavings bool
	TZ              string
	UTCDateTime     SystemDateTime
}

// UserInfo represents one ONVIF user entry.
type UserInfo struct {
	Username  string
	Password  string
	UserLevel string
}

// Provider supplies operation data for a Device Service.
type Provider interface {
	// --- Identity / capabilities ---
	DeviceInfo(ctx context.Context) (DeviceInfo, error)
	Services(ctx context.Context, includeCapability bool) ([]ServiceDescriptor, error)
	GetServiceCapabilities(ctx context.Context) (DeviceServiceCapabilities, error)
	GetCapabilities(ctx context.Context, category string) (CapabilitySet, error)
	WsdlURL(ctx context.Context) (string, error)

	// --- Discovery (§7.3) ---
	GetDiscoveryMode(ctx context.Context) (DiscoveryInfo, error)
	SetDiscoveryMode(ctx context.Context, mode string) error
	GetScopes(ctx context.Context) ([]ScopeEntry, error)
	SetScopes(ctx context.Context, scopes []string) error
	AddScopes(ctx context.Context, scopes []string) error
	RemoveScopes(ctx context.Context, scopes []string) ([]string, error)

	// --- Network configuration (§7.4) ---
	GetHostname(ctx context.Context) (HostnameInfo, error)
	SetHostname(ctx context.Context, name string) error
	GetDNS(ctx context.Context) (DNSInfo, error)
	SetDNS(ctx context.Context, info DNSInfo) error
	GetNetworkInterfaces(ctx context.Context) ([]NetworkInterfaceInfo, error)
	GetNetworkProtocols(ctx context.Context) ([]NetworkProtocol, error)
	SetNetworkProtocols(ctx context.Context, protocols []NetworkProtocol) error
	GetNetworkDefaultGateway(ctx context.Context) (DefaultGatewayInfo, error)
	SetNetworkDefaultGateway(ctx context.Context, info DefaultGatewayInfo) error

	// --- System (§7.5) ---
	GetSystemDateAndTime(ctx context.Context) (SystemDateAndTimeInfo, error)
	SetSystemDateAndTime(ctx context.Context, params SetSystemDateAndTimeParams) error
	SetSystemFactoryDefault(ctx context.Context, factoryDefault string) error
	SystemReboot(ctx context.Context) (string, error)

	// --- User handling (§7.6) ---
	GetUsers(ctx context.Context) ([]UserInfo, error)
	CreateUsers(ctx context.Context, users []UserInfo) error
	SetUser(ctx context.Context, users []UserInfo) error
	DeleteUsers(ctx context.Context, usernames []string) error
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
