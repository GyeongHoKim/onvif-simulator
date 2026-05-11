package simulator

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/rtsp"
)

func TestRegisterMJPEGSiblings_NoProfilesReturnsEmpty(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.DiscardHandler)
	srv := rtsp.New(freePort(t), rtsp.WithLogger(logger))
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	out, err := registerMJPEGSiblings(srv, nil, logger)
	if err != nil {
		t.Fatalf("registerMJPEGSiblings: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no siblings, got %d", len(out))
	}
}

func TestRegisterMJPEGSiblings_SkipsFileSiblingWhenTranscoderUnavailable(t *testing.T) {
	// When ffmpeg is a placeholder, NewTranscodingSource fails and the sibling is skipped.
	logger := slog.New(slog.DiscardHandler)
	srv := rtsp.New(freePort(t), rtsp.WithLogger(logger))
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	fixture := fixtureH264Abs(t)
	src, err := rtsp.NewFileSource(fixture)
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	if _, addErr := srv.AddSource("main", src); addErr != nil {
		t.Fatalf("AddSource: %v", addErr)
	}

	probed := []config.ProfileConfig{{
		Name:          "main",
		Token:         "main",
		MediaFilePath: fixture,
		Encoding:      rtsp.CodecH264,
		Width:         320,
		Height:        240,
		FPS:           15,
	}}

	out, err := registerMJPEGSiblings(srv, probed, logger)
	if err != nil {
		t.Fatalf("registerMJPEGSiblings: %v", err)
	}
	// With a real embedded ffmpeg, one sibling registers; with a placeholder, none.
	if len(out) > 1 {
		t.Fatalf("unexpected sibling count %d", len(out))
	}
}

func TestBuildMJPEGSiblingSource_RPICamParentNotRegistered(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.DiscardHandler)
	srv := rtsp.New(freePort(t), rtsp.WithLogger(logger))
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	parent := &config.ProfileConfig{
		Kind:     config.ProfileKindRPICam,
		Token:    "cam1",
		RPICam:   &config.RPICamConfig{Width: 1920, Height: 1080, FPS: 30},
		Width:    1920,
		Height:   1080,
		FPS:      30,
		Encoding: rtsp.CodecH264,
	}
	_, err := buildMJPEGSiblingSource(parent, srv, logger)
	if !errors.Is(err, errMJPEGParentSourceMissing) {
		t.Fatalf("expected errMJPEGParentSourceMissing, got %v", err)
	}
}

func TestBuildMJPEGSiblingSource_UnsupportedParentKind(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.DiscardHandler)
	srv := rtsp.New(freePort(t), rtsp.WithLogger(logger))
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	parent := &config.ProfileConfig{
		Kind:          "unknown-kind",
		Token:         "x",
		MediaFilePath: "/dev/null",
	}
	_, err := buildMJPEGSiblingSource(parent, srv, logger)
	if !errors.Is(err, errMJPEGUnsupportedParentKind) {
		t.Fatalf("expected errMJPEGUnsupportedParentKind, got %v", err)
	}
}
