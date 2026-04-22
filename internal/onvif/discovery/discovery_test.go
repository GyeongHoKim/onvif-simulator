package discovery_test

import (
	"strings"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/discovery"
)

func TestParseProbe_Table1Style(t *testing.T) {
	t.Parallel()
	const xmlDoc = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
  xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
  xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>
    <a:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</a:Action>
    <a:MessageID>uuid:0a6dc791-2be6-4991-9af1-454778a1917a</a:MessageID>
    <a:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To>
  </s:Header>
  <s:Body>
    <d:Probe>
      <d:Types>tds:Device</d:Types>
    </d:Probe>
  </s:Body>
</s:Envelope>`

	p, err := discovery.ParseProbe([]byte(xmlDoc))
	if err != nil {
		t.Fatalf("ParseProbe: %v", err)
	}
	if p.MessageID != "uuid:0a6dc791-2be6-4991-9af1-454778a1917a" {
		t.Fatalf("MessageID: got %q", p.MessageID)
	}
	if len(p.Types) != 1 || p.Types[0] != "tds:Device" {
		t.Fatalf("Types: %#v", p.Types)
	}
}

func TestProbeMatches(t *testing.T) {
	t.Parallel()
	probe := &discovery.IncomingProbe{
		Types:   []string{"tds:Device"},
		Scopes:  []string{"onvif://www.onvif.org/name/cam1"},
		MatchBy: discovery.MatchByRFC3986,
	}
	devTypes := []string{"tds:Device"}
	devScopes := []string{"onvif://www.onvif.org/name/cam1/extra"}
	if !discovery.ProbeMatches(probe, devTypes, devScopes) {
		t.Fatal("expected match")
	}
	if discovery.ProbeMatches(probe, []string{"other:Thing"}, devScopes) {
		t.Fatal("expected type mismatch")
	}
}

func TestMarshalHelloRoundTripFields(t *testing.T) {
	t.Parallel()
	p := &discovery.HelloParams{
		MessageID:       discovery.NewMessageID(),
		Address:         "urn:uuid:11111111-2222-4333-8444-555555555555",
		Types:           []string{"tds:Device"},
		Scopes:          []string{"onvif://www.onvif.org/name/x"},
		XAddrs:          []string{"http://127.0.0.1:8080/onvif/device_service"},
		MetadataVersion: 1,
		InstanceID:      1,
		MessageNumber:   1,
	}
	b, err := discovery.MarshalHello(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "tds:Device") {
		t.Fatalf("output: %s", b)
	}
}

func TestAppSequence(t *testing.T) {
	t.Parallel()
	a := discovery.NewAppSequence(7)
	if a.NextMessageNumber() != 1 || a.NextMessageNumber() != 2 {
		t.Fatal("unexpected sequence")
	}
}
