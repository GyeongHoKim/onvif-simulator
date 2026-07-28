package simulator

import (
	"context"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/ptzsvc"
)

func TestEPTZState_GetStatus_InitialState(t *testing.T) {
	st := newEPTZState()
	status := st.getStatus()
	if status.Position == nil || status.Position.PanTilt == nil || status.Position.Zoom == nil {
		t.Fatal("initial position should not be nil")
	}
	if status.Position.PanTilt.X != 0 || status.Position.PanTilt.Y != 0 {
		t.Fatalf("initial pan/tilt should be (0,0), got (%f,%f)",
			status.Position.PanTilt.X, status.Position.PanTilt.Y)
	}
	if status.Position.Zoom.X != 0 {
		t.Fatalf("initial zoom should be 0, got %f", status.Position.Zoom.X)
	}
	if status.MoveStatus == nil || status.MoveStatus.PanTilt != "IDLE" {
		t.Fatal("initial move status should be IDLE")
	}
}

func TestEPTZState_AbsoluteMove(t *testing.T) {
	st := newEPTZState()
	st.absoluteMove(ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0.5, Y: -0.3},
		Zoom:    &ptzsvc.Zoom{X: 0.8},
	})
	status := st.getStatus()
	if status.Position.PanTilt.X != 0.5 {
		t.Fatalf("want pan=0.5, got %f", status.Position.PanTilt.X)
	}
	if status.Position.PanTilt.Y != -0.3 {
		t.Fatalf("want tilt=-0.3, got %f", status.Position.PanTilt.Y)
	}
	if status.Position.Zoom.X != 0.8 {
		t.Fatalf("want zoom=0.8, got %f", status.Position.Zoom.X)
	}
}

func TestEPTZState_AbsoluteMove_Clamp(t *testing.T) {
	st := newEPTZState()
	st.absoluteMove(ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 5.0, Y: -5.0},
		Zoom:    &ptzsvc.Zoom{X: 2.0},
	})
	status := st.getStatus()
	if status.Position.PanTilt.X != 1.0 {
		t.Fatalf("want pan clamped to 1.0, got %f", status.Position.PanTilt.X)
	}
	if status.Position.PanTilt.Y != -1.0 {
		t.Fatalf("want tilt clamped to -1.0, got %f", status.Position.PanTilt.Y)
	}
	if status.Position.Zoom.X != 1.0 {
		t.Fatalf("want zoom clamped to 1.0, got %f", status.Position.Zoom.X)
	}
}

func TestEPTZState_RelativeMove(t *testing.T) {
	st := newEPTZState()
	st.absoluteMove(ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0.3, Y: 0.3},
	})
	st.relativeMove(ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0.1, Y: -0.2},
	})
	status := st.getStatus()
	if !floatEq(status.Position.PanTilt.X, 0.4) {
		t.Fatalf("want pan=0.4, got %f", status.Position.PanTilt.X)
	}
	if !floatEq(status.Position.PanTilt.Y, 0.1) {
		t.Fatalf("want tilt=0.1, got %f", status.Position.PanTilt.Y)
	}
}

func floatEq(a, b float64) bool {
	const eps = 1e-9
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}

func TestEPTZState_ContinuousMove(t *testing.T) {
	st := newEPTZState()
	st.continuousMove(ptzsvc.Speed{
		PanTilt: &ptzsvc.PanTilt{X: 0.5, Y: 0.5},
		Zoom:    &ptzsvc.Zoom{X: 0.1},
	})
	status := st.getStatus()
	if status.Position.PanTilt.X != 0.5 {
		t.Fatalf("want pan=0.5, got %f", status.Position.PanTilt.X)
	}
	if status.Position.Zoom.X != 0.1 {
		t.Fatalf("want zoom=0.1, got %f", status.Position.Zoom.X)
	}
}

func TestEPTZState_Stop(t *testing.T) {
	t.Helper()
	st := newEPTZState()
	pt := true
	zm := true
	st.stop(&pt, &zm)
	// Stop is a no-op in simulator; just verify no panic.
}

func TestEPTZState_SetAndGetPresets(t *testing.T) {
	st := newEPTZState()
	st.absoluteMove(ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0.7, Y: 0.2},
	})
	token := st.setPreset(strPtr("Home"), nil)
	if token != "preset_1" {
		t.Fatalf("want preset_1, got %s", token)
	}
	presets := st.getPresets()
	if len(presets) != 1 {
		t.Fatalf("want 1 preset, got %d", len(presets))
	}
	if presets[0].Name != "Home" {
		t.Fatalf("want name Home, got %s", presets[0].Name)
	}
	if presets[0].Position == nil || presets[0].Position.PanTilt.X != 0.7 {
		t.Fatalf("preset position not captured correctly")
	}
}

func TestEPTZState_SetPreset_CustomToken(t *testing.T) {
	st := newEPTZState()
	customToken := "my_preset"
	token := st.setPreset(nil, &customToken)
	if token != "my_preset" {
		t.Fatalf("want my_preset, got %s", token)
	}
}

func TestEPTZState_RemovePreset(t *testing.T) {
	st := newEPTZState()
	st.setPreset(strPtr("Test"), nil)
	if err := st.removePreset("preset_1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	presets := st.getPresets()
	if len(presets) != 0 {
		t.Fatalf("want 0 presets after removal, got %d", len(presets))
	}
}

func TestEPTZState_RemovePreset_NotFound(t *testing.T) {
	st := newEPTZState()
	if err := st.removePreset("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent preset")
	}
}

func TestEPTZState_GotoPreset(t *testing.T) {
	st := newEPTZState()
	st.absoluteMove(ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0.9, Y: 0.9},
	})
	st.setPreset(strPtr("Corner"), nil)
	st.absoluteMove(ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0, Y: 0},
	})
	if err := st.gotoPreset("preset_1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status := st.getStatus()
	if status.Position.PanTilt.X != 0.9 {
		t.Fatalf("want pan=0.9 after goto, got %f", status.Position.PanTilt.X)
	}
}

func TestEPTZState_GotoPreset_NotFound(t *testing.T) {
	st := newEPTZState()
	if err := st.gotoPreset("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent preset")
	}
}

func TestEPTZState_GotoHomePosition(t *testing.T) {
	st := newEPTZState()
	st.absoluteMove(ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0.5, Y: 0.5},
	})
	st.setHomePosition()
	st.absoluteMove(ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: -0.5, Y: -0.5},
	})
	st.gotoHomePosition()
	status := st.getStatus()
	if status.Position.PanTilt.X != 0.5 {
		t.Fatalf("want pan=0.5 after goto home, got %f", status.Position.PanTilt.X)
	}
}

func TestEPTZState_SetHomePosition(t *testing.T) {
	st := newEPTZState()
	st.absoluteMove(ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0.3, Y: -0.3},
		Zoom:    &ptzsvc.Zoom{X: 0.5},
	})
	st.setHomePosition()
	st.absoluteMove(ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0, Y: 0},
		Zoom:    &ptzsvc.Zoom{X: 0},
	})
	st.gotoHomePosition()
	status := st.getStatus()
	if status.Position.PanTilt.X != 0.3 || status.Position.Zoom.X != 0.5 {
		t.Fatalf("home position not set correctly: %+v", status.Position)
	}
}

func TestPTZProvider_GetStatus(t *testing.T) {
	sim := &Simulator{}
	prov := newPTZProvider(sim)
	status, err := prov.GetStatus(context.Background(), "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Position == nil {
		t.Fatal("position should not be nil")
	}
}

func TestPTZProvider_GetNodes(t *testing.T) {
	sim := &Simulator{}
	prov := newPTZProvider(sim)
	nodes, err := prov.GetNodes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	if nodes[0].Token != defaultPTZNodeToken {
		t.Fatalf("want token %s, got %s", defaultPTZNodeToken, nodes[0].Token)
	}
}

func TestPTZProvider_GetConfigurations(t *testing.T) {
	sim := &Simulator{}
	prov := newPTZProvider(sim)
	cfgs, err := prov.GetConfigurations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfgs) != 1 {
		t.Fatalf("want 1 config, got %d", len(cfgs))
	}
}

func TestPTZProvider_GetConfigurationOptions(t *testing.T) {
	sim := &Simulator{}
	prov := newPTZProvider(sim)
	opts, err := prov.GetConfigurationOptions(context.Background(), defaultPTZConfigToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts.PanTiltPositionSpaceRange) == 0 {
		t.Fatal("want at least one PanTilt space range")
	}
}

func TestPTZProvider_Presets(t *testing.T) {
	sim := &Simulator{}
	prov := newPTZProvider(sim)
	ctx := context.Background()

	presets, err := prov.GetPresets(ctx, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(presets) != 0 {
		t.Fatalf("want 0 presets initially, got %d", len(presets))
	}

	token, err := prov.SetPreset(ctx, "main", strPtr("Home"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "preset_1" {
		t.Fatalf("want preset_1, got %s", token)
	}

	presets, err = prov.GetPresets(ctx, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(presets) != 1 {
		t.Fatalf("want 1 preset, got %d", len(presets))
	}

	removeErr := prov.RemovePreset(ctx, "main", token)
	if removeErr != nil {
		t.Fatalf("unexpected error: %v", removeErr)
	}
	presets, err = prov.GetPresets(ctx, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(presets) != 0 {
		t.Fatalf("want 0 presets after removal, got %d", len(presets))
	}
}

func TestPTZProvider_Move(t *testing.T) {
	sim := &Simulator{}
	prov := newPTZProvider(sim)
	ctx := context.Background()

	// Absolute move
	if err := prov.AbsoluteMove(ctx, "main", ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0.5, Y: -0.5},
		Zoom:    &ptzsvc.Zoom{X: 0.3},
	}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, err := prov.GetStatus(ctx, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Position.PanTilt.X != 0.5 {
		t.Fatalf("want pan=0.5, got %f", status.Position.PanTilt.X)
	}

	// Relative move
	if relErr := prov.RelativeMove(ctx, "main", ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0.1, Y: 0.1},
	}, nil); relErr != nil {
		t.Fatalf("unexpected error: %v", relErr)
	}
	status, err = prov.GetStatus(ctx, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Position.PanTilt.X != 0.6 {
		t.Fatalf("want pan=0.6, got %f", status.Position.PanTilt.X)
	}

	// Continuous move
	if contErr := prov.ContinuousMove(ctx, "main", ptzsvc.Speed{
		PanTilt: &ptzsvc.PanTilt{X: 0.1, Y: 0.1},
	}, nil); contErr != nil {
		t.Fatalf("unexpected error: %v", contErr)
	}

	// Stop
	pt := true
	zm := true
	if stopErr := prov.Stop(ctx, "main", &pt, &zm); stopErr != nil {
		t.Fatalf("unexpected error: %v", stopErr)
	}
}

func TestPTZProvider_GotoPreset(t *testing.T) {
	sim := &Simulator{}
	prov := newPTZProvider(sim)
	ctx := context.Background()

	if err := prov.AbsoluteMove(ctx, "main", ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0.8, Y: 0.8},
	}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := prov.SetPreset(ctx, "main", strPtr("Corner"), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := prov.AbsoluteMove(ctx, "main", ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0, Y: 0},
	}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := prov.GotoPreset(ctx, "main", "preset_1", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, err := prov.GetStatus(ctx, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Position.PanTilt.X != 0.8 {
		t.Fatalf("want pan=0.8, got %f", status.Position.PanTilt.X)
	}
}

func TestPTZProvider_GotoHomePosition(t *testing.T) {
	sim := &Simulator{}
	prov := newPTZProvider(sim)
	ctx := context.Background()

	if err := prov.AbsoluteMove(ctx, "main", ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0.5, Y: 0.5},
	}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := prov.SetHomePosition(ctx, "main"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := prov.AbsoluteMove(ctx, "main", ptzsvc.Vector{
		PanTilt: &ptzsvc.PanTilt{X: 0, Y: 0},
	}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := prov.GotoHomePosition(ctx, "main", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status, err := prov.GetStatus(ctx, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Position.PanTilt.X != 0.5 {
		t.Fatalf("want pan=0.5, got %f", status.Position.PanTilt.X)
	}
}

func TestPTZProvider_ServiceCapabilities(t *testing.T) {
	sim := &Simulator{}
	prov := newPTZProvider(sim)
	caps, err := prov.ServiceCapabilities(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !caps.MoveStatus || !caps.StatusPosition {
		t.Fatal("expected MoveStatus and StatusPosition to be true")
	}
}

// strPtr is a test helper.
func strPtr(s string) *string { return &s }
