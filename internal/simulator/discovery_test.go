package simulator

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/wsdiscovery"
)

// errDiscoveryTestOther is a sentinel for errorsIsContextCanceled negative coverage.
var errDiscoveryTestOther = errors.New("discovery test: unrelated error")

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
	if errorsIsContextCanceled(errDiscoveryTestOther) {
		t.Fatal("expected false for unrelated error")
	}
}

func TestReplyToOrAnonymous(t *testing.T) {
	if got := replyToOrAnonymous("  "); got != wsdiscovery.WSAAnonymous {
		t.Fatalf("empty reply-to: got %q want %q", got, wsdiscovery.WSAAnonymous)
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

func TestHandleDiscoveryDatagramSendsProbeMatch(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	const probeXML = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
  xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
  xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>
    <a:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</a:Action>
    <a:MessageID>uuid:probe-unit-test</a:MessageID>
    <a:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To>
  </s:Header>
  <s:Body>
    <d:Probe>
      <d:Types>tds:Device</d:Types>
    </d:Probe>
  </s:Body>
</s:Envelope>`

	pc, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := pc.Close(); closeErr != nil {
			t.Errorf("Close: %v", closeErr)
		}
	})

	addr := pc.LocalAddr()
	la, ok := addr.(*net.UDPAddr)
	if !ok {
		t.Fatalf("LocalAddr: got %T", addr)
	}
	cfg := sim.ConfigSnapshot()
	httpPort := cfg.Network.HTTPPort

	done := make(chan struct{})
	go func() {
		defer close(done)
		sim.handleDiscoveryDatagram(la, []byte(probeXML), "127.0.0.1", httpPort)
	}()

	buf := make([]byte, 65535)
	if deadlineErr := pc.SetReadDeadline(time.Now().Add(3 * time.Second)); deadlineErr != nil {
		t.Fatalf("SetReadDeadline: %v", deadlineErr)
	}
	n, _, readErr := pc.ReadFromUDP(buf)
	if readErr != nil {
		t.Fatalf("ReadFromUDP: %v", readErr)
	}
	<-done

	body := buf[:n]
	if !bytes.Contains(body, []byte("ProbeMatches")) {
		t.Fatalf("expected ProbeMatches in reply, got %q", string(body))
	}
	if !bytes.Contains(body, []byte(wsdiscovery.ActionProbeMatches)) {
		t.Fatalf("expected Probe Action URI in reply")
	}
}
