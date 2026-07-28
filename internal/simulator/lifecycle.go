package simulator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/ffmpeg"
	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/devicesvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/imgsvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/media2svc"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/mediasvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/ptzsvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/rpicamera"
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
		s.logger.Error("simulator: listen", "addr", addr, "err", err)
		return fmt.Errorf("simulator: listen %s: %w", addr, err)
	}

	rtspLogger := s.rootLogger.With("component", "rtsp")
	rtspServer, probedProfiles, mjpegSiblings, rtspErr := startRTSPServer(&cfg, rtspLogger)
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
	s.derivedMJPEGProfiles = mjpegSiblings
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
//
// Each handler is wrapped in obs.RequestMiddleware so every SOAP request gets
// a generated/echoed request id, a request-scoped logger, and a one-line
// summary at request completion (level scaled by HTTP status).
func buildHTTPServer(s *Simulator) *http.Server {
	httpLogger := s.rootLogger.With("component", "http")
	mux := http.NewServeMux()
	mux.Handle(devicesvc.DeviceServicePath, obs.RequestMiddleware(s.devHandler, httpLogger))
	mux.Handle(mediasvc.MediaServicePath, obs.RequestMiddleware(s.medHandler, httpLogger))
	mux.Handle(media2svc.Media2ServicePath, obs.RequestMiddleware(s.media2Handler, httpLogger))
	mux.Handle(ptzsvc.PTZServicePath, obs.RequestMiddleware(s.ptzHandler, httpLogger))
	mux.Handle(imgsvc.ImagingServicePath, obs.RequestMiddleware(s.imgHandler, httpLogger))
	mux.Handle(eventsvc.EventServicePath, obs.RequestMiddleware(s.evtHandler, httpLogger))
	mux.Handle(eventsvc.SubscriptionManagerPath, obs.RequestMiddleware(s.subHandler, httpLogger))
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

// errUnknownSourceKind is the wrapped sentinel for buildRTSPSource's default
// branch — guards against future Kind values added without a constructor.
var errUnknownSourceKind = errors.New("simulator: unknown source kind")

// startRTSPServer boots the embedded RTSP server (if at least one profile
// declares a usable source) and returns the probed profiles whose encoder
// fields the lifecycle should publish back into Simulator.cfg, plus the
// auto-generated Profile S §7.9 MJPEG sibling profiles the lifecycle
// should store on Simulator.derivedMJPEGProfiles.
//
// A profile is "usable" when ProfileConfig.HasSource reports true: kind=file
// with a non-empty media_file_path, or kind=rpicam with rpicam params. When
// no profile is usable the returned server is nil and StreamURI continues to
// emit the conventional rtsp://host:port/<token> URL even without an active
// listener, so clients can detect the absence as "no media on that path".
//
// For every usable profile the function also registers a sibling MJPEG
// RTSP source under <token>_JPEG: kind=file siblings transcode via the
// embedded ffmpeg helper, kind=rpicam siblings hook the mtxrpicam
// secondary stream (Pi ISP hardware JPEG). When the ffmpeg helper is not
// available on the current build (placeholder binary), kind=file siblings
// are silently skipped and only the H.264/H.265 path stays registered so
// the simulator still boots; the MJPEG path will be unavailable until a
// real ffmpeg binary is fetched.
func startRTSPServer(
	cfg *config.Config, logger *slog.Logger,
) (srv *rtsp.Server, probed, mjpegSiblings []config.ProfileConfig, err error) {
	hasSource := false
	for i := range cfg.Media.Profiles {
		if cfg.Media.Profiles[i].HasSource() {
			hasSource = true
			break
		}
	}
	if !hasSource {
		return nil, nil, nil, nil
	}

	srv = rtsp.New(cfg.Network.RTSPPortOrDefault(), rtsp.WithLogger(logger))
	if startErr := srv.Start(); startErr != nil {
		return nil, nil, nil, fmt.Errorf("simulator: start rtsp server: %w", startErr)
	}

	probed = make([]config.ProfileConfig, len(cfg.Media.Profiles))
	copy(probed, cfg.Media.Profiles)
	for i := range probed {
		p := &probed[i]
		if !p.HasSource() {
			continue
		}
		src, err := buildRTSPSource(p, logger)
		if err != nil {
			srv.Stop()
			return nil, nil, nil, fmt.Errorf(
				"simulator: register rtsp source %q: %w",
				p.Token, err,
			)
		}
		probe, err := srv.AddSource(p.Token, src)
		if err != nil {
			srv.Stop()
			return nil, nil, nil, fmt.Errorf(
				"simulator: register rtsp source %q: %w",
				p.Token, err,
			)
		}
		// Auto-fill in-memory encoder metadata from the source (probe for
		// file kind; configured params for rpicam). Persisted values (if
		// any) are still written to disk as-is by the config helpers; this
		// overwrite affects only the live ConfigSnapshot.
		p.Encoding = probe.Codec
		p.Width = probe.Width
		p.Height = probe.Height
		p.FPS = probe.FPS
	}

	mjpegSiblings, sibErr := registerMJPEGSiblings(srv, probed, logger)
	if sibErr != nil {
		srv.Stop()
		return nil, nil, nil, sibErr
	}
	return srv, probed, mjpegSiblings, nil
}

// registerMJPEGSiblings creates and registers one MJPEG RTSP source per
// usable user profile so Profile S §7.9 is satisfied. The function
// returns the in-memory ProfileConfig entries the caller should hand to
// Simulator.derivedMJPEGProfiles so GetProfiles enumerates them.
//
// The h264Sources map is keyed by parent token and supplies the
// rpicam-tagged source that needs the secondary-MJPEG callback wired
// up; it is unused for kind=file siblings.
func registerMJPEGSiblings(
	srv *rtsp.Server,
	probed []config.ProfileConfig,
	logger *slog.Logger,
) ([]config.ProfileConfig, error) {
	siblings := deriveMJPEGSiblings(probed)
	if len(siblings) == 0 {
		return nil, nil
	}
	out := make([]config.ProfileConfig, 0, len(siblings))
	for i := range siblings {
		sib := siblings[i]
		parentTok := MJPEGSiblingParentToken(sib.Token)
		parent := findProfileByToken(probed, parentTok)
		if parent == nil {
			continue
		}
		sibLogger := logger.With("profile", sib.Token, "parent", parentTok)
		src, buildErr := buildMJPEGSiblingSource(parent, srv, sibLogger)
		if buildErr != nil {
			sibLogger.Warn("simulator: skip mjpeg sibling — source unavailable",
				"err", buildErr)
			continue
		}
		if _, addErr := srv.AddSource(sib.Token, src); addErr != nil {
			return nil, fmt.Errorf(
				"simulator: register mjpeg sibling %q: %w",
				sib.Token, addErr,
			)
		}
		out = append(out, sib)
	}
	return out, nil
}

// buildMJPEGSiblingSource constructs the rtsp.Source feeding the MJPEG
// sibling for a kind=file or kind=rpicam parent profile. kind=file uses
// the embedded ffmpeg transcoder; kind=rpicam attaches the secondary
// stream callback to the parent's rpicam source (which is already
// registered by startRTSPServer's main loop).
//
// Returns (nil, nil) when the parent's kind cannot produce an MJPEG
// sibling (e.g. an unknown kind defended against in validateProfileSource);
// returns (nil, error) when the transcoder is unavailable.
func buildMJPEGSiblingSource(
	parent *config.ProfileConfig, srv *rtsp.Server, logger *slog.Logger,
) (rtsp.Source, error) {
	switch parent.Kind {
	case "", config.ProfileKindFile:
		params := ffmpegParamsForProfile(parent)
		return rtsp.NewTranscodingSource(params, parent.Width, parent.Height, parent.FPS, logger)
	case config.ProfileKindRPICam:
		// The MJPEG sibling for an rpicam profile is fed by the parent's
		// secondary stream callback. Build a plain MJPEGSource here and
		// wire it into the parent via rtsp.AttachMJPEG below. The parent
		// rpicam source is already registered (under parent.Token) by
		// startRTSPServer's main loop; we look it up via srv to attach.
		mjpeg := rtsp.NewMJPEGSource(&rtsp.ProbeResult{
			Codec:  rtsp.CodecMJPEG,
			Width:  parent.Width,
			Height: parent.Height,
			FPS:    parent.FPS,
		}, logger)
		parentSrc := srv.SourceFor(parent.Token)
		if parentSrc == nil {
			return nil, errMJPEGParentSourceMissing
		}
		if !rtsp.AttachMJPEG(parentSrc, mjpeg.Push) {
			return nil, errMJPEGParentNotRPICam
		}
		return mjpeg, nil
	default:
		return nil, errMJPEGUnsupportedParentKind
	}
}

// ffmpegParamsForProfile maps a kind=file parent profile to the ffmpeg
// transcoder configuration. The simulator loops the source file forever
// to mirror the H.264 looper's behavior so an MJPEG client sees the
// same continuous loop a parallel H.264 client does.
func ffmpegParamsForProfile(p *config.ProfileConfig) ffmpeg.Params {
	return ffmpeg.Params{
		MediaFilePath: p.MediaFilePath,
		FPS:           p.FPS,
		LoopForever:   true,
	}
}

func findProfileByToken(profiles []config.ProfileConfig, token string) *config.ProfileConfig {
	for i := range profiles {
		if profiles[i].Token == token {
			return &profiles[i]
		}
	}
	return nil
}

var (
	errMJPEGParentSourceMissing   = errors.New("simulator: parent rpicam source not registered")
	errMJPEGParentNotRPICam       = errors.New("simulator: parent source is not an rpicam source")
	errMJPEGUnsupportedParentKind = errors.New("simulator: cannot derive MJPEG sibling for this profile kind")
)

// buildRTSPSource maps a ProfileConfig source kind to the matching
// rtsp.Source. The kind=rpicam branch returns rpicamera.ErrUnsupported on
// builds without the rpicam tag.
func buildRTSPSource(p *config.ProfileConfig, logger *slog.Logger) (rtsp.Source, error) {
	switch effectiveKind := p.Kind; effectiveKind {
	case "", config.ProfileKindFile:
		return rtsp.NewFileSource(p.MediaFilePath)
	case config.ProfileKindRPICam:
		// Conversions are bounded by validateRPICam (CameraID >= 0, Width/
		// Height in [0,4096], FPS in [0,120], Bitrate/IDRPeriod >= 0), so
		// the int → uint32/float32 narrowing here cannot overflow.
		params := rpicamera.Params{
			CameraID:   uint32(p.RPICam.CameraID), //nolint:gosec // bounded by validateRPICam
			Width:      uint32(p.RPICam.Width),    //nolint:gosec // bounded by validateRPICam
			Height:     uint32(p.RPICam.Height),   //nolint:gosec // bounded by validateRPICam
			FPS:        float32(p.RPICam.FPS),
			Bitrate:    uint32(p.RPICam.Bitrate),   //nolint:gosec // bounded by validateRPICam
			IDRPeriod:  uint32(p.RPICam.IDRPeriod), //nolint:gosec // bounded by validateRPICam
			HFlip:      p.RPICam.HFlip,
			VFlip:      p.RPICam.VFlip,
			Brightness: p.RPICam.Brightness,
			Contrast:   p.RPICam.Contrast,
			Saturation: p.RPICam.Saturation,
			Sharpness:  p.RPICam.Sharpness,
		}
		return rtsp.NewRPICamSource(
			params,
			p.RPICam.Width, p.RPICam.Height, p.RPICam.FPS,
			logger.With("profile", p.Token),
		)
	default:
		return nil, fmt.Errorf("%w: %q", errUnknownSourceKind, p.Kind)
	}
}

// Stop gracefully shuts down. Idempotent.
//
// When the simulator built its own logger (Options.Logger == nil), Stop also
// flushes and closes the underlying log file. Callers that injected an
// explicit Logger keep responsibility for closing their own State.
func (s *Simulator) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		s.closeOwnedLogger()
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

	s.closeOwnedLogger()
	return shutdownErr
}

// closeOwnedLogger releases the simulator-owned LogState exactly once.
// No-op when the caller injected an explicit Logger (LogState is nil).
func (s *Simulator) closeOwnedLogger() {
	s.mu.Lock()
	state := s.logState
	s.logState = nil
	s.mu.Unlock()
	if state != nil {
		_ = state.Close() //nolint:errcheck // best-effort flush on shutdown
	}
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
