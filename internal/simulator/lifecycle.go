package simulator

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/devicesvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/mediasvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/rtsp"
)

// Start boots the HTTP server, the broker reaper, and WS-Discovery. Idempotent.
func (s *Simulator) Start(_ context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	cfg := s.cfg
	s.mu.Unlock()

	addr := ":" + strconv.Itoa(cfg.Network.HTTPPort)
	lc := &net.ListenConfig{}
	listener, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return fmt.Errorf("simulator: listen %s: %w", addr, err)
	}

	rtspServer, probedProfiles, rtspErr := startRTSPServer(&cfg)
	if rtspErr != nil {
		_ = listener.Close() //nolint:errcheck // best-effort during failure
		return rtspErr
	}

	server := buildHTTPServer(s)
	listenAddr := listener.Addr().String()

	s.broker.Start()
	host, port := splitListenAddr(listenAddr, cfg.Network.HTTPPort)
	subMgrAddr := httpURL(host, port, eventsvc.SubscriptionManagerPath)
	s.broker.UpdateConfig(brokerConfigWithAddr(brokerConfigFromConfig(&cfg), subMgrAddr))

	discCtx, discCancel := context.WithCancel(context.Background())
	discDone := make(chan struct{})
	go func() {
		defer close(discDone)
		s.runDiscovery(discCtx, host, port)
	}()
	go serveAndIgnoreClosed(server, listener)

	s.mu.Lock()
	s.server = server
	s.rtspServer = rtspServer
	if probedProfiles != nil {
		s.cfg.Media.Profiles = probedProfiles
	}
	s.listenAddr = listenAddr
	s.started = time.Now().UTC()
	s.running = true
	s.discoveryCancel = discCancel
	s.discoveryDone = discDone
	s.mu.Unlock()

	return nil
}

// buildHTTPServer wires the ONVIF service handlers onto a fresh HTTP server.
// The server itself is started by the caller via Serve so the listener can be
// closed before Start returns on early-error paths.
func buildHTTPServer(s *Simulator) *http.Server {
	mux := http.NewServeMux()
	mux.Handle(devicesvc.DeviceServicePath, s.devHandler)
	mux.Handle(mediasvc.MediaServicePath, s.medHandler)
	mux.Handle(eventsvc.EventServicePath, s.evtHandler)
	mux.Handle(eventsvc.SubscriptionManagerPath, s.subHandler)
	return &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// splitListenAddr parses listener.Addr().String() into a (host, port) pair,
// falling back to the configured HTTPPort when the listener returned a value
// we can't decode.
func splitListenAddr(listenAddr string, fallbackPort int) (host string, port int) {
	_, portStr, splitErr := net.SplitHostPort(listenAddr)
	if splitErr != nil {
		portStr = strconv.Itoa(fallbackPort)
	}
	parsed, convErr := strconv.Atoi(portStr)
	if convErr != nil {
		parsed = fallbackPort
	}
	return localAddrForXAddr(), parsed
}

// serveAndIgnoreClosed serves until the listener closes. We swallow
// ErrServerClosed because Stop drains already.
func serveAndIgnoreClosed(server *http.Server, listener net.Listener) {
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		_ = err
	}
}

// startRTSPServer boots the embedded RTSP server (if at least one profile
// has a media file path configured) and returns the probed profiles whose
// encoder fields the lifecycle should publish back into Simulator.cfg.
//
// When no profile has MediaFilePath set, the returned server is nil and the
// simulator falls back to passing through the legacy ProfileConfig.RTSP
// field via media_provider.StreamURI. This keeps existing tests/configs
// working until callers migrate to the new field.
func startRTSPServer(cfg *config.Config) (*rtsp.Server, []config.ProfileConfig, error) {
	hasFilePath := false
	for i := range cfg.Media.Profiles {
		if cfg.Media.Profiles[i].MediaFilePath != "" {
			hasFilePath = true
			break
		}
	}
	if !hasFilePath {
		return nil, nil, nil
	}

	srv := rtsp.New(cfg.Network.RTSPPortOrDefault())
	if err := srv.Start(); err != nil {
		return nil, nil, fmt.Errorf("simulator: start rtsp server: %w", err)
	}

	probed := make([]config.ProfileConfig, len(cfg.Media.Profiles))
	copy(probed, cfg.Media.Profiles)
	for i := range probed {
		p := &probed[i]
		if p.MediaFilePath == "" {
			continue
		}
		probe, err := srv.AddSource(p.Token, p.MediaFilePath)
		if err != nil {
			srv.Stop()
			return nil, nil, fmt.Errorf(
				"simulator: register rtsp source %q (%s): %w",
				p.Token, p.MediaFilePath, err,
			)
		}
		// Auto-fill in-memory encoder metadata from the file. Persisted
		// values (if any) are still written to disk as-is by the config
		// helpers; this overwrite affects only the live ConfigSnapshot.
		p.Encoding = probe.Codec
		p.Width = probe.Width
		p.Height = probe.Height
		p.FPS = probe.FPS
	}
	return srv, probed, nil
}

// Stop gracefully shuts down. Idempotent.
func (s *Simulator) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	server := s.server
	rtspSrv := s.rtspServer
	cancel := s.discoveryCancel
	done := s.discoveryDone
	s.running = false
	s.server = nil
	s.rtspServer = nil
	s.discoveryCancel = nil
	s.discoveryDone = nil
	s.mu.Unlock()

	s.sendByeMulticast()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	var shutdownErr error
	if server != nil {
		shutdownErr = server.Shutdown(ctx)
	}
	if rtspSrv != nil {
		rtspSrv.Stop()
	}

	s.broker.Stop()

	s.mu.Lock()
	s.listenAddr = ""
	s.started = time.Time{}
	s.mu.Unlock()
	return shutdownErr
}

// Running reports whether Start has been called and Stop has not yet returned.
func (s *Simulator) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Status returns a consistent snapshot of the current state.
func (s *Simulator) Status() Status {
	s.mu.Lock()
	running := s.running
	listenAddr := s.listenAddr
	started := s.started
	cfg := s.cfg
	s.mu.Unlock()

	topicCount := 0
	for _, t := range cfg.Events.Topics {
		if t.Enabled {
			topicCount++
		}
	}
	status := Status{
		Running:       running,
		ListenAddr:    listenAddr,
		StartedAt:     started,
		DiscoveryMode: cfg.Runtime.DiscoveryMode,
		ProfileCount:  len(cfg.Media.Profiles),
		TopicCount:    topicCount,
		UserCount:     len(cfg.Auth.Users),
		RecentEvents:  s.ring.snapshot(),
	}
	if running {
		status.Uptime = time.Since(started)
	}
	return status
}
