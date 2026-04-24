package tui

import (
	"context"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/simulator"
)

// simulatorAdapter wraps *simulator.Simulator to produce TUI-local Status and
// UserView values. The adapter is trivial since both packages share the same
// field layouts; it exists to keep the TUI compilable against a mock (via
// SimulatorAPI) during tests and against the real simulator in production.
type simulatorAdapter struct {
	sim *simulator.Simulator
}

func newSimulatorAdapter(sim *simulator.Simulator) *simulatorAdapter {
	return &simulatorAdapter{sim: sim}
}

func (a *simulatorAdapter) Start(ctx context.Context) error { return a.sim.Start(ctx) }
func (a *simulatorAdapter) Stop(ctx context.Context) error  { return a.sim.Stop(ctx) }
func (a *simulatorAdapter) Running() bool                   { return a.sim.Running() }

func (a *simulatorAdapter) Status() Status {
	s := a.sim.Status()
	events := make([]EventRecord, len(s.RecentEvents))
	for i, e := range s.RecentEvents {
		events[i] = EventRecord{
			Time:    e.Time,
			Topic:   e.Topic,
			Source:  e.Source,
			Payload: e.Payload,
		}
	}
	return Status{
		Running:        s.Running,
		ListenAddr:     s.ListenAddr,
		StartedAt:      s.StartedAt,
		Uptime:         s.Uptime,
		DiscoveryMode:  s.DiscoveryMode,
		ProfileCount:   s.ProfileCount,
		TopicCount:     s.TopicCount,
		UserCount:      s.UserCount,
		ActivePullSubs: s.ActivePullSubs,
		RecentEvents:   events,
	}
}

func (a *simulatorAdapter) ConfigSnapshot() config.Config { return a.sim.ConfigSnapshot() }

func (a *simulatorAdapter) Users() []UserView {
	in := a.sim.Users()
	out := make([]UserView, len(in))
	for i, u := range in {
		out[i] = UserView{Username: u.Username, Roles: u.Roles}
	}
	return out
}

func (a *simulatorAdapter) Motion(src string, state bool)         { a.sim.Motion(src, state) }
func (a *simulatorAdapter) ImageTooBlurry(src string, state bool) { a.sim.ImageTooBlurry(src, state) }
func (a *simulatorAdapter) ImageTooDark(src string, state bool)   { a.sim.ImageTooDark(src, state) }
func (a *simulatorAdapter) ImageTooBright(src string, state bool) { a.sim.ImageTooBright(src, state) }
func (a *simulatorAdapter) DigitalInput(tok string, state bool)   { a.sim.DigitalInput(tok, state) }
func (a *simulatorAdapter) SyncProperty(topic, src, tok, data string, state bool) {
	a.sim.SyncProperty(topic, src, tok, data, state)
}
func (a *simulatorAdapter) PublishRaw(topic, message string) { a.sim.PublishRaw(topic, message) }

func (a *simulatorAdapter) SetDiscoveryMode(mode string) error { return a.sim.SetDiscoveryMode(mode) }
func (a *simulatorAdapter) SetHostname(name string) error      { return a.sim.SetHostname(name) }

func (a *simulatorAdapter) AddProfile(p config.ProfileConfig) error { //nolint:gocritic // matches upstream signature
	return a.sim.AddProfile(p)
}
func (a *simulatorAdapter) RemoveProfile(token string) error { return a.sim.RemoveProfile(token) }
func (a *simulatorAdapter) SetProfileRTSP(token, rtsp string) error {
	return a.sim.SetProfileRTSP(token, rtsp)
}
func (a *simulatorAdapter) SetProfileSnapshotURI(token, uri string) error {
	return a.sim.SetProfileSnapshotURI(token, uri)
}
func (a *simulatorAdapter) SetProfileEncoder(token, enc string, w, h, fps, br, gop int) error {
	return a.sim.SetProfileEncoder(token, enc, w, h, fps, br, gop)
}

func (a *simulatorAdapter) SetTopicEnabled(name string, enabled bool) error {
	return a.sim.SetTopicEnabled(name, enabled)
}

func (a *simulatorAdapter) AddUser(u config.UserConfig) error    { return a.sim.AddUser(u) }
func (a *simulatorAdapter) UpsertUser(u config.UserConfig) error { return a.sim.UpsertUser(u) }
func (a *simulatorAdapter) RemoveUser(username string) error     { return a.sim.RemoveUser(username) }
func (a *simulatorAdapter) SetAuthEnabled(enabled bool) error    { return a.sim.SetAuthEnabled(enabled) }
