// Package config loads and validates onvif-simulator.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// FileName is the default on-disk config file name in the working directory.
const FileName = "onvif-simulator.json"

// CurrentVersion is the only supported config schema version.
const CurrentVersion = 1

// Config is the on-disk JSON shape.
type Config struct {
	Version int           `json:"version"`
	Device  DeviceConfig  `json:"device"`
	Network NetworkConfig `json:"network"`
	Media   MediaConfig   `json:"media"`
	Auth    AuthConfig    `json:"auth,omitempty"`
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
	HTTPPort  int      `json:"http_port"`
	Interface string   `json:"interface,omitempty"`
	XAddrs    []string `json:"xaddrs,omitempty"`
}

// MediaConfig holds the list of ONVIF media profiles this device advertises.
type MediaConfig struct {
	Profiles []ProfileConfig `json:"profiles"`
}

// ProfileConfig describes a single ONVIF media profile.
//
// RTSP and SnapshotURI are pass-through: the simulator does not run an RTP
// server or snapshot endpoint itself — it returns these URIs verbatim from
// GetStreamUri / GetSnapshotUri so clients can connect to a user-provided
// external process.
type ProfileConfig struct {
	Name             string `json:"name"`
	Token            string `json:"token"`
	RTSP             string `json:"rtsp"`
	Encoding         string `json:"encoding"`
	Width            int    `json:"width"`
	Height           int    `json:"height"`
	FPS              int    `json:"fps"`
	Bitrate          int    `json:"bitrate,omitempty"`
	GOPLength        int    `json:"gop_length,omitempty"`
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
)

var (
	errNilConfig = errors.New("config: nil Config")

	errDeviceFieldRequired = errors.New("config: must not be empty")

	errProfileFieldRequired       = errors.New("config: must not be empty")
	errProfileRTSPInvalid         = errors.New("config: profile.rtsp must be a valid rtsp:// URL")
	errProfileDimension           = errors.New("config: must be greater than 0")
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
	validEncodings  = map[string]bool{"H264": true, "H265": true, "MJPEG": true}
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
	return validateAuth(&c.Auth)
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
	for i, x := range n.XAddrs {
		if err := validateXAddr(i, strings.TrimSpace(x)); err != nil {
			return err
		}
	}
	return nil
}

func validateMedia(m *MediaConfig) error {
	if len(m.Profiles) == 0 {
		return ErrMediaNoProfiles
	}
	seenProfileTokens := make(map[string]bool, len(m.Profiles))
	for i := range m.Profiles {
		if err := validateProfile(i, &m.Profiles[i], seenProfileTokens); err != nil {
			return err
		}
	}
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
	if err := validateRTSPURI(prefix+".rtsp", p.RTSP); err != nil {
		return err
	}
	if !validEncodings[p.Encoding] {
		return fmt.Errorf("config: %s.encoding: %w (got %q)", prefix, ErrProfileEncodingInvalid, p.Encoding)
	}
	for _, f := range []struct {
		name string
		val  int
	}{
		{prefix + ".width", p.Width},
		{prefix + ".height", p.Height},
		{prefix + ".fps", p.FPS},
	} {
		if f.val <= 0 {
			return fmt.Errorf("config: %s: %w", f.name, errProfileDimension)
		}
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

func validateRTSPURI(field, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("config: %s: %w", field, errProfileRTSPInvalid)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "rtsp" || u.Host == "" {
		return fmt.Errorf("config: %s: %w", field, errProfileRTSPInvalid)
	}
	return nil
}

// Load reads and validates ./onvif-simulator.json relative to the process working directory.
func Load() (Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("config: get working directory: %w", err)
	}
	data, err := fs.ReadFile(os.DirFS(wd), FileName)
	if err != nil {
		return Config{}, fmt.Errorf("config: read %s: %w", FileName, err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("config: parse %s: %w", FileName, err)
	}
	if err := Validate(&c); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Save writes cfg to ./onvif-simulator.json relative to the working directory.
// It validates before writing and replaces the destination atomically when possible.
func Save(cfg *Config) error {
	if cfg == nil {
		return errNilConfig
	}
	if err := Validate(cfg); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	data = append(data, '\n')

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("config: get working directory: %w", err)
	}
	path := filepath.Join(wd, FileName)

	tmp, err := os.CreateTemp(wd, "."+FileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("config: create temp in %q: %w", wd, err)
	}
	tmpPath := tmp.Name()

	if _, werr := tmp.Write(data); werr != nil {
		return joinSaveErrors(fmt.Errorf("config: write temp: %w", werr), tmp.Close(), os.Remove(tmpPath))
	}
	if cerr := tmp.Close(); cerr != nil {
		return joinSaveErrors(fmt.Errorf("config: close temp: %w", cerr), os.Remove(tmpPath))
	}
	if rerr := os.Rename(tmpPath, path); rerr != nil {
		return joinSaveErrors(fmt.Errorf("config: rename to %s: %w", FileName, rerr), os.Remove(tmpPath))
	}
	return nil
}

func joinSaveErrors(primary error, rest ...error) error {
	return errors.Join(append([]error{primary}, rest...)...)
}
