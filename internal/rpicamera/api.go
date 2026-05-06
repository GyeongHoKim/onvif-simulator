package rpicamera

import "errors"

// ErrUnsupported is returned by Open on builds without the rpicam tag, or on
// platforms outside linux/arm and linux/arm64. Callers detect it via
// errors.Is so they can produce a friendly "this build does not support the
// Raspberry Pi camera" message instead of a stack trace.
var ErrUnsupported = errors.New("rpicamera: not built into this binary")

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
