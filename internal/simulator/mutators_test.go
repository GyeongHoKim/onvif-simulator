package simulator

import (
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

//nolint:gocyclo,cyclop // sweep test exercises every mutator and asserts emission order.
func TestMutatorRecordsAllKinds(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	var seen []string
	sim.opts.OnMutation = func(m MutationRecord) { seen = append(seen, m.Kind) }

	if err := sim.SetDiscoveryMode(discoveryModeNonDiscoverable); err != nil {
		t.Fatalf("SetDiscoveryMode: %v", err)
	}
	if err := sim.SetHostname("simhost"); err != nil {
		t.Fatalf("SetHostname: %v", err)
	}
	if err := sim.AddProfile(config.ProfileConfig{
		Name: "extra", Token: "extra_tok",
	}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if err := sim.SetProfileMediaFilePath("extra_tok", "/var/onvif/extra.mp4"); err != nil {
		t.Fatalf("SetProfileMediaFilePath: %v", err)
	}
	if err := sim.SetProfileSnapshotURI("extra_tok", "http://127.0.0.1/snap.jpg"); err != nil {
		t.Fatalf("SetProfileSnapshotURI: %v", err)
	}
	if err := sim.SetTopicEnabled("tns1:VideoSource/ImageTooDark", true); err != nil {
		t.Fatalf("SetTopicEnabled: %v", err)
	}
	if err := sim.SetEventsTopics([]config.TopicConfig{
		{Name: "tns1:VideoSource/MotionAlarm", Enabled: true},
	}); err != nil {
		t.Fatalf("SetEventsTopics: %v", err)
	}
	if err := sim.AddUser(config.UserConfig{
		Username: "operator", Password: "pw", Role: config.RoleOperator,
	}); err != nil {
		t.Fatalf("AddUser: %v", err)
	}
	if err := sim.UpsertUser(config.UserConfig{
		Username: "operator", Password: "pw2", Role: config.RoleOperator,
	}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if err := sim.SetAuthEnabled(true); err != nil {
		t.Fatalf("SetAuthEnabled: %v", err)
	}
	if err := sim.SetAuthEnabled(false); err != nil {
		t.Fatalf("SetAuthEnabled false: %v", err)
	}
	if err := sim.RemoveUser("operator"); err != nil {
		t.Fatalf("RemoveUser: %v", err)
	}
	if err := sim.RemoveProfile("extra_tok"); err != nil {
		t.Fatalf("RemoveProfile: %v", err)
	}

	wantKinds := []string{
		"SetDiscoveryMode", "SetHostname", "AddProfile",
		"SetProfileMediaFilePath", "SetProfileSnapshotURI",
		"SetTopicEnabled", "SetEventsTopics",
		"AddUser", "UpsertUser", "SetAuthEnabled", "SetAuthEnabled", "RemoveUser",
		"RemoveProfile",
	}
	if len(seen) != len(wantKinds) {
		t.Fatalf("expected %d mutation records, got %d (%v)", len(wantKinds), len(seen), seen)
	}
	for i, want := range wantKinds {
		if seen[i] != want {
			t.Fatalf("record %d: want %s, got %s", i, want, seen[i])
		}
	}
}

func TestAddUserDuplicateErrors(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	user := config.UserConfig{Username: "alice", Password: "x", Role: config.RoleUser}
	if err := sim.AddUser(user); err != nil {
		t.Fatalf("first AddUser: %v", err)
	}
	if err := sim.AddUser(user); err == nil {
		t.Fatal("expected duplicate user error")
	}
}

func TestSetTopicEnabledMissingErrors(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	if err := sim.SetTopicEnabled("tns1:Nope/Nope", true); err == nil {
		t.Fatal("expected ErrTopicNotFound")
	}
}

func TestSetProfileKindAndRPICam(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	var seen []string
	sim.opts.OnMutation = func(m MutationRecord) { seen = append(seen, m.Kind) }

	if err := sim.AddProfile(config.ProfileConfig{
		Name: "rpi", Token: "rpi_tok",
	}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	rp := &config.RPICamConfig{CameraID: 0, Width: 1920, Height: 1080, FPS: 30}
	if err := sim.SetProfileRPICam("rpi_tok", rp); err != nil {
		t.Fatalf("SetProfileRPICam: %v", err)
	}

	snap := sim.ConfigSnapshot()
	got := findProfile(t, &snap, "rpi_tok")
	if got.Kind != config.ProfileKindRPICam {
		t.Fatalf("kind = %q, want rpicam", got.Kind)
	}
	if got.RPICam == nil || got.RPICam.Width != 1920 {
		t.Fatalf("rpicam params not persisted: %+v", got.RPICam)
	}

	// Flipping to file via SetProfileKind clears the rpicam params.
	if err := sim.SetProfileKind("rpi_tok", config.ProfileKindFile); err != nil {
		t.Fatalf("SetProfileKind file: %v", err)
	}
	snap2 := sim.ConfigSnapshot()
	after := findProfile(t, &snap2, "rpi_tok")
	if after.RPICam != nil {
		t.Fatalf("rpicam should be cleared after kind=file: %+v", after.RPICam)
	}

	if !contains(seen, "SetProfileRPICam") || !contains(seen, "SetProfileKind") {
		t.Fatalf("missing mutation records: %v", seen)
	}
}

func findProfile(t *testing.T, snap *config.Config, token string) config.ProfileConfig {
	t.Helper()
	for i := range snap.Media.Profiles {
		if snap.Media.Profiles[i].Token == token {
			return snap.Media.Profiles[i]
		}
	}
	t.Fatalf("profile %q not in snapshot", token)
	return config.ProfileConfig{}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
