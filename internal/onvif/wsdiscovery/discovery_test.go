package wsdiscovery_test

import (
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/wsdiscovery"
)

// ── ParseProbe ─────────────────────────────────────────────────────────────

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

	p, err := wsdiscovery.ParseProbe([]byte(xmlDoc))
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

func TestParseProbe_WithScopesAndMatchBy(t *testing.T) {
	t.Parallel()
	const xmlDoc = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
  xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
  xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>
    <a:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</a:Action>
    <a:MessageID>uuid:test-scopes</a:MessageID>
    <a:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To>
    <a:ReplyTo><a:Address>http://example.com/reply</a:Address></a:ReplyTo>
  </s:Header>
  <s:Body>
    <d:Probe>
      <d:Types>tds:Device</d:Types>
      <d:Scopes MatchBy="http://schemas.xmlsoap.org/ws/2005/04/discovery/rfc3986">onvif://www.onvif.org/name/cam1</d:Scopes>
    </d:Probe>
  </s:Body>
</s:Envelope>`

	p, err := wsdiscovery.ParseProbe([]byte(xmlDoc))
	if err != nil {
		t.Fatalf("ParseProbe: %v", err)
	}
	if p.MatchBy != wsdiscovery.MatchByRFC3986 {
		t.Fatalf("MatchBy: got %q", p.MatchBy)
	}
	if len(p.Scopes) != 1 || p.Scopes[0] != "onvif://www.onvif.org/name/cam1" {
		t.Fatalf("Scopes: %#v", p.Scopes)
	}
	if p.ReplyToAddress != "http://example.com/reply" {
		t.Fatalf("ReplyToAddress: got %q", p.ReplyToAddress)
	}
}

func TestParseProbe_ErrNotProbe(t *testing.T) {
	t.Parallel()
	// Body에 Probe 대신 Resolve가 들어있는 경우.
	const xmlDoc = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
  xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
  xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>
    <a:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Resolve</a:Action>
    <a:MessageID>uuid:test</a:MessageID>
    <a:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To>
  </s:Header>
  <s:Body>
    <d:Resolve>
      <a:EndpointReference><a:Address>urn:uuid:dev</a:Address></a:EndpointReference>
    </d:Resolve>
  </s:Body>
</s:Envelope>`

	_, err := wsdiscovery.ParseProbe([]byte(xmlDoc))
	if err != wsdiscovery.ErrNotProbe {
		t.Fatalf("expected ErrNotProbe, got %v", err)
	}
}

func TestParseProbe_InvalidXML(t *testing.T) {
	t.Parallel()
	_, err := wsdiscovery.ParseProbe([]byte("not xml at all"))
	if err == nil {
		t.Fatal("expected error for invalid XML, got nil")
	}
}

// ── IncomingProbe.Validate ─────────────────────────────────────────────────

func TestIncomingProbeValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		probe   wsdiscovery.IncomingProbe
		wantErr bool
	}{
		{
			name: "valid",
			probe: wsdiscovery.IncomingProbe{
				MessageID: "uuid:test",
				Action:    wsdiscovery.ActionProbe,
				To:        wsdiscovery.ToDiscovery,
			},
		},
		{
			name:    "empty MessageID",
			probe:   wsdiscovery.IncomingProbe{Action: wsdiscovery.ActionProbe, To: wsdiscovery.ToDiscovery},
			wantErr: true,
		},
		{
			name:    "wrong Action",
			probe:   wsdiscovery.IncomingProbe{MessageID: "uuid:test", Action: "wrong", To: wsdiscovery.ToDiscovery},
			wantErr: true,
		},
		{
			name:    "wrong To",
			probe:   wsdiscovery.IncomingProbe{MessageID: "uuid:test", Action: wsdiscovery.ActionProbe, To: "wrong"},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.probe.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// ── ProbeMatches ───────────────────────────────────────────────────────────

func TestProbeMatches(t *testing.T) {
	t.Parallel()
	probe := &wsdiscovery.IncomingProbe{
		Types:   []string{"tds:Device"},
		Scopes:  []string{"onvif://www.onvif.org/name/cam1"},
		MatchBy: wsdiscovery.MatchByRFC3986,
	}
	devTypes := []string{"tds:Device"}
	devScopes := []string{"onvif://www.onvif.org/name/cam1/extra"}
	if !wsdiscovery.ProbeMatches(probe, devTypes, devScopes) {
		t.Fatal("expected match")
	}
	if wsdiscovery.ProbeMatches(probe, []string{"other:Thing"}, devScopes) {
		t.Fatal("expected type mismatch")
	}
}

func TestProbeMatches_NilProbe(t *testing.T) {
	t.Parallel()
	if wsdiscovery.ProbeMatches(nil, []string{"tds:Device"}, nil) {
		t.Fatal("nil probe should not match")
	}
}

func TestProbeMatches_EmptyTypesMatchesAny(t *testing.T) {
	t.Parallel()
	probe := &wsdiscovery.IncomingProbe{MatchBy: wsdiscovery.MatchByRFC3986}
	if !wsdiscovery.ProbeMatches(probe, []string{"tds:Device"}, nil) {
		t.Fatal("empty probe Types should match any device type")
	}
}

func TestProbeMatches_EmptyScopesMatchesAny(t *testing.T) {
	t.Parallel()
	probe := &wsdiscovery.IncomingProbe{
		Types:   []string{"tds:Device"},
		MatchBy: wsdiscovery.MatchByRFC3986,
	}
	devScopes := []string{"onvif://www.onvif.org/name/cam1"}
	if !wsdiscovery.ProbeMatches(probe, []string{"tds:Device"}, devScopes) {
		t.Fatal("empty probe Scopes should match any device scope")
	}
}

func TestProbeMatches_Strcmp0_Exact(t *testing.T) {
	t.Parallel()
	probe := &wsdiscovery.IncomingProbe{
		Scopes:  []string{"onvif://www.onvif.org/name/cam1"},
		MatchBy: wsdiscovery.MatchByStrcmp0,
	}
	if !wsdiscovery.ProbeMatches(probe, nil, []string{"onvif://www.onvif.org/name/cam1"}) {
		t.Fatal("strcmp0: exact match should succeed")
	}
	if wsdiscovery.ProbeMatches(probe, nil, []string{"onvif://www.onvif.org/name/cam1/extra"}) {
		t.Fatal("strcmp0: prefix-only match should fail")
	}
}

func TestProbeMatches_UnknownMatchBy(t *testing.T) {
	t.Parallel()
	probe := &wsdiscovery.IncomingProbe{
		Scopes:  []string{"onvif://www.onvif.org/name/cam1"},
		MatchBy: "unknown://matchby",
	}
	if wsdiscovery.ProbeMatches(probe, nil, []string{"onvif://www.onvif.org/name/cam1"}) {
		t.Fatal("unknown MatchBy should not match")
	}
}

func TestProbeMatches_ScopeProbeLongerThanDevice(t *testing.T) {
	t.Parallel()
	probe := &wsdiscovery.IncomingProbe{
		Scopes:  []string{"onvif://www.onvif.org/name/cam1/extra"},
		MatchBy: wsdiscovery.MatchByRFC3986,
	}
	if wsdiscovery.ProbeMatches(probe, nil, []string{"onvif://www.onvif.org/name/cam1"}) {
		t.Fatal("probe scope longer than device scope should not match")
	}
}

func TestProbeMatches_ScopeDifferentScheme(t *testing.T) {
	t.Parallel()
	probe := &wsdiscovery.IncomingProbe{
		Scopes:  []string{"http://www.onvif.org/name/cam1"},
		MatchBy: wsdiscovery.MatchByRFC3986,
	}
	if wsdiscovery.ProbeMatches(probe, nil, []string{"onvif://www.onvif.org/name/cam1"}) {
		t.Fatal("different scheme should not match")
	}
}

func TestProbeMatches_ScopeDifferentHost(t *testing.T) {
	t.Parallel()
	probe := &wsdiscovery.IncomingProbe{
		Scopes:  []string{"onvif://www.other.org/name/cam1"},
		MatchBy: wsdiscovery.MatchByRFC3986,
	}
	if wsdiscovery.ProbeMatches(probe, nil, []string{"onvif://www.onvif.org/name/cam1"}) {
		t.Fatal("different host should not match")
	}
}

// ── MatchResolve ───────────────────────────────────────────────────────────

func TestMatchResolve(t *testing.T) {
	t.Parallel()
	resolve := &wsdiscovery.IncomingResolve{Address: "urn:uuid:device-1"}
	if !wsdiscovery.MatchResolve(resolve, "urn:uuid:device-1") {
		t.Fatal("expected match")
	}
	if wsdiscovery.MatchResolve(resolve, "urn:uuid:device-2") {
		t.Fatal("expected no match for different address")
	}
}

func TestMatchResolve_NilResolve(t *testing.T) {
	t.Parallel()
	if wsdiscovery.MatchResolve(nil, "urn:uuid:device-1") {
		t.Fatal("nil resolve should not match")
	}
}

func TestMatchResolve_CaseInsensitive(t *testing.T) {
	t.Parallel()
	resolve := &wsdiscovery.IncomingResolve{Address: "URN:UUID:DEVICE-1"}
	if !wsdiscovery.MatchResolve(resolve, "urn:uuid:device-1") {
		t.Fatal("MatchResolve should be case-insensitive")
	}
}

func TestMatchResolve_EmptyAddress(t *testing.T) {
	t.Parallel()
	resolve := &wsdiscovery.IncomingResolve{Address: ""}
	if wsdiscovery.MatchResolve(resolve, "") {
		t.Fatal("empty address should not match")
	}
}

// ── HelloParams.Validate / MarshalHello ────────────────────────────────────

func TestMarshalHelloRoundTripFields(t *testing.T) {
	t.Parallel()
	p := &wsdiscovery.HelloParams{
		MessageID:       wsdiscovery.NewMessageID(),
		Address:         "urn:uuid:11111111-2222-4333-8444-555555555555",
		Types:           []string{"tds:Device"},
		Scopes:          []string{"onvif://www.onvif.org/name/x"},
		XAddrs:          []string{"http://127.0.0.1:8080/onvif/device_service"},
		MetadataVersion: 1,
		InstanceID:      1,
		MessageNumber:   1,
	}
	b, err := wsdiscovery.MarshalHello(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "tds:Device") {
		t.Fatalf("output: %s", b)
	}
}

func TestMarshalHello_NilParams(t *testing.T) {
	t.Parallel()
	_, err := wsdiscovery.MarshalHello(nil)
	if err == nil {
		t.Fatal("expected error for nil params")
	}
}

func TestHelloParamsValidate(t *testing.T) {
	t.Parallel()
	validBase := wsdiscovery.HelloParams{
		MessageID:     "uuid:test",
		Address:       "urn:uuid:device",
		MessageNumber: 1,
	}
	tests := []struct {
		name    string
		mutate  func(*wsdiscovery.HelloParams)
		wantErr bool
	}{
		{"valid", func(p *wsdiscovery.HelloParams) {}, false},
		{"empty MessageID", func(p *wsdiscovery.HelloParams) { p.MessageID = "" }, true},
		{"empty Address", func(p *wsdiscovery.HelloParams) { p.Address = "" }, true},
		{"zero MessageNumber", func(p *wsdiscovery.HelloParams) { p.MessageNumber = 0 }, true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := validBase
			tc.mutate(&p)
			err := p.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// ── ByeParams.Validate / MarshalBye ───────────────────────────────────────

func TestMarshalBye_Valid(t *testing.T) {
	t.Parallel()
	p := &wsdiscovery.ByeParams{
		MessageID:     wsdiscovery.NewMessageID(),
		Address:       "urn:uuid:device-1",
		InstanceID:    1,
		MessageNumber: 1,
	}
	b, err := wsdiscovery.MarshalBye(p)
	if err != nil {
		t.Fatalf("MarshalBye: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "urn:uuid:device-1") {
		t.Fatalf("Bye XML missing address: %s", s)
	}
	if !strings.Contains(s, wsdiscovery.ActionBye) {
		t.Fatalf("Bye XML missing action: %s", s)
	}
}

func TestMarshalBye_NilParams(t *testing.T) {
	t.Parallel()
	_, err := wsdiscovery.MarshalBye(nil)
	if err == nil {
		t.Fatal("expected error for nil params")
	}
}

func TestByeParamsValidate(t *testing.T) {
	t.Parallel()
	validBase := wsdiscovery.ByeParams{
		MessageID:     "uuid:test",
		Address:       "urn:uuid:device",
		MessageNumber: 1,
	}
	tests := []struct {
		name    string
		mutate  func(*wsdiscovery.ByeParams)
		wantErr bool
	}{
		{"valid", func(p *wsdiscovery.ByeParams) {}, false},
		{"empty MessageID", func(p *wsdiscovery.ByeParams) { p.MessageID = "" }, true},
		{"empty Address", func(p *wsdiscovery.ByeParams) { p.Address = "" }, true},
		{"zero MessageNumber", func(p *wsdiscovery.ByeParams) { p.MessageNumber = 0 }, true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := validBase
			tc.mutate(&p)
			err := p.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// ── ParseResolve / IncomingResolve.Validate ────────────────────────────────

func TestParseResolve_Valid(t *testing.T) {
	t.Parallel()
	const xmlDoc = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
  xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
  xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>
    <a:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Resolve</a:Action>
    <a:MessageID>uuid:resolve-test</a:MessageID>
    <a:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To>
  </s:Header>
  <s:Body>
    <d:Resolve>
      <a:EndpointReference>
        <a:Address>urn:uuid:device-endpoint</a:Address>
      </a:EndpointReference>
    </d:Resolve>
  </s:Body>
</s:Envelope>`

	r, err := wsdiscovery.ParseResolve([]byte(xmlDoc))
	if err != nil {
		t.Fatalf("ParseResolve: %v", err)
	}
	if r.MessageID != "uuid:resolve-test" {
		t.Fatalf("MessageID: got %q", r.MessageID)
	}
	if r.Address != "urn:uuid:device-endpoint" {
		t.Fatalf("Address: got %q", r.Address)
	}
}

func TestParseResolve_ErrNotResolve(t *testing.T) {
	t.Parallel()
	const xmlDoc = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
  xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
  xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>
    <a:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</a:Action>
    <a:MessageID>uuid:test</a:MessageID>
    <a:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To>
  </s:Header>
  <s:Body>
    <d:Probe><d:Types>tds:Device</d:Types></d:Probe>
  </s:Body>
</s:Envelope>`

	_, err := wsdiscovery.ParseResolve([]byte(xmlDoc))
	if err != wsdiscovery.ErrNotResolve {
		t.Fatalf("expected ErrNotResolve, got %v", err)
	}
}

func TestParseResolve_InvalidXML(t *testing.T) {
	t.Parallel()
	_, err := wsdiscovery.ParseResolve([]byte("not xml"))
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}

func TestIncomingResolveValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		resolve wsdiscovery.IncomingResolve
		wantErr bool
	}{
		{
			name: "valid",
			resolve: wsdiscovery.IncomingResolve{
				MessageID: "uuid:test",
				Action:    wsdiscovery.ActionResolve,
				To:        wsdiscovery.ToDiscovery,
				Address:   "urn:uuid:device",
			},
		},
		{
			name:    "empty MessageID",
			resolve: wsdiscovery.IncomingResolve{Action: wsdiscovery.ActionResolve, To: wsdiscovery.ToDiscovery, Address: "urn:uuid:device"},
			wantErr: true,
		},
		{
			name:    "wrong Action",
			resolve: wsdiscovery.IncomingResolve{MessageID: "uuid:test", Action: "wrong", To: wsdiscovery.ToDiscovery, Address: "urn:uuid:device"},
			wantErr: true,
		},
		{
			name:    "wrong To",
			resolve: wsdiscovery.IncomingResolve{MessageID: "uuid:test", Action: wsdiscovery.ActionResolve, To: "wrong", Address: "urn:uuid:device"},
			wantErr: true,
		},
		{
			name:    "empty Address",
			resolve: wsdiscovery.IncomingResolve{MessageID: "uuid:test", Action: wsdiscovery.ActionResolve, To: wsdiscovery.ToDiscovery},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.resolve.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// ── ProbeMatchesParams.Validate / MarshalProbeMatches ─────────────────────

func TestMarshalProbeMatches_Valid(t *testing.T) {
	t.Parallel()
	p := &wsdiscovery.ProbeMatchesParams{
		MessageID:  wsdiscovery.NewMessageID(),
		RelatesTo:  "uuid:probe-request",
		InstanceID: 1,
		MsgNumber:  1,
		Match: wsdiscovery.ProbeMatchContent{
			Address:         "urn:uuid:device-1",
			Types:           []string{"tds:Device"},
			Scopes:          []string{"onvif://www.onvif.org/name/cam1"},
			XAddrs:          []string{"http://192.168.1.1/onvif/device_service"},
			MetadataVersion: 1,
		},
	}
	b, err := wsdiscovery.MarshalProbeMatches(p)
	if err != nil {
		t.Fatalf("MarshalProbeMatches: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "ProbeMatches") {
		t.Fatalf("output missing ProbeMatches: %s", s)
	}
	if !strings.Contains(s, "urn:uuid:device-1") {
		t.Fatalf("output missing device address: %s", s)
	}
}

func TestMarshalProbeMatches_NilParams(t *testing.T) {
	t.Parallel()
	_, err := wsdiscovery.MarshalProbeMatches(nil)
	if err == nil {
		t.Fatal("expected error for nil params")
	}
}

func TestProbeMatchesParamsValidate(t *testing.T) {
	t.Parallel()
	validBase := wsdiscovery.ProbeMatchesParams{
		MessageID: "uuid:test",
		RelatesTo: "uuid:probe",
		MsgNumber: 1,
		Match:     wsdiscovery.ProbeMatchContent{Address: "urn:uuid:device"},
	}
	tests := []struct {
		name    string
		mutate  func(*wsdiscovery.ProbeMatchesParams)
		wantErr bool
	}{
		{"valid", func(p *wsdiscovery.ProbeMatchesParams) {}, false},
		{"empty MessageID", func(p *wsdiscovery.ProbeMatchesParams) { p.MessageID = "" }, true},
		{"empty RelatesTo", func(p *wsdiscovery.ProbeMatchesParams) { p.RelatesTo = "" }, true},
		{"zero MsgNumber", func(p *wsdiscovery.ProbeMatchesParams) { p.MsgNumber = 0 }, true},
		{"empty Match.Address", func(p *wsdiscovery.ProbeMatchesParams) { p.Match.Address = "" }, true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := validBase
			tc.mutate(&p)
			err := p.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// ── ResolveMatchesParams.Validate / MarshalResolveMatches ─────────────────

func TestMarshalResolveMatches_Valid(t *testing.T) {
	t.Parallel()
	p := &wsdiscovery.ResolveMatchesParams{
		MessageID:  wsdiscovery.NewMessageID(),
		RelatesTo:  "uuid:resolve-request",
		InstanceID: 1,
		MsgNumber:  1,
		Match: wsdiscovery.ResolveMatchContent{
			Address:         "urn:uuid:device-1",
			Types:           []string{"tds:Device"},
			XAddrs:          []string{"http://192.168.1.1/onvif/device_service"},
			MetadataVersion: 1,
		},
	}
	b, err := wsdiscovery.MarshalResolveMatches(p)
	if err != nil {
		t.Fatalf("MarshalResolveMatches: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "ResolveMatches") {
		t.Fatalf("output missing ResolveMatches: %s", s)
	}
	if !strings.Contains(s, "urn:uuid:device-1") {
		t.Fatalf("output missing device address: %s", s)
	}
}

func TestMarshalResolveMatches_NilParams(t *testing.T) {
	t.Parallel()
	_, err := wsdiscovery.MarshalResolveMatches(nil)
	if err == nil {
		t.Fatal("expected error for nil params")
	}
}

func TestResolveMatchesParamsValidate(t *testing.T) {
	t.Parallel()
	validBase := wsdiscovery.ResolveMatchesParams{
		MessageID: "uuid:test",
		RelatesTo: "uuid:resolve",
		MsgNumber: 1,
		Match: wsdiscovery.ResolveMatchContent{
			Address: "urn:uuid:device",
			XAddrs:  []string{"http://192.168.1.1/onvif/device_service"},
		},
	}
	tests := []struct {
		name    string
		mutate  func(*wsdiscovery.ResolveMatchesParams)
		wantErr bool
	}{
		{"valid", func(p *wsdiscovery.ResolveMatchesParams) {}, false},
		{"empty MessageID", func(p *wsdiscovery.ResolveMatchesParams) { p.MessageID = "" }, true},
		{"empty RelatesTo", func(p *wsdiscovery.ResolveMatchesParams) { p.RelatesTo = "" }, true},
		{"zero MsgNumber", func(p *wsdiscovery.ResolveMatchesParams) { p.MsgNumber = 0 }, true},
		{"empty Match.Address", func(p *wsdiscovery.ResolveMatchesParams) { p.Match.Address = "" }, true},
		{"empty Match.XAddrs", func(p *wsdiscovery.ResolveMatchesParams) { p.Match.XAddrs = nil }, true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := validBase
			tc.mutate(&p)
			err := p.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// ── AppSequence ───────────────────────────────────────────────────────────

func TestAppSequence(t *testing.T) {
	t.Parallel()
	a := wsdiscovery.NewAppSequence(7)
	if a.NextMessageNumber() != 1 || a.NextMessageNumber() != 2 {
		t.Fatal("unexpected sequence")
	}
}

func TestAppSequence_Concurrent(t *testing.T) {
	t.Parallel()
	a := wsdiscovery.NewAppSequence(1)
	const goroutines = 100
	results := make([]uint32, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			results[i] = a.NextMessageNumber()
		}()
	}
	wg.Wait()

	seen := make(map[uint32]bool, goroutines)
	for _, v := range results {
		if seen[v] {
			t.Fatalf("duplicate MessageNumber %d under concurrent access", v)
		}
		seen[v] = true
	}
}

// ── NewMessageID ──────────────────────────────────────────────────────────

func TestNewMessageID_Format(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(`^uuid:[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for i := 0; i < 10; i++ {
		id := wsdiscovery.NewMessageID()
		if !re.MatchString(id) {
			t.Fatalf("NewMessageID format invalid: %q", id)
		}
	}
}

func TestNewMessageID_Unique(t *testing.T) {
	t.Parallel()
	const n = 100
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		id := wsdiscovery.NewMessageID()
		if seen[id] {
			t.Fatalf("duplicate MessageID: %q", id)
		}
		seen[id] = true
	}
}
