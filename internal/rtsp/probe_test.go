package rtsp

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProbeH264(t *testing.T) {
	res, err := Probe(filepath.Join("testdata", "short_h264.mp4"))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if res.Codec != CodecH264 {
		t.Errorf("codec = %q, want %q", res.Codec, CodecH264)
	}
	if res.Width != 320 || res.Height != 240 {
		t.Errorf("dims = %dx%d, want 320x240", res.Width, res.Height)
	}
	if res.FPS != 15 {
		t.Errorf("fps = %d, want 15", res.FPS)
	}
	if len(res.SPS) == 0 || len(res.PPS) == 0 {
		t.Errorf("expected non-empty SPS/PPS, got SPS=%d PPS=%d", len(res.SPS), len(res.PPS))
	}
	if len(res.VPS) != 0 {
		t.Errorf("H264 should have no VPS, got %d bytes", len(res.VPS))
	}
}

func TestProbeH265(t *testing.T) {
	res, err := Probe(filepath.Join("testdata", "short_h265.mp4"))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if res.Codec != CodecH265 {
		t.Errorf("codec = %q, want %q", res.Codec, CodecH265)
	}
	if res.Width != 320 || res.Height != 240 {
		t.Errorf("dims = %dx%d, want 320x240", res.Width, res.Height)
	}
	if res.FPS != 15 {
		t.Errorf("fps = %d, want 15", res.FPS)
	}
	if len(res.SPS) == 0 || len(res.PPS) == 0 || len(res.VPS) == 0 {
		t.Errorf(
			"expected non-empty VPS/SPS/PPS, got VPS=%d SPS=%d PPS=%d",
			len(res.VPS), len(res.SPS), len(res.PPS),
		)
	}
}

func TestProbeMissingFile(t *testing.T) {
	_, err := Probe(filepath.Join("testdata", "does_not_exist.mp4"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected ErrNotExist, got %v", err)
	}
}

func TestProbeNotMp4(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "garbage-*.mp4")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := tmp.WriteString("not an mp4"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := Probe(tmp.Name()); err == nil {
		t.Fatal("expected parse error for non-mp4 input")
	}
}
