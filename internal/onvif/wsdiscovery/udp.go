package discovery

import (
	"context"
	"errors"
	"net"
	"time"
)

var (
	errUDPEmptyPayload = errors.New("discovery: empty UDP payload")
	errUDPNilAddr      = errors.New("discovery: nil UDP address")
	errUDPNilHandler   = errors.New("discovery: nil multicast handler")
)

const (
	udpMaxDatagram    = 65535
	multicastReadPoll = 500 * time.Millisecond
)

func udpClose(c *net.UDPConn) {
	if c == nil {
		return
	}
	_ = c.Close() //nolint:errcheck // UDP close errors are ignored on shutdown paths.
}

// MulticastUDPAddr is the standard IPv4 WS-Discovery multicast destination.
func MulticastUDPAddr() *net.UDPAddr {
	return &net.UDPAddr{
		IP:   net.ParseIP(MulticastIPv4),
		Port: DefaultUDPPort,
	}
}

// SendMulticast sends payload to the IPv4 discovery multicast group (SOAP/UDP: UTF-8 XML in one datagram).
func SendMulticast(payload []byte) error {
	if len(payload) == 0 {
		return errUDPEmptyPayload
	}
	laddr := &net.UDPAddr{IP: net.IPv4zero, Port: 0}
	c, err := net.ListenUDP("udp4", laddr)
	if err != nil {
		return err
	}
	defer udpClose(c)
	_, err = c.WriteToUDP(payload, MulticastUDPAddr())
	return err
}

// SendUDP sends a unicast datagram (e.g. Probe Match / Resolve Match to the request source).
func SendUDP(to *net.UDPAddr, payload []byte) error {
	if to == nil {
		return errUDPNilAddr
	}
	if len(payload) == 0 {
		return errUDPEmptyPayload
	}
	c, err := net.DialUDP("udp4", nil, to)
	if err != nil {
		return err
	}
	defer udpClose(c)
	_, err = c.Write(payload)
	return err
}

// ListenMulticast receives discovery multicast on :3702 until ctx is canceled.
func ListenMulticast(ctx context.Context, iface *net.Interface, handler func(*net.UDPAddr, []byte)) error {
	if handler == nil {
		return errUDPNilHandler
	}
	gaddr := &net.UDPAddr{IP: net.ParseIP(MulticastIPv4), Port: DefaultUDPPort}
	c, err := net.ListenMulticastUDP("udp4", iface, gaddr)
	if err != nil {
		return err
	}
	defer udpClose(c)

	buf := make([]byte, udpMaxDatagram)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := c.SetReadDeadline(time.Now().Add(multicastReadPoll)); err != nil {
			return err
		}
		n, from, err := c.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		p := make([]byte, n)
		copy(p, buf[:n])
		handler(from, p)
	}
}
