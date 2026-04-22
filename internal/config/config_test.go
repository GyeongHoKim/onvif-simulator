package config_test

import (
	"errors"
	"os"
	"reflect"
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

func TestValidate(t *testing.T) {
	t.Parallel()

	if err := config.Validate(&validConfig); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}

	t.Run("wrong version", func(t *testing.T) {
		t.Parallel()
		c := validConfig
		c.Version = 0
		if err := config.Validate(&c); !errors.Is(err, config.ErrInvalidVersion) {
			t.Fatalf("expected ErrInvalidVersion, got %v", err)
		}
	})

	t.Run("missing device uuid", func(t *testing.T) {
		t.Parallel()
		c := validConfig
		c.Device.UUID = ""
		if err := config.Validate(&c); !errors.Is(err, config.ErrDeviceUUIDRequired) {
			t.Fatalf("expected ErrDeviceUUIDRequired, got %v", err)
		}
	})

	t.Run("invalid device uuid", func(t *testing.T) {
		t.Parallel()
		c := validConfig
		c.Device.UUID = "not-a-uuid"
		if err := config.Validate(&c); !errors.Is(err, config.ErrDeviceUUIDInvalid) {
			t.Fatalf("expected ErrDeviceUUIDInvalid, got %v", err)
		}
	})

	t.Run("invalid http port", func(t *testing.T) {
		t.Parallel()
		c := validConfig
		c.Network.HTTPPort = 0
		if err := config.Validate(&c); !errors.Is(err, config.ErrNetworkPortInvalid) {
			t.Fatalf("expected ErrNetworkPortInvalid, got %v", err)
		}
	})

	t.Run("no profiles", func(t *testing.T) {
		t.Parallel()
		c := validConfig
		c.Media.Profiles = nil
		if err := config.Validate(&c); !errors.Is(err, config.ErrMediaNoProfiles) {
			t.Fatalf("expected ErrMediaNoProfiles, got %v", err)
		}
	})

	t.Run("invalid profile encoding", func(t *testing.T) {
		t.Parallel()
		c := validConfig
		c.Media.Profiles = []config.ProfileConfig{{
			Name:     "main",
			Token:    "tok",
			RTSP:     "rtsp://127.0.0.1:8554/main",
			Encoding: "HEVC",
			Width:    1920,
			Height:   1080,
			FPS:      30,
		}}
		if err := config.Validate(&c); !errors.Is(err, config.ErrProfileEncodingInvalid) {
			t.Fatalf("expected ErrProfileEncodingInvalid, got %v", err)
		}
	})

	t.Run("auth incomplete", func(t *testing.T) {
		t.Parallel()
		c := validConfig
		c.Auth = config.AuthConfig{Username: "admin"}
		if err := config.Validate(&c); !errors.Is(err, config.ErrAuthIncomplete) {
			t.Fatalf("expected ErrAuthIncomplete, got %v", err)
		}
	})
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
				{
					Name:     "sub",
					Token:    "profile_sub",
					RTSP:     "rtsp://127.0.0.1:8554/sub",
					Encoding: "H264",
					Width:    640,
					Height:   480,
					FPS:      15,
				},
			},
		},
		Auth: config.AuthConfig{
			Username: "admin",
			Password: "secret",
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
