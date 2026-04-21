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

// FileName is the default on-disk config file name in the working directory (see README).
const FileName = "onvif-simulator.json"

// CurrentVersion is the only supported config schema version.
const CurrentVersion = 1

// Config is the on-disk JSON shape (see onvif-simulator.example.json).
type Config struct {
	Version   int             `json:"version"`
	MainRTSP  string          `json:"main_rtsp_uri"`
	SubRTSP   string          `json:"sub_rtsp_uri"`
	Discovery DiscoveryConfig `json:"discovery,omitempty"`
}

// DiscoveryConfig holds WS-Discovery / ONVIF Hello-related fields (Core §7.3, WS-Discovery §4.1).
// Omit or leave at zero values to disable validation of discovery (e.g. simulator not advertising yet).
// If EndpointAddress is set, Types, Scopes, XAddrs, InstanceID, and MetadataVersion are required.
type DiscoveryConfig struct {
	// EndpointAddress is the stable a:Address inside Hello (ONVIF: URN:UUID).
	EndpointAddress string `json:"endpoint_address"`
	// Types are d:Types QNames (e.g. tds:Device).
	Types []string `json:"types"`
	// Scopes are absolute scope URIs for d:Scopes.
	Scopes []string `json:"scopes"`
	// XAddrs are device service URLs (http/https) for d:XAddrs.
	XAddrs []string `json:"xaddrs"`
	// InstanceID is d:AppSequence @InstanceId (stable for this simulated device).
	InstanceID uint32 `json:"instance_id"`
	// MetadataVersion is d:MetadataVersion; increment when Types/Scopes/XAddrs metadata changes.
	MetadataVersion uint32 `json:"metadata_version"`
}

var (
	// ErrInvalidVersion means config.version does not match a supported schema.
	ErrInvalidVersion = errors.New("config: unsupported version")
	// ErrEmptyRTSPURI means a required rtsp URI field was empty.
	ErrEmptyRTSPURI = errors.New("config: rtsp uri must not be empty")
	// ErrInvalidRTSPURL means the value is not a valid URL.
	ErrInvalidRTSPURL = errors.New("config: invalid rtsp url")
	// ErrInvalidRTSPScheme means the URL scheme is not rtsp.
	ErrInvalidRTSPScheme = errors.New("config: rtsp uri scheme must be rtsp")
	// ErrRTSPHostRequired means the URL has no host component.
	ErrRTSPHostRequired = errors.New("config: rtsp uri must include host")

	// ErrDiscoveryIncomplete means discovery fields were partially set without endpoint_address.
	ErrDiscoveryIncomplete = errors.New("config: discovery.endpoint_address required when other discovery fields are set")
	// ErrDiscoveryEndpointURN means endpoint_address is not a valid UUID URN.
	ErrDiscoveryEndpointURN = errors.New("config: discovery.endpoint_address must look like urn:uuid:<uuid>")
)

var (
	errDiscoveryTypesWhenConfigured = errors.New(
		"config: discovery.types must be non-empty when discovery is configured",
	)
	errDiscoveryTypeEntryEmpty       = errors.New("config: discovery type entry must not be empty")
	errDiscoveryScopeEntryEmpty      = errors.New("config: discovery scope entry must not be empty")
	errDiscoveryScopesWhenConfigured = errors.New(
		"config: discovery.scopes must be non-empty when discovery is configured",
	)
	errDiscoveryScopeNotAbsolute = errors.New("config: discovery scope must be an absolute URI")
	errDiscoveryScopeOnvifPrefix = errors.New(
		"config: discovery onvif scope must use onvif://www.onvif.org/ prefix",
	)
	errDiscoveryScopeHTTPHost        = errors.New("config: discovery http(s) scope must include host")
	errDiscoveryXAddrsWhenConfigured = errors.New(
		"config: discovery.xaddrs must be non-empty when discovery is configured",
	)
	errDiscoveryInstanceID = errors.New(
		"config: discovery.instance_id must be >= 1 when discovery is configured",
	)
	errDiscoveryMetadataVersion = errors.New(
		"config: discovery.metadata_version must be >= 1 when discovery is configured",
	)
	errHTTPURLFieldEmpty  = errors.New("config: field must not be empty")
	errHTTPURLInvalid     = errors.New("config: invalid url")
	errHTTPURLScheme      = errors.New("config: field scheme must be http or https")
	errHTTPURLHost        = errors.New("config: field must include host")
	errNilConfig          = errors.New("config: nil Config")
	errNilDiscoveryConfig = errors.New("config: nil DiscoveryConfig pointer")
)

// Validate checks required fields and RTSP URL shape.
func Validate(c *Config) error {
	if c == nil {
		return errNilConfig
	}
	if c.Version != CurrentVersion {
		return fmt.Errorf("%w (got %d, want %d)", ErrInvalidVersion, c.Version, CurrentVersion)
	}
	if err := validateRTSPURI("main_rtsp_uri", c.MainRTSP); err != nil {
		return err
	}
	if err := validateRTSPURI("sub_rtsp_uri", c.SubRTSP); err != nil {
		return err
	}
	return validateDiscovery(&c.Discovery)
}

var uuidURNPattern = regexp.MustCompile(`(?i)^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validateDiscovery(d *DiscoveryConfig) error {
	if d == nil {
		return errNilDiscoveryConfig
	}
	hasEndpoint := strings.TrimSpace(d.EndpointAddress) != ""
	if !hasEndpoint && discoveryPartiallySet(d) {
		return ErrDiscoveryIncomplete
	}
	if !hasEndpoint {
		return nil
	}
	if !uuidURNPattern.MatchString(strings.TrimSpace(d.EndpointAddress)) {
		return fmt.Errorf("%w", ErrDiscoveryEndpointURN)
	}
	if err := validateDiscoveryTypes(d.Types); err != nil {
		return err
	}
	if err := validateDiscoveryScopesSection(d.Scopes); err != nil {
		return err
	}
	if err := validateDiscoveryXAddrs(d.XAddrs); err != nil {
		return err
	}
	if d.InstanceID < 1 {
		return errDiscoveryInstanceID
	}
	if d.MetadataVersion < 1 {
		return errDiscoveryMetadataVersion
	}
	return nil
}

func validateDiscoveryTypes(types []string) error {
	if len(types) == 0 {
		return errDiscoveryTypesWhenConfigured
	}
	for i, t := range types {
		if strings.TrimSpace(t) == "" {
			return fmt.Errorf("config: discovery.types[%d]: %w", i, errDiscoveryTypeEntryEmpty)
		}
	}
	return nil
}

func validateDiscoveryScopesSection(scopes []string) error {
	if len(scopes) == 0 {
		return errDiscoveryScopesWhenConfigured
	}
	return validateDiscoveryScopes(scopes)
}

func validateDiscoveryXAddrs(xaddrs []string) error {
	if len(xaddrs) == 0 {
		return errDiscoveryXAddrsWhenConfigured
	}
	for i, x := range xaddrs {
		field := fmt.Sprintf("discovery.xaddrs[%d]", i)
		if err := validateHTTPURL(field, strings.TrimSpace(x)); err != nil {
			return err
		}
	}
	return nil
}

func discoveryPartiallySet(d *DiscoveryConfig) bool {
	if d == nil {
		return false
	}
	if len(d.Types) > 0 || len(d.Scopes) > 0 || len(d.XAddrs) > 0 {
		return true
	}
	if d.InstanceID != 0 || d.MetadataVersion != 0 {
		return true
	}
	return false
}

func validateDiscoveryScopes(scopes []string) error {
	for i, s := range scopes {
		raw := strings.TrimSpace(s)
		if raw == "" {
			return fmt.Errorf("config: discovery.scopes[%d]: %w", i, errDiscoveryScopeEntryEmpty)
		}
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("config: discovery.scopes[%d]: %w", i, err)
		}
		if u.Scheme == "" {
			return fmt.Errorf("config: discovery.scopes[%d]: %w", i, errDiscoveryScopeNotAbsolute)
		}
		switch strings.ToLower(u.Scheme) {
		case "onvif":
			if !strings.HasPrefix(strings.ToLower(raw), "onvif://www.onvif.org/") {
				return fmt.Errorf("config: discovery.scopes[%d]: %w", i, errDiscoveryScopeOnvifPrefix)
			}
		case "http", "https":
			if u.Host == "" {
				return fmt.Errorf("config: discovery.scopes[%d]: %w", i, errDiscoveryScopeHTTPHost)
			}
		default:
			// Vendor-specific or other schemes: must be parseable absolute URI (scheme set above).
		}
	}
	return nil
}

func validateHTTPURL(field, raw string) error {
	if raw == "" {
		return fmt.Errorf("config: %s: %w", field, errHTTPURLFieldEmpty)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("config: %s: %w: %w", field, errHTTPURLInvalid, err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("config: %s: %w: got %q", field, errHTTPURLScheme, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("config: %s: %w", field, errHTTPURLHost)
	}
	return nil
}

func validateRTSPURI(field, raw string) error {
	if raw == "" {
		return fmt.Errorf("%w (%s)", ErrEmptyRTSPURI, field)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w (%s): %w", ErrInvalidRTSPURL, field, err)
	}
	if u.Scheme != "rtsp" {
		return fmt.Errorf("%w (%s): got scheme %q", ErrInvalidRTSPScheme, field, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("%w (%s)", ErrRTSPHostRequired, field)
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
// It validates before writing and replaces the destination atomically when possible
// (same directory rename).
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
		return joinSaveErrors(
			fmt.Errorf("config: write temp: %w", werr),
			tmp.Close(),
			os.Remove(tmpPath),
		)
	}
	if cerr := tmp.Close(); cerr != nil {
		return joinSaveErrors(
			fmt.Errorf("config: close temp: %w", cerr),
			os.Remove(tmpPath),
		)
	}
	if rerr := os.Rename(tmpPath, path); rerr != nil {
		return joinSaveErrors(
			fmt.Errorf("config: rename to %s: %w", FileName, rerr),
			os.Remove(tmpPath),
		)
	}
	return nil
}

func joinSaveErrors(primary error, rest ...error) error {
	return errors.Join(append([]error{primary}, rest...)...)
}
