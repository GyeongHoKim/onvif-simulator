package rtsp

import (
	"path/filepath"
	"testing"
)

func TestNewFileSourceMissing(t *testing.T) {
	t.Parallel()
	if _, err := NewFileSource(filepath.Join("testdata", "missing.mp4")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewFileSourceProbesH264(t *testing.T) {
	t.Parallel()
	src, err := NewFileSource(filepath.Join("testdata", "short_h264.mp4"))
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	probe := src.Describe()
	if probe == nil {
		t.Fatal("Describe returned nil")
	}
	if probe.Codec != CodecH264 {
		t.Fatalf("codec = %q", probe.Codec)
	}
	if len(probe.SPS) == 0 || len(probe.PPS) == 0 {
		t.Fatalf("probe missing SPS/PPS: %+v", probe)
	}
}

func TestNewFileSourceProbesH265(t *testing.T) {
	t.Parallel()
	src, err := NewFileSource(filepath.Join("testdata", "short_h265.mp4"))
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	if got := src.Describe().Codec; got != CodecH265 {
		t.Fatalf("codec = %q", got)
	}
}
