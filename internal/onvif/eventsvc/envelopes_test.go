package eventsvc

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestEndpointReferenceEnvelope_MarshalsReferenceParameters(t *testing.T) {
	epr := endpointReferenceEnvelope{
		Address: "http://consumer.example/sink",
		ReferenceParameters: &referenceParametersEnvelope{
			InnerXML: "<wsa:MyHeader>v</wsa:MyHeader>",
		},
	}
	out, err := xml.Marshal(epr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "<wsa:Address>http://consumer.example/sink</wsa:Address>") {
		t.Errorf("missing Address element in %q", got)
	}
	if !strings.Contains(got, "<wsa:ReferenceParameters><wsa:MyHeader>v</wsa:MyHeader></wsa:ReferenceParameters>") {
		t.Errorf("missing ReferenceParameters in %q", got)
	}
	if strings.Index(got, "Address") > strings.Index(got, "ReferenceParameters") {
		t.Errorf("Address must precede ReferenceParameters in %q", got)
	}
}

func TestEndpointReferenceEnvelope_OmitsReferenceParametersWhenNil(t *testing.T) {
	epr := endpointReferenceEnvelope{Address: "http://x/y"}
	out, err := xml.Marshal(epr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)
	if strings.Contains(got, "ReferenceParameters") {
		t.Errorf("ReferenceParameters must be omitted when nil; got %q", got)
	}
}
