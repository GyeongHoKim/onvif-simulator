package ffmpeg

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBuildFFmpegArgs_DefaultsAndOrdering(t *testing.T) {
	t.Parallel()
	args := buildFFmpegArgs(Params{MediaFilePath: "/tmp/x.mp4"})
	if !containsSlice(args, "-c:v", "mjpeg") {
		t.Errorf("missing -c:v mjpeg in %v", args)
	}
	if !containsSlice(args, "-f", "mpjpeg") {
		t.Errorf("missing -f mpjpeg in %v", args)
	}
	if !containsSlice(args, "-q:v", "5") {
		t.Errorf("expected default quality=5, got %v", args)
	}
	// pipe:1 is always the last positional so ffmpeg writes stdout.
	if args[len(args)-1] != "pipe:1" {
		t.Errorf("last arg = %q, expected pipe:1", args[len(args)-1])
	}
}

func TestBuildFFmpegArgs_LoopForever(t *testing.T) {
	t.Parallel()
	args := buildFFmpegArgs(Params{MediaFilePath: "/x.mp4", LoopForever: true})
	if !containsSlice(args, "-stream_loop", "-1") {
		t.Errorf("expected -stream_loop -1 for LoopForever, got %v", args)
	}
}

func TestBuildFFmpegArgs_OmitsStreamLoopByDefault(t *testing.T) {
	t.Parallel()
	args := buildFFmpegArgs(Params{MediaFilePath: "/x.mp4"})
	for _, a := range args {
		if a == "-stream_loop" {
			t.Fatalf("did not expect -stream_loop without LoopForever; got %v", args)
		}
	}
}

func TestBuildFFmpegArgs_HonorsExplicitFPS(t *testing.T) {
	t.Parallel()
	args := buildFFmpegArgs(Params{MediaFilePath: "/x.mp4", FPS: 24})
	if !containsSlice(args, "-r", "24") {
		t.Errorf("expected -r 24 from Params.FPS=24, got %v", args)
	}
}

func TestBuildFFmpegArgs_HonorsExplicitQuality(t *testing.T) {
	t.Parallel()
	args := buildFFmpegArgs(Params{MediaFilePath: "/x.mp4", Quality: 12})
	if !containsSlice(args, "-q:v", "12") {
		t.Errorf("expected -q:v 12, got %v", args)
	}
}

func TestOpen_RejectsNilCallback(t *testing.T) {
	t.Parallel()
	_, err := Open(Params{MediaFilePath: "/x.mp4"}, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil callback")
	}
	if !strings.Contains(err.Error(), "onJPEG") {
		t.Errorf("expected onJPEG nil-check error, got %v", err)
	}
}

func TestOpen_RejectsEmptyMediaFilePath(t *testing.T) {
	t.Parallel()
	_, err := Open(Params{}, nil, func(int64, time.Time, []byte) {})
	if err == nil {
		t.Fatal("expected error for empty MediaFilePath")
	}
}

func TestOpen_ReturnsUnsupportedOnPlaceholderBuild(t *testing.T) {
	t.Parallel()
	if Available() == nil {
		t.Skip("real ffmpeg binary fetched; placeholder gate inactive")
	}
	_, err := Open(Params{MediaFilePath: "/tmp/x.mp4"}, nil,
		func(int64, time.Time, []byte) {})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}

func containsSlice(haystack []string, a, b string) bool {
	for i := 0; i+1 < len(haystack); i++ {
		if haystack[i] == a && haystack[i+1] == b {
			return true
		}
	}
	return false
}
