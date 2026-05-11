package rtsp

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/ffmpeg"
)

var (
	errTestTranscoderWait = errors.New("test transcoder wait failure")
	errTestTranscoderOpen = errors.New("test transcoder open failure")
)

func withOpenTranscoderHook(t *testing.T, hook func(ffmpeg.Params, *slog.Logger, ffmpeg.OnJPEGDataFunc) (transcoderSession, error)) {
	t.Helper()
	prev := openTranscoderHook
	openTranscoderHook = hook
	t.Cleanup(func() { openTranscoderHook = prev })
}

type fakeTranscoder struct {
	waitFn  func() error
	closeFn func()
	closed  sync.Once
}

func (f *fakeTranscoder) Close() {
	f.closed.Do(func() {
		if f.closeFn != nil {
			f.closeFn()
		}
	})
}

func (f *fakeTranscoder) Wait() error {
	if f.waitFn != nil {
		return f.waitFn()
	}
	return nil
}

func TestTranscodingSource_Run_transcoderExitsCleanly(t *testing.T) {
	withOpenTranscoderHook(t, func(
		ffmpeg.Params, *slog.Logger, ffmpeg.OnJPEGDataFunc,
	) (transcoderSession, error) {
		return &fakeTranscoder{}, nil
	})

	ts := &TranscodingSource{
		MJPEGSource: NewMJPEGSource(&ProbeResult{
			Codec: CodecMJPEG, Width: 640, Height: 480, FPS: 30,
		}, nil),
		params: ffmpeg.Params{MediaFilePath: "/tmp/x.mp4"},
		logger: slog.New(slog.DiscardHandler),
	}
	if err := ts.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestTranscodingSource_Run_transcoderWaitError(t *testing.T) {
	withOpenTranscoderHook(t, func(
		ffmpeg.Params, *slog.Logger, ffmpeg.OnJPEGDataFunc,
	) (transcoderSession, error) {
		return &fakeTranscoder{waitFn: func() error { return errTestTranscoderWait }}, nil
	})

	ts := &TranscodingSource{
		MJPEGSource: NewMJPEGSource(&ProbeResult{
			Codec: CodecMJPEG, Width: 640, Height: 480, FPS: 30,
		}, nil),
		params: ffmpeg.Params{MediaFilePath: "/tmp/x.mp4"},
		logger: slog.New(slog.DiscardHandler),
	}
	err := ts.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from transcoder Wait")
	}
	if !errors.Is(err, errTestTranscoderWait) {
		t.Fatalf("error chain: %v", err)
	}
}

func TestTranscodingSource_Run_openError(t *testing.T) {
	withOpenTranscoderHook(t, func(
		ffmpeg.Params, *slog.Logger, ffmpeg.OnJPEGDataFunc,
	) (transcoderSession, error) {
		return nil, errTestTranscoderOpen
	})

	ts := &TranscodingSource{
		MJPEGSource: NewMJPEGSource(&ProbeResult{
			Codec: CodecMJPEG, Width: 640, Height: 480, FPS: 30,
		}, nil),
		params: ffmpeg.Params{MediaFilePath: "/tmp/x.mp4"},
		logger: slog.New(slog.DiscardHandler),
	}
	err := ts.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from open")
	}
	if !errors.Is(err, errTestTranscoderOpen) {
		t.Fatalf("expected wrapped openErr, got %v", err)
	}
}

func TestTranscodingSource_Run_contextCanceled(t *testing.T) {
	unblock := make(chan struct{})
	withOpenTranscoderHook(t, func(
		ffmpeg.Params, *slog.Logger, ffmpeg.OnJPEGDataFunc,
	) (transcoderSession, error) {
		return &fakeTranscoder{
			waitFn: func() error {
				<-unblock
				return nil
			},
			closeFn: func() { close(unblock) },
		}, nil
	})

	ts := &TranscodingSource{
		MJPEGSource: NewMJPEGSource(&ProbeResult{
			Codec: CodecMJPEG, Width: 640, Height: 480, FPS: 30,
		}, nil),
		params: ffmpeg.Params{MediaFilePath: "/tmp/x.mp4"},
		logger: slog.New(slog.DiscardHandler),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ts.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestTranscodingSource_Run_shutdownCallsTranscoderCloseOnce(t *testing.T) {
	var closes atomic.Int32
	withOpenTranscoderHook(t, func(
		ffmpeg.Params, *slog.Logger, ffmpeg.OnJPEGDataFunc,
	) (transcoderSession, error) {
		return &fakeTranscoder{
			waitFn: func() error { return nil },
			closeFn: func() {
				closes.Add(1)
			},
		}, nil
	})

	ts := &TranscodingSource{
		MJPEGSource: NewMJPEGSource(&ProbeResult{
			Codec: CodecMJPEG, Width: 640, Height: 480, FPS: 30,
		}, nil),
		params: ffmpeg.Params{MediaFilePath: "/tmp/x.mp4"},
		logger: slog.New(slog.DiscardHandler),
	}
	if err := ts.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if closes.Load() != 1 {
		t.Fatalf("expected exactly one transcoder Close from shutdown, got %d", closes.Load())
	}
}
