//go:build !rpicam

package simulator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

// TestStartFailsForRPICamOnDefaultBuild verifies that configuring kind=rpicam
// surfaces a friendly ErrUnsupported on a binary built without the rpicam
// tag (i.e. every channel except cli-rpi). The error must thread through
// the simulator startup so the operator sees one clear diagnostic.
func TestStartFailsForRPICamOnDefaultBuild(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, config.FileName)

	cfg := config.Config{
		Version: config.CurrentVersion,
		Device: config.DeviceConfig{
			UUID:         "urn:uuid:00000000-0000-4000-8000-000000000099",
			Manufacturer: "Test", Model: "RPi", Serial: "SN-99",
		},
		Network: config.NetworkConfig{
			HTTPPort: freePort(t),
			RTSPPort: freePort(t),
		},
		Media: config.MediaConfig{Profiles: []config.ProfileConfig{{
			Name: "main", Token: "profile_main",
			Kind: config.ProfileKindRPICam,
			RPICam: &config.RPICamConfig{
				CameraID: 0, Width: 1920, Height: 1080, FPS: 30,
			},
		}}},
	}
	data, err := json.MarshalIndent(&cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if writeErr := os.WriteFile(cfgPath, data, 0o600); writeErr != nil {
		t.Fatalf("write config: %v", writeErr)
	}
	defer config.SetPath("")

	sim, err := New(Options{EventBufferSize: 16, ConfigPath: cfgPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	startErr := sim.Start(context.Background())
	if startErr == nil {
		_ = sim.Stop(context.Background()) //nolint:errcheck // best-effort during teardown
		t.Fatal("expected Start to fail when kind=rpicam on a non-rpicam build")
	}
	if !strings.Contains(startErr.Error(), "rpicamera") &&
		!strings.Contains(startErr.Error(), "rtsp source") {
		t.Fatalf("expected error to mention rpicamera/rtsp source, got %v", startErr)
	}
}
