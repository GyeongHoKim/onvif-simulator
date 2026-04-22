package wsdiscovery

import (
	"errors"
	"strings"
)

var (
	errProbeMatchesMessageIDRequired  = errors.New("discovery: ProbeMatches MessageID is required")
	errProbeMatchesRelatesToRequired  = errors.New("discovery: ProbeMatches RelatesTo is required")
	errProbeMatchesMessageNumberRange = errors.New("discovery: ProbeMatches MessageNumber must be >= 1")
	errProbeMatchesParamsNil          = errors.New("discovery: nil ProbeMatchesParams")
	errProbeMatchEndpointRequired     = errors.New("discovery: ProbeMatch Address is required")
)

// ProbeMatchContent is the body of a single d:ProbeMatch (WS-Discovery §5.3).
type ProbeMatchContent struct {
	Address         string
	Types           []string
	Scopes          []string
	XAddrs          []string
	MetadataVersion uint32
}

// ProbeMatchesParams builds a unicast ProbeMatches response (WS-Discovery §5.3, Table 2).
type ProbeMatchesParams struct {
	MessageID  string
	RelatesTo  string
	To         string
	InstanceID uint32
	MsgNumber  uint32
	Match      ProbeMatchContent
}

// Validate checks ProbeMatches fields.
func (p *ProbeMatchesParams) Validate() error {
	if p == nil {
		return errProbeMatchesParamsNil
	}
	switch {
	case strings.TrimSpace(p.MessageID) == "":
		return errProbeMatchesMessageIDRequired
	case strings.TrimSpace(p.RelatesTo) == "":
		return errProbeMatchesRelatesToRequired
	case p.MsgNumber < 1:
		return errProbeMatchesMessageNumberRange
	case strings.TrimSpace(p.Match.Address) == "":
		return errProbeMatchEndpointRequired
	default:
		return nil
	}
}

// MarshalProbeMatches builds a UTF-8 SOAP 1.2 ProbeMatches envelope (unicast to reply endpoint).
func MarshalProbeMatches(p *ProbeMatchesParams) ([]byte, error) {
	if p == nil {
		return nil, errProbeMatchesParamsNil
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	to := strings.TrimSpace(p.To)
	if to == "" {
		to = WSAAnonymous
	}
	return marshalMatchResponse(
		ActionProbeMatches,
		"<d:ProbeMatches>", "</d:ProbeMatches>", "d:ProbeMatch",
		p.MessageID, to, p.RelatesTo, p.InstanceID, p.MsgNumber,
		p.Match.Types,
		p.Match.Address,
		p.Match.Scopes,
		p.Match.XAddrs,
		p.Match.MetadataVersion,
	)
}
