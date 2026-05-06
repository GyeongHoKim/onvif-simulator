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

// rpicamSource bundles a LiveSource with the rpicamera.Camera that feeds
// it. Run spawns the camera helper; ctx cancellation tears it down.
type rpicamSource struct {
	*LiveSource
	params rpicamera.Params
	logger *slog.Logger

	closeOnce sync.Once
}

// Run opens the camera helper, drains its access units into LiveSource, and
// blocks until ctx is canceled, the helper exits, or LiveSource.Run returns.
func (r *rpicamSource) Run(ctx context.Context) error {
	cam, err := rpicamera.Open(r.params, r.logger, func(pts int64, ntp time.Time, au [][]byte) {
		r.Push(AccessUnit{PTS: pts, NTP: ntp, NALs: au})
	})
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
