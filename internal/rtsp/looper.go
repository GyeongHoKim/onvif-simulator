package rtsp

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph264"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/pmp4"
	"github.com/pion/rtp"
)

// videoClockRate is the RTP clock rate for both H264 and H265 (RFC 6184 / RFC
// 7798). The MP4 sample timescale typically differs, so we re-scale the PTS
// before stamping RTP packets.
const videoClockRate = 90000

// ErrMediaHasNoFormat means a description.Media was registered without any
// Format entry — this is a programming error inside this package.
var ErrMediaHasNoFormat = errors.New("rtsp: media has no format")

// ErrUnsupportedRTPFormat is returned by createEncoder for any gortsplib
// format other than H264/H265.
var ErrUnsupportedRTPFormat = errors.New("rtsp: unsupported rtp format")

// codecEncoder is the small surface looper needs from gortsplib's H264/H265
// RTP encoders so we can keep one code path for both codecs.
type codecEncoder interface {
	Encode(au [][]byte) ([]*rtp.Packet, error)
}

// looper reads access units from an mp4 file, paces playback against
// wall-clock time, and writes RTP packets to a ServerStream. When the file
// ends the looper rewinds and continues with monotonically-increasing RTP
// timestamps so clients see one continuous stream.
type looper struct {
	path  string
	probe *ProbeResult

	stream *gortsplib.ServerStream
	media  *description.Media
}

func newLooper(
	path string,
	stream *gortsplib.ServerStream,
	media *description.Media,
	probe *ProbeResult,
) *looper {
	return &looper{
		path:   path,
		probe:  probe,
		stream: stream,
		media:  media,
	}
}

func (l *looper) run(ctx context.Context) {
	enc, err := l.createEncoder()
	if err != nil {
		return
	}

	startTS, err := randomUint32()
	if err != nil {
		return
	}

	// rtpOffset accumulates across loop iterations so the timestamp stays
	// monotonic when the file rewinds.
	rtpOffset := startTS
	for {
		if ctx.Err() != nil {
			return
		}
		nextOffset, err := l.playOnce(ctx, enc, rtpOffset)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			// Any other error during a single loop iteration ends the
			// goroutine — clients will get connection-closed and can
			// reconnect once the source is reconfigured.
			return
		}
		rtpOffset = nextOffset
	}
}

// playOnce iterates the file once. It returns the RTP timestamp to use as the
// base for the next iteration so the stream remains monotonic across rewinds.
func (l *looper) playOnce(
	ctx context.Context,
	enc codecEncoder,
	baseTS uint32,
) (uint32, error) {
	f, err := openMP4(l.path)
	if err != nil {
		return baseTS, err
	}
	defer closeQuiet(f)

	pres := &pmp4.Presentation{}
	if parseErr := pres.Unmarshal(f); parseErr != nil {
		return baseTS, fmt.Errorf("rtsp: parse mp4 during playback: %w", parseErr)
	}
	track, err := videoTrack(pres)
	if err != nil {
		return baseTS, err
	}

	startWall := time.Now()
	var elapsed90k uint32 // PTS of the next sample in 90kHz units, relative to baseTS
	lastTS := baseTS
	for _, sample := range track.Samples {
		if ctx.Err() != nil {
			return lastTS, ctx.Err()
		}

		// Pacing: sleep until this sample's wall-clock playback time. Skipping
		// the first sample lets the first frame go out immediately.
		target := time.Duration(elapsed90k) * time.Second / time.Duration(videoClockRate)
		drift := target - time.Since(startWall)
		if drift > 0 {
			select {
			case <-ctx.Done():
				return lastTS, ctx.Err()
			case <-time.After(drift):
			}
		}

		payload, payloadErr := sample.GetPayload()
		if payloadErr != nil {
			return lastTS, fmt.Errorf("rtsp: read sample: %w", payloadErr)
		}
		nalus, splitErr := splitAVCC(payload)
		if splitErr != nil {
			return lastTS, splitErr
		}
		if len(nalus) == 0 {
			elapsed90k += scale90k(sample.Duration, track.TimeScale)
			continue
		}

		ts := baseTS + elapsed90k
		packets, encErr := enc.Encode(nalus)
		if encErr != nil {
			return lastTS, fmt.Errorf("rtsp: encode rtp: %w", encErr)
		}
		for _, pkt := range packets {
			pkt.Timestamp = ts
			if writeErr := l.stream.WritePacketRTP(l.media, pkt); writeErr != nil {
				return lastTS, fmt.Errorf("rtsp: write rtp: %w", writeErr)
			}
		}
		lastTS = ts

		elapsed90k += scale90k(sample.Duration, track.TimeScale)
	}

	// Reserve room for one extra frame at the end so the next iteration's
	// timestamp does not collide with the last packet of this iteration.
	return lastTS + 1, nil
}

func (l *looper) createEncoder() (codecEncoder, error) {
	if len(l.media.Formats) == 0 {
		return nil, ErrMediaHasNoFormat
	}
	switch f := l.media.Formats[0].(type) {
	case *format.H264:
		e, err := f.CreateEncoder()
		if err != nil {
			return nil, err
		}
		return rtpEncH264{e}, nil
	case *format.H265:
		e, err := f.CreateEncoder()
		if err != nil {
			return nil, err
		}
		return rtpEncH265{e}, nil
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupportedRTPFormat, f)
	}
}

type rtpEncH264 struct{ *rtph264.Encoder }

func (e rtpEncH264) Encode(au [][]byte) ([]*rtp.Packet, error) {
	return e.Encoder.Encode(au)
}

type rtpEncH265 struct{ *rtph265.Encoder }

func (e rtpEncH265) Encode(au [][]byte) ([]*rtp.Packet, error) {
	return e.Encoder.Encode(au)
}

// videoTrack returns the first track whose Codec matches one of the supported
// MP4 video codecs.
func videoTrack(p *pmp4.Presentation) (*pmp4.Track, error) {
	for _, t := range p.Tracks {
		if t.Codec != nil && t.Codec.IsVideo() {
			return t, nil
		}
	}
	return nil, ErrNoVideoTrack
}

// scale90k converts a duration in the source TimeScale into 90 kHz units.
func scale90k(dur, timescale uint32) uint32 {
	if timescale == 0 {
		return 0
	}
	scaled := uint64(dur) * uint64(videoClockRate) / uint64(timescale)
	if scaled > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(scaled)
}

// splitAVCC parses the AVCC (length-prefixed NAL) byte layout used by mp4
// samples for both H264 and H265 into a slice of raw NAL units. An empty
// buffer is allowed and yields zero NAL units rather than an error so the
// caller can simply skip the sample.
func splitAVCC(buf []byte) ([][]byte, error) {
	if len(buf) == 0 {
		return nil, nil
	}
	au := h264.AVCC{}
	if err := au.Unmarshal(buf); err != nil {
		if errors.Is(err, h264.ErrAVCCNoNALUs) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, nil
		}
		return nil, fmt.Errorf("rtsp: parse avcc: %w", err)
	}
	return au, nil
}

func randomUint32() (uint32, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]), nil
}
