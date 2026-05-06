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
	"github.com/pion/rtp"

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

// H.264 NAL unit types used by parameter-set extraction (ITU-T H.264 §7.3.1).
const (
	h264NALTypeSPS = 7
	h264NALTypePPS = 8
)

// maxGOPCachePackets caps the per-source GOP cache. At 30 fps with a
// V4L2-default IDRPeriod of 60 and a few RTP packets per AU, a typical GOP is
// 200–300 packets; 512 leaves head-room for higher-bitrate or longer GoPs
// without unbounded growth if an IDR somehow never arrives.
const maxGOPCachePackets = 512

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

	closeMu sync.Mutex
	closed  bool

	readyOnce sync.Once
	ready     chan struct{}

	// gopMu guards gopCache. The cache holds the RTP packets emitted for
	// the most recent complete (or in-progress) GOP — i.e. the last IDR's
	// packets followed by every subsequent non-IDR's packets, until the
	// next IDR replaces it. ReplayGOP snapshots this under the lock and
	// writes it to a newly-joining session so the client receives a
	// keyframe immediately instead of waiting for the next IDR period.
	gopMu    sync.Mutex
	gopCache []*rtp.Packet
}

// NewLiveSource builds a LiveSource for the codec described by probe. SPS
// and PPS may be empty on the probe — they are extracted from the first
// IDR access unit and written into the gortsplib H264 format before the
// ready signal releases DESCRIBE, so the SDP carries sprop-parameter-sets
// and clients are not stuck on "non-existing PPS referenced". logger may
// be nil.
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
// camera FPS. Push is safe to call concurrently with Close — pushes after
// Close are dropped.
func (l *LiveSource) Push(au AccessUnit) {
	l.closeMu.Lock()
	defer l.closeMu.Unlock()
	if l.closed {
		return
	}
	select {
	case l.in <- au:
	default:
		l.logger.Warn("rtsp livesource: buffer full, dropping access unit")
	}
}

// Close releases the AU channel. Idempotent and safe to call concurrently
// with Push.
func (l *LiveSource) Close() {
	l.closeMu.Lock()
	defer l.closeMu.Unlock()
	if l.closed {
		return
	}
	l.closed = true
	close(l.in)
}

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
			isIDR := hasH264IDR(au.NALs)
			if isIDR {
				l.fillH264ParameterSets(au.NALs)
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
			if err := l.writeAU(enc, au, isIDR); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			}
		}
	}
}

func (l *LiveSource) writeAU(enc *rtph264.Encoder, au AccessUnit, isIDR bool) error {
	pkts, err := enc.Encode(au.NALs)
	if err != nil {
		l.logger.Warn("rtsp livesource: rtp encode", "err", err)
		return nil
	}
	ts := uint32(au.PTS) //nolint:gosec // mod-2^32 wrap is the RTP timestamp semantic
	for _, pkt := range pkts {
		pkt.Timestamp = ts
	}

	// Update the GOP cache before broadcasting. Capturing before the
	// stream write keeps the cache consistent with what readers actually
	// receive, even if a write returns an error mid-batch.
	l.updateGOPCache(pkts, isIDR)

	for _, pkt := range pkts {
		if writeErr := l.stream.WritePacketRTP(l.media, pkt); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// updateGOPCache replaces or extends the cached GOP. Calling this with isIDR
// resets the cache to the IDR's packets; otherwise the new packets are
// appended. Appends after the cache reaches maxGOPCachePackets are dropped
// instead of growing without bound — the early portion of the GOP still
// bootstraps the decoder, and the next IDR rebuilds the cache from scratch.
func (l *LiveSource) updateGOPCache(pkts []*rtp.Packet, isIDR bool) {
	l.gopMu.Lock()
	defer l.gopMu.Unlock()
	if isIDR {
		l.gopCache = append(l.gopCache[:0], pkts...)
		return
	}
	if len(l.gopCache) == 0 {
		// No IDR has anchored the cache yet — Run drops pre-IDR access
		// units, so this branch only fires if the producer feeds us a
		// non-IDR AU after Ready closed without the corresponding IDR
		// having been cached. Skip the append rather than seeding the
		// cache with un-decodable slices.
		return
	}
	if len(l.gopCache)+len(pkts) > maxGOPCachePackets {
		l.logger.Warn("rtsp livesource: gop cache cap reached, dropping append",
			"have", len(l.gopCache), "incoming", len(pkts))
		return
	}
	l.gopCache = append(l.gopCache, pkts...)
}

// ReplayGOP writes the cached GOP to ss so a client that joined mid-stream
// receives the most recent IDR (and the slices that depend on it) before the
// live broadcast continues. Called from Server's OnPlay handler before the
// session is added to the stream's active-reader set, so existing readers do
// not see duplicates. A no-op when no GOP has been cached yet.
//
// Each cached *rtp.Packet is shallow-copied before write because gortsplib
// rewrites SSRC on the way out, and a stream-level broadcast may have already
// stamped the live SSRC onto the original. The session writer overwrites
// SSRC again, so the value we put on the clone does not matter.
func (l *LiveSource) ReplayGOP(ss *gortsplib.ServerSession) {
	if l.media == nil {
		return
	}

	l.gopMu.Lock()
	pkts := make([]*rtp.Packet, len(l.gopCache))
	for i, src := range l.gopCache {
		clone := *src
		pkts[i] = &clone
	}
	l.gopMu.Unlock()

	if len(pkts) == 0 {
		return
	}

	for _, pkt := range pkts {
		if err := ss.WritePacketRTP(l.media, pkt); err != nil {
			l.logger.Warn("rtsp livesource: replay gop write", "err", err)
			return
		}
	}
	l.logger.Debug("rtsp livesource: replayed gop", "packets", len(pkts))
}

// fillH264ParameterSets pulls SPS (NAL type 7) and PPS (NAL type 8) out of
// the first IDR access unit and writes them into the attached gortsplib
// format.H264 so DESCRIBE emits sprop-parameter-sets in the SDP.
//
// Without this, clients that subscribe between IDRs never see SPS/PPS in
// either the SDP (empty) or in-band (already passed) and decode loops
// indefinitely on "non-existing PPS referenced".
func (l *LiveSource) fillH264ParameterSets(nals [][]byte) {
	if l.media == nil || len(l.media.Formats) == 0 {
		return
	}
	h, ok := l.media.Formats[0].(*format.H264)
	if !ok {
		return
	}
	if len(h.SPS) > 0 && len(h.PPS) > 0 {
		return
	}
	var sps, pps []byte
	for _, nal := range nals {
		if len(nal) == 0 {
			continue
		}
		switch nal[0] & 0x1F {
		case h264NALTypeSPS:
			sps = nal
		case h264NALTypePPS:
			pps = nal
		}
	}
	if len(h.SPS) == 0 && len(sps) > 0 {
		h.SPS = sps
	}
	if len(h.PPS) == 0 && len(pps) > 0 {
		h.PPS = pps
	}
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
