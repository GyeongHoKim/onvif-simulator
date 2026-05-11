//go:build !rpicam || !linux || (!arm && !arm64)

package rpicamera

import (
	"log/slog"
	"time"
)

// OnDataFunc receives one H.264 access unit per call (parity with the
// rpicam-tagged build's signature).
type OnDataFunc func(pts int64, ntp time.Time, au [][]byte)

// Available returns ErrUnsupported on the disabled build so callers can
// detect support without attempting to spawn the helper.
func Available() error { return ErrUnsupported }

// Camera is the disabled-build placeholder. Its methods are inert because
// Open never succeeds on this build.
type Camera struct{}

// Open is the disabled stub: every call returns ErrUnsupported. Callers
// detect this with errors.Is and surface a build-channel hint. The
// secondary-stream callback is accepted for signature parity with the
// rpicam-tagged build; on this build it is ignored.
func Open(_ Params, _ *slog.Logger, _ OnDataFunc, _ OnMJPEGDataFunc) (*Camera, error) {
	return nil, ErrUnsupported
}

// Close is a no-op on the disabled build.
func (*Camera) Close() {}

// Wait returns nil immediately on the disabled build.
func (*Camera) Wait() error { return nil }

// ReloadParams returns ErrUnsupported on the disabled build.
func (*Camera) ReloadParams(_ Params) error { return ErrUnsupported }
