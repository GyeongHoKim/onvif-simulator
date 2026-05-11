// Package ffmpeg embeds a minimal LGPL-only static ffmpeg binary per
// supported platform and spawns it as a subprocess to transcode an MP4
// (H.264/H.265) into a continuous MJPEG byte stream. The simulator wires
// each MJPEG sibling profile's RTSP source to one Transcoder so Profile S
// §7.9 is satisfied without taking a CGO dependency.
//
// The binary is selected by runtime.GOOS/GOARCH and extracted to a
// temporary directory at Open time, mirroring the internal/rpicamera
// pattern. The package compiles on every supported platform; when the
// embedded blob is a placeholder (no real binary fetched yet, e.g. before
// CI runs scripts/fetch-ffmpeg.sh), Open returns ErrUnsupported so the
// simulator can fail with a clear hint instead of trying to exec a 1-byte
// file.
package ffmpeg

import (
	"errors"
	"time"
)

// ErrUnsupported is returned when no real ffmpeg binary is embedded for
// the current platform — usually because CI has not run the fetch script,
// or because the platform is genuinely unsupported. Callers detect it via
// errors.Is.
var ErrUnsupported = errors.New("ffmpeg: no embedded binary available for this platform")

// Params configures one Transcoder instance.
type Params struct {
	// MediaFilePath is an absolute path to the source media (typically an
	// .mp4 with H.264 or H.265 video). The Transcoder reads this file
	// directly with ffmpeg's input demuxer; no in-process preprocessing.
	MediaFilePath string

	// Quality is the MJPEG quantization quality scale fed to ffmpeg's
	// -q:v flag. Range 2 (best) to 31 (worst). 5 matches mediamtx's
	// stock value; lower numbers produce larger but cleaner frames.
	// Zero falls back to 5.
	Quality int

	// FPS is the output frame rate in frames per second. Zero falls back
	// to the source's native FPS. The simulator passes the probed source
	// FPS so the MJPEG sibling mirrors the H.264 cadence.
	FPS int

	// LoopForever, when true, instructs ffmpeg to rewind the input file
	// at EOF so the simulator's MJPEG sibling streams continuously for
	// kind=file profiles (mirroring the H.264 looper's behavior).
	LoopForever bool
}

// OnJPEGDataFunc receives one complete JPEG image per call. pts is in the
// 90 kHz RTP clock; ntp is the wall-clock anchor for RTCP SR. jpeg is a
// self-contained JPEG bytestream starting with SOI (0xFFD8) and ending
// with EOI (0xFFD9). The callback runs on the Transcoder's reader
// goroutine — heavy work should be offloaded.
type OnJPEGDataFunc func(pts int64, ntp time.Time, jpeg []byte)

// Available reports whether a usable ffmpeg binary is embedded for the
// current platform. Returns nil on success; ErrUnsupported otherwise.
// Use this to detect support without actually spawning the helper.
func Available() error { return availableImpl() }
