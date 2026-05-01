package snapshot_test

import (
	"bytes"
	"errors"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/snapshot"
)

func TestExtract_H264(t *testing.T) {
	t.Parallel()
	assertExtractDecodesJPEG(t, "testdata/short_h264.mp4")
}

func TestExtract_H265(t *testing.T) {
	t.Parallel()
	assertExtractDecodesJPEG(t, "testdata/short_h265.mp4")
}

func TestExtract_FileMissing(t *testing.T) {
	t.Parallel()

	_, err := snapshot.Extract(filepath.Join(t.TempDir(), "does_not_exist.mp4"))
	if err == nil {
		t.Fatalf("expected error for missing file, got nil")
	}
}

func TestExtract_NotAVideo(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	notVideo := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(notVideo, []byte("not an mp4"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := snapshot.Extract(notVideo)
	if err == nil {
		t.Fatalf("expected error for non-video file, got nil")
	}
	// Could be either ffmpeg refusing the input or our ErrNoVideoStream;
	// both are acceptable as long as Extract reports the failure.
	if errors.Is(err, snapshot.ErrNoVideoStream) {
		return
	}
}

func assertExtractDecodesJPEG(t *testing.T, path string) {
	t.Helper()

	bytesOut, err := snapshot.Extract(path)
	if err != nil {
		t.Fatalf("Extract(%s): %v", path, err)
	}
	if len(bytesOut) < 4 {
		t.Fatalf("Extract returned %d bytes; want >= 4", len(bytesOut))
	}
	if !bytes.HasPrefix(bytesOut, []byte{0xFF, 0xD8, 0xFF}) {
		t.Fatalf("Extract output missing JPEG SOI marker; first bytes = % x", bytesOut[:4])
	}
	// Decode the result with the standard library to verify the bytes are
	// not just a valid header.
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(bytesOut))
	if err != nil {
		t.Fatalf("jpeg.DecodeConfig: %v", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		t.Fatalf("decoded image has zero dimensions: %dx%d", cfg.Width, cfg.Height)
	}
}
