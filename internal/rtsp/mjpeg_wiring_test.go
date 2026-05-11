package rtsp

import (
	"errors"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/ffmpeg"
	"github.com/GyeongHoKim/onvif-simulator/internal/rpicamera"
)

func TestAttachMJPEG_OnNonRPICamSourceReturnsFalse(t *testing.T) {
	t.Parallel()
	probe := &ProbeResult{Codec: CodecMJPEG, Width: 640, Height: 480, FPS: 30}
	src := NewMJPEGSource(probe, nil)
	if AttachMJPEG(src, func(JPEGFrame) {}) {
		t.Fatal("AttachMJPEG should return false for a non-rpicam source")
	}
}

func TestAttachMJPEG_OnRPICamSourceWiresCallback(t *testing.T) {
	t.Parallel()
	// NewRPICamSource short-circuits with ErrUnsupported on builds without
	// the rpicam tag, so we exercise AttachMJPEG by constructing the
	// struct directly with the unexported source type.
	live := NewLiveSource(&ProbeResult{Codec: CodecH264, Width: 1920, Height: 1080, FPS: 30}, nil)
	src := &rpicamSource{LiveSource: live, params: rpicamera.Params{}}

	called := false
	if !AttachMJPEG(src, func(JPEGFrame) { called = true }) {
		t.Fatal("AttachMJPEG should accept an rpicamSource")
	}
	if src.onMJPEG == nil {
		t.Fatal("AttachMJPEG should install the onMJPEG callback")
	}
	src.onMJPEG(1, time.Now(), []byte{0xFF, 0xD8, 0xFF, 0xD9})
	if !called {
		t.Fatal("installed callback was not invoked when frame arrived")
	}
}

func TestNewTranscodingSource_RejectsEmptyMediaPath(t *testing.T) {
	t.Parallel()
	_, err := NewTranscodingSource(ffmpeg.Params{}, 0, 0, 0, nil)
	if err == nil {
		t.Fatal("expected error from missing MediaFilePath / unavailable ffmpeg")
	}
	// Either way is acceptable: placeholder build returns ErrUnsupported,
	// availability success then fails on the MediaFilePath check.
	if !errors.Is(err, ffmpeg.ErrUnsupported) && err.Error() == "" {
		t.Fatalf("unexpected error shape: %v", err)
	}
}

func TestNewRPICamSource_DefaultBuildReturnsUnsupported(t *testing.T) {
	t.Parallel()
	if err := rpicamera.Available(); err == nil {
		t.Skip("rpicam tag is active in this build; skipping default-build assertion")
	}
	_, err := NewRPICamSource(rpicamera.Params{Width: 1920, Height: 1080, FPS: 30}, 1920, 1080, 30, nil)
	if !errors.Is(err, rpicamera.ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}
