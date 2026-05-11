package rpicamera

import (
	"errors"
	"time"
)

// ErrUnsupported is returned by Open on builds without the rpicam tag, or on
// platforms outside linux/arm and linux/arm64. Callers detect it via
// errors.Is so they can produce a friendly "this build does not support the
// Raspberry Pi camera" message instead of a stack trace.
var ErrUnsupported = errors.New("rpicamera: not built into this binary")

// OnMJPEGDataFunc receives one complete JPEG image per call from the
// mtxrpicam secondary stream. pts is in the 90 kHz RTP clock; ntp is the
// wall-clock capture time; jpeg is a self-contained JPEG bytestream (SOI to
// EOI) produced by the Pi ISP's hardware JPEG encoder. The simulator wires
// this callback into an rtsp.MJPEGSource to satisfy Profile S §7.9 without
// software transcoding for the rpicam path.
//
// Pass a non-nil callback to Open to enable the secondary stream; passing
// nil keeps the helper's secondary stream disabled (zero Pi resource cost
// when no MJPEG sibling profile is registered).
type OnMJPEGDataFunc func(pts int64, ntp time.Time, jpeg []byte)

// Params is the simulator-facing subset of mtxrpicam parameters. Field names
// match the corresponding mediamtx YAML keys so upstream documentation
// applies unchanged. The full upstream parameter set is filled with sensible
// defaults inside the package.
type Params struct {
	CameraID   uint32
	Width      uint32
	Height     uint32
	FPS        float32
	Bitrate    uint32
	IDRPeriod  uint32
	HFlip      bool
	VFlip      bool
	Brightness float32
	Contrast   float32
	Saturation float32
	Sharpness  float32
}
