package rtsp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
)

func TestBuildMediaMJPEG(t *testing.T) {
	t.Parallel()
	probe := &ProbeResult{Codec: CodecMJPEG, Width: 640, Height: 480, FPS: 30}
	media, err := buildMedia(probe)
	if err != nil {
		t.Fatalf("buildMedia(MJPEG) returned error: %v", err)
	}
	if len(media.Formats) != 1 {
		t.Fatalf("expected one format, got %d", len(media.Formats))
	}
	if _, ok := media.Formats[0].(*format.MJPEG); !ok {
		t.Errorf("expected *format.MJPEG, got %T", media.Formats[0])
	}
}

func TestBuildMediaRejectsUnknownCodec(t *testing.T) {
	t.Parallel()
	_, err := buildMedia(&ProbeResult{Codec: "FAKE"})
	if err == nil {
		t.Fatal("expected error for unknown codec")
	}
}

func TestMJPEGSourceDescribe(t *testing.T) {
	t.Parallel()
	probe := &ProbeResult{Codec: CodecMJPEG, Width: 1280, Height: 720, FPS: 30}
	src := NewMJPEGSource(probe, nil)
	got := src.Describe()
	if got != probe {
		t.Errorf("Describe() should return the same pointer as constructed; got %p want %p", got, probe)
	}
}

func TestMJPEGSourceReadyBlocksUntilFirstFrame(t *testing.T) {
	t.Parallel()
	probe := &ProbeResult{Codec: CodecMJPEG, Width: 640, Height: 480, FPS: 30}
	src := NewMJPEGSource(probe, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := src.Ready(ctx); err == nil {
		t.Fatal("Ready should block until first frame; expected timeout error")
	}
}

func TestMJPEGSourcePushAfterCloseIsDropped(t *testing.T) {
	t.Parallel()
	probe := &ProbeResult{Codec: CodecMJPEG, Width: 640, Height: 480, FPS: 30}
	src := NewMJPEGSource(probe, nil)
	src.Close()
	// Should not panic.
	src.Push(JPEGFrame{PTS: 1, NTP: time.Now(), Image: []byte{0xFF, 0xD8, 0xFF, 0xD9}})
	// Idempotent close.
	src.Close()
}

func TestMJPEGSourceRunExitsOnCancel(t *testing.T) {
	t.Parallel()
	probe := &ProbeResult{Codec: CodecMJPEG, Width: 640, Height: 480, FPS: 30}
	src := NewMJPEGSource(probe, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- src.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run on cancel should return context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not exit within 1s of ctx cancel")
	}
}

func TestMJPEGSourceRunExitsOnChannelClose(t *testing.T) {
	t.Parallel()
	probe := &ProbeResult{Codec: CodecMJPEG, Width: 640, Height: 480, FPS: 30}
	src := NewMJPEGSource(probe, nil)
	done := make(chan error, 1)
	go func() { done <- src.Run(context.Background()) }()
	src.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run on close should return nil, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not exit within 1s of Close")
	}
}

func TestMJPEGSourceAttachStreamStores(t *testing.T) {
	t.Parallel()
	probe := &ProbeResult{Codec: CodecMJPEG, Width: 640, Height: 480, FPS: 30}
	src := NewMJPEGSource(probe, nil)
	// AttachStream just stores pointers; verify it doesn't panic on nil.
	src.AttachStream(nil, nil)
	if src.stream != nil || src.media != nil {
		t.Errorf("expected nil pointers after nil AttachStream, got %v, %v", src.stream, src.media)
	}
}
