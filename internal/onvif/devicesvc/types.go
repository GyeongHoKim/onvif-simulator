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

// Provider supplies operation data for the ONVIF Device Management Service.
// It covers the full ONVIF Profile S Device Management operation set
// (ONVIF Device Management Service Spec §7.3–§7.6).
//
// Write operations that affect persistent state must delegate to config
// mutation helpers (config.SetDiscoveryMode, config.SetHostname, etc.) so
// the on-disk config remains the single source of truth; the provider
// implementation must not update its own fields directly.
//
// All methods must be safe for concurrent use.
type Provider interface {
	// DeviceInfo returns static device identity (manufacturer, model, serial…).
	// Maps to GetDeviceInformation.
	DeviceInfo(ctx context.Context) (DeviceInfo, error)

	// Services returns the list of ONVIF service endpoints.
	// includeCapability=true requests capability details for each service endpoint.
	// Maps to GetServices.
	Services(ctx context.Context, includeCapability bool) ([]ServiceDescriptor, error)

	// GetServiceCapabilities returns the Device Service capability flags.
	// Maps to GetServiceCapabilities.
	GetServiceCapabilities(ctx context.Context) (DeviceServiceCapabilities, error)

	// GetCapabilities returns the combined capability block.
	// category is one of "All", "Device", "Media", "Events", "Imaging", "PTZ".
	// Maps to GetCapabilities.
	GetCapabilities(ctx context.Context, category string) (CapabilitySet, error)

	// WsdlURL returns the URL of the device WSDL.
	// Maps to GetWsdlUrl.
	WsdlURL(ctx context.Context) (string, error)

	// --- Discovery (§7.3) ---

	// GetDiscoveryMode returns the current WS-Discovery mode.
	GetDiscoveryMode(ctx context.Context) (DiscoveryInfo, error)
	// SetDiscoveryMode changes the WS-Discovery mode to "Discoverable" or
	// "NonDiscoverable" and persists via config.SetDiscoveryMode.
	SetDiscoveryMode(ctx context.Context, mode string) error
	// GetScopes returns the list of ONVIF scope URIs.
	GetScopes(ctx context.Context) ([]ScopeEntry, error)
	// SetScopes replaces the configurable scope URIs.
	SetScopes(ctx context.Context, scopes []string) error
	// AddScopes appends scope URIs.
	AddScopes(ctx context.Context, scopes []string) error
	// RemoveScopes deletes scope URIs; returns the removed entries.
	RemoveScopes(ctx context.Context, scopes []string) ([]string, error)

	// --- Network configuration (§7.4) ---

	// GetHostname returns the current hostname and whether it came from DHCP.
	GetHostname(ctx context.Context) (HostnameInfo, error)
	// SetHostname changes the hostname; persists via config.SetHostname.
	SetHostname(ctx context.Context, name string) error
	// GetDNS returns the current DNS configuration.
	GetDNS(ctx context.Context) (DNSInfo, error)
	// SetDNS updates the DNS configuration; persists via config.SetDNS.
	SetDNS(ctx context.Context, info DNSInfo) error
	// GetNetworkInterfaces returns the list of network interfaces.
	// The simulator returns a single virtual interface derived from the config.
	GetNetworkInterfaces(ctx context.Context) ([]NetworkInterfaceInfo, error)
	// GetNetworkProtocols returns the enabled/disabled status of HTTP, HTTPS, RTSP.
	GetNetworkProtocols(ctx context.Context) ([]NetworkProtocol, error)
	// SetNetworkProtocols updates the protocol list; persists via config.SetNetworkProtocols.
	SetNetworkProtocols(ctx context.Context, protocols []NetworkProtocol) error
	// GetNetworkDefaultGateway returns the configured default gateway addresses.
	GetNetworkDefaultGateway(ctx context.Context) (DefaultGatewayInfo, error)
	// SetNetworkDefaultGateway updates the default gateway; persists via config.SetDefaultGateway.
	SetNetworkDefaultGateway(ctx context.Context, info DefaultGatewayInfo) error

	// --- System (§7.5) ---

	// GetSystemDateAndTime returns the device clock configuration.
	GetSystemDateAndTime(ctx context.Context) (SystemDateAndTimeInfo, error)
	// SetSystemDateAndTime updates the clock configuration; persists via config.SetSystemDateAndTime.
	SetSystemDateAndTime(ctx context.Context, params SetSystemDateAndTimeParams) error
	// SetSystemFactoryDefault resets the device; factoryDefault is "Hard" or "Soft".
	SetSystemFactoryDefault(ctx context.Context, factoryDefault string) error
	// SystemReboot triggers a device reboot; returns the delay message.
	SystemReboot(ctx context.Context) (string, error)

	// --- User handling (§7.6) ---

	// GetUsers returns the list of ONVIF users.
	GetUsers(ctx context.Context) ([]UserInfo, error)
	// CreateUsers adds new users.
	CreateUsers(ctx context.Context, users []UserInfo) error
	// SetUser updates existing user attributes.
	SetUser(ctx context.Context, users []UserInfo) error
	// DeleteUsers removes users by username.
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
