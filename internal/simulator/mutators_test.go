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
