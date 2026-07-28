package simulator

import (
	"context"
	"fmt"
	"sync"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/imgsvc"
)

var imgDefaultMode = "AUTO"

var (
	defaultBrightnessRange = imgsvc.FloatRange{Min: 0.0, Max: 1.0}
	defaultContrastRange   = imgsvc.FloatRange{Min: 0.0, Max: 1.0}
	defaultSharpnessRange  = imgsvc.FloatRange{Min: 0.0, Max: 1.0}
)

// imagingState holds per-video-source imaging settings.
type imagingState struct {
	mu       sync.Mutex
	settings map[string]*imgsvc.Settings // keyed by VideoSourceToken
	presets  map[string][]imgsvc.Preset  // keyed by VideoSourceToken
	current  map[string]string           // VideoSourceToken → current preset token
}

func newImagingState() *imagingState {
	return &imagingState{
		settings: make(map[string]*imgsvc.Settings),
		presets:  make(map[string][]imgsvc.Preset),
		current:  make(map[string]string),
	}
}

func (s *imagingState) getSettings(vst string) imgsvc.Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.settings[vst]; ok {
		return *st
	}
	return defaultImagingSettings()
}

func (s *imagingState) setSettings(vst string, settings *imgsvc.Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[vst] = settings
}

func (s *imagingState) getPresets(vst string) []imgsvc.Preset {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.presets[vst]
}

func (s *imagingState) getCurrentPreset(vst string) *imgsvc.Preset {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.current[vst]
	if !ok {
		return nil
	}
	for i := range s.presets[vst] {
		if s.presets[vst][i].Token == token {
			return &s.presets[vst][i]
		}
	}
	return nil
}

func (s *imagingState) setCurrentPreset(vst, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.presets[vst] {
		if s.presets[vst][i].Token == token {
			s.current[vst] = token
			return nil
		}
	}
	return fmt.Errorf("%w: %s", imgsvc.ErrPresetNotFound, token)
}

func defaultImagingSettings() imgsvc.Settings {
	brightness := 0.5
	contrast := 0.5
	sharpness := 0.5
	expMode := imgDefaultMode
	wbMode := imgDefaultMode
	irFilter := imgDefaultMode
	focusMode := imgDefaultMode
	return imgsvc.Settings{
		Brightness:       &brightness,
		Contrast:         &contrast,
		Sharpness:        &sharpness,
		ExposureMode:     &expMode,
		WhiteBalanceMode: &wbMode,
		IrCutFilter:      &irFilter,
		FocusMode:        &focusMode,
	}
}

// imgProvider implements imgsvc.Provider for the simulator.
type imgProvider struct {
	sim   *Simulator
	state *imagingState
}

func newImagingProvider(s *Simulator) *imgProvider {
	prov := &imgProvider{
		sim:   s,
		state: newImagingState(),
	}
	// Register default imaging presets for all video sources.
	prov.registerDefaultPresets()
	return prov
}

func (p *imgProvider) registerDefaultPresets() {
	presets := []imgsvc.Preset{
		{Token: "preset_clear_weather", Name: "Clear Weather", Type: "ClearWeather"},
		{Token: "preset_night", Name: "Night", Type: "Night"},
		{Token: "preset_indoor", Name: "Indoor", Type: "Indoor"},
	}
	cfg := p.sim.snapshotConfig()
	for i := range cfg.Media.Profiles {
		vst := cfg.Media.Profiles[i].VideoSourceToken
		if vst == "" {
			vst = cfg.Media.Profiles[i].Token
		}
		p.state.mu.Lock()
		p.state.presets[vst] = presets
		p.state.mu.Unlock()
	}
}

func (*imgProvider) ServiceCapabilities(_ context.Context) (imgsvc.ServiceCapabilities, error) {
	return imgsvc.ServiceCapabilities{ImageStabilization: false}, nil
}

func (p *imgProvider) GetImagingSettings(
	_ context.Context, videoSourceToken string,
) (imgsvc.Settings, error) {
	return p.state.getSettings(videoSourceToken), nil
}

func (p *imgProvider) SetImagingSettings(
	_ context.Context, videoSourceToken string, settings *imgsvc.Settings, _ *bool,
) error {
	p.state.setSettings(videoSourceToken, settings)
	return nil
}

func (*imgProvider) GetOptions(_ context.Context, _ string) (imgsvc.Options, error) {
	return imgsvc.Options{
		Brightness:            defaultBrightnessRange,
		Contrast:              defaultContrastRange,
		Sharpness:             defaultSharpnessRange,
		ExposureModes:         []string{"AUTO", "MANUAL"},
		ExposurePriorities:    []string{"LowNoise", "FrameRate"},
		WhiteBalanceModes:     []string{"AUTO", "MANUAL"},
		IrCutFilterModes:      []string{"ON", "OFF", "AUTO"},
		BacklightCompModes:    []string{"OFF", "ON"},
		WideDynamicRangeModes: []string{"OFF", "ON"},
		FocusModes:            []string{"AUTO", "MANUAL"},
	}, nil
}

func (*imgProvider) GetStatus(_ context.Context, _ string) (imgsvc.Status, error) {
	return imgsvc.Status{
		FocusStatus: &imgsvc.FocusStatus{
			Position: "IDLE",
		},
	}, nil
}

func (p *imgProvider) GetPresets(
	_ context.Context, videoSourceToken string,
) ([]imgsvc.Preset, error) {
	return p.state.getPresets(videoSourceToken), nil
}

func (p *imgProvider) GetCurrentPreset(
	_ context.Context, videoSourceToken string,
) (*imgsvc.Preset, error) {
	return p.state.getCurrentPreset(videoSourceToken), nil
}

func (p *imgProvider) SetCurrentPreset(
	_ context.Context, videoSourceToken, presetToken string,
) error {
	return p.state.setCurrentPreset(videoSourceToken, presetToken)
}
