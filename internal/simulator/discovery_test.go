package simulator

import (
	"context"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

func TestXAddrsForFromConfig(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			HTTPPort: 9090,
			XAddrs:   []string{"http://example.com:9090/onvif/device_service"},
		},
	}
	got := xAddrsFor(cfg)
	if len(got) != 1 || got[0] != "http://example.com:9090/onvif/device_service" {
		t.Fatalf("unexpected xaddrs: %v", got)
	}
}

func TestXAddrsForFallback(t *testing.T) {
	cfg := &config.Config{Network: config.NetworkConfig{HTTPPort: 8080}}
	got := xAddrsFor(cfg)
	if len(got) != 1 {
		t.Fatalf("expected 1 fallback xaddr, got %d", len(got))
	}
}

func TestNextMessageNumberIncrements(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	a := sim.nextMessageNumber()
	b := sim.nextMessageNumber()
	if b != a+1 {
		t.Fatalf("expected sequential numbers, got %d then %d", a, b)
	}
}

func TestDiscoveryEnabledMatchesConfig(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	if !sim.discoveryEnabled() {
		t.Fatal("expected discoveryEnabled=true by default")
	}
	if err := sim.SetDiscoveryMode(discoveryModeNonDiscoverable); err != nil {
		t.Fatalf("SetDiscoveryMode: %v", err)
	}
	if sim.discoveryEnabled() {
		t.Fatal("expected discoveryEnabled=false after SetDiscoveryMode(NonDiscoverable)")
	}
}

func TestErrorsIsContextCanceled(t *testing.T) {
	if !errorsIsContextCanceled(context.Canceled) {
		t.Fatal("expected true for context.Canceled")
	}
	if !errorsIsContextCanceled(context.DeadlineExceeded) {
		t.Fatal("expected true for context.DeadlineExceeded")
	}
}

func TestReplyToOrAnonymous(t *testing.T) {
	if got := replyToOrAnonymous("  "); got == "" {
		t.Fatal("expected fallback for empty reply-to")
	}
	if got := replyToOrAnonymous("urn:x"); got != "urn:x" {
		t.Fatalf("expected verbatim address, got %q", got)
	}
}

func TestSendHelloMulticastNonDiscoverable(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	if err := sim.SetDiscoveryMode(discoveryModeNonDiscoverable); err != nil {
		t.Fatalf("SetDiscoveryMode: %v", err)
	}
	sim.sendHelloMulticast() // no-op; must not panic
}

func TestSendByeMulticastNonDiscoverable(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	if err := sim.SetDiscoveryMode(discoveryModeNonDiscoverable); err != nil {
		t.Fatalf("SetDiscoveryMode: %v", err)
	}
	sim.sendByeMulticast() // no-op; must not panic
}

func TestBuildHelloParams(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	params := sim.buildHelloParams()
	if params == nil {
		t.Fatal("expected non-nil hello params")
	}
	if params.Address == "" {
		t.Fatal("expected device UUID in Address")
	}
	if len(params.XAddrs) == 0 {
		t.Fatal("expected at least one XAddr in hello params")
	}
}

func TestHandleDiscoveryDatagramWhenDisabled(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	if err := sim.SetDiscoveryMode(discoveryModeNonDiscoverable); err != nil {
		t.Fatalf("SetDiscoveryMode: %v", err)
	}
	// Must return early without crashing when discovery is disabled.
	sim.handleDiscoveryDatagram(nil, []byte("garbage"), "127.0.0.1", 8080)
}

func TestHandleDiscoveryDatagramInvalidPayload(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	// ParseProbe fails on non-XML; handleDiscoveryDatagram must return silently.
	sim.handleDiscoveryDatagram(nil, []byte("not xml"), "127.0.0.1", 8080)
}
