// Package config manages the on-disk configuration for the ONVIF simulator.
//
// # File location
//
// The config file is named "onvif-simulator.json". Front-ends resolve its
// location at startup via DefaultPath (the OS-standard user config dir
// returned by os.UserConfigDir, joined with "onvif-simulator/") and call
// SetPath to make Load/Save/Update use it. When SetPath has not been
// called, Load and Save fall back to the working directory — kept for
// tests and ad-hoc CLI use.
//
// First-run helpers (Default, EnsureExists) create a baseline config at
// the resolved path so users can launch the GUI by double-clicking the
// .app bundle without first hand-crafting onvif-simulator.json.
//
// # Schema versioning
//
// Every config file must set "version": 1. Future breaking changes will
// increment this number; the loader rejects mismatches immediately.
//
// # Typical usage
//
// Load once at startup and pass the result to the simulator:
//
//	cfg, err := config.Load()
//
// Mutate individual fields at runtime via the targeted helpers; never
// construct or overwrite a Config value by hand:
//
//	// Persist a new media profile added by the user via GUI/TUI
//	if err := config.AddProfile(config.ProfileConfig{...}); err != nil { ... }
//
//	// Flip the discovery mode from the TUI
//	if err := config.SetDiscoveryMode("NonDiscoverable"); err != nil { ... }
//
//	// Toggle an event topic on/off
//	if err := config.SetTopicEnabled("tns1:VideoSource/MotionAlarm", true); err != nil { ... }
//
// All helpers load → mutate → validate → save atomically; concurrent calls
// are serialized by an internal mutex.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// FileName is the on-disk config file name.
const FileName = "onvif-simulator.json"

// DirName is the per-app subdirectory under os.UserConfigDir.
const DirName = "onvif-simulator"

const (
	configDirMode  os.FileMode = 0o700
	configFileMode os.FileMode = 0o600
)

// CurrentVersion is the only supported config schema version.
const CurrentVersion = 1

// Config is the on-disk JSON shape.
type Config struct {
	Version int           `json:"version"`
	Device  DeviceConfig  `json:"device"`
	Network NetworkConfig `json:"network"`
	Media   MediaConfig   `json:"media"`
	Auth    AuthConfig    `json:"auth,omitempty"`
	Events  EventsConfig  `json:"events,omitempty"`
	Runtime RuntimeConfig `json:"runtime,omitempty"`
}

// DeviceConfig describes who this device is.
type DeviceConfig struct {
	UUID         string   `json:"uuid"`
	Manufacturer string   `json:"manufacturer"`
	Model        string   `json:"model"`
	Serial       string   `json:"serial"`
	Firmware     string   `json:"firmware,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
}

// NetworkConfig describes how clients reach this device.
type NetworkConfig struct {
	HTTPPort int `json:"http_port"`
	// RTSPPort is the TCP port the embedded RTSP server listens on. 0 means
	// "use DefaultRTSPPort"; the simulator reads RTSPPortOrDefault rather
	// than the raw field so older configs keep working without migration.
	RTSPPort  int      `json:"rtsp_port,omitempty"`
	Interface string   `json:"interface,omitempty"`
	XAddrs    []string `json:"xaddrs,omitempty"`
}

// DefaultRTSPPort is the standard RTSP control port. Used when
// NetworkConfig.RTSPPort is 0.
const DefaultRTSPPort = 8554

// RTSPPortOrDefault returns the explicit RTSPPort when non-zero, otherwise
// DefaultRTSPPort. Callers should use this instead of reading the raw field.
func (n NetworkConfig) RTSPPortOrDefault() int {
	if n.RTSPPort == 0 {
		return DefaultRTSPPort
	}
	return n.RTSPPort
}

// RuntimeConfig holds simulator state that the Device Service exposes via
// Get/Set operations (discovery mode, hostname, DNS, gateway, protocols,
// system date/time). These fields are read and written at runtime through the
// ONVIF Device Management operations; they are persisted so that the simulator
// survives restarts with the last applied values.
type RuntimeConfig struct {
	DiscoveryMode     string                   `json:"discovery_mode,omitempty"`
	Hostname          string                   `json:"hostname,omitempty"`
	DNS               DNSConfig                `json:"dns,omitempty"`
	DefaultGateway    DefaultGatewayConfig     `json:"default_gateway,omitempty"`
	NetworkProtocols  []NetworkProtocol        `json:"network_protocols,omitempty"`
	NetworkInterfaces []NetworkInterfaceConfig `json:"network_interfaces,omitempty"`
	SystemDateAndTime SystemDateTimeConfig     `json:"system_date_and_time,omitempty"`
}

// NetworkInterfaceConfig mirrors the ONVIF NetworkInterface type for
// SetNetworkInterfaces / GetNetworkInterfaces persistence.
type NetworkInterfaceConfig struct {
	// Token is the interface identifier (e.g. "eth0").
	Token     string                `json:"token"`
	Enabled   bool                  `json:"enabled"`
	HwAddress string                `json:"hw_address,omitempty"`
	MTU       int                   `json:"mtu,omitempty"`
	IPv4      *NetworkInterfaceIPv4 `json:"ipv4,omitempty"`
}

// NetworkInterfaceIPv4 holds the IPv4 settings for one interface.
type NetworkInterfaceIPv4 struct {
	Enabled bool `json:"enabled"`
	DHCP    bool `json:"dhcp"`
	// Manual holds manually assigned addresses in CIDR notation (e.g. "192.168.1.10/24").
	Manual []string `json:"manual,omitempty"`
}

// DNSConfig mirrors the ONVIF DNSInformation type.
type DNSConfig struct {
	FromDHCP     bool     `json:"from_dhcp,omitempty"`
	SearchDomain []string `json:"search_domain,omitempty"`
	// DNSManual holds manually configured DNS server addresses.
	DNSManual []string `json:"dns_manual,omitempty"`
}

// DefaultGatewayConfig mirrors the ONVIF NetworkGateway type.
type DefaultGatewayConfig struct {
	IPv4Address []string `json:"ipv4_address,omitempty"`
	IPv6Address []string `json:"ipv6_address,omitempty"`
}

// NetworkProtocol mirrors the ONVIF NetworkProtocol type.
type NetworkProtocol struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Port    []int  `json:"port,omitempty"`
}

// SystemDateTimeConfig mirrors the ONVIF SystemDateTime type.
type SystemDateTimeConfig struct {
	// DateTimeType is "Manual" or "NTP".
	DateTimeType    string `json:"date_time_type,omitempty"`
	DaylightSavings bool   `json:"daylight_savings,omitempty"`
	// TZ is the POSIX timezone string, e.g. "UTC" or "KST-9".
	TZ string `json:"tz,omitempty"`
	// ManualDateTimeUTC holds the manually set UTC time when DateTimeType is
	// "Manual". Format: RFC3339, e.g. "2026-01-15T12:00:00Z". The provider
	// restores this value on startup and prefers it over the system clock when
	// DateTimeType == "Manual".
	ManualDateTimeUTC string `json:"manual_date_time_utc,omitempty"`
}

// EventsConfig configures the Event Service and the set of topics the
// simulator advertises.
type EventsConfig struct {
	// MaxPullPoints is the maximum number of concurrent pull-point
	// subscriptions. 0 means no limit is advertised (defaults to 10).
	MaxPullPoints int `json:"max_pull_points,omitempty"`
	// SubscriptionTimeout is the default duration used when a
	// CreatePullPointSubscription request omits InitialTerminationTime.
	// Accepts Go durations (e.g. "1h", "30m") or ISO 8601 PT durations
	// (e.g. "PT1H", "PT30M"). Defaults to "1h" if empty.
	SubscriptionTimeout string `json:"subscription_timeout,omitempty"`
	// Topics declares which ONVIF event topics the simulator supports.
	// Each entry controls whether the topic appears in GetEventProperties
	// and can be triggered via the EventBroker.Publish API.
	Topics []TopicConfig `json:"topics,omitempty"`
}

// TopicConfig describes one ONVIF event topic the simulator advertises.
type TopicConfig struct {
	// Name is the tns1: topic expression, e.g.
	// "tns1:VideoSource/MotionAlarm".
	Name string `json:"name"`
	// Enabled controls whether the topic appears in GetEventProperties and
	// whether EventBroker.Publish accepts it.
	Enabled bool `json:"enabled"`
}

// MediaConfig holds the list of ONVIF media profiles this device advertises.
type MediaConfig struct {
	Profiles []ProfileConfig `json:"profiles"`
	// MaxVideoEncoderInstances is returned by GetGuaranteedNumberOfVideoEncoderInstances.
	// 0 means "report 1 per profile" (safe simulator default).
	MaxVideoEncoderInstances int              `json:"max_video_encoder_instances,omitempty"`
	MetadataConfigurations   []MetadataConfig `json:"metadata_configurations,omitempty"`
}

// MetadataConfig describes one ONVIF metadata configuration entry.
// The simulator does not produce a real metadata RTP stream; these values
// are returned verbatim by the Metadata Configuration operations so that
// clients can discover and bind metadata configurations to profiles.
type MetadataConfig struct {
	Token string `json:"token"`
	Name  string `json:"name"`
	// Analytics enables the analytics module in this metadata stream.
	Analytics bool `json:"analytics,omitempty"`
	// PTZStatus enables PTZ position/move status in this metadata stream.
	PTZStatus bool `json:"ptz_status,omitempty"`
	// Events enables event notifications in this metadata stream.
	Events bool `json:"events,omitempty"`
}

// ProfileConfig describes a single ONVIF media profile.
//
// MediaFilePath is the path to a local mp4 file that the embedded RTSP
// server loops to produce this profile's stream. SnapshotURI is still
// pass-through (the simulator does not synthesize snapshots yet).
//
// Encoding, Width, Height, FPS, Bitrate, and GOPLength are auto-detected
// from the mp4 file at simulator startup and overwritten in memory; they
// are still marshaled into the on-disk config so a stopped simulator
// still has the last-known values and so the GUI's read-only badges
// (which receive them via the Wails ConfigSnapshot bridge) work.
type ProfileConfig struct {
	Name          string `json:"name"`
	Token         string `json:"token"`
	MediaFilePath string `json:"media_file_path,omitempty"`

	Encoding  string `json:"encoding,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	FPS       int    `json:"fps,omitempty"`
	Bitrate   int    `json:"bitrate,omitempty"`
	GOPLength int    `json:"gop_length,omitempty"`

	SnapshotURI      string `json:"snapshot_uri,omitempty"`
	VideoSourceToken string `json:"video_source_token,omitempty"`
}

// DefaultVideoSourceToken is used when a ProfileConfig does not specify a
// VideoSourceToken. The ONVIF Media service dedups source tokens across
// profiles when building the GetVideoSources response.
const DefaultVideoSourceToken = "VS_DEFAULT"

// AuthConfig configures authentication for all ONVIF services.
// When Enabled is true, Users must contain at least one entry
// (or JWT.Enabled must be true and have key material).
type AuthConfig struct {
	Enabled bool         `json:"enabled"`
	Users   []UserConfig `json:"users,omitempty"`
	Digest  DigestConfig `json:"digest,omitempty"`
	JWT     JWTConfig    `json:"jwt,omitempty"`
}

// UserConfig is one credential entry for HTTP Digest and WS-UsernameToken.
type UserConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// Known ONVIF user levels (per §5.9.4.2). Custom role names are allowed.
const (
	RoleAdministrator = "Administrator"
	RoleOperator      = "Operator"
	RoleUser          = "User"
	RoleExtended      = "Extended"
)

// DigestConfig tunes HTTP Digest authentication parameters.
type DigestConfig struct {
	Realm      string   `json:"realm,omitempty"`
	Algorithms []string `json:"algorithms,omitempty"`
	NonceTTL   string   `json:"nonce_ttl,omitempty"`
}

// JWTConfig tunes JWT Bearer token validation.
type JWTConfig struct {
	Enabled       bool     `json:"enabled,omitempty"`
	Issuer        string   `json:"issuer,omitempty"`
	Audience      string   `json:"audience,omitempty"`
	JWKSURL       string   `json:"jwks_url,omitempty"`
	PublicKeyPEM  []string `json:"public_key_pem,omitempty"`
	Algorithms    []string `json:"algorithms,omitempty"`
	UsernameClaim string   `json:"username_claim,omitempty"`
	RolesClaim    string   `json:"roles_claim,omitempty"`
	ClockSkew     string   `json:"clock_skew,omitempty"`
	RequireTLS    *bool    `json:"require_tls,omitempty"`
}

var (
	// ErrInvalidVersion means config.version does not match a supported schema.
	ErrInvalidVersion = errors.New("config: unsupported version")

	// ErrDeviceUUIDRequired means device.uuid is empty.
	ErrDeviceUUIDRequired = errors.New("config: device.uuid is required")
	// ErrDeviceUUIDInvalid means device.uuid is not a valid urn:uuid: URI.
	ErrDeviceUUIDInvalid = errors.New("config: device.uuid must be urn:uuid:<uuid>")

	// ErrNetworkPortInvalid means network.http_port is out of the valid range.
	ErrNetworkPortInvalid = errors.New("config: network.http_port must be between 1 and 65535")

	// ErrNetworkRTSPPortInvalid means network.rtsp_port is set but out of range.
	// 0 is allowed and falls back to DefaultRTSPPort.
	ErrNetworkRTSPPortInvalid = errors.New("config: network.rtsp_port must be 0 or between 1 and 65535")

	// ErrNetworkPortConflict means network.http_port and network.rtsp_port
	// resolve to the same TCP port; the simulator cannot bind both there.
	ErrNetworkPortConflict = errors.New("config: network.rtsp_port must differ from network.http_port")

	// ErrMediaNoProfiles means media.profiles is empty.
	ErrMediaNoProfiles = errors.New("config: media.profiles must have at least one entry")

	// ErrProfileEncodingInvalid means profile.encoding is not a supported value.
	ErrProfileEncodingInvalid = errors.New("config: profile.encoding must be one of H264, H265, MJPEG")

	// ErrAuthUsersRequired means auth.enabled=true but auth.users is empty.
	ErrAuthUsersRequired = errors.New("config: auth.users must contain at least one user when auth.enabled is true")
	// ErrAuthUserIncomplete means a user entry is missing username, password, or role.
	ErrAuthUserIncomplete = errors.New("config: auth.users entry requires username, password, and role")
	// ErrAuthRoleReserved means a custom role uses the reserved onvif: prefix.
	ErrAuthRoleReserved = errors.New("config: auth.users role must not start with onvif: prefix")
	// ErrAuthRoleWhitespace means a custom role name contains whitespace (forbidden per §5.9.4.5).
	ErrAuthRoleWhitespace = errors.New("config: auth.users role must not contain whitespace")
	// ErrAuthUsernameDuplicate means two user entries share the same username.
	ErrAuthUsernameDuplicate = errors.New("config: auth.users username must be unique")
	// ErrAuthDigestAlgorithm means digest.algorithms contains an unsupported entry.
	ErrAuthDigestAlgorithm = errors.New("config: auth.digest.algorithms must be a subset of [MD5, SHA-256]")
	// ErrAuthDigestNonceTTL means digest.nonce_ttl is not a valid Go duration.
	ErrAuthDigestNonceTTL = errors.New("config: auth.digest.nonce_ttl must be a Go duration (e.g. 5m)")
	// ErrAuthJWTAlgorithm means jwt.algorithms contains an unsupported entry.
	ErrAuthJWTAlgorithm = errors.New(
		"config: auth.jwt.algorithms must be a subset of [RS256, RS384, RS512, ES256, ES384, ES512]")
	// ErrAuthJWTClockSkew means jwt.clock_skew is not a valid Go duration.
	ErrAuthJWTClockSkew = errors.New("config: auth.jwt.clock_skew must be a Go duration (e.g. 30s)")
	// ErrAuthJWTKeyMaterial means jwt is enabled but no JWKS URL or PEM key was configured.
	ErrAuthJWTKeyMaterial = errors.New("config: auth.jwt requires jwks_url or public_key_pem when enabled")

	// ErrDiscoveryModeInvalid means runtime.discovery_mode is not a supported value.
	ErrDiscoveryModeInvalid = errors.New("config: runtime.discovery_mode must be Discoverable or NonDiscoverable")

	// ErrNetworkProtocolNameEmpty means a network protocol entry has an empty name.
	ErrNetworkProtocolNameEmpty = errors.New("config: runtime.network_protocols entry requires a non-empty name")

	// ErrNetworkInterfaceTokenRequired means a network interface entry has an empty token.
	ErrNetworkInterfaceTokenRequired = errors.New(
		"config: runtime.network_interfaces entry requires a non-empty token")
	// ErrNetworkInterfaceTokenDuplicate means two network interface entries share the same token.
	ErrNetworkInterfaceTokenDuplicate = errors.New("config: runtime.network_interfaces token must be unique")
	// ErrNetworkInterfaceMTU means a network interface entry has an out-of-range MTU.
	ErrNetworkInterfaceMTU = errors.New("config: runtime.network_interfaces entry mtu must be between 0 and 65535")
	// ErrNetworkInterfaceHwAddress means a network interface entry has a malformed MAC address.
	ErrNetworkInterfaceHwAddress = errors.New(
		"config: runtime.network_interfaces entry hw_address must be a valid MAC address")
	// ErrNetworkInterfaceCIDR means a network interface manual address is not valid CIDR notation.
	ErrNetworkInterfaceCIDR = errors.New(
		"config: runtime.network_interfaces entry ipv4.manual must be valid CIDR notation (e.g. 192.168.1.10/24)")

	// ErrMetadataTokenRequired means a metadata configuration entry has an empty token.
	ErrMetadataTokenRequired = errors.New("config: media.metadata_configurations entry requires a non-empty token")
	// ErrMetadataTokenDuplicate means two metadata configuration entries share the same token.
	ErrMetadataTokenDuplicate = errors.New("config: media.metadata_configurations token must be unique")

	// ErrEventsSubscriptionTimeoutInvalid means events.subscription_timeout is not a valid duration.
	ErrEventsSubscriptionTimeoutInvalid = errors.New(
		"config: events.subscription_timeout must be a Go duration (e.g. 1h) or ISO 8601 PT duration (e.g. PT1H)")

	// ErrEventsTopicNameEmpty means a topic entry has an empty name.
	ErrEventsTopicNameEmpty = errors.New("config: events.topics entry requires a non-empty name")

	// ErrEventsTopicNameDuplicate means two topic entries share the same name.
	ErrEventsTopicNameDuplicate = errors.New("config: events.topics name must be unique")
)

var (
	errNilConfig = errors.New("config: nil Config")

	errDeviceFieldRequired = errors.New("config: must not be empty")

	errProfileFieldRequired       = errors.New("config: must not be empty")
	errProfileMediaFilePathBlank  = errors.New("config: profile.media_file_path must not be only whitespace")
	errProfileBitrateNegative     = errors.New("config: profile.bitrate must be >= 0")
	errProfileGOPNegative         = errors.New("config: profile.gop_length must be >= 0")
	errProfileSnapshotURIInvalid  = errors.New("config: profile.snapshot_uri must be an http(s) URL")
	errProfileVideoSourceTokenWS  = errors.New("config: profile.video_source_token must not contain whitespace")
	errProfileVideoSourceTokenLen = errors.New("config: profile.video_source_token must not be empty when set")

	errScopeEntryEmpty       = errors.New("config: scope entry must not be empty")
	errScopeNotAbsolute      = errors.New("config: scope must be an absolute URI")
	errScopeOnvifPrefix      = errors.New("config: onvif scope must use onvif://www.onvif.org/ prefix")
	errScopeHTTPHost         = errors.New("config: http(s) scope must include host")
	errXAddrFieldEmpty       = errors.New("config: xaddr must not be empty")
	errXAddrInvalid          = errors.New("config: invalid xaddr url")
	errXAddrScheme           = errors.New("config: xaddr scheme must be http or https")
	errXAddrHost             = errors.New("config: xaddr must include host")
	errProfileTokenDuplicate = errors.New("config: profile token must be unique")
)

const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

var (
	uuidURNPattern  = regexp.MustCompile(`(?i)^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	validDigestAlgs = map[string]bool{"MD5": true, "SHA-256": true}
	validJWTAlgs    = map[string]bool{
		"RS256": true, "RS384": true, "RS512": true,
		"ES256": true, "ES384": true, "ES512": true,
	}
	whitespacePattern = regexp.MustCompile(`\s`)
)

// Validate checks all required fields and their formats.
func Validate(c *Config) error {
	if c == nil {
		return errNilConfig
	}
	if c.Version != CurrentVersion {
		return fmt.Errorf("%w (got %d, want %d)", ErrInvalidVersion, c.Version, CurrentVersion)
	}
	if err := validateDevice(&c.Device); err != nil {
		return err
	}
	if err := validateNetwork(&c.Network); err != nil {
		return err
	}
	if err := validateMedia(&c.Media); err != nil {
		return err
	}
	if err := validateAuth(&c.Auth); err != nil {
		return err
	}
	if err := validateRuntime(&c.Runtime); err != nil {
		return err
	}
	return validateEvents(&c.Events)
}

func validateDevice(d *DeviceConfig) error {
	if strings.TrimSpace(d.UUID) == "" {
		return ErrDeviceUUIDRequired
	}
	if !uuidURNPattern.MatchString(strings.TrimSpace(d.UUID)) {
		return ErrDeviceUUIDInvalid
	}
	for _, f := range []struct{ name, val string }{
		{"device.manufacturer", d.Manufacturer},
		{"device.model", d.Model},
		{"device.serial", d.Serial},
	} {
		if strings.TrimSpace(f.val) == "" {
			return fmt.Errorf("config: %s: %w", f.name, errDeviceFieldRequired)
		}
	}
	return validateScopes(d.Scopes)
}

func validateNetwork(n *NetworkConfig) error {
	if n.HTTPPort < 1 || n.HTTPPort > 65535 {
		return ErrNetworkPortInvalid
	}
	if n.RTSPPort < 0 || n.RTSPPort > 65535 {
		return ErrNetworkRTSPPortInvalid
	}
	if n.RTSPPort != 0 && n.RTSPPort == n.HTTPPort {
		return ErrNetworkPortConflict
	}
	for i, x := range n.XAddrs {
		if err := validateXAddr(i, strings.TrimSpace(x)); err != nil {
			return err
		}
	}
	return nil
}

func validateMedia(m *MediaConfig) error {
	// Empty Profiles is valid at the schema level — Default() emits an
	// empty slice on first run and the operator fills it in via the GUI/TUI
	// or by editing the JSON directly. The simulator's Start path warns
	// when no profile has a media_file_path configured.
	seenProfileTokens := make(map[string]bool, len(m.Profiles))
	for i := range m.Profiles {
		if err := validateProfile(i, &m.Profiles[i], seenProfileTokens); err != nil {
			return err
		}
	}
	seenMetadataTokens := make(map[string]bool, len(m.MetadataConfigurations))
	for i := range m.MetadataConfigurations {
		if err := validateMetadataConfig(i, &m.MetadataConfigurations[i], seenMetadataTokens); err != nil {
			return err
		}
	}
	return nil
}

func validateMetadataConfig(i int, m *MetadataConfig, seen map[string]bool) error {
	prefix := fmt.Sprintf("media.metadata_configurations[%d]", i)
	if strings.TrimSpace(m.Token) == "" {
		return fmt.Errorf("config: %s: %w", prefix, ErrMetadataTokenRequired)
	}
	if seen[m.Token] {
		return fmt.Errorf("config: %s.token %q: %w", prefix, m.Token, ErrMetadataTokenDuplicate)
	}
	seen[m.Token] = true
	return nil
}

func validateProfile(i int, p *ProfileConfig, seenProfileTokens map[string]bool) error {
	prefix := fmt.Sprintf("media.profiles[%d]", i)
	for _, f := range []struct{ name, val string }{
		{prefix + ".name", p.Name},
		{prefix + ".token", p.Token},
	} {
		if strings.TrimSpace(f.val) == "" {
			return fmt.Errorf("config: %s: %w", f.name, errProfileFieldRequired)
		}
	}
	if seenProfileTokens[p.Token] {
		return fmt.Errorf("config: %s.token %q: %w", prefix, p.Token, errProfileTokenDuplicate)
	}
	seenProfileTokens[p.Token] = true
	if err := validateMediaFilePath(prefix+".media_file_path", p.MediaFilePath); err != nil {
		return err
	}
	if p.Bitrate < 0 {
		return fmt.Errorf("config: %s.bitrate: %w", prefix, errProfileBitrateNegative)
	}
	if p.GOPLength < 0 {
		return fmt.Errorf("config: %s.gop_length: %w", prefix, errProfileGOPNegative)
	}
	if err := validateSnapshotURI(prefix+".snapshot_uri", p.SnapshotURI); err != nil {
		return err
	}
	return validateVideoSourceToken(prefix+".video_source_token", p.VideoSourceToken)
}

// validateMediaFilePath rejects whitespace-only paths. An empty string is
// allowed at the schema level — callers that need a file (the embedded RTSP
// server) enforce presence at runtime so config can be edited incrementally.
func validateMediaFilePath(field, raw string) error {
	if raw == "" {
		return nil
	}
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("config: %s: %w", field, errProfileMediaFilePathBlank)
	}
	return nil
}

func validateSnapshotURI(field, raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != schemeHTTP && u.Scheme != schemeHTTPS) || u.Host == "" {
		return fmt.Errorf("config: %s: %w", field, errProfileSnapshotURIInvalid)
	}
	return nil
}

func validateVideoSourceToken(field, raw string) error {
	if raw == "" {
		return nil
	}
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("config: %s: %w", field, errProfileVideoSourceTokenLen)
	}
	if whitespacePattern.MatchString(raw) {
		return fmt.Errorf("config: %s: %w", field, errProfileVideoSourceTokenWS)
	}
	return nil
}

func validateAuth(a *AuthConfig) error {
	if !a.Enabled && len(a.Users) == 0 && !a.JWT.Enabled {
		return nil
	}
	if a.Enabled && len(a.Users) == 0 && !a.JWT.Enabled {
		return ErrAuthUsersRequired
	}
	seen := make(map[string]bool, len(a.Users))
	for i := range a.Users {
		if err := validateUser(i, &a.Users[i], seen); err != nil {
			return err
		}
	}
	if err := validateDigest(&a.Digest); err != nil {
		return err
	}
	return validateJWT(&a.JWT)
}

func validateUser(i int, u *UserConfig, seen map[string]bool) error {
	prefix := fmt.Sprintf("auth.users[%d]", i)
	if strings.TrimSpace(u.Username) == "" ||
		strings.TrimSpace(u.Password) == "" ||
		strings.TrimSpace(u.Role) == "" {
		return fmt.Errorf("config: %s: %w", prefix, ErrAuthUserIncomplete)
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(u.Role)), "onvif:") {
		return fmt.Errorf("config: %s.role %q: %w", prefix, u.Role, ErrAuthRoleReserved)
	}
	if whitespacePattern.MatchString(u.Role) {
		return fmt.Errorf("config: %s.role %q: %w", prefix, u.Role, ErrAuthRoleWhitespace)
	}
	if seen[u.Username] {
		return fmt.Errorf("config: %s.username %q: %w", prefix, u.Username, ErrAuthUsernameDuplicate)
	}
	seen[u.Username] = true
	return nil
}

func validateDigest(d *DigestConfig) error {
	for _, alg := range d.Algorithms {
		if !validDigestAlgs[alg] {
			return fmt.Errorf("config: auth.digest.algorithms %q: %w", alg, ErrAuthDigestAlgorithm)
		}
	}
	if d.NonceTTL != "" {
		if _, err := time.ParseDuration(d.NonceTTL); err != nil {
			return fmt.Errorf("config: auth.digest.nonce_ttl %q: %w", d.NonceTTL, ErrAuthDigestNonceTTL)
		}
	}
	return nil
}

func validateJWT(j *JWTConfig) error {
	for _, alg := range j.Algorithms {
		if !validJWTAlgs[alg] {
			return fmt.Errorf("config: auth.jwt.algorithms %q: %w", alg, ErrAuthJWTAlgorithm)
		}
	}
	if j.ClockSkew != "" {
		if _, err := time.ParseDuration(j.ClockSkew); err != nil {
			return fmt.Errorf("config: auth.jwt.clock_skew %q: %w", j.ClockSkew, ErrAuthJWTClockSkew)
		}
	}
	if j.Enabled && j.JWKSURL == "" && len(j.PublicKeyPEM) == 0 {
		return ErrAuthJWTKeyMaterial
	}
	return nil
}

func validateScopes(scopes []string) error {
	for i, s := range scopes {
		raw := strings.TrimSpace(s)
		if raw == "" {
			return fmt.Errorf("config: device.scopes[%d]: %w", i, errScopeEntryEmpty)
		}
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("config: device.scopes[%d]: %w", i, err)
		}
		if u.Scheme == "" {
			return fmt.Errorf("config: device.scopes[%d]: %w", i, errScopeNotAbsolute)
		}
		switch strings.ToLower(u.Scheme) {
		case "onvif":
			if !strings.HasPrefix(strings.ToLower(raw), "onvif://www.onvif.org/") {
				return fmt.Errorf("config: device.scopes[%d]: %w", i, errScopeOnvifPrefix)
			}
		case "http", "https":
			if u.Host == "" {
				return fmt.Errorf("config: device.scopes[%d]: %w", i, errScopeHTTPHost)
			}
		}
	}
	return nil
}

func validateXAddr(i int, raw string) error {
	if raw == "" {
		return fmt.Errorf("config: network.xaddrs[%d]: %w", i, errXAddrFieldEmpty)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("config: network.xaddrs[%d]: %w: %w", i, errXAddrInvalid, err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("config: network.xaddrs[%d]: %w: got %q", i, errXAddrScheme, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("config: network.xaddrs[%d]: %w", i, errXAddrHost)
	}
	return nil
}

var validDiscoveryModes = map[string]bool{
	"Discoverable":    true,
	"NonDiscoverable": true,
}

func validateRuntime(r *RuntimeConfig) error {
	if r.DiscoveryMode != "" && !validDiscoveryModes[r.DiscoveryMode] {
		return fmt.Errorf("config: runtime.discovery_mode %q: %w", r.DiscoveryMode, ErrDiscoveryModeInvalid)
	}
	for i, p := range r.NetworkProtocols {
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("config: runtime.network_protocols[%d]: %w", i, ErrNetworkProtocolNameEmpty)
		}
	}
	seenInterfaceTokens := make(map[string]bool, len(r.NetworkInterfaces))
	for i := range r.NetworkInterfaces {
		if err := validateNetworkInterface(i, &r.NetworkInterfaces[i], seenInterfaceTokens); err != nil {
			return err
		}
	}
	if r.SystemDateAndTime.ManualDateTimeUTC != "" {
		if _, err := time.Parse(time.RFC3339, r.SystemDateAndTime.ManualDateTimeUTC); err != nil {
			return fmt.Errorf(
				"config: runtime.system_date_and_time.manual_date_time_utc %q: "+
					"must be RFC3339 (e.g. 2006-01-02T15:04:05Z): %w",
				r.SystemDateAndTime.ManualDateTimeUTC, err)
		}
	}
	return nil
}

func validateNetworkInterface(i int, iface *NetworkInterfaceConfig, seen map[string]bool) error {
	prefix := fmt.Sprintf("runtime.network_interfaces[%d]", i)
	if strings.TrimSpace(iface.Token) == "" {
		return fmt.Errorf("config: %s: %w", prefix, ErrNetworkInterfaceTokenRequired)
	}
	if seen[iface.Token] {
		return fmt.Errorf("config: %s.token %q: %w", prefix, iface.Token, ErrNetworkInterfaceTokenDuplicate)
	}
	seen[iface.Token] = true
	if iface.MTU < 0 || iface.MTU > 65535 {
		return fmt.Errorf("config: %s.mtu %d: %w", prefix, iface.MTU, ErrNetworkInterfaceMTU)
	}
	if iface.HwAddress != "" {
		if _, err := net.ParseMAC(iface.HwAddress); err != nil {
			return fmt.Errorf("config: %s.hw_address %q: %w", prefix, iface.HwAddress, ErrNetworkInterfaceHwAddress)
		}
	}
	if iface.IPv4 != nil {
		for j, m := range iface.IPv4.Manual {
			if _, _, err := net.ParseCIDR(m); err != nil {
				return fmt.Errorf("config: %s.ipv4.manual[%d] %q: %w", prefix, j, m, ErrNetworkInterfaceCIDR)
			}
		}
	}
	return nil
}

// isValidSubscriptionTimeout reports whether s is a valid subscription timeout:
// a Go duration (e.g. "1h", "30m") or an ISO 8601 PT duration subset
// (e.g. "PT1H", "PT30M", "PT1H30M"). Absolute RFC3339 timestamps are not
// accepted here; subscription_timeout is a duration, not a point in time.
func isValidSubscriptionTimeout(s string) bool {
	if _, err := time.ParseDuration(s); err == nil {
		return true
	}
	upper := strings.ToUpper(s)
	if !strings.HasPrefix(upper, "PT") {
		return false
	}
	return validatePTDuration(upper[2:])
}

// validatePTDuration reports whether rest (already uppercased, "PT" prefix
// already stripped) is a valid PT-duration token sequence using H, M, S units.
func validatePTDuration(rest string) bool {
	if rest == "" {
		return false
	}
	for rest != "" {
		i := 0
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			i++
		}
		if i == 0 || i >= len(rest) {
			return false
		}
		unit := rest[i]
		rest = rest[i+1:]
		if unit != 'H' && unit != 'M' && unit != 'S' {
			return false
		}
	}
	return true
}

func validateEvents(e *EventsConfig) error {
	if e.SubscriptionTimeout != "" {
		if !isValidSubscriptionTimeout(e.SubscriptionTimeout) {
			return fmt.Errorf("config: events.subscription_timeout %q: %w",
				e.SubscriptionTimeout, ErrEventsSubscriptionTimeoutInvalid)
		}
	}
	seen := make(map[string]bool, len(e.Topics))
	for i, t := range e.Topics {
		if strings.TrimSpace(t.Name) == "" {
			return fmt.Errorf("config: events.topics[%d]: %w", i, ErrEventsTopicNameEmpty)
		}
		if seen[t.Name] {
			return fmt.Errorf("config: events.topics[%d].name %q: %w", i, t.Name, ErrEventsTopicNameDuplicate)
		}
		seen[t.Name] = true
	}
	return nil
}

// pathMu guards the active config path. Read by every Load/Save, written
// only by SetPath; an RWMutex keeps the read path lock-free in the common
// case.
var (
	pathMu     sync.RWMutex
	activePath string
)

// SetPath overrides the file path Load/Save/Update will use.  Front-ends
// (CLI, GUI, TUI) call this once at startup with the result of DefaultPath
// (or an explicit -config flag).  Pass "" to revert to the working-directory
// fallback (useful in tests).
func SetPath(p string) {
	pathMu.Lock()
	defer pathMu.Unlock()
	activePath = p
}

// Path returns the currently configured path, or "" when SetPath has not
// been called.
func Path() string {
	pathMu.RLock()
	defer pathMu.RUnlock()
	return activePath
}

// resolvePath returns the active path. Without SetPath, falls back to
// FileName in the working directory — kept for tests and ad-hoc CLI use.
func resolvePath() (string, error) {
	if p := Path(); p != "" {
		return p, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("config: get working directory: %w", err)
	}
	return filepath.Join(wd, FileName), nil
}

// Resolve picks the path Load/Save should target. An explicit override
// (typically the CLI -config flag) wins when non-empty; otherwise we fall
// back to DefaultPath. Front-ends call this once at startup, pass the
// result to SetPath, and (optionally) EnsureExists.
func Resolve(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return DefaultPath()
}

// DefaultPath returns the OS-standard user config location:
//
//	macOS:   ~/Library/Application Support/onvif-simulator/onvif-simulator.json
//	Linux:   $XDG_CONFIG_HOME/onvif-simulator/onvif-simulator.json
//	         (or ~/.config/onvif-simulator/onvif-simulator.json)
//	Windows: %AppData%\onvif-simulator\onvif-simulator.json
//
// This is what GUI/TUI/CLI callers should use when the user has not passed
// an explicit -config flag.
func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: resolve user config dir: %w", err)
	}
	return filepath.Join(base, DirName, FileName), nil
}

// Default returns a baseline Config that passes Validate. Used by
// EnsureExists on first-run when the resolved path does not exist yet.
// The values mirror onvif-simulator.example.json's minimum viable shape
// (auth disabled so the simulator boots without credentials).
func Default() Config {
	return Config{
		Version: CurrentVersion,
		Device: DeviceConfig{
			UUID:         "urn:uuid:00000000-0000-4000-8000-000000000001",
			Manufacturer: "ONVIF Simulator",
			Model:        "SimCam-100",
			Serial:       "SN-0001",
			Firmware:     "0.1.0",
			Scopes: []string{
				"onvif://www.onvif.org/Profile/Streaming",
				"onvif://www.onvif.org/name/simulator",
				"onvif://www.onvif.org/hardware/virtual",
			},
		},
		Network: NetworkConfig{HTTPPort: 8080, RTSPPort: DefaultRTSPPort},
		Media:   MediaConfig{Profiles: nil},
		Events: EventsConfig{
			MaxPullPoints:       defaultMaxPullPoints,
			SubscriptionTimeout: "1h",
			Topics: []TopicConfig{
				{Name: "tns1:VideoSource/MotionAlarm", Enabled: true},
				{Name: "tns1:VideoSource/ImageTooBlurry", Enabled: true},
				{Name: "tns1:VideoSource/ImageTooDark", Enabled: true},
				{Name: "tns1:VideoSource/ImageTooBright", Enabled: true},
				{Name: "tns1:Device/Trigger/DigitalInput", Enabled: true},
			},
		},
		Runtime: RuntimeConfig{DiscoveryMode: "Discoverable"},
	}
}

const defaultMaxPullPoints = 10

// EnsureExists writes Default() to p when p does not yet exist, creating
// the parent directory as needed. Returns true if it created the file.
// Existing files are left untouched.
func EnsureExists(p string) (bool, error) {
	if _, err := os.Stat(p); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("config: stat %s: %w", p, err)
	}
	if err := os.MkdirAll(filepath.Dir(p), configDirMode); err != nil {
		return false, fmt.Errorf("config: mkdir %s: %w", filepath.Dir(p), err)
	}
	cfg := Default()
	if err := writeAtomic(p, &cfg); err != nil {
		return false, err
	}
	return true, nil
}

// Load reads and validates the active config file. On success it returns a
// fully validated Config that callers should treat as read-only; mutate
// individual fields only through the targeted helpers in update.go.
func Load() (Config, error) {
	p, err := resolvePath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(p) //nolint:gosec // p comes from SetPath/working dir, both trusted.
	if err != nil {
		return Config{}, fmt.Errorf("config: read %s: %w", filepath.Base(p), err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("config: parse %s: %w", filepath.Base(p), err)
	}
	if err := Validate(&c); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Save validates cfg and writes it to the active path. The write is atomic
// (write-to-temp + rename) to prevent corruption on crash. Prefer the
// targeted helpers in update.go over calling Save directly; they load,
// mutate, and save under the package mutex.
func Save(cfg *Config) error {
	if cfg == nil {
		return errNilConfig
	}
	if err := Validate(cfg); err != nil {
		return err
	}
	p, err := resolvePath()
	if err != nil {
		return err
	}
	return writeAtomic(p, cfg)
}

// writeAtomic marshals cfg and writes it to path via temp file + rename,
// creating the parent directory if needed. Caller has already validated.
func writeAtomic(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if mkErr := os.MkdirAll(dir, configDirMode); mkErr != nil {
		return fmt.Errorf("config: mkdir %s: %w", dir, mkErr)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("config: create temp in %q: %w", dir, err)
	}
	tmpPath := tmp.Name()
	if chmodErr := os.Chmod(tmpPath, configFileMode); chmodErr != nil {
		return joinSaveErrors(
			fmt.Errorf("config: chmod temp: %w", chmodErr),
			tmp.Close(),
			os.Remove(tmpPath),
		)
	}
	if _, werr := tmp.Write(data); werr != nil {
		return joinSaveErrors(
			fmt.Errorf("config: write temp: %w", werr),
			tmp.Close(),
			os.Remove(tmpPath),
		)
	}
	if cerr := tmp.Close(); cerr != nil {
		return joinSaveErrors(fmt.Errorf("config: close temp: %w", cerr), os.Remove(tmpPath))
	}
	if rerr := os.Rename(tmpPath, path); rerr != nil {
		return joinSaveErrors(
			fmt.Errorf("config: rename to %s: %w", filepath.Base(path), rerr),
			os.Remove(tmpPath),
		)
	}
	return nil
}

func joinSaveErrors(primary error, rest ...error) error {
	return errors.Join(append([]error{primary}, rest...)...)
}
