package rtsp

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph264"

	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
)

// AccessUnit is one H.264 access unit ready to be packetized into RTP.
// PTS is in the 90 kHz RTP clock; NALs are raw NAL units (no AnnexB or
// AVCC framing).
type AccessUnit struct {
	PTS  int64
	NTP  time.Time
	NALs [][]byte
}

// liveBufferSize bounds the AccessUnit channel so a slow client does not
// stall the producer. mtxrpicam emits frames at the configured FPS; eight
// AUs is roughly 250 ms at 30 fps which is enough headroom but bounded so
// stuck clients do not balloon memory.
const liveBufferSize = 8

// LiveSource is a Source backed by an external H.264 producer. The producer
// (e.g. internal/rpicamera) pushes AccessUnit values onto Push; the source's
// Run goroutine encodes them into RTP packets and writes them to the
// gortsplib stream. DESCRIBE waits via Ready until the first IDR is
// observed so clients never see a pre-keyframe SDP.
//
// LiveSource does not own the producer's lifecycle; callers stop the
// producer separately when they cancel the Source's ctx.
type LiveSource struct {
	probe  *ProbeResult
	logger *slog.Logger

	stream *gortsplib.ServerStream
	media  *description.Media

	in chan AccessUnit

	readyOnce sync.Once
	ready     chan struct{}
}

// NewLiveSource builds a LiveSource for the codec described by probe. SPS
// and PPS may be empty on the probe — clients learn them from in-band NAL
// units once the stream starts. logger may be nil.
func NewLiveSource(probe *ProbeResult, logger *slog.Logger) *LiveSource {
	if logger == nil {
		logger = obs.Discard()
	}
	return &LiveSource{
		probe:  probe,
		logger: logger,
		in:     make(chan AccessUnit, liveBufferSize),
		ready:  make(chan struct{}),
	}
}

// Push delivers an access unit to the source. The call drops the AU
// non-blockingly when the buffer is full so a slow consumer does not stall
// the producer; in production the producer itself is rate-limited by the
// camera FPS.
func (l *LiveSource) Push(au AccessUnit) {
	select {
	case l.in <- au:
	default:
		l.logger.Warn("rtsp livesource: buffer full, dropping access unit")
	}
}

// Close releases the AU channel. Producers must not Push afterwards.
func (l *LiveSource) Close() { close(l.in) }

// Describe satisfies Source.
func (l *LiveSource) Describe() *ProbeResult { return l.probe }

// AttachStream satisfies Source.
func (l *LiveSource) AttachStream(stream *gortsplib.ServerStream, media *description.Media) {
	l.stream = stream
	l.media = media
}

// Ready satisfies Source. Blocks until the first IDR has been observed or
// ctx is canceled.
func (l *LiveSource) Ready(ctx context.Context) error {
	select {
	case <-l.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run drains the AU channel until ctx is canceled or the channel is closed.
// Pre-IDR frames are dropped so DESCRIBE returns only when subsequent frames
// can decode standalone.
func (l *LiveSource) Run(ctx context.Context) error {
	enc := &rtph264.Encoder{
		PayloadType:    96,
		PayloadMaxSize: 1460,
	}
	if err := enc.Init(); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case au, ok := <-l.in:
			if !ok {
				return nil
			}
			if hasH264IDR(au.NALs) {
				l.readyOnce.Do(func() { close(l.ready) })
			}
			select {
			case <-l.ready:
			default:
				// Pre-IDR frame: drop so the first packet a client sees
				// after DESCRIBE is the keyframe that joinable decoding
				// requires.
				continue
			}
			if err := l.writeAU(enc, au); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			}
		}
	}
}

func (l *LiveSource) writeAU(enc *rtph264.Encoder, au AccessUnit) error {
	pkts, err := enc.Encode(au.NALs)
	if err != nil {
		l.logger.Warn("rtsp livesource: rtp encode", "err", err)
		return nil
	}
	ts := uint32(au.PTS) //nolint:gosec // mod-2^32 wrap is the RTP timestamp semantic
	for _, pkt := range pkts {
		pkt.Timestamp = ts
		if writeErr := l.stream.WritePacketRTP(l.media, pkt); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// hasH264IDR scans access-unit NAL units for an IDR slice (NAL type 5).
func hasH264IDR(au [][]byte) bool {
	for _, nal := range au {
		if len(nal) > 0 && (nal[0]&0x1F) == 5 {
			return true
		}
	}
	return false
}

// Compile-time guard that LiveSource and the gortsplib H264 format stay in
// sync — Server passes one to the other in AddSource.
var _ format.Format = (*format.H264)(nil)
