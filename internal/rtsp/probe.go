// Package rtsp provides an embedded RTSP server that loops local mp4 files.
//
// Each ONVIF media profile is mapped to one URL path on a single shared RTSP
// listener (e.g. rtsp://<host>:8554/<profile-token>). For every registered
// source the server reads an mp4 file from disk, packetizes its access units
// into RTP, and rewinds when the file ends so the stream loops indefinitely.
package rtsp

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	mp4codecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/pmp4"
)

// Codec names used in ProbeResult and exposed via ONVIF
// VideoEncoderConfiguration.
const (
	CodecH264 = "H264"
	CodecH265 = "H265"
)

// ErrNoVideoTrack is returned when an mp4 file does not contain a video track
// in a supported codec (H264 or H265).
var ErrNoVideoTrack = errors.New("rtsp: mp4 file has no supported video track")

// ProbeResult is the metadata extracted from an mp4 file. The simulator uses
// these values to populate the ONVIF VideoEncoderConfiguration that describes
// the embedded stream.
type ProbeResult struct {
	Codec  string // "H264" or "H265"
	Width  int
	Height int
	FPS    int

	// SPS, PPS, VPS are codec parameter sets. VPS is only set for H265.
	// They are needed to configure the gortsplib RTP packetizer.
	SPS []byte
	PPS []byte
	VPS []byte
}

// Probe opens the mp4 file at path, finds the first supported video track, and
// returns codec metadata. It is safe to call before the simulator's RTSP server
// is started — Probe does not retain any file handle.
func Probe(path string) (*ProbeResult, error) {
	f, err := openMP4(path)
	if err != nil {
		return nil, fmt.Errorf("rtsp: open %s: %w", path, err)
	}
	defer closeQuiet(f)

	return probeReader(f)
}

func probeReader(r io.ReadSeeker) (*ProbeResult, error) {
	pres := &pmp4.Presentation{}
	if err := pres.Unmarshal(r); err != nil {
		return nil, fmt.Errorf("rtsp: parse mp4: %w", err)
	}

	for _, t := range pres.Tracks {
		switch c := t.Codec.(type) {
		case *mp4codecs.H264:
			return probeH264(t, c)
		case *mp4codecs.H265:
			return probeH265(t, c)
		}
	}
	return nil, ErrNoVideoTrack
}

func probeH264(t *pmp4.Track, c *mp4codecs.H264) (*ProbeResult, error) {
	sps := &h264.SPS{}
	if err := sps.Unmarshal(c.SPS); err != nil {
		return nil, fmt.Errorf("rtsp: parse H264 SPS: %w", err)
	}
	return &ProbeResult{
		Codec:  CodecH264,
		Width:  sps.Width(),
		Height: sps.Height(),
		FPS:    fpsFromSamples(t),
		SPS:    c.SPS,
		PPS:    c.PPS,
	}, nil
}

func probeH265(t *pmp4.Track, c *mp4codecs.H265) (*ProbeResult, error) {
	sps := &h265.SPS{}
	if err := sps.Unmarshal(c.SPS); err != nil {
		return nil, fmt.Errorf("rtsp: parse H265 SPS: %w", err)
	}
	return &ProbeResult{
		Codec:  CodecH265,
		Width:  sps.Width(),
		Height: sps.Height(),
		FPS:    fpsFromSamples(t),
		SPS:    c.SPS,
		PPS:    c.PPS,
		VPS:    c.VPS,
	}, nil
}

// openMP4 wraps os.Open with the //nolint:gosec exception applied — mp4 paths
// always originate from operator configuration, never from network input.
func openMP4(path string) (*os.File, error) {
	return os.Open(path) //nolint:gosec // operator-controlled config path
}

// closeQuiet swallows io.Closer errors. The simulator's mp4 sources are
// read-only, so a Close failure carries no information the caller can act on.
func closeQuiet(c io.Closer) { _ = c.Close() } //nolint:errcheck // see comment

// fpsFromSamples derives the average frame rate from sample durations. We use
// the average over the whole track rather than the first sample only because
// some encoders write a slightly different first-sample duration.
func fpsFromSamples(t *pmp4.Track) int {
	if len(t.Samples) == 0 || t.TimeScale == 0 {
		return 0
	}
	var totalDuration uint64
	for _, s := range t.Samples {
		totalDuration += uint64(s.Duration)
	}
	if totalDuration == 0 {
		return 0
	}
	num := uint64(len(t.Samples)) * uint64(t.TimeScale)
	rounded := (num + totalDuration/2) / totalDuration
	if rounded > uint64(^uint(0)>>1) {
		return 0
	}
	return int(rounded)
}
