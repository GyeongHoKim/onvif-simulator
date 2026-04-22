package wsdiscovery

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

func xmlEscape(b *strings.Builder, s string) error {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		return err
	}
	b.Write(buf.Bytes())
	return nil
}

func writeEscapedElement(b *strings.Builder, name, text string) error {
	b.WriteByte('<')
	b.WriteString(name)
	b.WriteByte('>')
	if err := xmlEscape(b, text); err != nil {
		return err
	}
	b.WriteString("</")
	b.WriteString(name)
	b.WriteByte('>')
	return nil
}

func needsTDS(types []string) bool {
	for _, t := range types {
		if strings.HasPrefix(t, "tds:") {
			return true
		}
	}
	return false
}

func writeEnvelopeOpen(b *strings.Builder, types []string) {
	b.WriteString(`<s:Envelope xmlns:s="`)
	b.WriteString(soapNS)
	b.WriteString(`" xmlns:a="`)
	b.WriteString(wsaNS)
	b.WriteString(`" xmlns:d="`)
	b.WriteString(discNS)
	if needsTDS(types) {
		b.WriteString(`" xmlns:tds="`)
		b.WriteString(NSDeviceService)
	}
	b.WriteString(`">`)
}

// writeDiscoveryHeader writes Action, MessageID, optional RelatesTo, To, and d:AppSequence.
func writeDiscoveryHeader(
	b *strings.Builder,
	action, messageID, to string,
	relatesTo string,
	instanceID, messageNumber uint32,
) error {
	b.WriteString(`<s:Header>`)
	if err := writeEscapedElement(b, "a:Action", action); err != nil {
		return err
	}
	if err := writeEscapedElement(b, "a:MessageID", messageID); err != nil {
		return err
	}
	if strings.TrimSpace(relatesTo) != "" {
		if err := writeEscapedElement(b, "a:RelatesTo", relatesTo); err != nil {
			return err
		}
	}
	if err := writeEscapedElement(b, "a:To", to); err != nil {
		return err
	}
	fmt.Fprintf(b, `<d:AppSequence InstanceId="%d" MessageNumber="%d" />`, instanceID, messageNumber)
	b.WriteString(`</s:Header>`)
	return nil
}

// writeHelloLikeBody writes d:Hello / d:ProbeMatch-shaped body (EndpointReference, Types, …).
func writeHelloLikeBody(
	b *strings.Builder,
	parentTag string,
	endpointAddr string,
	types, scopes, xaddrs []string,
	metadataVersion uint32,
) error {
	b.WriteByte('<')
	b.WriteString(parentTag)
	b.WriteByte('>')

	b.WriteString(`<a:EndpointReference><a:Address>`)
	if err := xmlEscape(b, endpointAddr); err != nil {
		return err
	}
	b.WriteString(`</a:Address></a:EndpointReference>`)

	if len(types) > 0 {
		b.WriteString(`<d:Types>`)
		if err := xmlEscape(b, strings.Join(types, " ")); err != nil {
			return err
		}
		b.WriteString(`</d:Types>`)
	}
	if len(scopes) > 0 {
		b.WriteString(`<d:Scopes>`)
		if err := xmlEscape(b, strings.Join(scopes, " ")); err != nil {
			return err
		}
		b.WriteString(`</d:Scopes>`)
	}
	if len(xaddrs) > 0 {
		b.WriteString(`<d:XAddrs>`)
		if err := xmlEscape(b, strings.Join(xaddrs, " ")); err != nil {
			return err
		}
		b.WriteString(`</d:XAddrs>`)
	}

	fmt.Fprintf(b, `<d:MetadataVersion>%d</d:MetadataVersion>`, metadataVersion)
	b.WriteString(`</`)
	b.WriteString(parentTag)
	b.WriteByte('>')
	return nil
}

// marshalMatchResponse builds ProbeMatches or ResolveMatches SOAP envelopes (shared shape).
func marshalMatchResponse(
	action string,
	listOpen, listClose, itemTag string,
	messageID, to, relatesTo string,
	instanceID, msgNum uint32,
	types []string,
	endpoint string,
	scopes, xaddrs []string,
	meta uint32,
) ([]byte, error) {
	var b strings.Builder
	b.WriteString(xml.Header)
	writeEnvelopeOpen(&b, types)
	if err := writeDiscoveryHeader(
		&b, action, messageID, to, relatesTo, instanceID, msgNum,
	); err != nil {
		return nil, err
	}
	b.WriteString(`<s:Body>`)
	b.WriteString(listOpen)
	if err := writeHelloLikeBody(
		&b, itemTag, endpoint, types, scopes, xaddrs, meta,
	); err != nil {
		return nil, err
	}
	b.WriteString(listClose)
	b.WriteString(`</s:Body></s:Envelope>`)
	return []byte(b.String()), nil
}
