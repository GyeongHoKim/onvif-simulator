package simulator

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/wsdiscovery"
)

// runDiscovery sends Hello on start and listens for Probe/Resolve until ctx
// is canceled. Respects DiscoveryMode: when NonDiscoverable, emission is
// suppressed but the listener keeps running for administrative visibility.
func (s *Simulator) runDiscovery(ctx context.Context, host string, port int) {
	s.sendHelloMulticast()

	err := wsdiscovery.ListenMulticast(ctx, nil, func(from *net.UDPAddr, buf []byte) {
		s.handleDiscoveryDatagram(from, buf, host, port)
	})
	if err != nil && !errorsIsContextCanceled(err) {
		// Listener exited on error; simulator keeps running without discovery.
		return
	}
}

func errorsIsContextCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// sendHelloMulticast emits a Hello datagram if discovery is enabled. No-op
// otherwise.
func (s *Simulator) sendHelloMulticast() {
	if !s.discoveryEnabled() {
		return
	}
	payload, err := wsdiscovery.MarshalHello(s.buildHelloParams())
	if err != nil {
		return
	}
	_ = wsdiscovery.SendMulticast(payload) //nolint:errcheck // best-effort multicast Hello.
}

// sendByeMulticast emits a Bye datagram if discovery is enabled. No-op
// otherwise.
func (s *Simulator) sendByeMulticast() {
	if !s.discoveryEnabled() {
		return
	}
	cfg := s.snapshotConfig()
	msgID := wsdiscovery.NewMessageID()
	byeParams := &wsdiscovery.ByeParams{
		MessageID:     msgID,
		Address:       cfg.Device.UUID,
		InstanceID:    s.instanceID,
		MessageNumber: s.nextMessageNumber(),
	}
	payload, err := wsdiscovery.MarshalBye(byeParams)
	if err != nil {
		return
	}
	_ = wsdiscovery.SendMulticast(payload) //nolint:errcheck // best-effort multicast Bye.
}

func (s *Simulator) discoveryEnabled() bool {
	cfg := s.snapshotConfig()
	return cfg.Runtime.DiscoveryMode != "NonDiscoverable"
}

// buildHelloParams assembles HelloParams from the current config.
func (s *Simulator) buildHelloParams() *wsdiscovery.HelloParams {
	cfg := s.snapshotConfig()
	return &wsdiscovery.HelloParams{
		MessageID:       wsdiscovery.NewMessageID(),
		Address:         cfg.Device.UUID,
		Types:           []string{"tds:Device"},
		Scopes:          append([]string(nil), cfg.Device.Scopes...),
		XAddrs:          xAddrsFor(&cfg),
		MetadataVersion: 1,
		InstanceID:      s.instanceID,
		MessageNumber:   s.nextMessageNumber(),
	}
}

func xAddrsFor(cfg *config.Config) []string {
	if len(cfg.Network.XAddrs) > 0 {
		return append([]string(nil), cfg.Network.XAddrs...)
	}
	host := localAddrForXAddr()
	return []string{httpURL(host, cfg.Network.HTTPPort, "/onvif/device_service")}
}

// handleDiscoveryDatagram parses incoming UDP and replies with ProbeMatch
// when a Probe targets this device.
func (s *Simulator) handleDiscoveryDatagram(from *net.UDPAddr, buf []byte, host string, port int) {
	if !s.discoveryEnabled() {
		return
	}
	probe, err := wsdiscovery.ParseProbe(buf)
	if err != nil {
		return
	}
	if err := probe.Validate(); err != nil {
		return
	}
	cfg := s.snapshotConfig()
	deviceTypes := []string{"tds:Device"}
	if !wsdiscovery.ProbeMatches(probe, deviceTypes, cfg.Device.Scopes) {
		return
	}
	xaddrs := cfg.Network.XAddrs
	if len(xaddrs) == 0 {
		xaddrs = []string{httpURL(host, port, "/onvif/device_service")}
	}
	params := &wsdiscovery.ProbeMatchesParams{
		MessageID:  wsdiscovery.NewMessageID(),
		RelatesTo:  probe.MessageID,
		To:         replyToOrAnonymous(probe.ReplyToAddress),
		InstanceID: s.instanceID,
		MsgNumber:  s.nextMessageNumber(),
		Match: wsdiscovery.ProbeMatchContent{
			Address:         cfg.Device.UUID,
			Types:           deviceTypes,
			Scopes:          cfg.Device.Scopes,
			XAddrs:          xaddrs,
			MetadataVersion: 1,
		},
	}
	payload, marshalErr := wsdiscovery.MarshalProbeMatches(params)
	if marshalErr != nil {
		return
	}
	_ = wsdiscovery.SendUDP(from, payload) //nolint:errcheck // best-effort unicast reply.
}

func replyToOrAnonymous(replyTo string) string {
	r := strings.TrimSpace(replyTo)
	if r == "" {
		return wsdiscovery.WSAAnonymous
	}
	return r
}

// nextMessageNumber returns a monotonically increasing message number for
// WS-Discovery messages. Starts at 1.
func (s *Simulator) nextMessageNumber() uint32 {
	return atomic.AddUint32(&s.msgNumberSeq, 1)
}
