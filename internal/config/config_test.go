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
			{
				Name:     "main",
				Token:    "profile_main",
				RTSP:     "rtsp://127.0.0.1:8554/main",
				Encoding: "H264",
				Width:    1920,
				Height:   1080,
				FPS:      30,
			},
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
			name:    "no profiles",
			mutate:  func(c *config.Config) { c.Media.Profiles = nil },
			wantErr: config.ErrMediaNoProfiles,
		},
		{
			name: "invalid profile encoding",
			mutate: func(c *config.Config) {
				c.Media.Profiles = []config.ProfileConfig{{
					Name: "main", Token: "tok", RTSP: "rtsp://127.0.0.1:8554/main",
					Encoding: "HEVC", Width: 1920, Height: 1080, FPS: 30,
				}}
			},
			wantErr: config.ErrProfileEncodingInvalid,
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
		return config.ProfileConfig{
			Name: "main", Token: "t", RTSP: "rtsp://127.0.0.1:8554/main",
			Encoding: "H264", Width: 1920, Height: 1080, FPS: 30,
		}
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
		Name: "main", Token: "t", RTSP: "rtsp://127.0.0.1:8554/main",
		Encoding: "H264", Width: 1920, Height: 1080, FPS: 30,
		Bitrate: 4096, GOPLength: 60,
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
				{
					Name:     "main",
					Token:    "profile_main",
					RTSP:     "rtsp://127.0.0.1:8554/main",
					Encoding: "H264",
					Width:    1920,
					Height:   1080,
					FPS:      30,
				},
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
	// Copy the repo example into a temp cwd and ensure it validates.
	src, err := os.Open(filepath.Join("..", "..", "onvif-simulator.example.json"))
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
