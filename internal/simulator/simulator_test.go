package simulator

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/rtsp"
)

// freePort asks the OS for an unused TCP port (on 127.0.0.1). There is an
// inherent race — another process could claim it before the simulator binds
// — but it is unlikely in the test harness.
func freePort(t *testing.T) int {
	t.Helper()
	lc := &net.ListenConfig{}
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		_ = l.Close() //nolint:errcheck // failing the test takes precedence.
		t.Fatal("probe listener returned non-TCP address")
	}
	if err := l.Close(); err != nil {
		t.Fatalf("probe close: %v", err)
	}
	return addr.Port
}

// newTestSimulator writes a minimal valid config into a temp directory and
// returns a freshly-constructed *Simulator pointed at that file via the
// explicit ConfigPath option. The cleanup hook resets the package-level
// config path so subsequent tests do not see a stale value.
func newTestSimulator(t *testing.T) (sim *Simulator, cleanup func()) {
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
		Network: config.NetworkConfig{HTTPPort: freePort(t)},
		Media: config.MediaConfig{Profiles: []config.ProfileConfig{{
			Name: "main", Token: "profile_main",
			MediaFilePath: "",
			Encoding:      rtsp.CodecH264, Width: 1920, Height: 1080, FPS: 30,
		}}},
		Events: config.EventsConfig{
			Topics: []config.TopicConfig{
				{Name: "tns1:VideoSource/MotionAlarm", Enabled: true},
				{Name: "tns1:VideoSource/ImageTooDark", Enabled: false},
			},
		},
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

func TestNewReadsConfig(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	cfg := sim.ConfigSnapshot()
	if cfg.Device.Serial != "SN-1" {
		t.Fatalf("expected serial SN-1, got %q", cfg.Device.Serial)
	}
}

func TestLifecycleIdempotency(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	ctx := context.Background()
	if err := sim.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !sim.Running() {
		t.Fatal("expected Running=true after Start")
	}
	if err := sim.Start(ctx); err != nil {
		t.Fatalf("second Start: %v", err)
	}

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sim.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if sim.Running() {
		t.Fatal("expected Running=false after Stop")
	}
	if err := sim.Stop(stopCtx); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestMotionUpdatesRing(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	var seen int
	sim.opts.OnEvent = func(_ EventRecord) { seen++ }

	sim.Motion("VS0", true)
	sim.Motion("VS0", false)

	if seen != 2 {
		t.Fatalf("expected 2 OnEvent calls, got %d", seen)
	}
	events := sim.Status().RecentEvents
	if len(events) != 2 {
		t.Fatalf("expected 2 ring entries, got %d", len(events))
	}
	if events[0].Topic != "tns1:VideoSource/MotionAlarm" {
		t.Fatalf("unexpected topic %q", events[0].Topic)
	}
}

func TestDisabledTopicIsNoOp(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	sim.ImageTooDark("VS0", true)
	if len(sim.Status().RecentEvents) != 0 {
		t.Fatalf("expected 0 ring entries for disabled topic")
	}
}

func TestMutatorErrorPropagation(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	if err := sim.RemoveProfile("bogus"); err == nil {
		t.Fatal("expected error for missing profile")
	}
}

func TestMutatorAppliesToSnapshot(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	if err := sim.SetDiscoveryMode(discoveryModeNonDiscoverable); err != nil {
		t.Fatalf("SetDiscoveryMode: %v", err)
	}
	if got := sim.ConfigSnapshot().Runtime.DiscoveryMode; got != discoveryModeNonDiscoverable {
		t.Fatalf("expected NonDiscoverable in snapshot, got %q", got)
	}
}

func TestUsersSnapshotExcludesPasswords(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	if err := sim.UpsertUser(config.UserConfig{
		Username: "admin", Password: "s3cret", Role: config.RoleAdministrator,
	}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	views := sim.Users()
	if len(views) != 1 {
		t.Fatalf("expected 1 user view, got %d", len(views))
	}
	if views[0].Username != "admin" {
		t.Fatalf("expected admin, got %q", views[0].Username)
	}
	if len(views[0].Roles) == 0 {
		t.Fatal("expected at least one role")
	}
}

func TestClampBufferSize(t *testing.T) {
	for _, tc := range []struct {
		in, want int
	}{
		{0, defaultEventBufferSize},
		{1, minEventBufferSize},
		{minEventBufferSize, minEventBufferSize},
		{2048, maxEventBufferSize},
	} {
		if got := clampBufferSize(tc.in); got != tc.want {
			t.Fatalf("clampBufferSize(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestBrokerConfigFromConfigCustomTimeout(t *testing.T) {
	cfg := &config.Config{
		Events: config.EventsConfig{
			SubscriptionTimeout: "2m",
			Topics: []config.TopicConfig{
				{Name: "tns1:VideoSource/MotionAlarm", Enabled: true},
			},
		},
	}
	bc := brokerConfigFromConfig(cfg)
	if bc.SubscriptionTimeout == 0 {
		t.Fatal("expected non-zero custom subscription timeout")
	}
}

func TestBrokerConfigFromConfigInvalidTimeout(t *testing.T) {
	cfg := &config.Config{
		Events: config.EventsConfig{
			SubscriptionTimeout: "not-a-duration",
		},
	}
	bc := brokerConfigFromConfig(cfg)
	// Invalid duration falls back to the event package default (non-zero).
	if bc.SubscriptionTimeout == 0 {
		t.Fatal("expected fallback subscription timeout")
	}
}

func TestBrokerConfigWithAddr(t *testing.T) {
	wantAddr := "http://127.0.0.1:9000/mgr"
	got := brokerConfigWithAddr(brokerConfigFromConfig(&config.Config{}), wantAddr)
	if got.SubscriptionManagerAddr != wantAddr {
		t.Fatalf("brokerConfigWithAddr: got %q, want %q", got.SubscriptionManagerAddr, wantAddr)
	}
}

func TestLocalAddrForXAddr(t *testing.T) {
	addr := localAddrForXAddr()
	if addr == "" {
		t.Fatal("expected non-empty local addr from localAddrForXAddr")
	}
}

func TestCloneConfigNetworkInterfaces(t *testing.T) {
	cfg := &config.Config{
		Runtime: config.RuntimeConfig{
			NetworkInterfaces: []config.NetworkInterfaceConfig{
				{
					Token:   "eth0",
					Enabled: true,
					IPv4: &config.NetworkInterfaceIPv4{
						Enabled: true,
						DHCP:    false,
						Manual:  []string{"192.168.1.10/24"},
					},
				},
			},
		},
	}
	out := cloneConfig(cfg)
	if len(out.Runtime.NetworkInterfaces) != 1 {
		t.Fatal("expected 1 interface in clone")
	}
	if out.Runtime.NetworkInterfaces[0].IPv4 == nil {
		t.Fatal("expected IPv4 config preserved in clone")
	}
	out.Runtime.NetworkInterfaces[0].IPv4.Manual[0] = "10.0.0.1/8"
	if cfg.Runtime.NetworkInterfaces[0].IPv4.Manual[0] != "192.168.1.10/24" {
		t.Fatal("clone should not share slice backing with original")
	}
}

func TestCloneConfigNilSlices(t *testing.T) {
	cfg := &config.Config{}
	out := cloneConfig(cfg)
	if out.Device.Scopes != nil {
		t.Fatal("nil Scopes should remain nil after clone")
	}
	if out.Network.XAddrs != nil {
		t.Fatal("nil XAddrs should remain nil after clone")
	}
}

func TestStatusTopicCountOnlyEnabled(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	st := sim.Status()
	// Config has MotionAlarm (enabled) and ImageTooDark (disabled) → 1 enabled.
	if st.TopicCount != 1 {
		t.Fatalf("expected 1 enabled topic in Status, got %d", st.TopicCount)
	}
}
