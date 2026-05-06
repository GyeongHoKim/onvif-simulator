package rpicamera_test

import (
	"errors"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/rpicamera"
)

// TestOpenReturnsUnsupportedOnDefaultBuild ensures the simulator's default
// build channel surfaces a clean ErrUnsupported when an operator configures
// kind=rpicam without the rpicam-tagged binary. The rpicam-tagged build
// substitutes a real Camera in camera_rpicam.go, but its initialization is
// only exercised on actual Pi hardware (manual e2e step in the plan).
func TestOpenReturnsUnsupportedOnDefaultBuild(t *testing.T) {
	t.Parallel()
	cam, err := rpicamera.Open(
		rpicamera.Params{Width: 1920, Height: 1080, FPS: 30},
		nil,
		func(int64, time.Time, [][]byte) {},
	)
	_ = cam
	if !errors.Is(err, rpicamera.ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}

func TestDisabledStubMethods(t *testing.T) {
	t.Parallel()
	var c rpicamera.Camera
	c.Close()
	if err := c.Wait(); err != nil {
		t.Fatalf("Wait on default build should be nil, got %v", err)
	}
	if err := c.ReloadParams(rpicamera.Params{}); !errors.Is(err, rpicamera.ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported from ReloadParams, got %v", err)
	}
}

func TestParamsHydrateDefaults(t *testing.T) {
	t.Parallel()
	got := rpicamera.HydrateForTest(rpicamera.Params{})
	if got.Width != 1920 || got.Height != 1080 {
		t.Fatalf("default resolution lost: %dx%d", got.Width, got.Height)
	}
	if got.FPS != 30 {
		t.Fatalf("default fps lost: %v", got.FPS)
	}
	if got.Bitrate == 0 {
		t.Fatalf("default bitrate must be non-zero")
	}
	if got.IDRPeriod == 0 {
		t.Fatalf("default idr_period must be non-zero")
	}
}

func TestParamsHydratePreservesCallerValues(t *testing.T) {
	t.Parallel()
	got := rpicamera.HydrateForTest(rpicamera.Params{
		Width: 640, Height: 480, FPS: 15, Bitrate: 1_000_000, IDRPeriod: 30,
		HFlip: true, VFlip: true,
	})
	if got.Width != 640 || got.Height != 480 || got.FPS != 15 {
		t.Fatalf("user values not preserved: %+v", got)
	}
	if got.Bitrate != 1_000_000 || got.IDRPeriod != 30 {
		t.Fatalf("user bitrate/idr_period not preserved: %+v", got)
	}
	if !got.HFlip || !got.VFlip {
		t.Fatalf("flip flags not preserved")
	}
}
