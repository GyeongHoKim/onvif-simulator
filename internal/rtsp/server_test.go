package rtsp

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
)

// freePort asks the OS for an unused TCP port. The port may have been recycled
// by the time the caller binds to it, but for a single-process test the
// likelihood is acceptable.
func freePort(t *testing.T) int {
	t.Helper()
	lc := &net.ListenConfig{}
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("listener returned non-TCP address")
	}
	if closeErr := l.Close(); closeErr != nil {
		t.Fatalf("close: %v", closeErr)
	}
	return addr.Port
}

// readRTP connects an RTSP client to rtsp://127.0.0.1:port/<token>, performs
// DESCRIBE/SETUP/PLAY, and waits up to deadline for at least one RTP packet
// to arrive. It returns the number of RTP packets seen.
func readRTP(t *testing.T, port int, token string, deadline time.Duration) int {
	t.Helper()

	u := fmt.Sprintf("rtsp://127.0.0.1:%d/%s", port, token)
	c := &gortsplib.Client{}

	parsedURL, err := base.ParseURL(u)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	c.Scheme = parsedURL.Scheme
	c.Host = parsedURL.Host
	if startErr := c.Start(); startErr != nil {
		t.Fatalf("start client: %v", startErr)
	}
	defer c.Close()

	desc, _, err := c.Describe(parsedURL)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if len(desc.Medias) == 0 {
		t.Fatal("no medias in description")
	}

	if err := c.SetupAll(desc.BaseURL, desc.Medias); err != nil {
		t.Fatalf("setup: %v", err)
	}

	var got atomic.Int32
	c.OnPacketRTPAny(func(_ *description.Media, _ format.Format, _ *rtp.Packet) {
		got.Add(1)
	})

	if _, err := c.Play(nil); err != nil {
		t.Fatalf("play: %v", err)
	}

	endAt := time.Now().Add(deadline)
	for time.Now().Before(endAt) {
		if got.Load() > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return int(got.Load())
}

func TestServerStartStopIdempotent(t *testing.T) {
	s := New(freePort(t))
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("Start (2nd): %v", err)
	}
	s.Stop()
	s.Stop() // second Stop must be a no-op
}

func TestServerAddSourceBeforeStart(t *testing.T) {
	s := New(freePort(t))
	defer s.Stop()
	_, err := s.AddSource("main", filepath.Join("testdata", "short_h264.mp4"))
	if err == nil {
		t.Fatal("expected error when adding source before Start")
	}
}

func TestServerAddSourceDuplicate(t *testing.T) {
	s := New(freePort(t))
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	path := filepath.Join("testdata", "short_h264.mp4")
	if _, err := s.AddSource("main", path); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	_, err := s.AddSource("main", path)
	if !errors.Is(err, ErrSourceExists) {
		t.Errorf("expected ErrSourceExists, got %v", err)
	}
}

func TestServerAddSourceMissingFile(t *testing.T) {
	s := New(freePort(t))
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	_, err := s.AddSource("main", filepath.Join("testdata", "missing.mp4"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestServerRemoveSourceUnknown(t *testing.T) {
	s := New(freePort(t))
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	err := s.RemoveSource("does-not-exist")
	if !errors.Is(err, ErrSourceNotFound) {
		t.Errorf("expected ErrSourceNotFound, got %v", err)
	}
}

func TestServerStreamH264(t *testing.T) {
	port := freePort(t)
	s := New(port)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	probe, err := s.AddSource("main", filepath.Join("testdata", "short_h264.mp4"))
	if err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if probe.Codec != CodecH264 {
		t.Fatalf("probe codec = %q", probe.Codec)
	}

	if got := readRTP(t, port, "main", 3*time.Second); got == 0 {
		t.Fatal("expected at least one RTP packet")
	}

	if err := s.RemoveSource("main"); err != nil {
		t.Errorf("RemoveSource: %v", err)
	}
}

func TestServerStreamH265(t *testing.T) {
	port := freePort(t)
	s := New(port)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	if _, err := s.AddSource("sub", filepath.Join("testdata", "short_h265.mp4")); err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	if got := readRTP(t, port, "sub", 3*time.Second); got == 0 {
		t.Fatal("expected at least one RTP packet")
	}
}

func TestServerDescribeUnknownPath(t *testing.T) {
	port := freePort(t)
	s := New(port)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	c := &gortsplib.Client{}
	parsedURL, err := base.ParseURL(fmt.Sprintf("rtsp://127.0.0.1:%d/nope", port))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c.Scheme = parsedURL.Scheme
	c.Host = parsedURL.Host
	if startErr := c.Start(); startErr != nil {
		t.Fatalf("client start: %v", startErr)
	}
	defer c.Close()

	_, _, err = c.Describe(parsedURL)
	if err == nil {
		t.Fatal("expected describe error for unknown path")
	}
}
