package simulator

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/devicesvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/mediasvc"
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

	mux := http.NewServeMux()
	mux.Handle(devicesvc.DeviceServicePath, s.devHandler)
	mux.Handle(mediasvc.MediaServicePath, s.medHandler)
	mux.Handle(eventsvc.EventServicePath, s.evtHandler)
	mux.Handle(eventsvc.SubscriptionManagerPath, s.subHandler)

	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	listenAddr := listener.Addr().String()

	s.broker.Start()

	_, portStr, splitErr := net.SplitHostPort(listenAddr)
	if splitErr != nil {
		portStr = strconv.Itoa(cfg.Network.HTTPPort)
	}
	port, convErr := strconv.Atoi(portStr)
	if convErr != nil {
		port = cfg.Network.HTTPPort
	}
	host := localAddrForXAddr()
	subMgrAddr := httpURL(host, port, eventsvc.SubscriptionManagerPath)
	s.broker.UpdateConfig(brokerConfigWithAddr(brokerConfigFromConfig(&cfg), subMgrAddr))

	discCtx, discCancel := context.WithCancel(context.Background())
	discDone := make(chan struct{})
	go func() {
		defer close(discDone)
		s.runDiscovery(discCtx, host, port)
	}()

	go func() {
		serveErr := server.Serve(listener)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			// Without a logger here we can't do better than ignoring the
			// post-Stop error; Stop already drains.
			_ = serveErr
		}
	}()

	s.mu.Lock()
	s.server = server
	s.listenAddr = listenAddr
	s.started = time.Now().UTC()
	s.running = true
	s.discoveryCancel = discCancel
	s.discoveryDone = discDone
	s.mu.Unlock()

	return nil
}

// Stop gracefully shuts down. Idempotent.
func (s *Simulator) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	server := s.server
	cancel := s.discoveryCancel
	done := s.discoveryDone
	s.running = false
	s.server = nil
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
