package wsdiscovery_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/wsdiscovery"
)

var errPayloadMismatch = errors.New("payload mismatch")

// lockedBuffer is an io.Writer safe for concurrent slog output + test reads.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedBuffer) contains(sub []byte) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return bytes.Contains(l.buf.Bytes(), sub)
}

func TestMulticastUDPAddr(t *testing.T) {
	t.Parallel()
	a := wsdiscovery.MulticastUDPAddr()
	if a == nil {
		t.Fatal("MulticastUDPAddr returned nil")
	}
	if got := a.Port; got != wsdiscovery.DefaultUDPPort {
		t.Errorf("Port=%d want %d", got, wsdiscovery.DefaultUDPPort)
	}
	if !a.IP.Equal(net.ParseIP(wsdiscovery.MulticastIPv4)) {
		t.Errorf("IP=%v want multicast %s", a.IP, wsdiscovery.MulticastIPv4)
	}
}

func TestSendMulticastEmptyPayload(t *testing.T) {
	t.Parallel()
	err := wsdiscovery.SendMulticast(nil)
	if err == nil {
		t.Fatal("expected error for empty payload")
	}
	err = wsdiscovery.SendMulticast([]byte{})
	if err == nil {
		t.Fatal("expected error for empty payload")
	}
}

func TestSendUDPErrors(t *testing.T) {
	t.Parallel()
	err := wsdiscovery.SendUDP(nil, []byte("x"))
	if err == nil {
		t.Fatal("expected error for nil destination")
	}
	err = wsdiscovery.SendUDP(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}, nil)
	if err == nil {
		t.Fatal("expected error for empty payload")
	}
}

func TestSendUDPUnicastRoundTrip(t *testing.T) {
	t.Parallel()
	payload := []byte("wsd-udp-test")

	lc := &net.ListenConfig{}
	pc, err := lc.ListenPacket(t.Context(), "udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() }) //nolint:errcheck // test cleanup

	udpAddr, ok := pc.LocalAddr().(*net.UDPAddr)
	if !ok {
		t.Fatal("local addr is not UDP")
	}

	errCh := make(chan error, 1)
	go func() {
		buf := make([]byte, 2048)
		_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint:errcheck // best-effort deadline
		n, _, rerr := pc.ReadFrom(buf)
		if rerr != nil {
			errCh <- rerr
			return
		}
		if !bytes.Equal(buf[:n], payload) {
			errCh <- errPayloadMismatch
			return
		}
		errCh <- nil
	}()

	if err := wsdiscovery.SendUDP(udpAddr, payload); err != nil {
		t.Fatalf("SendUDP: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("receive side: %v", err)
	}
}

func TestSendMulticastSelfReceive(t *testing.T) {
	if testing.Short() {
		t.Skip("multicast I/O")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payload := []byte("wsd-multicast-self")
	got := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- wsdiscovery.ListenMulticastWithLogger(
			ctx,
			nil,
			obs.Discard(),
			func(_ *net.UDPAddr, p []byte) {
				select {
				case got <- append([]byte(nil), p...):
				default:
				}
			},
		)
	}()

	deadline := time.Now().Add(3 * time.Second)
	var received []byte
	recvOK := false
	for time.Now().Before(deadline) {
		if err := wsdiscovery.SendMulticast(payload); err != nil {
			if strings.Contains(err.Error(), "no route to host") {
				t.Skip("IPv4 multicast route unavailable:", err)
			}
			t.Fatalf("SendMulticast: %v", err)
		}
		select {
		case b := <-got:
			received = b
			recvOK = true
		case <-time.After(50 * time.Millisecond):
		}
		if recvOK {
			break
		}
	}
	if !recvOK {
		t.Fatal("timed out waiting for multicast datagram")
	}
	if !bytes.Equal(received, payload) {
		t.Fatalf("payload mismatch: %q vs %q", received, payload)
	}

	cancel()
	listenErr := <-errCh
	if listenErr != nil && !errors.Is(listenErr, context.Canceled) {
		t.Fatalf("listener returned %v", listenErr)
	}
}

func TestListenMulticastWithLoggerNilHandler(t *testing.T) {
	t.Parallel()
	err := wsdiscovery.ListenMulticastWithLogger(
		context.Background(),
		nil,
		obs.Discard(),
		nil,
	)
	if err == nil {
		t.Fatal("expected error for nil handler")
	}
}

func TestListenMulticastWithLoggerContextCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := wsdiscovery.ListenMulticastWithLogger(
		ctx,
		nil,
		obs.Discard(),
		func(*net.UDPAddr, []byte) {},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v want context.Canceled", err)
	}
}

func TestListenMulticastWithLoggerEmitsListenDebug(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("multicast I/O")
	}
	var out lockedBuffer
	logger := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- wsdiscovery.ListenMulticastWithLogger(
			ctx,
			nil,
			logger,
			func(*net.UDPAddr, []byte) {},
		)
	}()

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if out.contains([]byte("listening")) {
			cancel()
			listenErr := <-errCh
			if listenErr != nil && !errors.Is(listenErr, context.Canceled) && !errors.Is(listenErr, context.DeadlineExceeded) {
				t.Fatalf("listener returned %v", listenErr)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-errCh
	t.Fatal("expected debug log containing listening")
}
