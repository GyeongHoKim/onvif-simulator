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
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpmjpeg"

	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
)

// JPEGFrame is one complete JPEG image ready for RFC 2435 packetization. PTS
// is in the 90 kHz RTP clock; Image is a self-contained JPEG bytestream
// starting with SOI (0xFFD8) and ending with EOI (0xFFD9).
type JPEGFrame struct {
	PTS   int64
	NTP   time.Time
	Image []byte
}

// mjpegBufferSize bounds the JPEG frame channel. JPEG frames are larger than
// H.264 access units but the producer rate is the same (capped by source FPS),
// so a 32-frame buffer absorbs short consumer stalls without unbounded growth.
const mjpegBufferSize = 32

// MJPEGSource is a Source backed by an external JPEG-frame producer (an ffmpeg
// transcoder for kind=file profiles, or the rpicam secondary MJPEG stream for
// kind=rpicam profiles). The producer pushes JPEGFrame values via Push; the
// source's Run goroutine packetizes each frame as RTP/JPEG (RFC 2435) and
// writes the packets to the gortsplib stream.
//
// Unlike LiveSource, MJPEG carries no parameter sets and every frame is
// intra-only, so DESCRIBE only waits for the first frame to arrive rather
// than for an IDR + parameter sets. There is also no GOP cache because
// mid-stream joiners can decode the next frame standalone.
//
// MJPEGSource does not own the producer's lifecycle; callers stop the
// producer separately when they cancel the Source's ctx.
type MJPEGSource struct {
	probe  *ProbeResult
	logger *slog.Logger

	stream *gortsplib.ServerStream
	media  *description.Media

	in chan JPEGFrame

	closeMu sync.Mutex
	closed  bool

	readyOnce sync.Once
	ready     chan struct{}
}

// NewMJPEGSource builds an MJPEGSource described by probe. The probe's Codec
// must be CodecMJPEG; width/height/fps are used only for SDP construction and
// ONVIF advertisement. logger may be nil.
func NewMJPEGSource(probe *ProbeResult, logger *slog.Logger) *MJPEGSource {
	if logger == nil {
		logger = obs.Discard()
	}
	return &MJPEGSource{
		probe:  probe,
		logger: logger,
		in:     make(chan JPEGFrame, mjpegBufferSize),
		ready:  make(chan struct{}),
	}
}

// Push delivers a JPEG frame to the source. The call drops the frame
// non-blockingly when the buffer is full so a slow consumer does not stall
// the producer. Push is safe to call concurrently with Close — pushes after
// Close are dropped.
func (m *MJPEGSource) Push(f JPEGFrame) {
	m.closeMu.Lock()
	defer m.closeMu.Unlock()
	if m.closed {
		return
	}
	select {
	case m.in <- f:
	default:
		m.logger.Warn("rtsp mjpegsource: buffer full, dropping frame")
	}
}

// Close releases the frame channel. Idempotent and safe to call concurrently
// with Push.
func (m *MJPEGSource) Close() {
	m.closeMu.Lock()
	defer m.closeMu.Unlock()
	if m.closed {
		return
	}
	m.closed = true
	close(m.in)
}

// Describe satisfies Source.
func (m *MJPEGSource) Describe() *ProbeResult { return m.probe }

// AttachStream satisfies Source.
func (m *MJPEGSource) AttachStream(stream *gortsplib.ServerStream, media *description.Media) {
	m.stream = stream
	m.media = media
}

// Ready satisfies Source. Blocks until the first JPEG frame has arrived (so
// DESCRIBE never returns a stream that cannot yet be played), or ctx is
// canceled.
func (m *MJPEGSource) Ready(ctx context.Context) error {
	select {
	case <-m.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run drains the frame channel until ctx is canceled or the channel is
// closed. Frames before the first arrival close the ready channel so DESCRIBE
// unblocks; every frame is encoded as RTP/JPEG and written to the stream.
func (m *MJPEGSource) Run(ctx context.Context) error {
	enc := &rtpmjpeg.Encoder{}
	if err := enc.Init(); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case frame, ok := <-m.in:
			if !ok {
				return nil
			}
			packetized, err := m.writeFrame(enc, frame)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			}
			if packetized {
				m.readyOnce.Do(func() { close(m.ready) })
			}
		}
	}
}

// writeFrame returns packetized=true when the frame produced at least one
// RTP packet that was successfully written. Empty payloads and frames the
// encoder refuses (e.g. progressive JPEG) return packetized=false so the
// ready signal does not flip on a frame that never reaches the wire.
func (m *MJPEGSource) writeFrame(enc *rtpmjpeg.Encoder, f JPEGFrame) (bool, error) {
	if len(f.Image) == 0 {
		return false, nil
	}
	pkts, err := enc.Encode(f.Image)
	if err != nil {
		// Some encoders emit JPEG variants that RFC 2435 cannot packetize
		// (e.g. progressive, non-yuvj420/422). Log and drop the frame rather
		// than tearing the source down — the producer can keep emitting.
		m.logger.Warn("rtsp mjpegsource: rtp encode", "err", err, "bytes", len(f.Image))
		return false, nil
	}
	if len(pkts) == 0 {
		return false, nil
	}
	ts := uint32(f.PTS) //nolint:gosec // mod-2^32 wrap is the RTP timestamp semantic
	for _, pkt := range pkts {
		pkt.Timestamp = ts
		if writeErr := m.stream.WritePacketRTPWithNTP(m.media, pkt, f.NTP); writeErr != nil {
			return false, writeErr
		}
	}
	return true, nil
}

// Compile-time guard that MJPEGSource and the gortsplib MJPEG format stay in
// sync — Server passes one to the other in AddSource.
var _ format.Format = (*format.MJPEG)(nil)
