package rtsp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"

	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
)

// ErrSourceExists is returned by AddSource when a source with the same token
// is already registered. Callers should RemoveSource first when reconfiguring.
var ErrSourceExists = errors.New("rtsp: source already registered")

// ErrSourceNotFound is returned by RemoveSource for an unknown token.
var ErrSourceNotFound = errors.New("rtsp: source not found")

// ErrServerNotStarted is returned by AddSource when the underlying gortsplib
// listener has not been opened yet.
var ErrServerNotStarted = errors.New("rtsp: server not started")

// ErrUnsupportedCodec is returned by buildMedia for codecs other than H264 and
// H265 (we currently only packetize those two).
var ErrUnsupportedCodec = errors.New("rtsp: unsupported codec")

// ErrNilSource is returned by AddSource when the caller passes a nil Source.
var ErrNilSource = errors.New("rtsp: nil source")

// ErrSourceDescribeNil is returned by AddSource when Source.Describe yields a
// nil ProbeResult — a programming error in the Source implementation.
var ErrSourceDescribeNil = errors.New("rtsp: source.Describe returned nil")

// Server is an embedded RTSP server that maps each registered source to a
// distinct URL path. One gortsplib.Server is shared across all sources; each
// source has its own ServerStream and a goroutine that loops the source's mp4.
type Server struct {
	port int

	mu      sync.RWMutex
	srv     *gortsplib.Server
	sources map[string]*source
	started bool
	logger  *slog.Logger
}

type source struct {
	token   string
	stream  *gortsplib.ServerStream
	media   *description.Media
	src     Source
	probe   *ProbeResult
	cancel  context.CancelFunc
	stopped chan struct{}
}

// Option customizes a Server.
type Option func(*Server)

// WithLogger installs a structured logger. Nil falls back to discard.
func WithLogger(logger *slog.Logger) Option {
	return func(s *Server) {
		if logger == nil {
			logger = obs.Discard()
		}
		s.logger = logger
	}
}

// New constructs a Server bound to the given TCP port. The caller must invoke
// Start before AddSource.
func New(port int, opts ...Option) *Server {
	s := &Server{
		port:    port,
		sources: make(map[string]*source),
		logger:  obs.Discard(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Port reports the TCP port the server is configured to listen on.
func (s *Server) Port() int { return s.port }

// Start opens the RTSP listener. It is idempotent — a second call is a no-op.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}
	srv := &gortsplib.Server{
		Handler:     &serverHandler{owner: s},
		RTSPAddress: ":" + strconv.Itoa(s.port),
	}
	if err := srv.Start(); err != nil {
		s.logger.Warn("rtsp: start listener", "port", s.port, "err", err)
		return fmt.Errorf("rtsp: start listener on port %d: %w", s.port, err)
	}
	s.srv = srv
	s.started = true
	s.logger.Info("rtsp: listener started", "port", s.port)
	return nil
}

// Stop closes every active source and shuts the listener down. Idempotent.
func (s *Server) Stop() {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	tokens := make([]string, 0, len(s.sources))
	for tok := range s.sources {
		tokens = append(tokens, tok)
	}
	s.mu.Unlock()

	for _, tok := range tokens {
		_ = s.RemoveSource(tok) //nolint:errcheck // best-effort during shutdown
	}

	s.mu.Lock()
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
	s.started = false
	s.mu.Unlock()
}

// AddSource registers a Source under the URL path /<token>. The Source's
// Describe() drives SDP construction; AttachStream binds it to the gortsplib
// stream; Run starts in its own goroutine. Use NewFileSource to wrap a local
// mp4 file (the most common case).
func (s *Server) AddSource(token string, src Source) (*ProbeResult, error) {
	if src == nil {
		return nil, fmt.Errorf("%w: token=%q", ErrNilSource, token)
	}
	probe := src.Describe()
	if probe == nil {
		return nil, fmt.Errorf("%w: token=%q", ErrSourceDescribeNil, token)
	}

	media, err := buildMedia(probe)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return nil, ErrServerNotStarted
	}
	if _, ok := s.sources[token]; ok {
		return nil, fmt.Errorf("%w: %s", ErrSourceExists, token)
	}

	desc := &description.Session{Medias: []*description.Media{media}}
	stream := &gortsplib.ServerStream{Server: s.srv, Desc: desc}
	if initErr := stream.Initialize(); initErr != nil {
		return nil, fmt.Errorf("rtsp: initialize stream %s: %w", token, initErr)
	}

	src.AttachStream(stream, media)

	// cancel is invoked from RemoveSource/Stop and also deferred in the
	// goroutine so gosec G118 sees a definite call site; calling cancel
	// twice is safe per context semantics.
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer cancel()
		defer close(stopped)
		if runErr := src.Run(ctx); runErr != nil && !errors.Is(runErr, context.Canceled) {
			s.logger.Warn("rtsp: source run terminated", "token", token, "err", runErr)
		}
	}()

	s.sources[token] = &source{
		token:   token,
		stream:  stream,
		media:   media,
		src:     src,
		probe:   probe,
		cancel:  cancel,
		stopped: stopped,
	}
	return probe, nil
}

// RemoveSource stops the looper for token and closes the associated stream.
func (s *Server) RemoveSource(token string) error {
	s.mu.Lock()
	src, ok := s.sources[token]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrSourceNotFound, token)
	}
	delete(s.sources, token)
	s.mu.Unlock()

	src.cancel()
	<-src.stopped
	src.stream.Close()
	return nil
}

// streamFor looks up the ServerStream for a URL path requested by an RTSP
// client. The caller (handler) holds the read lock.
func (s *Server) streamFor(path string) *gortsplib.ServerStream {
	token := pathToken(path)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if src, ok := s.sources[token]; ok {
		return src.stream
	}
	return nil
}

// streamAndSourceFor returns both the gortsplib stream and the underlying
// Source for an URL path. OnDescribe uses the Source to wait for readiness
// before responding.
func (s *Server) streamAndSourceFor(path string) (*gortsplib.ServerStream, Source) {
	token := pathToken(path)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if src, ok := s.sources[token]; ok {
		return src.stream, src.src
	}
	return nil, nil
}

// describeReadyTimeout caps how long OnDescribe waits for a live source to
// produce its first IDR. File sources return immediately so the timeout is
// effectively only for live capture. Tuned for libcamera startup which
// commonly takes 1–3 seconds before the first keyframe.
const describeReadyTimeout = 8 * time.Second

// pathToken strips a leading slash and any trailing query/control segments
// from the URL path so /foo, foo, and /foo/trackID=0 all resolve to "foo".
func pathToken(p string) string {
	for p != "" && p[0] == '/' {
		p = p[1:]
	}
	for i := range len(p) {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return p
}

func buildMedia(p *ProbeResult) (*description.Media, error) {
	switch p.Codec {
	case CodecH264:
		return &description.Media{
			Type: description.MediaTypeVideo,
			Formats: []format.Format{&format.H264{
				PayloadTyp:        96,
				PacketizationMode: 1,
				SPS:               p.SPS,
				PPS:               p.PPS,
			}},
		}, nil
	case CodecH265:
		return &description.Media{
			Type: description.MediaTypeVideo,
			Formats: []format.Format{&format.H265{
				PayloadTyp: 96,
				VPS:        p.VPS,
				SPS:        p.SPS,
				PPS:        p.PPS,
			}},
		}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedCodec, p.Codec)
	}
}

// serverHandler implements gortsplib.ServerHandler. It is intentionally tiny
// — DESCRIBE, SETUP, and PLAY all just look up the stream registered for the
// requested URL path and return it.
type serverHandler struct{ owner *Server }

func (*serverHandler) OnConnOpen(*gortsplib.ServerHandlerOnConnOpenCtx)         {}
func (*serverHandler) OnConnClose(*gortsplib.ServerHandlerOnConnCloseCtx)       {}
func (*serverHandler) OnSessionOpen(*gortsplib.ServerHandlerOnSessionOpenCtx)   {}
func (*serverHandler) OnSessionClose(*gortsplib.ServerHandlerOnSessionCloseCtx) {}

func (h *serverHandler) OnDescribe(
	ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	stream, src := h.owner.streamAndSourceFor(ctx.Path)
	if stream == nil {
		return &base.Response{StatusCode: base.StatusNotFound}, nil, nil
	}
	if src != nil {
		readyCtx, cancel := context.WithTimeout(context.Background(), describeReadyTimeout)
		defer cancel()
		if err := src.Ready(readyCtx); err != nil {
			h.owner.logger.Warn("rtsp: describe waited for source readiness",
				"path", ctx.Path, "err", err)
			return &base.Response{StatusCode: base.StatusServiceUnavailable}, nil, nil
		}
	}
	return &base.Response{StatusCode: base.StatusOK}, stream, nil
}

func (h *serverHandler) OnSetup(
	ctx *gortsplib.ServerHandlerOnSetupCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	stream := h.owner.streamFor(ctx.Path)
	if stream == nil {
		return &base.Response{StatusCode: base.StatusNotFound}, nil, nil
	}
	return &base.Response{StatusCode: base.StatusOK}, stream, nil
}

func (*serverHandler) OnPlay(*gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	return &base.Response{StatusCode: base.StatusOK}, nil
}
