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
	if err := sim.SetDiscoveryMode("NonDiscoverable"); err != nil {
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
