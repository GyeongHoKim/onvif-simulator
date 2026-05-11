package rtsp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/ffmpeg"
	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
)

// errTranscodingMediaFilePathRequired is returned by NewTranscodingSource
// when Params arrives without a source file path.
var errTranscodingMediaFilePathRequired = errors.New(
	"rtsp: TranscodingSource requires Params.MediaFilePath")

// TranscodingSource wraps an MJPEGSource with an ffmpeg.Transcoder that
// reads a local media file (typically the H.264/H.265 MP4 a kind=file
// profile points at), transcodes it to MJPEG via the embedded ffmpeg
// helper, and pushes each JPEG frame into the source for RTP packetization.
//
// One TranscodingSource backs each MJPEG sibling profile in the kind=file
// branch of the simulator's wiring. The kind=rpicam path uses a plain
// MJPEGSource fed directly from the rpicamera secondary stream and does
// not need a transcoder.
type TranscodingSource struct {
	*MJPEGSource

	params ffmpeg.Params
	logger *slog.Logger

	closeOnce sync.Once
}

// NewTranscodingSource builds a TranscodingSource for the configured
// source file. width/height/fps describe the *output* MJPEG stream and
// are advertised via Describe(); ffmpeg takes its dimensions from the
// input file unless Params.FPS is non-zero. logger may be nil.
//
// Returns ffmpeg.ErrUnsupported on builds without a real ffmpeg binary
// embedded (placeholder only); callers should surface this so a kind=file
// MJPEG sibling fails fast at simulator startup rather than at first
// DESCRIBE.
func NewTranscodingSource(
	params ffmpeg.Params, width, height, fps int, logger *slog.Logger,
) (Source, error) {
	if err := ffmpeg.Available(); err != nil {
		return nil, err
	}
	if params.MediaFilePath == "" {
		return nil, errTranscodingMediaFilePathRequired
	}
	if logger == nil {
		logger = obs.Discard()
	}
	probe := &ProbeResult{
		Codec:  CodecMJPEG,
		Width:  width,
		Height: height,
		FPS:    fps,
	}
	return &TranscodingSource{
		MJPEGSource: NewMJPEGSource(probe, logger),
		params:      params,
		logger:      logger,
	}, nil
}

// Run spawns the ffmpeg subprocess and drains its JPEG output into the
// embedded MJPEGSource until ctx is canceled, the transcoder exits, or
// the source's Run loop returns.
func (t *TranscodingSource) Run(ctx context.Context) error {
	tc, err := ffmpeg.Open(t.params, t.logger, func(pts int64, ntp time.Time, jpeg []byte) {
		t.Push(JPEGFrame{PTS: pts, NTP: ntp, Image: jpeg})
	})
	if err != nil {
		return fmt.Errorf("rtsp: open transcoder: %w", err)
	}

	tcDone := make(chan error, 1)
	go func() { tcDone <- tc.Wait() }()

	srcDone := make(chan error, 1)
	go func() { srcDone <- t.MJPEGSource.Run(ctx) }()

	shutdown := func() {
		t.closeOnce.Do(func() {
			tc.Close()
			t.Close()
		})
	}

	select {
	case tcErr := <-tcDone:
		shutdown()
		<-srcDone
		if tcErr != nil {
			return fmt.Errorf("rtsp: transcoder exited: %w", tcErr)
		}
		return nil
	case srcErr := <-srcDone:
		shutdown()
		<-tcDone
		return srcErr
	case <-ctx.Done():
		shutdown()
		<-tcDone
		<-srcDone
		return ctx.Err()
	}
}
