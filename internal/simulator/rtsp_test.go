package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/mediasvc"
)

const fixtureH264Rel = "../rtsp/testdata/short_h264.mp4"

// fixtureH264Abs returns the absolute path to the H264 mp4 fixture committed
// alongside the internal/rtsp package. We rely on the file being present in
// the repository so tests do not need to call ffmpeg.
func fixtureH264Abs(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(fixtureH264Rel)
	if err != nil {
		t.Fatalf("abs fixture path: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("fixture mp4 missing at %s: %v", abs, err)
	}
	return abs
}

// newSimulatorWithMediaFile writes a config whose single profile points at the
// shared H264 fixture and constructs a *Simulator on a free port.
func newSimulatorWithMediaFile(t *testing.T, mediaPath string) (sim *Simulator, cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, config.FileName)

	cfg := config.Config{
		Version: 1,
		Device: config.DeviceConfig{
			UUID:         "urn:uuid:00000000-0000-4000-8000-000000000001",
			Manufacturer: "Test",
			Model:        "SimCam",
			Serial:       "SN-1",
			Scopes:       []string{"onvif://www.onvif.org/name/test"},
		},
		Network: config.NetworkConfig{
			HTTPPort: freePort(t),
			RTSPPort: freePort(t),
		},
		Media: config.MediaConfig{Profiles: []config.ProfileConfig{{
			Name:          "main",
			Token:         "profile_main",
			MediaFilePath: mediaPath,
		}}},
	}
	data, err := json.MarshalIndent(&cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if writeErr := os.WriteFile(cfgPath, data, 0o600); writeErr != nil {
		t.Fatalf("write config: %v", writeErr)
	}

	cleanup = func() { config.SetPath("") }
	s, newErr := New(Options{EventBufferSize: 16, ConfigPath: cfgPath})
	if newErr != nil {
		cleanup()
		t.Fatalf("New: %v", newErr)
	}
	return s, cleanup
}

func TestStartAutoFillsEncoderFromMediaFile(t *testing.T) {
	sim, cleanup := newSimulatorWithMediaFile(t, fixtureH264Abs(t))
	defer cleanup()

	ctx := context.Background()
	if err := sim.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = sim.Stop(ctx) }() //nolint:errcheck // best-effort during teardown

	cfg := sim.ConfigSnapshot()
	if len(cfg.Media.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(cfg.Media.Profiles))
	}
	p := cfg.Media.Profiles[0]
	if p.Encoding != "H264" {
		t.Errorf("Encoding = %q, want H264 (auto-filled)", p.Encoding)
	}
	if p.Width != 320 || p.Height != 240 {
		t.Errorf("dimensions = %dx%d, want 320x240 (auto-filled)", p.Width, p.Height)
	}
	if p.FPS != 15 {
		t.Errorf("FPS = %d, want 15 (auto-filled)", p.FPS)
	}
}

func TestStreamURIUsesEmbeddedServerForMediaFilePath(t *testing.T) {
	sim, cleanup := newSimulatorWithMediaFile(t, fixtureH264Abs(t))
	defer cleanup()

	ctx := context.Background()
	if err := sim.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = sim.Stop(ctx) }() //nolint:errcheck // best-effort during teardown

	uri, err := sim.mediaProv.StreamURI(ctx, "profile_main", mediasvc.StreamSetup{})
	if err != nil {
		t.Fatalf("StreamURI: %v", err)
	}
	rtspPort := sim.ConfigSnapshot().Network.RTSPPortOrDefault()
	wantSuffix := fmt.Sprintf(":%d/profile_main", rtspPort)
	if !strings.HasPrefix(uri.URI, "rtsp://") || !strings.HasSuffix(uri.URI, wantSuffix) {
		t.Errorf("URI = %q, want rtsp://...%s", uri.URI, wantSuffix)
	}
}

func TestStartFailsForMissingMediaFile(t *testing.T) {
	sim, cleanup := newSimulatorWithMediaFile(t, "/nonexistent/sample.mp4")
	defer cleanup()

	ctx := context.Background()
	err := sim.Start(ctx)
	if err == nil {
		_ = sim.Stop(ctx) //nolint:errcheck // best-effort during teardown
		t.Fatal("expected Start to fail for missing media file")
	}
	if !strings.Contains(err.Error(), "rtsp source") {
		t.Errorf("expected error to mention rtsp source, got %v", err)
	}
}
