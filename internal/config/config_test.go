package config_test

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

var validConfig = config.Config{
	Version: config.CurrentVersion,
	Device: config.DeviceConfig{
		UUID:         "urn:uuid:11111111-2222-4333-8444-555555555555",
		Manufacturer: "Acme",
		Model:        "SimCam-100",
		Serial:       "SN-001",
	},
	Network: config.NetworkConfig{
		HTTPPort: 8080,
	},
	Media: config.MediaConfig{
		Profiles: []config.ProfileConfig{
			{Name: "main", Token: "profile_main"},
		},
	},
}

func TestValidateAcceptsCanonicalConfig(t *testing.T) {
	t.Parallel()
	if err := config.Validate(&validConfig); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	t.Parallel()

	userOK := []config.UserConfig{{Username: "u", Password: "p", Role: config.RoleUser}}

	cases := []struct {
		name    string
		mutate  func(c *config.Config)
		wantErr error
	}{
		{
			name:    "wrong version",
			mutate:  func(c *config.Config) { c.Version = 0 },
			wantErr: config.ErrInvalidVersion,
		},
		{
			name:    "missing device uuid",
			mutate:  func(c *config.Config) { c.Device.UUID = "" },
			wantErr: config.ErrDeviceUUIDRequired,
		},
		{
			name:    "invalid device uuid",
			mutate:  func(c *config.Config) { c.Device.UUID = "not-a-uuid" },
			wantErr: config.ErrDeviceUUIDInvalid,
		},
		{
			name:    "invalid http port",
			mutate:  func(c *config.Config) { c.Network.HTTPPort = 0 },
			wantErr: config.ErrNetworkPortInvalid,
		},
		{
			name:    "auth enabled without users",
			mutate:  func(c *config.Config) { c.Auth = config.AuthConfig{Enabled: true} },
			wantErr: config.ErrAuthUsersRequired,
		},
		{
			name: "auth user missing fields",
			mutate: func(c *config.Config) {
				c.Auth = config.AuthConfig{Enabled: true, Users: []config.UserConfig{{Username: "admin"}}}
			},
			wantErr: config.ErrAuthUserIncomplete,
		},
		{
			name: "auth role reserved prefix",
			mutate: func(c *config.Config) {
				c.Auth = config.AuthConfig{Enabled: true, Users: []config.UserConfig{{Username: "u", Password: "p", Role: "onvif:custom"}}}
			},
			wantErr: config.ErrAuthRoleReserved,
		},
		{
			name: "auth role whitespace",
			mutate: func(c *config.Config) {
				c.Auth = config.AuthConfig{Enabled: true, Users: []config.UserConfig{{Username: "u", Password: "p", Role: "my role"}}}
			},
			wantErr: config.ErrAuthRoleWhitespace,
		},
		{
			name: "auth username duplicate",
			mutate: func(c *config.Config) {
				c.Auth = config.AuthConfig{Enabled: true, Users: []config.UserConfig{
					{Username: "admin", Password: "p", Role: config.RoleAdministrator},
					{Username: "admin", Password: "q", Role: config.RoleOperator},
				}}
			},
			wantErr: config.ErrAuthUsernameDuplicate,
		},
		{
			name: "auth digest algorithm invalid",
			mutate: func(c *config.Config) {
				c.Auth = config.AuthConfig{Enabled: true, Users: userOK}
				c.Auth.Digest.Algorithms = []string{"MD5", "SHA-512"}
			},
			wantErr: config.ErrAuthDigestAlgorithm,
		},
		{
			name: "auth digest nonce ttl invalid",
			mutate: func(c *config.Config) {
				c.Auth = config.AuthConfig{Enabled: true, Users: userOK}
				c.Auth.Digest.NonceTTL = "forever"
			},
			wantErr: config.ErrAuthDigestNonceTTL,
		},
		{
			name: "auth jwt algorithm invalid",
			mutate: func(c *config.Config) {
				c.Auth = config.AuthConfig{Enabled: true, Users: userOK}
				c.Auth.JWT.Algorithms = []string{"HS256"}
			},
			wantErr: config.ErrAuthJWTAlgorithm,
		},
		{
			name: "auth jwt enabled without key material",
			mutate: func(c *config.Config) {
				c.Auth = config.AuthConfig{Enabled: true, Users: userOK}
				c.Auth.JWT.Enabled = true
			},
			wantErr: config.ErrAuthJWTKeyMaterial,
		},
		{
			name: "auth jwt clock skew invalid",
			mutate: func(c *config.Config) {
				c.Auth = config.AuthConfig{Enabled: true, Users: userOK}
				c.Auth.JWT.ClockSkew = "tomorrow"
			},
			wantErr: config.ErrAuthJWTClockSkew,
		},
		{
			name:    "logging level invalid",
			mutate:  func(c *config.Config) { c.Logging.Level = "trace" },
			wantErr: config.ErrLoggingLevelInvalid,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := validConfig
			tc.mutate(&c)
			if err := config.Validate(&c); !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestValidateRejectsProfileExtras exercises the optional-field validation
// branches added for the Media service pass-through fields.
func TestValidateRejectsProfileExtras(t *testing.T) {
	t.Parallel()

	base := func() config.ProfileConfig {
		return config.ProfileConfig{Name: "main", Token: "t"}
	}

	cases := []struct {
		name      string
		wantField string
		mutate    func(*config.ProfileConfig)
	}{
		{"bitrate negative", ".bitrate", func(p *config.ProfileConfig) { p.Bitrate = -1 }},
		{"gop length negative", ".gop_length", func(p *config.ProfileConfig) { p.GOPLength = -1 }},
		{"snapshot uri malformed", ".snapshot_uri", func(p *config.ProfileConfig) { p.SnapshotURI = "://not-a-url" }},
		{"snapshot uri wrong scheme", ".snapshot_uri", func(p *config.ProfileConfig) { p.SnapshotURI = "ftp://host/snap.jpg" }},
		{"snapshot uri no host", ".snapshot_uri", func(p *config.ProfileConfig) { p.SnapshotURI = "http:///snap.jpg" }},
		{"video source token with space", ".video_source_token", func(p *config.ProfileConfig) { p.VideoSourceToken = "bad token" }},
		{"video source token whitespace only", ".video_source_token", func(p *config.ProfileConfig) { p.VideoSourceToken = "   " }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := validConfig
			p := base()
			tc.mutate(&p)
			c.Media.Profiles = []config.ProfileConfig{p}
			err := config.Validate(&c)
			if err == nil {
				t.Fatalf("%s: expected validation error, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantField) {
				t.Fatalf("%s: error %q missing field token %q", tc.name, err.Error(), tc.wantField)
			}
		})
	}
}

// TestValidateAcceptsProfileExtras makes sure the happy-path values for
// the new optional fields pass validation.
func TestValidateAcceptsProfileExtras(t *testing.T) {
	t.Parallel()
	c := validConfig
	c.Media.Profiles = []config.ProfileConfig{{
		Name: "main", Token: "t",
		MediaFilePath:    "/var/onvif/main.mp4",
		Bitrate:          4096,
		GOPLength:        60,
		SnapshotURI:      "https://host/snap.jpg",
		VideoSourceToken: "VS_MAIN",
	}}
	if err := config.Validate(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidateAcceptsMediaFilePathOnlyProfile ensures a profile authored for
// the embedded RTSP server (media_file_path set, RTSP omitted, encoder fields
// 0 to be auto-detected) passes validation.
func TestValidateAcceptsMediaFilePathOnlyProfile(t *testing.T) {
	t.Parallel()
	c := validConfig
	c.Media.Profiles = []config.ProfileConfig{{
		Name:          "main",
		Token:         "profile_main",
		MediaFilePath: "/var/onvif-simulator/main.mp4",
	}}
	if err := config.Validate(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsMediaFilePathWhitespace(t *testing.T) {
	t.Parallel()
	c := validConfig
	c.Media.Profiles = []config.ProfileConfig{{
		Name:          "main",
		Token:         "profile_main",
		MediaFilePath: "   ",
	}}
	err := config.Validate(&c)
	if err == nil {
		t.Fatal("expected validation error for whitespace media_file_path")
	}
	if !strings.Contains(err.Error(), ".media_file_path") {
		t.Fatalf("expected error to mention media_file_path, got %v", err)
	}
}

func TestValidateProfileSourceKind(t *testing.T) {
	t.Parallel()

	rpicamOK := func() *config.RPICamConfig {
		return &config.RPICamConfig{Width: 1920, Height: 1080, FPS: 30}
	}

	cases := []struct {
		name    string
		profile config.ProfileConfig
		wantErr error
	}{
		{
			name:    "default kind back-compat (file path set)",
			profile: config.ProfileConfig{Name: "n", Token: "t", MediaFilePath: "/x.mp4"},
			wantErr: nil,
		},
		{
			name:    "explicit kind=file",
			profile: config.ProfileConfig{Name: "n", Token: "t", Kind: config.ProfileKindFile, MediaFilePath: "/x.mp4"},
			wantErr: nil,
		},
		{
			name:    "kind=rpicam with rpicam set",
			profile: config.ProfileConfig{Name: "n", Token: "t", Kind: config.ProfileKindRPICam, RPICam: rpicamOK()},
			wantErr: nil,
		},
		{
			name:    "kind unknown rejected",
			profile: config.ProfileConfig{Name: "n", Token: "t", Kind: "v4l2"},
			wantErr: config.ErrProfileKindInvalid,
		},
		{
			name: "kind=file with rpicam rejected",
			profile: config.ProfileConfig{
				Name: "n", Token: "t", Kind: config.ProfileKindFile,
				MediaFilePath: "/x.mp4", RPICam: rpicamOK(),
			},
			wantErr: config.ErrProfileKindFileWithRPICam,
		},
		{
			name: "kind=rpicam with media_file_path rejected",
			profile: config.ProfileConfig{
				Name: "n", Token: "t", Kind: config.ProfileKindRPICam,
				MediaFilePath: "/x.mp4", RPICam: rpicamOK(),
			},
			wantErr: config.ErrProfileKindRPICamWithFile,
		},
		{
			name:    "kind=rpicam without rpicam rejected",
			profile: config.ProfileConfig{Name: "n", Token: "t", Kind: config.ProfileKindRPICam},
			wantErr: config.ErrProfileRPICamRequired,
		},
		{
			name: "rpicam camera_id negative rejected",
			profile: config.ProfileConfig{
				Name: "n", Token: "t", Kind: config.ProfileKindRPICam,
				RPICam: &config.RPICamConfig{CameraID: -1, Width: 640, Height: 480, FPS: 30},
			},
			wantErr: config.ErrProfileRPICamFieldRange,
		},
		{
			name: "rpicam width too large rejected",
			profile: config.ProfileConfig{
				Name: "n", Token: "t", Kind: config.ProfileKindRPICam,
				RPICam: &config.RPICamConfig{Width: 8192, Height: 1080, FPS: 30},
			},
			wantErr: config.ErrProfileRPICamFieldRange,
		},
		{
			name: "rpicam fps too high rejected",
			profile: config.ProfileConfig{
				Name: "n", Token: "t", Kind: config.ProfileKindRPICam,
				RPICam: &config.RPICamConfig{Width: 640, Height: 480, FPS: 240},
			},
			wantErr: config.ErrProfileRPICamFieldRange,
		},
		{
			name: "rpicam extra_args reserved",
			profile: config.ProfileConfig{
				Name: "n", Token: "t", Kind: config.ProfileKindRPICam,
				RPICam: &config.RPICamConfig{Width: 640, Height: 480, FPS: 30, ExtraArgs: []string{"--foo"}},
			},
			wantErr: config.ErrProfileRPICamExtraArgs,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := validConfig
			c.Media.Profiles = []config.ProfileConfig{tc.profile}
			err := config.Validate(&c)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("expected pass, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateRejectsDuplicateRPICamCameraID(t *testing.T) {
	t.Parallel()
	c := validConfig
	c.Media.Profiles = []config.ProfileConfig{
		{
			Name: "main", Token: "p1", Kind: config.ProfileKindRPICam,
			RPICam: &config.RPICamConfig{CameraID: 0, Width: 1920, Height: 1080, FPS: 30},
		},
		{
			Name: "sub", Token: "p2", Kind: config.ProfileKindRPICam,
			RPICam: &config.RPICamConfig{CameraID: 0, Width: 640, Height: 480, FPS: 15},
		},
	}
	err := config.Validate(&c)
	if !errors.Is(err, config.ErrProfileRPICamDuplicateCameraID) {
		t.Fatalf("expected ErrProfileRPICamDuplicateCameraID, got %v", err)
	}
}

func TestProfileConfigHasSource(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		p    config.ProfileConfig
		want bool
	}{
		{"empty", config.ProfileConfig{}, false},
		{"file with path", config.ProfileConfig{MediaFilePath: "/x.mp4"}, true},
		{"file kind no path", config.ProfileConfig{Kind: config.ProfileKindFile}, false},
		{"file kind whitespace path", config.ProfileConfig{Kind: config.ProfileKindFile, MediaFilePath: "  "}, false},
		{"rpicam kind with config", config.ProfileConfig{Kind: config.ProfileKindRPICam, RPICam: &config.RPICamConfig{}}, true},
		{"rpicam kind without config", config.ProfileConfig{Kind: config.ProfileKindRPICam}, false},
		{"unknown kind", config.ProfileConfig{Kind: "v4l2"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.p.HasSource(); got != tc.want {
				t.Fatalf("HasSource() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRPICamProfileJSONRoundTrip(t *testing.T) {
	t.Parallel()
	c := validConfig
	c.Media.Profiles = []config.ProfileConfig{{
		Name: "main", Token: "profile_main",
		Kind: config.ProfileKindRPICam,
		RPICam: &config.RPICamConfig{
			CameraID: 0, Width: 1920, Height: 1080, FPS: 30,
			Bitrate: 4_000_000, IDRPeriod: 60, HFlip: true,
		},
	}}
	raw, err := json.Marshal(&c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got config.Config
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, c) {
		t.Fatalf("round-trip mismatch:\nraw=%s\ngot=%+v\nwant=%+v", string(raw), got, c)
	}
}

func TestValidateNetworkRTSPPort(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		http     int
		rtsp     int
		wantErr  error
		wantPass bool
	}{
		{"rtsp 0 falls back to default", 8080, 0, nil, true},
		{"rtsp 8554 explicit", 8080, 8554, nil, true},
		{"rtsp negative", 8080, -1, config.ErrNetworkRTSPPortInvalid, false},
		{"rtsp out of range", 8080, 70000, config.ErrNetworkRTSPPortInvalid, false},
		{"rtsp clashes with http", 8080, 8080, config.ErrNetworkPortConflict, false},
		// http_port=DefaultRTSPPort with rtsp_port=0 must collide because
		// rtsp_port=0 falls back to DefaultRTSPPort.
		{"rtsp default clashes with http on 8554", 8554, 0, config.ErrNetworkPortConflict, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := validConfig
			c.Network.HTTPPort = tc.http
			c.Network.RTSPPort = tc.rtsp
			err := config.Validate(&c)
			if tc.wantPass {
				if err != nil {
					t.Fatalf("expected pass, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestRTSPPortOrDefault(t *testing.T) {
	t.Parallel()
	if got := (config.NetworkConfig{}).RTSPPortOrDefault(); got != config.DefaultRTSPPort {
		t.Errorf("zero RTSPPort = %d, want %d", got, config.DefaultRTSPPort)
	}
	if got := (config.NetworkConfig{RTSPPort: 9554}).RTSPPortOrDefault(); got != 9554 {
		t.Errorf("explicit RTSPPort = %d, want 9554", got)
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	want := config.Config{
		Version: config.CurrentVersion,
		Device: config.DeviceConfig{
			UUID:         "urn:uuid:aaaaaaaa-bbbb-4ccc-dddd-eeeeeeeeeeee",
			Manufacturer: "Acme",
			Model:        "SimCam-200",
			Serial:       "SN-999",
			Firmware:     "1.2.3",
			Scopes:       []string{"onvif://www.onvif.org/location/Seoul"},
		},
		Network: config.NetworkConfig{
			HTTPPort:  8443,
			Interface: "eth0",
			XAddrs:    []string{"http://192.168.1.100:8443/onvif/device_service"},
		},
		Media: config.MediaConfig{
			Profiles: []config.ProfileConfig{
				{Name: "main", Token: "profile_main", MediaFilePath: "/var/onvif/main.mp4"},
			},
		},
		Auth: config.AuthConfig{
			Enabled: true,
			Users: []config.UserConfig{
				{Username: "admin", Password: "secret", Role: config.RoleAdministrator},
				{Username: "ops", Password: "op-pass", Role: config.RoleOperator},
			},
			Digest: config.DigestConfig{
				Realm:      "onvif-simulator",
				Algorithms: []string{"MD5", "SHA-256"},
				NonceTTL:   "5m",
			},
		},
	}

	if err := config.Save(&want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip mismatch:\ngot  %+v\nwant %+v", got, want)
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected not exist, got %v", err)
	}
}

func TestExampleJSONLoads(t *testing.T) {
	// Each shipped example file must load and validate cleanly. Catches
	// schema drift that would otherwise reach users only after they cp the
	// file into the active config path.
	cases := []struct {
		name           string
		file           string
		wantProfileKnd string
	}{
		{"default channel", "onvif-simulator.example.json", config.ProfileKindFile},
		{"rpi channel", "onvif-simulator.example.rpi.json", config.ProfileKindRPICam},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, err := os.Open(filepath.Join("..", "..", tc.file))
			if err != nil {
				t.Fatalf("open example: %v", err)
			}
			defer func() {
				if cerr := src.Close(); cerr != nil {
					t.Logf("close src: %v", cerr)
				}
			}()
			dir := t.TempDir()
			t.Chdir(dir)
			dst, err := os.Create(filepath.Join(dir, config.FileName))
			if err != nil {
				t.Fatalf("create dst: %v", err)
			}
			if _, cpErr := io.Copy(dst, src); cpErr != nil {
				t.Fatalf("copy: %v", cpErr)
			}
			if cErr := dst.Close(); cErr != nil {
				t.Fatalf("close dst: %v", cErr)
			}
			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("Load example: %v", err)
			}
			if !cfg.Auth.Enabled || len(cfg.Auth.Users) == 0 {
				t.Fatalf("example should have auth enabled with users")
			}
			if len(cfg.Media.Profiles) == 0 {
				t.Fatalf("example must have at least one media profile")
			}
			for i, p := range cfg.Media.Profiles {
				kind := p.Kind
				if kind == "" {
					kind = config.ProfileKindFile
				}
				if kind != tc.wantProfileKnd {
					t.Fatalf("profile[%d].kind = %q, want %q", i, kind, tc.wantProfileKnd)
				}
			}
		})
	}
}

func TestAuthConfigJSONRoundTrip(t *testing.T) {
	t.Parallel()

	orig := config.AuthConfig{
		Enabled: true,
		Users:   []config.UserConfig{{Username: "a", Password: "b", Role: config.RoleUser}},
		Digest: config.DigestConfig{
			Realm:      "test",
			Algorithms: []string{"MD5"},
			NonceTTL:   "2m",
		},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got config.AuthConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, orig) {
		t.Fatalf("round-trip mismatch:\ngot  %+v\nwant %+v", got, orig)
	}
}
