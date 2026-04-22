package discovery

import (
	"encoding/xml"
	"errors"
	"strings"
)

var (
	// ErrNotResolve means the XML body is not a WS-Discovery Resolve.
	ErrNotResolve = errors.New("discovery: envelope is not a Resolve")

	errResolveMessageIDEmpty = errors.New("discovery: Resolve MessageID is required")
	errResolveActionInvalid  = errors.New("discovery: Resolve Action must be " + ActionResolve)
	errResolveToInvalid      = errors.New("discovery: Resolve To must be " + ToDiscovery)
	errResolveAddressEmpty   = errors.New("discovery: Resolve Address is required")
)

// IncomingResolve is a parsed WS-Discovery Resolve (WS-Discovery §6.1).
type IncomingResolve struct {
	Action         string
	MessageID      string
	To             string
	Address        string
	ReplyToAddress string
}

// Validate checks that the Resolve contains required WS-Discovery fields (§6.1).
func (r *IncomingResolve) Validate() error {
	switch {
	case strings.TrimSpace(r.MessageID) == "":
		return errResolveMessageIDEmpty
	case strings.TrimSpace(r.Action) != ActionResolve:
		return errResolveActionInvalid
	case strings.TrimSpace(r.To) != ToDiscovery:
		return errResolveToInvalid
	case strings.TrimSpace(r.Address) == "":
		return errResolveAddressEmpty
	default:
		return nil
	}
}

// ParseResolve decodes a SOAP 1.2 envelope whose body contains d:Resolve.
func ParseResolve(data []byte) (*IncomingResolve, error) {
	var env struct {
		Header soapHeader `xml:"http://www.w3.org/2003/05/soap-envelope Header"`
		Body   struct {
			Resolve *resolveXML `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Resolve"`
		} `xml:"http://www.w3.org/2003/05/soap-envelope Body"`
	}
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if env.Body.Resolve == nil {
		return nil, ErrNotResolve
	}
	r := env.Body.Resolve
	return &IncomingResolve{
		Action:         strings.TrimSpace(env.Header.Action),
		MessageID:      strings.TrimSpace(env.Header.MessageID),
		To:             strings.TrimSpace(env.Header.To),
		Address:        strings.TrimSpace(r.EndpointRef.Address),
		ReplyToAddress: replyToString(env.Header.ReplyTo),
	}, nil
}

type resolveXML struct {
	EndpointRef struct {
		Address string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing Address"`
	} `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing EndpointReference"`
}
