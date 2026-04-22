package wsdiscovery

import (
	"encoding/xml"
	"errors"
	"strings"
)

var (
	// ErrNotProbe means the XML body is not a WS-Discovery Probe.
	ErrNotProbe = errors.New("discovery: envelope is not a Probe")

	errProbeMessageIDEmpty = errors.New("discovery: Probe MessageID is required")
	errProbeActionInvalid  = errors.New("discovery: Probe Action must be " + ActionProbe)
	errProbeToInvalid      = errors.New("discovery: Probe To must be " + ToDiscovery)
)

// IncomingProbe is a parsed WS-Discovery Probe (WS-Discovery §5.2).
type IncomingProbe struct {
	Action         string
	MessageID      string
	To             string
	Types          []string
	Scopes         []string
	MatchBy        string
	ReplyToAddress string
}

// Validate checks that the Probe contains required WS-Discovery fields (§5.2).
func (p *IncomingProbe) Validate() error {
	switch {
	case strings.TrimSpace(p.MessageID) == "":
		return errProbeMessageIDEmpty
	case strings.TrimSpace(p.Action) != ActionProbe:
		return errProbeActionInvalid
	case strings.TrimSpace(p.To) != ToDiscovery:
		return errProbeToInvalid
	default:
		return nil
	}
}

// ParseProbe decodes a SOAP 1.2 envelope whose body contains d:Probe.
func ParseProbe(data []byte) (*IncomingProbe, error) {
	var env struct {
		Header soapHeader `xml:"http://www.w3.org/2003/05/soap-envelope Header"`
		Body   struct {
			Probe *probeXML `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Probe"`
		} `xml:"http://www.w3.org/2003/05/soap-envelope Body"`
	}
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if env.Body.Probe == nil {
		return nil, ErrNotProbe
	}
	p := env.Body.Probe
	matchBy := MatchByRFC2396
	if p.Scopes != nil && strings.TrimSpace(p.Scopes.MatchBy) != "" {
		matchBy = strings.TrimSpace(p.Scopes.MatchBy)
	}
	return &IncomingProbe{
		Action:         strings.TrimSpace(env.Header.Action),
		MessageID:      strings.TrimSpace(env.Header.MessageID),
		To:             strings.TrimSpace(env.Header.To),
		Types:          splitQNameList(p.Types),
		Scopes:         splitURIList(p.Scopes),
		MatchBy:        matchBy,
		ReplyToAddress: replyToString(env.Header.ReplyTo),
	}, nil
}

type probeXML struct {
	Types  string     `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Types"`
	Scopes *scopesXML `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Scopes"`
}

type scopesXML struct {
	MatchBy string `xml:"MatchBy,attr"`
	Inner   string `xml:",chardata"`
}

// soapHeader and replyTo are shared with resolve.go (same package).
type soapHeader struct {
	Action    string   `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing Action"`
	MessageID string   `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing MessageID"`
	To        string   `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing To"`
	ReplyTo   *replyTo `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing ReplyTo"`
}

type replyTo struct {
	Address string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing Address"`
}

func replyToString(r *replyTo) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Address)
}

func splitQNameList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

func splitURIList(sc *scopesXML) []string {
	if sc == nil {
		return nil
	}
	t := strings.TrimSpace(sc.Inner)
	if t == "" {
		return nil
	}
	return strings.Fields(t)
}
