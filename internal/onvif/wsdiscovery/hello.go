package wsdiscovery

import (
	"encoding/xml"
	"errors"
	"strings"
)

var (
	errHelloMessageIDRequired  = errors.New("discovery: hello MessageID is required")
	errHelloEndpointRequired   = errors.New("discovery: hello Address is required")
	errHelloMessageNumberRange = errors.New("discovery: hello MessageNumber must be >= 1")
	errHelloParamsNil          = errors.New("discovery: nil HelloParams")
)

// HelloParams carries the fields for a WS-Discovery Hello SOAP 1.2 envelope
// (see WS-Discovery §4.1; ONVIF Core §7.3.2 for Types, Scopes, XAddrs content).
type HelloParams struct {
	MessageID       string
	Address         string
	Types           []string
	Scopes          []string
	XAddrs          []string
	MetadataVersion uint32
	InstanceID      uint32
	MessageNumber   uint32
}

// Validate returns an error if required Hello fields are missing or invalid.
func (p *HelloParams) Validate() error {
	if p == nil {
		return errHelloParamsNil
	}
	switch {
	case strings.TrimSpace(p.MessageID) == "":
		return errHelloMessageIDRequired
	case strings.TrimSpace(p.Address) == "":
		return errHelloEndpointRequired
	case p.MessageNumber < 1:
		return errHelloMessageNumberRange
	default:
		return nil
	}
}

// MarshalHello builds a UTF-8 XML document for a one-way multicast Hello
// (SOAP 1.2 envelope per WS-Discovery §4.1, Table 6).
func MarshalHello(p *HelloParams) ([]byte, error) {
	if p == nil {
		return nil, errHelloParamsNil
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString(xml.Header)
	writeEnvelopeOpen(&b, p.Types)
	if err := writeDiscoveryHeader(
		&b, ActionHello, p.MessageID, ToDiscovery, "", p.InstanceID, p.MessageNumber,
	); err != nil {
		return nil, err
	}
	b.WriteString(`<s:Body>`)
	if err := writeHelloLikeBody(
		&b, "d:Hello", p.Address, p.Types, p.Scopes, p.XAddrs, p.MetadataVersion,
	); err != nil {
		return nil, err
	}
	b.WriteString(`</s:Body></s:Envelope>`)

	return []byte(b.String()), nil
}
