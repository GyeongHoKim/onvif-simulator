package tui

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/simulator"
)

func newTestAdapter(t *testing.T) (sa *simulatorAdapter, cleanup func()) {
	t.Helper()
	dir := t.TempDir()

	lc := &net.ListenConfig{}
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	tcpAddr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("expected *net.TCPAddr, got %T", l.Addr())
	}
	port := tcpAddr.Port
	if err = l.Close(); err != nil {
		t.Fatalf("close probe listener: %v", err)
	}

	cfg := config.Config{
		Version: 1,
		Device: config.DeviceConfig{
			UUID:         "urn:uuid:00000000-0000-4000-8000-aaaaaaaaaaaa",
			Manufacturer: "Test",
			Model:        "Sim",
			Serial:       "SN-A1",
		},
		Network: config.NetworkConfig{HTTPPort: port},
		Media: config.MediaConfig{Profiles: []config.ProfileConfig{{
			Name:     "main",
			Token:    "profile_main",
			RTSP:     "rtsp://127.0.0.1:8554/main",
			Encoding: "H264",
			Width:    1920,
			Height:   1080,
			FPS:      30,
		}}},
		Events: config.EventsConfig{
			Topics: []config.TopicConfig{
				{Name: "tns1:VideoSource/MotionAlarm", Enabled: true},
			},
		},
	}
	data, err := json.MarshalIndent(&cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	cfgPath := filepath.Join(dir, config.FileName)
	if err = os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err = os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	cleanup = func() {
		if chErr := os.Chdir(prev); chErr != nil {
			t.Errorf("restore working directory: %v", chErr)
		}
	}

	sim, err := simulator.New(simulator.Options{})
	if err != nil {
		cleanup()
		t.Fatalf("simulator.New: %v", err)
	}
	sa = newSimulatorAdapter(sim)
	return sa, cleanup
}

func TestSimulatorAdapterStatus(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()

	st := a.Status()
	if st.ProfileCount != 1 {
		t.Fatalf("expected 1 profile, got %d", st.ProfileCount)
	}
}

func TestSimulatorAdapterConfigSnapshot(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()

	cfg := a.ConfigSnapshot()
	if cfg.Device.Serial != "SN-A1" {
		t.Fatalf("expected serial SN-A1, got %q", cfg.Device.Serial)
	}
}

func TestSimulatorAdapterUsers(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()

	views := a.Users()
	_ = views
}

func TestSimulatorAdapterStartStop(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()

	if a.Running() {
		t.Fatal("expected not running before Start")
	}
	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !a.Running() {
		t.Fatal("expected running after Start")
	}
	if err := a.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if a.Running() {
		t.Fatal("expected not running after Stop")
	}
}

func TestSimulatorAdapterEventMethods(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()

	a.Motion("VS0", true)
	a.ImageTooBlurry("VS0", false)
	a.ImageTooDark("VS0", false)
	a.ImageTooBright("VS0", false)
	a.DigitalInput("DI0", false)
	a.SyncProperty("tns1:VideoSource/MotionAlarm", "X", "VS0", "State", false)
	a.PublishRaw("tns1:VideoSource/MotionAlarm", "<msg/>")
}

func TestSimulatorAdapterSetDiscoveryMode(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()

	if err := a.SetDiscoveryMode("NonDiscoverable"); err != nil {
		t.Fatalf("SetDiscoveryMode: %v", err)
	}
}

func TestSimulatorAdapterSetHostname(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()

	if err := a.SetHostname("simhost"); err != nil {
		t.Fatalf("SetHostname: %v", err)
	}
}

func TestSimulatorAdapterProfileOps(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()

	p := config.ProfileConfig{
		Name:     "extra",
		Token:    "extra_tok",
		RTSP:     "rtsp://localhost/extra",
		Encoding: "H264",
		Width:    320,
		Height:   240,
		FPS:      15,
	}
	if err := a.AddProfile(p); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if err := a.SetProfileRTSP("extra_tok", "rtsp://localhost/extra2"); err != nil {
		t.Fatalf("SetProfileRTSP: %v", err)
	}
	if err := a.SetProfileSnapshotURI("extra_tok", "http://localhost/snap.jpg"); err != nil {
		t.Fatalf("SetProfileSnapshotURI: %v", err)
	}
	if err := a.SetProfileEncoder("extra_tok", "H264", 640, 480, 30, 1024, 30); err != nil {
		t.Fatalf("SetProfileEncoder: %v", err)
	}
	if err := a.RemoveProfile("extra_tok"); err != nil {
		t.Fatalf("RemoveProfile: %v", err)
	}
}

func TestSimulatorAdapterTopicEnabled(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()

	if err := a.SetTopicEnabled("tns1:VideoSource/MotionAlarm", false); err != nil {
		t.Fatalf("SetTopicEnabled: %v", err)
	}
}

func TestSimulatorAdapterUserOps(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()

	u := config.UserConfig{Username: "user1", Password: "pw", Role: config.RoleUser}
	if err := a.AddUser(u); err != nil {
		t.Fatalf("AddUser: %v", err)
	}
	u.Password = "pw2"
	if err := a.UpsertUser(u); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if err := a.SetAuthEnabled(false); err != nil {
		t.Fatalf("SetAuthEnabled(false): %v", err)
	}
	if err := a.RemoveUser("user1"); err != nil {
		t.Fatalf("RemoveUser: %v", err)
	}
}
