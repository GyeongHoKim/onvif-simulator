package wsdiscovery

import (
	"errors"
	"strings"
)

var (
	errResolveMatchesMessageIDRequired  = errors.New("discovery: ResolveMatches MessageID is required")
	errResolveMatchesRelatesToRequired  = errors.New("discovery: ResolveMatches RelatesTo is required")
	errResolveMatchesMessageNumberRange = errors.New("discovery: ResolveMatches MessageNumber must be >= 1")
	errResolveMatchesParamsNil          = errors.New("discovery: nil ResolveMatchesParams")
	errResolveMatchEndpointRequired     = errors.New("discovery: ResolveMatch Address is required")
	errResolveMatchXAddrsRequired       = errors.New("discovery: ResolveMatch XAddrs must be non-empty")
)

// ResolveMatchContent is a single d:ResolveMatch (WS-Discovery §6.2).
type ResolveMatchContent struct {
	Address         string
	Types           []string
	Scopes          []string
	XAddrs          []string
	MetadataVersion uint32
}

// ResolveMatchesParams builds a unicast ResolveMatches response (WS-Discovery §6.2).
type ResolveMatchesParams struct {
	MessageID  string
	RelatesTo  string
	To         string
	InstanceID uint32
	MsgNumber  uint32
	Match      ResolveMatchContent
}

// Validate checks ResolveMatches fields.
func (p *ResolveMatchesParams) Validate() error {
	if p == nil {
		return errResolveMatchesParamsNil
	}
	switch {
	case strings.TrimSpace(p.MessageID) == "":
		return errResolveMatchesMessageIDRequired
	case strings.TrimSpace(p.RelatesTo) == "":
		return errResolveMatchesRelatesToRequired
	case p.MsgNumber < 1:
		return errResolveMatchesMessageNumberRange
	case strings.TrimSpace(p.Match.Address) == "":
		return errResolveMatchEndpointRequired
	case len(p.Match.XAddrs) == 0:
		return errResolveMatchXAddrsRequired
	default:
		return nil
	}
}

// MarshalResolveMatches builds a UTF-8 SOAP 1.2 ResolveMatches envelope.
func MarshalResolveMatches(p *ResolveMatchesParams) ([]byte, error) {
	if p == nil {
		return nil, errResolveMatchesParamsNil
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	to := strings.TrimSpace(p.To)
	if to == "" {
		to = WSAAnonymous
	}
	return marshalMatchResponse(
		ActionResolveMatches,
		"<d:ResolveMatches>", "</d:ResolveMatches>", "d:ResolveMatch",
		p.MessageID, to, p.RelatesTo, p.InstanceID, p.MsgNumber,
		p.Match.Types,
		p.Match.Address,
		p.Match.Scopes,
		p.Match.XAddrs,
		p.Match.MetadataVersion,
	)
}
