package discovery

import (
	"encoding/xml"
	"errors"
	"strings"
)

// ErrNotProbe means the XML body is not a WS-Discovery Probe.
var ErrNotProbe = errors.New("discovery: envelope is not a Probe")

// ErrNotResolve means the XML body is not a WS-Discovery Resolve.
var ErrNotResolve = errors.New("discovery: envelope is not a Resolve")

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

// IncomingResolve is a parsed WS-Discovery Resolve (WS-Discovery §6.1).
type IncomingResolve struct {
	Action         string
	MessageID      string
	To             string
	Address        string
	ReplyToAddress string
}

type soapEnvelope struct {
	Header soapHeader `xml:"http://www.w3.org/2003/05/soap-envelope Header"`
	Body   soapBody   `xml:"http://www.w3.org/2003/05/soap-envelope Body"`
}

type soapHeader struct {
	Action    string   `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing Action"`
	MessageID string   `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing MessageID"`
	To        string   `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing To"`
	ReplyTo   *replyTo `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing ReplyTo"`
}

type replyTo struct {
	Address string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing Address"`
}

type soapBody struct {
	Probe   *probeXML   `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Probe"`
	Resolve *resolveXML `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Resolve"`
}

type probeXML struct {
	Types  string     `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Types"`
	Scopes *scopesXML `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Scopes"`
}

type scopesXML struct {
	MatchBy string `xml:"MatchBy,attr"`
	Inner   string `xml:",chardata"`
}

type resolveXML struct {
	EndpointRef struct {
		Address string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing Address"`
	} `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing EndpointReference"`
}

// ParseProbe decodes a SOAP 1.2 envelope whose body contains d:Probe.
func ParseProbe(data []byte) (*IncomingProbe, error) {
	var env soapEnvelope
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
	ip := &IncomingProbe{
		Action:         strings.TrimSpace(env.Header.Action),
		MessageID:      strings.TrimSpace(env.Header.MessageID),
		To:             strings.TrimSpace(env.Header.To),
		Types:          splitQNameList(p.Types),
		Scopes:         splitURIList(p.Scopes),
		MatchBy:        matchBy,
		ReplyToAddress: replyToString(env.Header.ReplyTo),
	}
	return ip, nil
}

// ParseResolve decodes a SOAP 1.2 envelope whose body contains d:Resolve.
func ParseResolve(data []byte) (*IncomingResolve, error) {
	var env soapEnvelope
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if env.Body.Resolve == nil {
		return nil, ErrNotResolve
	}
	r := env.Body.Resolve
	ir := &IncomingResolve{
		Action:         strings.TrimSpace(env.Header.Action),
		MessageID:      strings.TrimSpace(env.Header.MessageID),
		To:             strings.TrimSpace(env.Header.To),
		Address:        strings.TrimSpace(r.EndpointRef.Address),
		ReplyToAddress: replyToString(env.Header.ReplyTo),
	}
	return ir, nil
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
