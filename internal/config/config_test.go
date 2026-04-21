package config_test

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	valid := config.Config{
		Version:  config.CurrentVersion,
		MainRTSP: "rtsp://localhost:8554/live",
		SubRTSP:  "rtsp://localhost:8554/live2",
	}
	if err := config.Validate(&valid); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}

	if err := config.Validate(&config.Config{Version: 0, MainRTSP: valid.MainRTSP, SubRTSP: valid.SubRTSP}); err == nil {
		t.Fatal("expected error for wrong version")
	}

	if err := config.Validate(&config.Config{Version: config.CurrentVersion, MainRTSP: "", SubRTSP: valid.SubRTSP}); err == nil {
		t.Fatal("expected error for empty main_rtsp_uri")
	}

	if err := config.Validate(&config.Config{
		Version:  config.CurrentVersion,
		MainRTSP: "http://example.com/x",
		SubRTSP:  valid.SubRTSP,
	}); err == nil {
		t.Fatal("expected error for non-rtsp scheme")
	}

	discoveryOK := config.DiscoveryConfig{
		EndpointAddress: "urn:uuid:11111111-2222-4333-8444-555555555555",
		Types:           []string{"tds:Device"},
		Scopes:          []string{"onvif://www.onvif.org/name/test"},
		XAddrs:          []string{"http://127.0.0.1:8080/onvif/device_service"},
		InstanceID:      1,
		MetadataVersion: 1,
	}
	cfgDisc := config.Config{
		Version:   config.CurrentVersion,
		MainRTSP:  valid.MainRTSP,
		SubRTSP:   valid.SubRTSP,
		Discovery: discoveryOK,
	}
	if err := config.Validate(&cfgDisc); err != nil {
		t.Fatalf("expected valid config with discovery: %v", err)
	}

	if err := config.Validate(&config.Config{
		Version:  config.CurrentVersion,
		MainRTSP: valid.MainRTSP,
		SubRTSP:  valid.SubRTSP,
		Discovery: config.DiscoveryConfig{
			Types: []string{"tds:Device"},
		},
	}); err == nil || !errors.Is(err, config.ErrDiscoveryIncomplete) {
		t.Fatalf("expected ErrDiscoveryIncomplete, got %v", err)
	}

	if err := config.Validate(&config.Config{
		Version:  config.CurrentVersion,
		MainRTSP: valid.MainRTSP,
		SubRTSP:  valid.SubRTSP,
		Discovery: config.DiscoveryConfig{
			EndpointAddress: "not-a-uuid",
			Types:           []string{"tds:Device"},
			Scopes:          []string{"onvif://www.onvif.org/name/x"},
			XAddrs:          []string{"http://127.0.0.1:8080/"},
			InstanceID:      1,
			MetadataVersion: 1,
		},
	}); err == nil || !errors.Is(err, config.ErrDiscoveryEndpointURN) {
		t.Fatalf("expected ErrDiscoveryEndpointURN, got %v", err)
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	want := config.Config{
		Version:  config.CurrentVersion,
		MainRTSP: "rtsp://127.0.0.1:8554/main",
		SubRTSP:  "rtsp://127.0.0.1:8554/sub",
		Discovery: config.DiscoveryConfig{
			EndpointAddress: "urn:uuid:aaaaaaaa-bbbb-4ccc-dddd-eeeeeeeeeeee",
			Types:           []string{"tds:Device"},
			Scopes:          []string{"onvif://www.onvif.org/name/roundtrip"},
			XAddrs:          []string{"https://127.0.0.1:8443/onvif/device_service"},
			InstanceID:      42,
			MetadataVersion: 3,
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
		t.Fatalf("round-trip: got %+v, want %+v", got, want)
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
