package wsdiscovery_test

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/wsdiscovery"
)

var errPayloadMismatch = errors.New("payload mismatch")

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

	// Allow the listener to bind before we send.
	time.Sleep(100 * time.Millisecond)

	if err := wsdiscovery.SendMulticast(payload); err != nil {
		t.Fatalf("SendMulticast: %v", err)
	}

	select {
	case b := <-got:
		if !bytes.Equal(b, payload) {
			t.Fatalf("payload mismatch: %q vs %q", b, payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for multicast datagram")
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

func TestListenMulticastDelegatesToDiscardLogger(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// ListenMulticast is ListenMulticastWithLogger(..., Discard(), ...).
	err := wsdiscovery.ListenMulticast(
		ctx,
		nil,
		func(*net.UDPAddr, []byte) {},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v want context.Canceled", err)
	}
}
