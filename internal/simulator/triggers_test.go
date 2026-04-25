package simulator

import (
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

func ensureTopics(t *testing.T, sim *Simulator, names ...string) {
	t.Helper()
	cfg := sim.ConfigSnapshot()
	have := make(map[string]int, len(cfg.Events.Topics))
	for i, tc := range cfg.Events.Topics {
		have[tc.Name] = i
	}
	for _, n := range names {
		if idx, ok := have[n]; ok {
			cfg.Events.Topics[idx].Enabled = true
			continue
		}
		cfg.Events.Topics = append(cfg.Events.Topics, config.TopicConfig{Name: n, Enabled: true})
	}
	if err := sim.SetEventsTopics(cfg.Events.Topics); err != nil {
		t.Fatalf("SetEventsTopics: %v", err)
	}
}

func TestImageTriggers(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	ensureTopics(t,
		sim,
		"tns1:VideoSource/ImageTooBlurry",
		"tns1:VideoSource/ImageTooDark",
		"tns1:VideoSource/ImageTooBright",
		"tns1:Device/Trigger/DigitalInput",
	)

	beforeLen := len(sim.Status().RecentEvents)

	sim.ImageTooBlurry("VS0", true)
	sim.ImageTooDark("VS0", true)
	sim.ImageTooBright("VS0", true)
	sim.DigitalInput("DI0", true)

	got := len(sim.Status().RecentEvents) - beforeLen
	if got != 4 {
		t.Fatalf("expected 4 ring entries, got %d", got)
	}
}

func TestSyncPropertyAndPublishRaw(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	ensureTopics(t, sim, "tns1:Custom/Topic")

	beforeLen := len(sim.Status().RecentEvents)

	sim.SyncProperty("tns1:VideoSource/MotionAlarm", "VideoSourceConfigurationToken", "VS0", "State", true)
	sim.PublishRaw("tns1:Custom/Topic", "<tt:Message/>")
	sim.PublishRaw("tns1:Unknown/Topic", "<tt:Message/>") // not present → no-op

	got := len(sim.Status().RecentEvents) - beforeLen
	if got != 2 {
		t.Fatalf("expected 2 ring entries (sync + raw), got %d", got)
	}
}

func TestSyncPropertyDisabledIsNoOp(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	beforeLen := len(sim.Status().RecentEvents)
	sim.SyncProperty("tns1:VideoSource/ImageTooDark", "X", "Y", "Z", true)
	if len(sim.Status().RecentEvents) != beforeLen {
		t.Fatal("expected no ring entry for disabled topic")
	}
}
