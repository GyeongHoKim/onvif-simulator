package wsdiscovery

import (
	"encoding/xml"
	"errors"
	"strings"
)

var (
	errByeMessageIDRequired  = errors.New("discovery: bye MessageID is required")
	errByeEndpointRequired   = errors.New("discovery: bye Address is required")
	errByeMessageNumberRange = errors.New("discovery: bye MessageNumber must be >= 1")
	errByeParamsNil          = errors.New("discovery: nil ByeParams")
)

// ByeParams is a WS-Discovery Bye one-way message (WS-Discovery §4.2, Table 7).
type ByeParams struct {
	MessageID     string
	Address       string
	InstanceID    uint32
	MessageNumber uint32
}

// Validate checks required Bye fields.
func (p *ByeParams) Validate() error {
	if p == nil {
		return errByeParamsNil
	}
	switch {
	case strings.TrimSpace(p.MessageID) == "":
		return errByeMessageIDRequired
	case strings.TrimSpace(p.Address) == "":
		return errByeEndpointRequired
	case p.MessageNumber < 1:
		return errByeMessageNumberRange
	default:
		return nil
	}
}

// MarshalBye builds a UTF-8 SOAP 1.2 Bye envelope for multicast.
func MarshalBye(p *ByeParams) ([]byte, error) {
	if p == nil {
		return nil, errByeParamsNil
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString(xml.Header)
	writeEnvelopeOpen(&b, nil)
	if err := writeDiscoveryHeader(
		&b, ActionBye, p.MessageID, ToDiscovery, "", p.InstanceID, p.MessageNumber,
	); err != nil {
		return nil, err
	}
	b.WriteString(`<s:Body><d:Bye>`)
	b.WriteString(`<a:EndpointReference><a:Address>`)
	if err := xmlEscape(&b, p.Address); err != nil {
		return nil, err
	}
	b.WriteString(`</a:Address></a:EndpointReference>`)
	b.WriteString(`</d:Bye></s:Body></s:Envelope>`)

	return []byte(b.String()), nil
}
