package deviceiosvc

import "encoding/xml"

// Response envelopes for DeviceIO operations.

type getServiceCapabilitiesResponse struct {
	XMLName      xml.Name                     `xml:"GetServiceCapabilitiesResponse"`
	XMLNS        string                       `xml:"xmlns,attr"`
	Capabilities deviceIOCapabilitiesEnvelope `xml:"tds:Capabilities"`
}

type deviceIOCapabilitiesEnvelope struct {
	RelayOutputs  int `xml:"RelayOutputs,attr"`
	DigitalInputs int `xml:"DigitalInputs,attr"`
}
