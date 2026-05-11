package rtsp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
	"github.com/GyeongHoKim/onvif-simulator/internal/rpicamera"
)

// NewRPICamSource adapts an rpicamera.Camera into a Source. Construction
// is cheap — the camera helper subprocess is not spawned until Run.
//
// On builds without the rpicam tag (or on platforms outside linux/arm{,64})
// rpicamera.Available returns ErrUnsupported and this constructor surfaces
// it unchanged so the simulator can fail with a clear "this build does not
// support the Raspberry Pi camera" message at startup.
func NewRPICamSource(p rpicamera.Params, width, height, fps int, logger *slog.Logger) (Source, error) {
	if err := rpicamera.Available(); err != nil {
		return nil, err
	}
	if logger == nil {
		logger = obs.Discard()
	}
	probe := &ProbeResult{
		Codec:  CodecH264,
		Width:  width,
		Height: height,
		FPS:    fps,
	}
	live := NewLiveSource(probe, logger)
	return &rpicamSource{LiveSource: live, params: p, logger: logger}, nil
}

// AttachMJPEG wires a callback that receives each JPEG frame from the
// helper's secondary MJPEG stream (Pi ISP hardware encoder). The simulator
// calls this after building the rpicam Source but before its Run goroutine
// starts so the secondary stream gets enabled at helper-open time. Calling
// AttachMJPEG once Run has already opened the helper is a no-op — the
// secondary stream can only be configured at startup.
func AttachMJPEG(s Source, push func(JPEGFrame)) bool {
	rs, ok := s.(*rpicamSource)
	if !ok {
		return false
	}
	rs.onMJPEG = func(pts int64, ntp time.Time, jpeg []byte) {
		push(JPEGFrame{PTS: pts, NTP: ntp, Image: jpeg})
	}
	return true
}

// rpicamSource bundles a LiveSource with the rpicamera.Camera that feeds
// it. Run spawns the camera helper; ctx cancellation tears it down.
type rpicamSource struct {
	*LiveSource
	params  rpicamera.Params
	logger  *slog.Logger
	onMJPEG rpicamera.OnMJPEGDataFunc

	closeOnce sync.Once
}

// Run opens the camera helper, drains its access units into LiveSource, and
// blocks until ctx is canceled, the helper exits, or LiveSource.Run returns.
// onMJPEG, when non-nil, enables the helper's secondary MJPEG stream and
// forwards each JPEG frame to the callback; the simulator's Section F
// wiring uses this to feed an MJPEGSource for the Profile S §7.9 sibling.
func (r *rpicamSource) Run(ctx context.Context) error {
	cam, err := rpicamera.Open(r.params, r.logger, func(pts int64, ntp time.Time, au [][]byte) {
		r.Push(AccessUnit{PTS: pts, NTP: ntp, NALs: au})
	}, r.onMJPEG)
	if err != nil {
		return fmt.Errorf("rpicamera: open: %w", err)
	}

	cameraDone := make(chan error, 1)
	go func() { cameraDone <- cam.Wait() }()

	liveDone := make(chan error, 1)
	go func() { liveDone <- r.LiveSource.Run(ctx) }()

	shutdown := func() {
		r.closeOnce.Do(func() {
			cam.Close()
			r.Close()
		})
	}

	select {
	case err := <-cameraDone:
		shutdown()
		<-liveDone
		if err != nil {
			return fmt.Errorf("rpicamera: camera exited: %w", err)
		}
		return nil
	case err := <-liveDone:
		shutdown()
		<-cameraDone
		return err
	case <-ctx.Done():
		shutdown()
		<-cameraDone
		<-liveDone
		return ctx.Err()
	}
}
