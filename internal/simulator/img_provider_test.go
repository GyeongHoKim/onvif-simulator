package simulator

import (
	"context"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/imgsvc"
)

func TestImagingState_GetSettings_Default(t *testing.T) {
	st := newImagingState()
	settings := st.getSettings("vs1")
	if settings.Brightness == nil || *settings.Brightness != 0.5 {
		t.Fatal("default brightness should be 0.5")
	}
	if settings.Contrast == nil || *settings.Contrast != 0.5 {
		t.Fatal("default contrast should be 0.5")
	}
}

func TestImagingState_SetSettings(t *testing.T) {
	st := newImagingState()
	brightness := 0.8
	settings := imgsvc.Settings{Brightness: &brightness}
	st.setSettings("vs1", settings)
	got := st.getSettings("vs1")
	if got.Brightness == nil || *got.Brightness != 0.8 {
		t.Fatalf("want brightness=0.8, got %v", got.Brightness)
	}
}

func TestImagingState_Presets(t *testing.T) {
	st := newImagingState()
	st.mu.Lock()
	st.presets["vs1"] = []imgsvc.Preset{
		{Token: "clear", Name: "Clear Weather", Type: "ClearWeather"},
		{Token: "night", Name: "Night", Type: "Night"},
	}
	st.mu.Unlock()

	presets := st.getPresets("vs1")
	if len(presets) != 2 {
		t.Fatalf("want 2 presets, got %d", len(presets))
	}

	// No current preset initially.
	current := st.getCurrentPreset("vs1")
	if current != nil {
		t.Fatal("expected nil current preset initially")
	}

	// Set current preset.
	if err := st.setCurrentPreset("vs1", "night"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	current = st.getCurrentPreset("vs1")
	if current == nil || current.Token != "night" {
		t.Fatalf("want current=night, got %v", current)
	}

	// Invalid preset token.
	if err := st.setCurrentPreset("vs1", "nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent preset")
	}
}

func TestImagingProvider_GetSettings(t *testing.T) {
	sim := &Simulator{}
	prov := newImagingProvider(sim)
	settings, err := prov.GetImagingSettings(context.Background(), "vs1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.Brightness == nil {
		t.Fatal("brightness should not be nil")
	}
}

func TestImagingProvider_SetSettings(t *testing.T) {
	sim := &Simulator{}
	prov := newImagingProvider(sim)
	brightness := 0.9
	settings := imgsvc.Settings{Brightness: &brightness}
	if err := prov.SetImagingSettings(context.Background(), "vs1", settings, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := prov.GetImagingSettings(context.Background(), "vs1")
	if got.Brightness == nil || *got.Brightness != 0.9 {
		t.Fatalf("want brightness=0.9, got %v", got.Brightness)
	}
}

func TestImagingProvider_GetOptions(t *testing.T) {
	sim := &Simulator{}
	prov := newImagingProvider(sim)
	opts, err := prov.GetOptions(context.Background(), "vs1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Brightness.Max != 1.0 {
		t.Fatalf("want max=1.0, got %f", opts.Brightness.Max)
	}
	if len(opts.ExposureModes) != 2 {
		t.Fatalf("want 2 exposure modes, got %d", len(opts.ExposureModes))
	}
}

func TestImagingProvider_GetStatus(t *testing.T) {
	sim := &Simulator{}
	prov := newImagingProvider(sim)
	status, err := prov.GetStatus(context.Background(), "vs1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.FocusStatus == nil || status.FocusStatus.Position != "IDLE" {
		t.Fatal("expected IDLE focus status")
	}
}

func TestImagingProvider_Presets(t *testing.T) {
	sim := &Simulator{}
	prov := newImagingProvider(sim)
	ctx := context.Background()

	// Manually register presets for testing (simulator has no profiles).
	prov.state.mu.Lock()
	prov.state.presets["vs1"] = []imgsvc.Preset{
		{Token: "clear", Name: "Clear Weather", Type: "ClearWeather"},
		{Token: "night", Name: "Night", Type: "Night"},
	}
	prov.state.mu.Unlock()

	presets, err := prov.GetPresets(ctx, "vs1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(presets) != 2 {
		t.Fatalf("want 2 presets, got %d", len(presets))
	}

	current, err := prov.GetCurrentPreset(ctx, "vs1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if current != nil {
		t.Fatal("expected nil current preset initially")
	}

	if err := prov.SetCurrentPreset(ctx, "vs1", presets[0].Token); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	current, err = prov.GetCurrentPreset(ctx, "vs1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if current == nil || current.Token != presets[0].Token {
		t.Fatalf("want current=%s, got %v", presets[0].Token, current)
	}
}

func TestImagingProvider_ServiceCapabilities(t *testing.T) {
	sim := &Simulator{}
	prov := newImagingProvider(sim)
	caps, err := prov.ServiceCapabilities(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caps.ImageStabilization {
		t.Fatal("expected ImageStabilization=false")
	}
}
