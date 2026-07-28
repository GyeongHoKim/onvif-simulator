package simulator

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/ptzsvc"
)

const (
	defaultPTZNodeToken    = "ePTZ_node"
	defaultPTZNodeName     = "ePTZ Digital Node"
	defaultPTZConfigToken  = "ePTZ_config"
	defaultPTZConfigName   = "ePTZ Configuration"
	defaultPTZHomeToken    = "ePTZ_home"
	defaultPTZProfileToken = "profile_main" // fallback when no profile specified

	// ePTZ coordinate ranges.
	ptzMinPan  = -1.0
	ptzMaxPan  = 1.0
	ptzMinTilt = -1.0
	ptzMaxTilt = 1.0
	ptzMinZoom = 0.0
	ptzMaxZoom = 1.0
)

// ePTZState holds the virtual PTZ position for one profile.
type ePTZState struct {
	mu       sync.Mutex
	position ptzsvc.Vector
	home     ptzsvc.Vector
	presets  map[string]ptzPreset
	nextID   int
}

type ptzPreset struct {
	token    string
	name     string
	position *ptzsvc.Vector
}

func newEPTZState() *ePTZState {
	return &ePTZState{
		position: ptzsvc.Vector{
			PanTilt: &ptzsvc.PanTilt{X: 0, Y: 0},
			Zoom:    &ptzsvc.Zoom{X: 0},
		},
		home: ptzsvc.Vector{
			PanTilt: &ptzsvc.PanTilt{X: 0, Y: 0},
			Zoom:    &ptzsvc.Zoom{X: 0},
		},
		presets: make(map[string]ptzPreset),
		nextID:  1,
	}
}

func (s *ePTZState) getStatus() ptzsvc.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := copyVector(&s.position)
	return ptzsvc.Status{
		Position: &pos,
		MoveStatus: &ptzsvc.MoveStatus{
			PanTilt: "IDLE",
			Zoom:    "IDLE",
		},
	}
}

func (s *ePTZState) continuousMove(vel ptzsvc.Speed) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if vel.PanTilt != nil {
		s.position.PanTilt.X = clamp(s.position.PanTilt.X+vel.PanTilt.X, ptzMinPan, ptzMaxPan)
		s.position.PanTilt.Y = clamp(s.position.PanTilt.Y+vel.PanTilt.Y, ptzMinTilt, ptzMaxTilt)
	}
	if vel.Zoom != nil {
		s.position.Zoom.X = clamp(s.position.Zoom.X+vel.Zoom.X, ptzMinZoom, ptzMaxZoom)
	}
}

func (s *ePTZState) absoluteMove(pos ptzsvc.Vector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pos.PanTilt != nil {
		s.position.PanTilt.X = clamp(pos.PanTilt.X, ptzMinPan, ptzMaxPan)
		s.position.PanTilt.Y = clamp(pos.PanTilt.Y, ptzMinTilt, ptzMaxTilt)
	}
	if pos.Zoom != nil {
		s.position.Zoom.X = clamp(pos.Zoom.X, ptzMinZoom, ptzMaxZoom)
	}
}

func (s *ePTZState) relativeMove(trans ptzsvc.Vector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if trans.PanTilt != nil {
		s.position.PanTilt.X = clamp(s.position.PanTilt.X+trans.PanTilt.X, ptzMinPan, ptzMaxPan)
		s.position.PanTilt.Y = clamp(s.position.PanTilt.Y+trans.PanTilt.Y, ptzMinTilt, ptzMaxTilt)
	}
	if trans.Zoom != nil {
		s.position.Zoom.X = clamp(s.position.Zoom.X+trans.Zoom.X, ptzMinZoom, ptzMaxZoom)
	}
}

func (s *ePTZState) stop(panTilt, zoom *bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// In a simulator, continuous move is instant. Stop is a no-op
	// but we acknowledge the request per ONVIF spec.
	_ = panTilt
	_ = zoom
}

func (s *ePTZState) setPreset(name *string, token *string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	tok := ""
	if token != nil && *token != "" {
		tok = *token
	} else {
		tok = "preset_" + strconv.Itoa(s.nextID)
		s.nextID++
	}
	n := tok
	if name != nil && *name != "" {
		n = *name
	}
	pos := copyVector(&s.position)
	s.presets[tok] = ptzPreset{
		token:    tok,
		name:     n,
		position: &pos,
	}
	return tok
}

func (s *ePTZState) removePreset(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.presets[token]; !ok {
		return fmt.Errorf("%w: %s", ptzsvc.ErrPresetNotFound, token)
	}
	delete(s.presets, token)
	return nil
}

func (s *ePTZState) getPresets() []ptzsvc.Preset {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ptzsvc.Preset, 0, len(s.presets))
	for _, p := range s.presets {
		pos := copyVector(p.position)
		out = append(out, ptzsvc.Preset{
			Token:    p.token,
			Name:     p.name,
			Position: &pos,
		})
	}
	return out
}

func (s *ePTZState) gotoPreset(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.presets[token]
	if !ok {
		return fmt.Errorf("%w: %s", ptzsvc.ErrPresetNotFound, token)
	}
	s.position = copyVector(p.position)
	return nil
}

func (s *ePTZState) gotoHomePosition() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.position = copyVector(&s.home)
}

func (s *ePTZState) setHomePosition() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.home = copyVector(&s.position)
}

func copyVector(v *ptzsvc.Vector) ptzsvc.Vector {
	if v == nil {
		return ptzsvc.Vector{}
	}
	out := ptzsvc.Vector{}
	if v.PanTilt != nil {
		out.PanTilt = &ptzsvc.PanTilt{X: v.PanTilt.X, Y: v.PanTilt.Y}
	}
	if v.Zoom != nil {
		out.Zoom = &ptzsvc.Zoom{X: v.Zoom.X}
	}
	return out
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// ptzProvider implements ptzsvc.Provider for the simulator.
type ptzProvider struct {
	sim    *Simulator
	states map[string]*ePTZState // keyed by profile token
	mu     sync.Mutex
}

func newPTZProvider(s *Simulator) *ptzProvider {
	return &ptzProvider{
		sim:    s,
		states: make(map[string]*ePTZState),
	}
}

func (p *ptzProvider) stateForProfile(profileToken string) *ePTZState {
	p.mu.Lock()
	defer p.mu.Unlock()
	tok := profileToken
	if tok == "" {
		tok = defaultPTZProfileToken
	}
	st, ok := p.states[tok]
	if !ok {
		st = newEPTZState()
		p.states[tok] = st
	}
	return st
}

func (p *ptzProvider) ServiceCapabilities(_ context.Context) (ptzsvc.ServiceCapabilities, error) {
	return ptzsvc.ServiceCapabilities{
		EFlip:          false,
		Reverse:        false,
		MoveStatus:     true,
		StatusPosition: true,
	}, nil
}

func (p *ptzProvider) GetNodes(_ context.Context) ([]ptzsvc.PTZNode, error) {
	return []ptzsvc.PTZNode{{
		Token: defaultPTZNodeToken,
		Name:  defaultPTZNodeName,
		SupportedPTZSpaces: map[string]ptzsvc.Space{
			"pan_tilt": {
				URI:    "http://www.onvif.org/ver10/tptz/PositionSpace",
				XRange: &ptzsvc.IntRange{Min: ptzMinPan, Max: ptzMaxPan},
				YRange: &ptzsvc.IntRange{Min: ptzMinTilt, Max: ptzMaxTilt},
			},
			"zoom": {
				URI:    "http://www.onvif.org/ver10/tptz/PositionSpace",
				XRange: &ptzsvc.IntRange{Min: ptzMinZoom, Max: ptzMaxZoom},
			},
		},
	}}, nil
}

func (p *ptzProvider) GetNode(_ context.Context, nodeToken string) (ptzsvc.PTZNode, error) {
	if nodeToken != defaultPTZNodeToken {
		return ptzsvc.PTZNode{}, fmt.Errorf("%w: node %s", ptzsvc.ErrInvalidArgs, nodeToken)
	}
	nodes, _ := p.GetNodes(nil)
	return nodes[0], nil
}

func (p *ptzProvider) GetConfigurations(_ context.Context) ([]ptzsvc.PTZConfiguration, error) {
	return []ptzsvc.PTZConfiguration{{
		Token:                                  defaultPTZConfigToken,
		Name:                                   defaultPTZConfigName,
		UseCount:                               1,
		NodeToken:                              defaultPTZNodeToken,
		DefaultAbsolutePantTiltPositionSpace:   "http://www.onvif.org/ver10/tptz/PositionSpace",
		DefaultAbsoluteZoomPositionSpace:       "http://www.onvif.org/ver10/tptz/PositionSpace",
		DefaultRelativePanTiltTranslationSpace: "http://www.onvif.org/ver10/tptz/TranslationSpace",
		DefaultRelativeZoomTranslationSpace:    "http://www.onvif.org/ver10/tptz/TranslationSpace",
		DefaultContinuousPanTiltVelocitySpace:  "http://www.onvif.org/ver10/tptz/VelocitySpace",
		DefaultContinuousZoomVelocitySpace:     "http://www.onvif.org/ver10/tptz/VelocitySpace",
	}}, nil
}

func (p *ptzProvider) GetConfiguration(_ context.Context, configToken string) (ptzsvc.PTZConfiguration, error) {
	if configToken != defaultPTZConfigToken {
		return ptzsvc.PTZConfiguration{}, fmt.Errorf("%w: config %s", ptzsvc.ErrInvalidArgs, configToken)
	}
	cfgs, _ := p.GetConfigurations(nil)
	return cfgs[0], nil
}

func (p *ptzProvider) GetConfigurationOptions(_ context.Context, _ string) (ptzsvc.ConfigurationOptions, error) {
	return ptzsvc.ConfigurationOptions{
		PanTiltPositionSpaceRange: []ptzsvc.SpaceRange{{
			URI:    "http://www.onvif.org/ver10/tptz/PositionSpace",
			XRange: ptzsvc.IntRange{Min: ptzMinPan, Max: ptzMaxPan},
			YRange: ptzsvc.IntRange{Min: ptzMinTilt, Max: ptzMaxTilt},
		}},
		ZoomPositionSpaceRange: []ptzsvc.SpaceRange{{
			URI:    "http://www.onvif.org/ver10/tptz/PositionSpace",
			XRange: ptzsvc.IntRange{Min: ptzMinZoom, Max: ptzMaxZoom},
		}},
		PanTiltTranslationSpaceRange: []ptzsvc.SpaceRange{{
			URI:    "http://www.onvif.org/ver10/tptz/TranslationSpace",
			XRange: ptzsvc.IntRange{Min: ptzMinPan, Max: ptzMaxPan},
			YRange: ptzsvc.IntRange{Min: ptzMinTilt, Max: ptzMaxTilt},
		}},
		ZoomTranslationSpaceRange: []ptzsvc.SpaceRange{{
			URI:    "http://www.onvif.org/ver10/tptz/TranslationSpace",
			XRange: ptzsvc.IntRange{Min: ptzMinZoom, Max: ptzMaxZoom},
		}},
		PanTiltVelocitySpaceRange: []ptzsvc.SpaceRange{{
			URI:    "http://www.onvif.org/ver10/tptz/VelocitySpace",
			XRange: ptzsvc.IntRange{Min: ptzMinPan, Max: ptzMaxPan},
			YRange: ptzsvc.IntRange{Min: ptzMinTilt, Max: ptzMaxTilt},
		}},
		ZoomVelocitySpaceRange: []ptzsvc.SpaceRange{{
			URI:    "http://www.onvif.org/ver10/tptz/VelocitySpace",
			XRange: ptzsvc.IntRange{Min: ptzMinZoom, Max: ptzMaxZoom},
		}},
		PTZTimeout: ptzsvc.IntRange{Min: 5, Max: 300},
	}, nil
}

func (p *ptzProvider) GetStatus(_ context.Context, profileToken string) (ptzsvc.Status, error) {
	return p.stateForProfile(profileToken).getStatus(), nil
}

func (p *ptzProvider) ContinuousMove(_ context.Context, profileToken string, velocity ptzsvc.Speed, _ *string) error {
	p.stateForProfile(profileToken).continuousMove(velocity)
	return nil
}

func (p *ptzProvider) AbsoluteMove(_ context.Context, profileToken string, position ptzsvc.Vector, _ *ptzsvc.Speed) error {
	p.stateForProfile(profileToken).absoluteMove(position)
	return nil
}

func (p *ptzProvider) RelativeMove(_ context.Context, profileToken string, translation ptzsvc.Vector, _ *ptzsvc.Speed) error {
	p.stateForProfile(profileToken).relativeMove(translation)
	return nil
}

func (p *ptzProvider) Stop(_ context.Context, profileToken string, panTilt, zoom *bool) error {
	p.stateForProfile(profileToken).stop(panTilt, zoom)
	return nil
}

func (p *ptzProvider) GetPresets(_ context.Context, profileToken string) ([]ptzsvc.Preset, error) {
	return p.stateForProfile(profileToken).getPresets(), nil
}

func (p *ptzProvider) SetPreset(_ context.Context, profileToken string, name, token *string) (string, error) {
	return p.stateForProfile(profileToken).setPreset(name, token), nil
}

func (p *ptzProvider) RemovePreset(_ context.Context, profileToken string, presetToken string) error {
	return p.stateForProfile(profileToken).removePreset(presetToken)
}

func (p *ptzProvider) GotoPreset(_ context.Context, profileToken string, presetToken string, _ *ptzsvc.Speed) error {
	return p.stateForProfile(profileToken).gotoPreset(presetToken)
}

func (p *ptzProvider) GotoHomePosition(_ context.Context, profileToken string, _ *ptzsvc.Speed) error {
	p.stateForProfile(profileToken).gotoHomePosition()
	return nil
}

func (p *ptzProvider) SetHomePosition(_ context.Context, profileToken string) error {
	p.stateForProfile(profileToken).setHomePosition()
	return nil
}
