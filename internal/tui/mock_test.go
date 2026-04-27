package tui

import (
	"context"
	"sync"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

// mockSim is an in-memory SimulatorAPI used by unit tests. It records
// mutator invocations so tests can assert side effects without spinning
// up the real simulator.
type mockSim struct {
	mu       sync.Mutex
	running  bool
	snapshot config.Config
	users    []UserView

	calls []string
}

func newMockSim() *mockSim {
	return &mockSim{
		snapshot: config.Config{
			Version: 1,
			Device: config.DeviceConfig{
				UUID:         "urn:uuid:11111111-1111-1111-1111-111111111111",
				Manufacturer: "Acme",
				Model:        "Sim",
				Serial:       "SN-0001",
			},
			Network: config.NetworkConfig{HTTPPort: 8080},
			Media: config.MediaConfig{
				Profiles: []config.ProfileConfig{
					{
						Name: "p0", Token: "tok0",
						MediaFilePath: "",
						Encoding:      "H264", Width: 1920, Height: 1080, FPS: 30,
						VideoSourceToken: "VS0",
					},
				},
			},
			Events: config.EventsConfig{
				Topics: []config.TopicConfig{
					{Name: topicMotion, Enabled: true},
					{Name: topicDigitalIn, Enabled: false},
				},
			},
			Runtime: config.RuntimeConfig{DiscoveryMode: "Discoverable"},
		},
	}
}

func (m *mockSim) record(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, name)
}

func (m *mockSim) callsCopy() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.calls))
	copy(out, m.calls)
	return out
}

func (m *mockSim) Start(_ context.Context) error {
	m.record("Start")
	m.mu.Lock()
	m.running = true
	m.mu.Unlock()
	return nil
}

func (m *mockSim) Stop(_ context.Context) error {
	m.record("Stop")
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()
	return nil
}

func (m *mockSim) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *mockSim) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	addr := ""
	if m.running {
		addr = "127.0.0.1:8080"
	}
	return Status{
		Running:       m.running,
		ListenAddr:    addr,
		StartedAt:     time.Time{},
		DiscoveryMode: m.snapshot.Runtime.DiscoveryMode,
		ProfileCount:  len(m.snapshot.Media.Profiles),
		TopicCount:    countEnabled(m.snapshot.Events.Topics),
		UserCount:     len(m.snapshot.Auth.Users),
	}
}

func countEnabled(ts []config.TopicConfig) int {
	n := 0
	for i := range ts {
		if ts[i].Enabled {
			n++
		}
	}
	return n
}

func (m *mockSim) ConfigSnapshot() config.Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshot
}

func (m *mockSim) Users() []UserView {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.users
}

func (m *mockSim) Motion(_ string, _ bool)                { m.record("Motion") }
func (m *mockSim) ImageTooBlurry(_ string, _ bool)        { m.record("ImageTooBlurry") }
func (m *mockSim) ImageTooDark(_ string, _ bool)          { m.record("ImageTooDark") }
func (m *mockSim) ImageTooBright(_ string, _ bool)        { m.record("ImageTooBright") }
func (m *mockSim) DigitalInput(_ string, _ bool)          { m.record("DigitalInput") }
func (m *mockSim) SyncProperty(_, _, _, _ string, _ bool) { m.record("SyncProperty") }
func (m *mockSim) PublishRaw(_, _ string)                 { m.record("PublishRaw") }

func (m *mockSim) SetDiscoveryMode(mode string) error {
	m.record("SetDiscoveryMode")
	m.mu.Lock()
	m.snapshot.Runtime.DiscoveryMode = mode
	m.mu.Unlock()
	return nil
}

func (m *mockSim) SetHostname(name string) error {
	m.record("SetHostname")
	m.mu.Lock()
	m.snapshot.Runtime.Hostname = name
	m.mu.Unlock()
	return nil
}

func (m *mockSim) AddProfile(p config.ProfileConfig) error { //nolint:gocritic // matches upstream signature
	m.record("AddProfile")
	m.mu.Lock()
	m.snapshot.Media.Profiles = append(m.snapshot.Media.Profiles, p)
	m.mu.Unlock()
	return nil
}

func (m *mockSim) RemoveProfile(token string) error {
	m.record("RemoveProfile")
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.snapshot.Media.Profiles[:0]
	for i := range m.snapshot.Media.Profiles {
		if m.snapshot.Media.Profiles[i].Token != token {
			out = append(out, m.snapshot.Media.Profiles[i])
		}
	}
	m.snapshot.Media.Profiles = out
	return nil
}

func (m *mockSim) SetProfileMediaFilePath(_, _ string) error {
	m.record("SetProfileMediaFilePath")
	return nil
}
func (m *mockSim) SetProfileSnapshotURI(_, _ string) error {
	m.record("SetProfileSnapshotURI")
	return nil
}

func (m *mockSim) SetTopicEnabled(name string, enabled bool) error {
	m.record("SetTopicEnabled")
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.snapshot.Events.Topics {
		if m.snapshot.Events.Topics[i].Name == name {
			m.snapshot.Events.Topics[i].Enabled = enabled
			return nil
		}
	}
	return nil
}

func (m *mockSim) AddUser(u config.UserConfig) error {
	m.record("AddUser")
	m.mu.Lock()
	m.snapshot.Auth.Users = append(m.snapshot.Auth.Users, u)
	m.mu.Unlock()
	return nil
}

func (m *mockSim) UpsertUser(_ config.UserConfig) error { m.record("UpsertUser"); return nil }
func (m *mockSim) RemoveUser(_ string) error            { m.record("RemoveUser"); return nil }
func (m *mockSim) SetAuthEnabled(enabled bool) error {
	m.record("SetAuthEnabled")
	m.mu.Lock()
	m.snapshot.Auth.Enabled = enabled
	m.mu.Unlock()
	return nil
}
