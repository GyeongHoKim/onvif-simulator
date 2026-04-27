package gui

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

const (
	defaultListenAddr = "0.0.0.0:8080"
	recentEventsCap   = 128
)

type simulatorStub struct {
	mu         sync.Mutex
	running    bool
	startedAt  time.Time
	cfg        config.Config
	users      []UserView
	recent     []EventRecord // newest first, bounded
	onEvent    func(EventRecord)
	onMutation func(MutationRecord)
}

func newSimulatorStub(onEvent func(EventRecord), onMutation func(MutationRecord)) *simulatorStub {
	cfg := defaultFakeConfig()
	s := &simulatorStub{
		cfg:        cfg,
		onEvent:    onEvent,
		onMutation: onMutation,
	}
	s.users = usersFromConfig(&s.cfg.Auth)
	return s
}

func defaultFakeConfig() config.Config {
	return config.Config{
		Version: config.CurrentVersion,
		Device: config.DeviceConfig{
			UUID:         "urn:uuid:00000000-0000-4000-8000-000000000001",
			Manufacturer: "ONVIF Simulator",
			Model:        "SimCam-100",
			Serial:       "SN-0001",
			Firmware:     "0.1.0",
			Scopes: []string{
				"onvif://www.onvif.org/Profile/Streaming",
				"onvif://www.onvif.org/name/simulator",
				"onvif://www.onvif.org/hardware/virtual",
			},
		},
		Network: config.NetworkConfig{HTTPPort: 8080, RTSPPort: config.DefaultRTSPPort},
		Media: config.MediaConfig{
			Profiles: []config.ProfileConfig{
				{
					Name: "main", Token: "profile_main",
					MediaFilePath:    "/var/onvif/main.mp4",
					SnapshotURI:      "http://127.0.0.1:8080/snapshot/main.jpg",
					VideoSourceToken: "VS_MAIN",
				},
				{
					Name: "sub", Token: "profile_sub",
					MediaFilePath: "/var/onvif/sub.mp4",
				},
			},
		},
		Auth: config.AuthConfig{
			Enabled: true,
			Users: []config.UserConfig{
				{Username: "admin", Password: "admin", Role: config.RoleAdministrator},
			},
			Digest: config.DigestConfig{
				Realm: "onvif-simulator", Algorithms: []string{"MD5"}, NonceTTL: "5m",
			},
			JWT: config.JWTConfig{
				Algorithms:    []string{"RS256"},
				UsernameClaim: "sub", RolesClaim: "roles", ClockSkew: "30s",
			},
		},
		Runtime: config.RuntimeConfig{
			DiscoveryMode: "Discoverable",
			Hostname:      "onvif-simulator",
			DNS: config.DNSConfig{
				SearchDomain: []string{"local"},
				DNSManual:    []string{"8.8.8.8", "8.8.4.4"},
			},
			DefaultGateway: config.DefaultGatewayConfig{IPv4Address: []string{"192.168.1.1"}},
			NetworkProtocols: []config.NetworkProtocol{
				{Name: "HTTP", Enabled: true, Port: []int{8080}},
				{Name: "RTSP", Enabled: true, Port: []int{554}},
				{Name: "HTTPS", Enabled: false, Port: []int{443}},
			},
			SystemDateAndTime: config.SystemDateTimeConfig{
				DateTimeType: "NTP", TZ: "UTC",
			},
		},
		Events: config.EventsConfig{
			MaxPullPoints:       10,
			SubscriptionTimeout: "1h",
			Topics: []config.TopicConfig{
				{Name: "tns1:VideoSource/MotionAlarm", Enabled: true},
				{Name: "tns1:VideoSource/ImageTooBlurry", Enabled: true},
				{Name: "tns1:VideoSource/ImageTooDark", Enabled: true},
				{Name: "tns1:VideoSource/ImageTooBright", Enabled: true},
				{Name: "tns1:Device/Trigger/DigitalInput", Enabled: true},
			},
		},
	}
}

func usersFromConfig(a *config.AuthConfig) []UserView {
	out := make([]UserView, 0, len(a.Users))
	for i := range a.Users {
		u := &a.Users[i]
		out = append(out, UserView{Username: u.Username, Roles: []string{u.Role}})
	}
	return out
}

// Lifecycle ---------------------------------------------------------------

func (s *simulatorStub) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	s.running = true
	s.startedAt = time.Now()
	return nil
}

func (s *simulatorStub) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.startedAt = time.Time{}
	return nil
}

func (s *simulatorStub) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *simulatorStub) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	topicCount := 0
	for i := range s.cfg.Events.Topics {
		if s.cfg.Events.Topics[i].Enabled {
			topicCount++
		}
	}
	uptime := time.Duration(0)
	listen := ""
	if s.running {
		uptime = time.Since(s.startedAt)
		listen = defaultListenAddr
	}
	recent := make([]EventRecord, len(s.recent))
	copy(recent, s.recent)
	return Status{
		Running:       s.running,
		ListenAddr:    listen,
		StartedAt:     s.startedAt,
		Uptime:        uptime,
		DiscoveryMode: s.cfg.Runtime.DiscoveryMode,
		ProfileCount:  len(s.cfg.Media.Profiles),
		TopicCount:    topicCount,
		UserCount:     len(s.users),
		RecentEvents:  recent,
	}
}

// Reads -------------------------------------------------------------------

func (s *simulatorStub) ConfigSnapshot() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneConfig(&s.cfg)
}

func (s *simulatorStub) Users() []UserView {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]UserView, len(s.users))
	copy(out, s.users)
	return out
}

// Event triggers ----------------------------------------------------------

func (s *simulatorStub) Motion(token string, state bool) {
	s.publish("tns1:VideoSource/MotionAlarm", token, fmt.Sprintf("state=%t", state))
}
func (s *simulatorStub) ImageTooBlurry(token string, state bool) {
	s.publish("tns1:VideoSource/ImageTooBlurry", token, fmt.Sprintf("state=%t", state))
}
func (s *simulatorStub) ImageTooDark(token string, state bool) {
	s.publish("tns1:VideoSource/ImageTooDark", token, fmt.Sprintf("state=%t", state))
}
func (s *simulatorStub) ImageTooBright(token string, state bool) {
	s.publish("tns1:VideoSource/ImageTooBright", token, fmt.Sprintf("state=%t", state))
}
func (s *simulatorStub) DigitalInput(token string, state bool) {
	s.publish("tns1:Device/Trigger/DigitalInput", token, fmt.Sprintf("state=%t", state))
}
func (s *simulatorStub) SyncProperty(topic, _, token, _ string, state bool) {
	s.publish(topic, token, fmt.Sprintf("synced=%t", state))
}
func (s *simulatorStub) PublishRaw(topic, _ string) {
	s.publish(topic, "", "raw")
}

func (s *simulatorStub) publish(topic, source, payload string) {
	if !s.topicEnabled(topic) {
		return
	}
	rec := EventRecord{Time: time.Now(), Topic: topic, Source: source, Payload: payload}
	s.mu.Lock()
	s.recent = append([]EventRecord{rec}, s.recent...)
	if len(s.recent) > recentEventsCap {
		s.recent = s.recent[:recentEventsCap]
	}
	cb := s.onEvent
	s.mu.Unlock()
	if cb != nil {
		cb(rec)
	}
}

func (s *simulatorStub) topicEnabled(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cfg.Events.Topics {
		if s.cfg.Events.Topics[i].Name == name {
			return s.cfg.Events.Topics[i].Enabled
		}
	}
	return false
}

// Mutators ----------------------------------------------------------------

func (s *simulatorStub) SetDiscoveryMode(mode string) error {
	if mode != "Discoverable" && mode != "NonDiscoverable" {
		return config.ErrDiscoveryModeInvalid
	}
	s.mu.Lock()
	s.cfg.Runtime.DiscoveryMode = mode
	s.mu.Unlock()
	s.notifyMutation("SetDiscoveryMode", "", mode)
	return nil
}

func (s *simulatorStub) SetHostname(name string) error {
	s.mu.Lock()
	s.cfg.Runtime.Hostname = name
	s.mu.Unlock()
	s.notifyMutation("SetHostname", "", name)
	return nil
}

func (s *simulatorStub) AddProfile(p *config.ProfileConfig) error {
	s.mu.Lock()
	for i := range s.cfg.Media.Profiles {
		if s.cfg.Media.Profiles[i].Token == p.Token {
			s.mu.Unlock()
			return fmt.Errorf("profile token %q: %w", p.Token, errStubDuplicateProfile)
		}
	}
	s.cfg.Media.Profiles = append(s.cfg.Media.Profiles, *p)
	s.mu.Unlock()
	if err := s.revalidate(); err != nil {
		s.mu.Lock()
		s.cfg.Media.Profiles = s.cfg.Media.Profiles[:len(s.cfg.Media.Profiles)-1]
		s.mu.Unlock()
		return err
	}
	s.notifyMutation("AddProfile", p.Token, p.Name)
	return nil
}

func (s *simulatorStub) RemoveProfile(token string) error {
	s.mu.Lock()
	idx := -1
	for i := range s.cfg.Media.Profiles {
		if s.cfg.Media.Profiles[i].Token == token {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.mu.Unlock()
		return fmt.Errorf("profile %q: %w", token, errStubProfileNotFound)
	}
	s.cfg.Media.Profiles = append(s.cfg.Media.Profiles[:idx], s.cfg.Media.Profiles[idx+1:]...)
	s.mu.Unlock()
	s.notifyMutation("RemoveProfile", token, "")
	return nil
}

func (s *simulatorStub) SetProfileMediaFilePath(token, path string) error {
	return s.mutateProfile(token, "SetProfileMediaFilePath", path, func(p *config.ProfileConfig) {
		p.MediaFilePath = path
	})
}

func (s *simulatorStub) SetProfileSnapshotURI(token, uri string) error {
	return s.mutateProfile(token, "SetProfileSnapshotURI", uri, func(p *config.ProfileConfig) {
		p.SnapshotURI = uri
	})
}

func (s *simulatorStub) mutateProfile(token, kind, detail string, fn func(*config.ProfileConfig)) error {
	s.mu.Lock()
	idx := -1
	for i := range s.cfg.Media.Profiles {
		if s.cfg.Media.Profiles[i].Token == token {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.mu.Unlock()
		return fmt.Errorf("profile %q: %w", token, errStubProfileNotFound)
	}
	backup := s.cfg.Media.Profiles[idx]
	fn(&s.cfg.Media.Profiles[idx])
	s.mu.Unlock()
	if err := s.revalidate(); err != nil {
		s.mu.Lock()
		s.cfg.Media.Profiles[idx] = backup
		s.mu.Unlock()
		return err
	}
	s.notifyMutation(kind, token, detail)
	return nil
}

func (s *simulatorStub) SetTopicEnabled(name string, enabled bool) error {
	s.mu.Lock()
	for i := range s.cfg.Events.Topics {
		if s.cfg.Events.Topics[i].Name == name {
			s.cfg.Events.Topics[i].Enabled = enabled
			s.mu.Unlock()
			s.notifyMutation("SetTopicEnabled", name, fmt.Sprintf("enabled=%t", enabled))
			return nil
		}
	}
	s.mu.Unlock()
	return fmt.Errorf("topic %q: %w", name, errStubTopicNotFound)
}

func (s *simulatorStub) SetEventsTopics(topics []config.TopicConfig) error {
	s.mu.Lock()
	backup := s.cfg.Events.Topics
	s.cfg.Events.Topics = append([]config.TopicConfig(nil), topics...)
	s.mu.Unlock()
	if err := s.revalidate(); err != nil {
		s.mu.Lock()
		s.cfg.Events.Topics = backup
		s.mu.Unlock()
		return err
	}
	s.notifyMutation("SetEventsTopics", "", fmt.Sprintf("%d topics", len(topics)))
	return nil
}

func (s *simulatorStub) AddUser(u *config.UserConfig) error {
	s.mu.Lock()
	for i := range s.cfg.Auth.Users {
		if s.cfg.Auth.Users[i].Username == u.Username {
			s.mu.Unlock()
			return fmt.Errorf("username %q: %w", u.Username, config.ErrAuthUsernameDuplicate)
		}
	}
	s.cfg.Auth.Users = append(s.cfg.Auth.Users, *u)
	s.users = usersFromConfig(&s.cfg.Auth)
	s.mu.Unlock()
	if err := s.revalidate(); err != nil {
		s.mu.Lock()
		s.cfg.Auth.Users = s.cfg.Auth.Users[:len(s.cfg.Auth.Users)-1]
		s.users = usersFromConfig(&s.cfg.Auth)
		s.mu.Unlock()
		return err
	}
	s.notifyMutation("AddUser", u.Username, u.Role)
	return nil
}

func (s *simulatorStub) UpsertUser(u *config.UserConfig) error {
	s.mu.Lock()
	idx := -1
	for i := range s.cfg.Auth.Users {
		if s.cfg.Auth.Users[i].Username == u.Username {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.mu.Unlock()
		return s.AddUser(u)
	}
	s.cfg.Auth.Users[idx] = *u
	s.users = usersFromConfig(&s.cfg.Auth)
	s.mu.Unlock()
	s.notifyMutation("UpsertUser", u.Username, u.Role)
	return nil
}

func (s *simulatorStub) RemoveUser(username string) error {
	s.mu.Lock()
	idx := -1
	for i := range s.cfg.Auth.Users {
		if s.cfg.Auth.Users[i].Username == username {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.mu.Unlock()
		return fmt.Errorf("username %q: %w", username, errStubUserNotFound)
	}
	s.cfg.Auth.Users = append(s.cfg.Auth.Users[:idx], s.cfg.Auth.Users[idx+1:]...)
	s.users = usersFromConfig(&s.cfg.Auth)
	s.mu.Unlock()
	s.notifyMutation("RemoveUser", username, "")
	return nil
}

func (s *simulatorStub) SetAuthEnabled(enabled bool) error {
	s.mu.Lock()
	backup := s.cfg.Auth.Enabled
	s.cfg.Auth.Enabled = enabled
	s.mu.Unlock()
	if err := s.revalidate(); err != nil {
		s.mu.Lock()
		s.cfg.Auth.Enabled = backup
		s.mu.Unlock()
		return err
	}
	s.notifyMutation("SetAuthEnabled", "", fmt.Sprintf("enabled=%t", enabled))
	return nil
}

func (s *simulatorStub) notifyMutation(kind, target, detail string) {
	s.mu.Lock()
	cb := s.onMutation
	s.mu.Unlock()
	if cb == nil {
		return
	}
	cb(MutationRecord{Time: time.Now(), Kind: kind, Target: target, Detail: detail})
}

// revalidate runs the real config.Validate so the stub's error surface matches
// the production simulator for the bits that care (profile format, user rules,
// topic names).
func (s *simulatorStub) revalidate() error {
	s.mu.Lock()
	snap := cloneConfig(&s.cfg)
	s.mu.Unlock()
	return config.Validate(&snap)
}

var (
	errStubDuplicateProfile = errors.New("duplicate profile token")
	errStubProfileNotFound  = errors.New("profile not found")
	errStubTopicNotFound    = errors.New("topic not found")
	errStubUserNotFound     = errors.New("user not found")
)

// cloneConfig deep-copies a Config so callers can mutate snapshots without
// racing the simulator's internal state.
func cloneConfig(c *config.Config) config.Config {
	out := *c
	out.Device.Scopes = append([]string(nil), c.Device.Scopes...)
	out.Network.XAddrs = append([]string(nil), c.Network.XAddrs...)
	out.Media.Profiles = append([]config.ProfileConfig(nil), c.Media.Profiles...)
	out.Media.MetadataConfigurations = append(
		[]config.MetadataConfig(nil), c.Media.MetadataConfigurations...,
	)
	out.Auth.Users = append([]config.UserConfig(nil), c.Auth.Users...)
	out.Auth.Digest.Algorithms = append([]string(nil), c.Auth.Digest.Algorithms...)
	out.Auth.JWT.Algorithms = append([]string(nil), c.Auth.JWT.Algorithms...)
	out.Auth.JWT.PublicKeyPEM = append([]string(nil), c.Auth.JWT.PublicKeyPEM...)
	out.Runtime.DNS.SearchDomain = append([]string(nil), c.Runtime.DNS.SearchDomain...)
	out.Runtime.DNS.DNSManual = append([]string(nil), c.Runtime.DNS.DNSManual...)
	out.Runtime.DefaultGateway.IPv4Address = append(
		[]string(nil), c.Runtime.DefaultGateway.IPv4Address...,
	)
	out.Runtime.DefaultGateway.IPv6Address = append(
		[]string(nil), c.Runtime.DefaultGateway.IPv6Address...,
	)
	out.Runtime.NetworkProtocols = append(
		[]config.NetworkProtocol(nil), c.Runtime.NetworkProtocols...,
	)
	out.Runtime.NetworkInterfaces = append(
		[]config.NetworkInterfaceConfig(nil), c.Runtime.NetworkInterfaces...,
	)
	out.Events.Topics = append([]config.TopicConfig(nil), c.Events.Topics...)
	return out
}
