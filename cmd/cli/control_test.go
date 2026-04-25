package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/simulator"
)

// chdirToTempConfig writes a minimal valid config into a temp dir and chdirs
// there so simulator.New can find it.
func chdirToTempConfig(t *testing.T) (cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		Version: 1,
		Device: config.DeviceConfig{
			UUID:         "urn:uuid:00000000-0000-4000-8000-000000000001",
			Manufacturer: "Test", Model: "SimCam", Serial: "SN-1",
			Scopes: []string{"onvif://www.onvif.org/name/test"},
		},
		Network: config.NetworkConfig{HTTPPort: 18081},
		Media: config.MediaConfig{Profiles: []config.ProfileConfig{{
			Name: "main", Token: "profile_main",
			RTSP:     "rtsp://127.0.0.1:8554/main",
			Encoding: "H264", Width: 1920, Height: 1080, FPS: 30,
		}}},
		Events: config.EventsConfig{
			Topics: []config.TopicConfig{
				{Name: "tns1:VideoSource/MotionAlarm", Enabled: true},
				{Name: "tns1:VideoSource/ImageTooBlurry", Enabled: true},
				{Name: "tns1:VideoSource/ImageTooDark", Enabled: true},
				{Name: "tns1:VideoSource/ImageTooBright", Enabled: true},
				{Name: "tns1:Device/Trigger/DigitalInput", Enabled: true},
			},
		},
	}
	data, err := json.MarshalIndent(&cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if writeErr := os.WriteFile(filepath.Join(dir, config.FileName), data, 0o600); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}
	prev, getErr := os.Getwd()
	if getErr != nil {
		t.Fatalf("getwd: %v", getErr)
	}
	if chErr := os.Chdir(dir); chErr != nil {
		t.Fatalf("chdir: %v", chErr)
	}
	return func() { _ = os.Chdir(prev) } //nolint:errcheck // best-effort restore.
}

func TestControlServerEventsAndShutdown(t *testing.T) {
	cleanup := chdirToTempConfig(t)
	defer cleanup()

	sim, err := simulator.New(simulator.Options{EventBufferSize: 16})
	if err != nil {
		t.Fatalf("simulator.New: %v", err)
	}
	ctrl, err := startControlServer(sim)
	if err != nil {
		t.Fatalf("startControlServer: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = ctrl.shutdown(ctx) //nolint:errcheck // shutdown error is non-fatal in tests.
	}()

	cases := []struct {
		path string
		body any
	}{
		{"/events/motion", tokenState{Token: "VS0", State: true}},
		{"/events/digital-input", tokenState{Token: "DI0", State: true}},
		{"/events/image-too-blurry", tokenState{Token: "VS0", State: true}},
		{"/events/image-too-dark", tokenState{Token: "VS0", State: false}},
		{"/events/image-too-bright", tokenState{Token: "VS0", State: true}},
		{"/events/sync", syncRequest{
			Topic: "tns1:VideoSource/MotionAlarm", SourceItemName: "VideoSourceConfigurationToken",
			SourceToken: "VS0", DataItemName: "State", State: true,
		}},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			data, err := json.Marshal(c.body)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
				"http://127.0.0.1:"+strconv.Itoa(ctrl.port)+c.path, bytes.NewReader(data))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do: %v", err)
			}
			_ = resp.Body.Close() //nolint:errcheck // not asserting body.
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("status=%d, want 204", resp.StatusCode)
			}
		})
	}
}

func TestControlServerRejectsGET(t *testing.T) {
	cleanup := chdirToTempConfig(t)
	defer cleanup()

	sim, err := simulator.New(simulator.Options{})
	if err != nil {
		t.Fatalf("simulator.New: %v", err)
	}
	ctrl, err := startControlServer(sim)
	if err != nil {
		t.Fatalf("startControlServer: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = ctrl.shutdown(ctx) //nolint:errcheck // shutdown error is non-fatal in tests.
	}()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"http://127.0.0.1:"+strconv.Itoa(ctrl.port)+"/events/motion", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // close error is non-actionable.
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d, want 405", resp.StatusCode)
	}
}

func TestControlServerRejectsBadJSON(t *testing.T) {
	cleanup := chdirToTempConfig(t)
	defer cleanup()

	sim, err := simulator.New(simulator.Options{})
	if err != nil {
		t.Fatalf("simulator.New: %v", err)
	}
	ctrl, err := startControlServer(sim)
	if err != nil {
		t.Fatalf("startControlServer: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = ctrl.shutdown(ctx) //nolint:errcheck // shutdown error is non-fatal in tests.
	}()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"http://127.0.0.1:"+strconv.Itoa(ctrl.port)+"/events/motion", strings.NewReader("not json"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // close error is non-actionable.
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", resp.StatusCode)
	}
}

func TestControlPortFileRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	path, err := writeControlPortFile(54321)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.HasPrefix(path, tmp) {
		t.Fatalf("expected port file under HOME, got %s", path)
	}

	port, err := readControlPortFile()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if port != 54321 {
		t.Fatalf("expected 54321, got %d", port)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := readControlPortFile(); err == nil {
		t.Fatal("expected error reading missing port file")
	}
}
