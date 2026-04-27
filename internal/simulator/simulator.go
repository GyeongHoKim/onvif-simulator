// Package simulator is the composition root that wires config, auth, event,
// and the ONVIF service handlers into a single runnable virtual device.
//
// Front-ends (CLI, TUI, GUI) drive the simulator exclusively through the
// methods on *Simulator; they never touch the underlying packages directly.
// See doc/design/simulator-api.md for the full public contract.
package simulator

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/event"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/devicesvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/mediasvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/rtsp"
)

// Options configures a new simulator at construction time.
// All fields are optional; zeros use sensible defaults.
type Options struct {
	// ConfigPath overrides the config file path. Empty uses the default
	// (onvif-simulator.json in the working directory).
	ConfigPath string

	// EventBufferSize controls the recent-events ring size surfaced by Status.
	// Defaults to 128. Clamped into [16, 1024].
	EventBufferSize int

	// OnEvent is called for every event published through the simulator's
	// typed helpers. Implementations must not block; they run on the
	// publisher's goroutine.
	OnEvent func(EventRecord)

	// OnMutation is called after every persisted config mutation. Same
	// contract as OnEvent — implementations must not block.
	OnMutation func(MutationRecord)
}

// EventRecord is one line of the recent-events ring buffer.
type EventRecord struct {
	Time    time.Time
	Topic   string
	Source  string
	Payload string
}

// MutationRecord summarizes a config mutation for audit / UI log.
type MutationRecord struct {
	Time   time.Time
	Kind   string
	Target string
	Detail string
}

// Status is a low-cost, read-mostly snapshot for the dashboard.
type Status struct {
	Running        bool
	ListenAddr     string
	StartedAt      time.Time
	Uptime         time.Duration
	DiscoveryMode  string
	ProfileCount   int
	TopicCount     int
	UserCount      int
	ActivePullSubs int
	RecentEvents   []EventRecord
}

// UserView is the safe-to-render projection of an auth.UserRecord.
type UserView struct {
	Username string
	Roles    []string
}

const (
	defaultEventBufferSize = 128
	minEventBufferSize     = 16
	maxEventBufferSize     = 1024
	uint32Mask             = 0xffffffff
)

// Simulator is the composition root. Construct with New; never copy.
type Simulator struct {
	opts Options

	// cfgPath is the on-disk location of the config file.
	cfgPath string

	// mu guards the mutable fields below (cfg, running, started, server,
	// listenAddr, discoveryCancel) as well as the authentication chain
	// rebuild on auth changes.
	mu              sync.Mutex
	cfg             config.Config
	running         bool
	started         time.Time
	server          *http.Server
	rtspServer      *rtsp.Server
	listenAddr      string
	discoveryCancel context.CancelFunc
	discoveryDone   chan struct{}

	// Core components. Immutable after New.
	store      *auth.MutableUserStore
	controller *auth.Controller
	broker     *event.Broker
	deviceProv *deviceProvider
	mediaProv  *mediaProvider
	devHandler *devicesvc.Handler
	medHandler *mediasvc.Handler
	evtHandler *eventsvc.EventServiceHandler
	subHandler *eventsvc.SubscriptionManagerHandler

	// Authentication chain. Rebuilt when auth config changes.
	authMu      sync.RWMutex
	authChain   auth.Authenticator
	authEnabled bool

	// Recent events ring buffer.
	ring *eventRing

	// WS-Discovery sequence numbers. instanceID is fixed at construction;
	// msgNumberSeq is incremented atomically.
	instanceID   uint32
	msgNumberSeq uint32
}

// New builds a simulator from the on-disk config. Panics only on internal
// misconfiguration (nil broker construction, unreachable default paths);
// everything else surfaces as a returned error.
//
// Config path resolution:
//   - opts.ConfigPath (typically the CLI -config flag) wins when non-empty.
//   - Otherwise we use config.DefaultPath (the OS-standard user config
//     directory) so double-clicking the macOS .app — where the working
//     directory is "/" — finds the same file the user edited last run.
//
// The resolved path is registered with config.SetPath so every mutation
// helper writes back to the same location, and config.EnsureExists creates
// a baseline file on first run.
func New(opts Options) (*Simulator, error) {
	cfgPath, err := config.Resolve(opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("simulator: resolve config path: %w", err)
	}
	config.SetPath(cfgPath)
	if _, ensureErr := config.EnsureExists(cfgPath); ensureErr != nil {
		return nil, fmt.Errorf("simulator: ensure config: %w", ensureErr)
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("simulator: load config: %w", err)
	}

	store := auth.NewMutableUserStore(nil)
	controller := auth.NewController(store)
	controller.SyncFromConfig(&cfg)

	ringSize := clampBufferSize(opts.EventBufferSize)

	sim := &Simulator{
		opts:         opts,
		cfgPath:      cfgPath,
		cfg:          cfg,
		store:        store,
		controller:   controller,
		ring:         newEventRing(ringSize),
		instanceID:   uint32(time.Now().Unix() & uint32Mask),
		msgNumberSeq: 0,
	}

	broker := event.New(brokerConfigFromConfig(&cfg))
	sim.broker = broker

	sim.deviceProv = newDeviceProvider(sim)
	sim.mediaProv = newMediaProvider(sim)

	if err := sim.rebuildAuthChain(&cfg); err != nil {
		return nil, fmt.Errorf("simulator: build auth chain: %w", err)
	}

	sim.devHandler = devicesvc.NewHandler(sim.deviceProv, devicesvc.WithAuthHook(
		devicesvc.AuthFunc(sim.deviceAuthHook),
	))
	sim.medHandler = mediasvc.NewHandler(sim.mediaProv, mediasvc.WithAuthHook(
		mediasvc.AuthFunc(sim.mediaAuthHook),
	))
	sim.evtHandler = eventsvc.NewEventServiceHandler(broker,
		eventsvc.WithEventAuthHook(eventsvc.AuthFunc(sim.eventAuthHook)),
	)
	sim.subHandler = eventsvc.NewSubscriptionManagerHandler(broker,
		eventsvc.WithSubscriptionManagerAuthHook(eventsvc.AuthFunc(sim.eventAuthHook)),
	)

	return sim, nil
}

func clampBufferSize(n int) int {
	if n == 0 {
		return defaultEventBufferSize
	}
	if n < minEventBufferSize {
		return minEventBufferSize
	}
	if n > maxEventBufferSize {
		return maxEventBufferSize
	}
	return n
}

func brokerConfigFromConfig(cfg *config.Config) event.BrokerConfig {
	topics := make([]event.TopicConfig, 0, len(cfg.Events.Topics))
	for _, t := range cfg.Events.Topics {
		topics = append(topics, event.TopicConfig{Name: t.Name, Enabled: t.Enabled})
	}
	timeout := event.DefaultSubscriptionTimeout
	if cfg.Events.SubscriptionTimeout != "" {
		if d, err := time.ParseDuration(cfg.Events.SubscriptionTimeout); err == nil && d > 0 {
			timeout = d
		}
	}
	return event.BrokerConfig{
		MaxPullPoints:       cfg.Events.MaxPullPoints,
		SubscriptionTimeout: timeout,
		Topics:              topics,
	}
}

func brokerConfigWithAddr(base event.BrokerConfig, addr string) event.BrokerConfig {
	base.SubscriptionManagerAddr = addr
	return base
}

// snapshotConfig returns a deep copy of the current config under the lock.
func (s *Simulator) snapshotConfig() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneConfig(&s.cfg)
}

// reloadFromDisk reads the config file again and updates live state. Called
// after every successful mutator.
func (s *Simulator) reloadFromDisk() error {
	fresh, err := config.Load()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = fresh
	s.mu.Unlock()

	s.broker.UpdateConfig(brokerConfigFromConfig(&fresh))
	s.controller.SyncFromConfig(&fresh)
	return s.rebuildAuthChain(&fresh)
}

// recordMutation emits an OnMutation callback (if set).
func (s *Simulator) recordMutation(kind, target, detail string) {
	if s.opts.OnMutation == nil {
		return
	}
	s.opts.OnMutation(MutationRecord{
		Time:   time.Now().UTC(),
		Kind:   kind,
		Target: target,
		Detail: detail,
	})
}

// cloneConfig produces a deep copy of cfg so callers may mutate safely.
func cloneConfig(c *config.Config) config.Config {
	out := *c
	out.Device.Scopes = cloneStrings(c.Device.Scopes)
	out.Network.XAddrs = cloneStrings(c.Network.XAddrs)
	out.Media.Profiles = cloneProfiles(c.Media.Profiles)
	out.Media.MetadataConfigurations = cloneMetadata(c.Media.MetadataConfigurations)
	out.Auth.Users = cloneUsers(c.Auth.Users)
	out.Auth.Digest.Algorithms = cloneStrings(c.Auth.Digest.Algorithms)
	out.Auth.JWT.Algorithms = cloneStrings(c.Auth.JWT.Algorithms)
	out.Auth.JWT.PublicKeyPEM = cloneStrings(c.Auth.JWT.PublicKeyPEM)
	out.Events.Topics = cloneTopics(c.Events.Topics)
	out.Runtime.DNS.SearchDomain = cloneStrings(c.Runtime.DNS.SearchDomain)
	out.Runtime.DNS.DNSManual = cloneStrings(c.Runtime.DNS.DNSManual)
	out.Runtime.DefaultGateway.IPv4Address = cloneStrings(c.Runtime.DefaultGateway.IPv4Address)
	out.Runtime.DefaultGateway.IPv6Address = cloneStrings(c.Runtime.DefaultGateway.IPv6Address)
	out.Runtime.NetworkProtocols = cloneProtocols(c.Runtime.NetworkProtocols)
	out.Runtime.NetworkInterfaces = cloneInterfaces(c.Runtime.NetworkInterfaces)
	return out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneProfiles(in []config.ProfileConfig) []config.ProfileConfig {
	if in == nil {
		return nil
	}
	out := make([]config.ProfileConfig, len(in))
	copy(out, in)
	return out
}

func cloneMetadata(in []config.MetadataConfig) []config.MetadataConfig {
	if in == nil {
		return nil
	}
	out := make([]config.MetadataConfig, len(in))
	copy(out, in)
	return out
}

func cloneUsers(in []config.UserConfig) []config.UserConfig {
	if in == nil {
		return nil
	}
	out := make([]config.UserConfig, len(in))
	copy(out, in)
	return out
}

func cloneTopics(in []config.TopicConfig) []config.TopicConfig {
	if in == nil {
		return nil
	}
	out := make([]config.TopicConfig, len(in))
	copy(out, in)
	return out
}

func cloneProtocols(in []config.NetworkProtocol) []config.NetworkProtocol {
	if in == nil {
		return nil
	}
	out := make([]config.NetworkProtocol, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].Port = cloneInts(in[i].Port)
	}
	return out
}

func cloneInts(in []int) []int {
	if in == nil {
		return nil
	}
	out := make([]int, len(in))
	copy(out, in)
	return out
}

func cloneInterfaces(in []config.NetworkInterfaceConfig) []config.NetworkInterfaceConfig {
	if in == nil {
		return nil
	}
	out := make([]config.NetworkInterfaceConfig, len(in))
	for i := range in {
		out[i] = in[i]
		if in[i].IPv4 != nil {
			clone := *in[i].IPv4
			clone.Manual = cloneStrings(in[i].IPv4.Manual)
			out[i].IPv4 = &clone
		}
	}
	return out
}

// localAddrForXAddr picks a usable local IP for composing XAddrs. It prefers
// the first non-loopback IPv4 address; loopback as a fallback.
func localAddrForXAddr() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}
		ip4 := ipnet.IP.To4()
		if ip4 != nil {
			return ip4.String()
		}
	}
	return "127.0.0.1"
}

// httpURL composes an HTTP URL given host and port, including the path.
func httpURL(host string, port int, path string) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + path
}
