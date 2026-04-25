// Adapter that bridges *simulator.Simulator to the GUI's simulatorAPI shape.
//
// The real Simulator returns its own Status / EventRecord / MutationRecord /
// UserView types and accepts AddProfile / AddUser by value. The GUI uses
// locally-defined mirrors (with JSON tags so Wails emits camelCase keys) and
// pointer-arg method shapes (so gocritic stays happy on the stub). This
// adapter is the one place those two shapes are reconciled.
package main

import (
	"context"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/simulator"
)

type simulatorAdapter struct {
	inner *simulator.Simulator
}

// newSimulatorAdapter wires the real Simulator behind the GUI's interface.
// onEvent / onMutation are passed verbatim through simulator.Options so the
// caller (NewApp) can fan records out to Wails event emitters.
func newSimulatorAdapter(
	configPath string,
	onEvent func(EventRecord),
	onMutation func(MutationRecord),
) (*simulatorAdapter, error) {
	opts := simulator.Options{
		ConfigPath: configPath,
		OnEvent: func(r simulator.EventRecord) {
			if onEvent != nil {
				onEvent(toLocalEvent(r))
			}
		},
		OnMutation: func(r simulator.MutationRecord) {
			if onMutation != nil {
				onMutation(toLocalMutation(r))
			}
		},
	}
	sim, err := simulator.New(opts)
	if err != nil {
		return nil, err
	}
	return &simulatorAdapter{inner: sim}, nil
}

func toLocalEvent(r simulator.EventRecord) EventRecord {
	return EventRecord{Time: r.Time, Topic: r.Topic, Source: r.Source, Payload: r.Payload}
}

func toLocalMutation(r simulator.MutationRecord) MutationRecord {
	return MutationRecord{Time: r.Time, Kind: r.Kind, Target: r.Target, Detail: r.Detail}
}

func toLocalUser(u simulator.UserView) UserView {
	return UserView{Username: u.Username, Roles: append([]string(nil), u.Roles...)}
}

func toLocalStatus(s *simulator.Status) Status {
	recent := make([]EventRecord, len(s.RecentEvents))
	for i := range s.RecentEvents {
		recent[i] = toLocalEvent(s.RecentEvents[i])
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
		RecentEvents:   recent,
	}
}

func (a *simulatorAdapter) Start(ctx context.Context) error { return a.inner.Start(ctx) }
func (a *simulatorAdapter) Stop(ctx context.Context) error  { return a.inner.Stop(ctx) }
func (a *simulatorAdapter) Running() bool                   { return a.inner.Running() }

func (a *simulatorAdapter) Status() Status {
	s := a.inner.Status()
	return toLocalStatus(&s)
}

func (a *simulatorAdapter) ConfigSnapshot() config.Config { return a.inner.ConfigSnapshot() }

func (a *simulatorAdapter) Users() []UserView {
	src := a.inner.Users()
	out := make([]UserView, len(src))
	for i := range src {
		out[i] = toLocalUser(src[i])
	}
	return out
}

func (a *simulatorAdapter) Motion(token string, state bool) { a.inner.Motion(token, state) }
func (a *simulatorAdapter) ImageTooBlurry(t string, s bool) { a.inner.ImageTooBlurry(t, s) }
func (a *simulatorAdapter) ImageTooDark(t string, s bool)   { a.inner.ImageTooDark(t, s) }
func (a *simulatorAdapter) ImageTooBright(t string, s bool) { a.inner.ImageTooBright(t, s) }
func (a *simulatorAdapter) DigitalInput(t string, s bool)   { a.inner.DigitalInput(t, s) }
func (a *simulatorAdapter) SyncProperty(topic, sourceItem, source, dataItem string, state bool) {
	a.inner.SyncProperty(topic, sourceItem, source, dataItem, state)
}
func (a *simulatorAdapter) PublishRaw(topic, msg string) { a.inner.PublishRaw(topic, msg) }

func (a *simulatorAdapter) SetDiscoveryMode(mode string) error {
	return a.inner.SetDiscoveryMode(mode)
}
func (a *simulatorAdapter) SetHostname(name string) error { return a.inner.SetHostname(name) }

func (a *simulatorAdapter) AddProfile(p *config.ProfileConfig) error { return a.inner.AddProfile(*p) }
func (a *simulatorAdapter) RemoveProfile(token string) error         { return a.inner.RemoveProfile(token) }
func (a *simulatorAdapter) SetProfileRTSP(token, rtsp string) error {
	return a.inner.SetProfileRTSP(token, rtsp)
}
func (a *simulatorAdapter) SetProfileSnapshotURI(token, uri string) error {
	return a.inner.SetProfileSnapshotURI(token, uri)
}
func (a *simulatorAdapter) SetProfileEncoder(
	token, encoding string, width, height, fps, bitrate, gop int,
) error {
	return a.inner.SetProfileEncoder(token, encoding, width, height, fps, bitrate, gop)
}

func (a *simulatorAdapter) SetTopicEnabled(name string, enabled bool) error {
	return a.inner.SetTopicEnabled(name, enabled)
}
func (a *simulatorAdapter) SetEventsTopics(topics []config.TopicConfig) error {
	return a.inner.SetEventsTopics(topics)
}

func (a *simulatorAdapter) AddUser(u *config.UserConfig) error    { return a.inner.AddUser(*u) }
func (a *simulatorAdapter) UpsertUser(u *config.UserConfig) error { return a.inner.UpsertUser(*u) }
func (a *simulatorAdapter) RemoveUser(username string) error      { return a.inner.RemoveUser(username) }
func (a *simulatorAdapter) SetAuthEnabled(enabled bool) error     { return a.inner.SetAuthEnabled(enabled) }
