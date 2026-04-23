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
// UUID, Manufacturer, Model, and Serial are required.
// These fields are used for both ONVIF GetDeviceInformation responses
// and WS-Discovery advertisement (Scopes are derived from them at runtime).
type DeviceConfig struct {
	// UUID is the stable WS-Discovery a:Address (must be urn:uuid:<uuid>).
	UUID         string `json:"uuid"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Serial       string `json:"serial"`
	// Firmware is optional; reported in GetDeviceInformation.
	Firmware string `json:"firmware,omitempty"`
	// Scopes are additional WS-Discovery scope URIs beyond the auto-derived ones.
	Scopes []string `json:"scopes,omitempty"`
}

// NetworkConfig describes how clients reach this device.
type NetworkConfig struct {
	// HTTPPort is the port the ONVIF device service listens on (required, 1–65535).
	HTTPPort int `json:"http_port"`
	// Interface is the network interface to bind (empty means all interfaces).
	Interface string `json:"interface,omitempty"`
	// XAddrs overrides the auto-derived WS-Discovery transport addresses.
	// If empty, XAddrs is derived from the bound IP and HTTPPort at runtime.
	XAddrs []string `json:"xaddrs,omitempty"`
}

// MediaConfig holds the list of ONVIF media profiles this device advertises.
type MediaConfig struct {
	Profiles []ProfileConfig `json:"profiles"`
}

// ProfileConfig describes a single ONVIF media profile.
// Name, Token, RTSP, Encoding, Width, Height, and FPS are all required.
type ProfileConfig struct {
	// Name is a human-readable label (e.g. "main", "sub").
	Name string `json:"name"`
	// Token is the ONVIF profile token (must be unique across profiles).
	Token    string `json:"token"`
	RTSP     string `json:"rtsp"`
	Encoding string `json:"encoding"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FPS      int    `json:"fps"`
}

// AuthConfig holds credentials for WS-Security Digest authentication.
// Username and Password must both be set or both be empty.
type AuthConfig struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
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

	// ErrAuthIncomplete means only one of username/password is set.
	ErrAuthIncomplete = errors.New("config: auth.username and auth.password must both be set or both be empty")
)

var (
	errNilConfig = errors.New("config: nil Config")

	errDeviceFieldRequired = errors.New("config: must not be empty")

	errProfileFieldRequired = errors.New("config: must not be empty")
	errProfileRTSPInvalid   = errors.New("config: profile.rtsp must be a valid rtsp:// URL")
	errProfileDimension     = errors.New("config: must be greater than 0")

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

var (
	uuidURNPattern = regexp.MustCompile(`(?i)^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	validEncodings = map[string]bool{"H264": true, "H265": true, "MJPEG": true}
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
	return nil
}

func validateAuth(a *AuthConfig) error {
	hasUser := strings.TrimSpace(a.Username) != ""
	hasPass := strings.TrimSpace(a.Password) != ""
	if hasUser != hasPass {
		return ErrAuthIncomplete
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
