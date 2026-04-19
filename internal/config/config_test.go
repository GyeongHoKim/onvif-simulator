package config_test

import (
	"errors"
	"os"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	valid := config.Config{
		Version:  config.CurrentVersion,
		MainRTSP: "rtsp://localhost:8554/live",
		SubRTSP:  "rtsp://localhost:8554/live2",
	}
	if err := config.Validate(valid); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}

	if err := config.Validate(config.Config{Version: 0, MainRTSP: valid.MainRTSP, SubRTSP: valid.SubRTSP}); err == nil {
		t.Fatal("expected error for wrong version")
	}

	if err := config.Validate(config.Config{Version: config.CurrentVersion, MainRTSP: "", SubRTSP: valid.SubRTSP}); err == nil {
		t.Fatal("expected error for empty main_rtsp_uri")
	}

	if err := config.Validate(config.Config{
		Version:  config.CurrentVersion,
		MainRTSP: "http://example.com/x",
		SubRTSP:  valid.SubRTSP,
	}); err == nil {
		t.Fatal("expected error for non-rtsp scheme")
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	want := config.Config{
		Version:  config.CurrentVersion,
		MainRTSP: "rtsp://127.0.0.1:8554/main",
		SubRTSP:  "rtsp://127.0.0.1:8554/sub",
	}
	if err := config.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Fatalf("round-trip: got %+v, want %+v", got, want)
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected not exist, got %v", err)
	}
}
