package eventsvc

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------- helpers --------------------------------------------------------------

type testStubProvider struct{}

func (testStubProvider) EventServiceCapabilities(context.Context) (ServiceCapabilities, error) {
	return ServiceCapabilities{WSPullPointSupport: true, MaxPullPoints: 10}, nil
}
func (testStubProvider) EventProperties(context.Context) (EventProperties, error) {
	return EventProperties{FixedTopicSet: true, TopicSet: "<stub/>"}, nil
}
func (testStubProvider) CreatePullPointSubscription(
	_ context.Context, _ CreatePullPointSubscriptionParams,
) (SubscriptionInfo, error) {
	return SubscriptionInfo{SubscriptionID: "sub-001"}, nil
}
func (testStubProvider) Subscribe(_ context.Context, _ SubscribeParams) (SubscriptionInfo, error) {
	return SubscriptionInfo{SubscriptionID: "sub-001"}, nil
}
func (testStubProvider) PullMessages(_ context.Context, _ string, _ PullMessagesParams) (PullMessagesResult, error) {
	return PullMessagesResult{}, nil
}
func (testStubProvider) SetSynchronizationPoint(_ context.Context, _ string) error { return nil }
func (testStubProvider) Renew(_ context.Context, _ string, _ RenewParams) (RenewResult, error) {
	return RenewResult{}, nil
}
func (testStubProvider) Unsubscribe(_ context.Context, _ string) error { return nil }

var errTestProviderBoom = errors.New("test provider boom")

type testErrProvider struct{}

func (testErrProvider) EventServiceCapabilities(context.Context) (ServiceCapabilities, error) {
	return ServiceCapabilities{}, errTestProviderBoom
}
func (testErrProvider) EventProperties(context.Context) (EventProperties, error) {
	return EventProperties{}, errTestProviderBoom
}
func (testErrProvider) CreatePullPointSubscription(
	_ context.Context, _ CreatePullPointSubscriptionParams,
) (SubscriptionInfo, error) {
	return SubscriptionInfo{}, errTestProviderBoom
}
func (testErrProvider) Subscribe(_ context.Context, _ SubscribeParams) (SubscriptionInfo, error) {
	return SubscriptionInfo{}, errTestProviderBoom
}
func (testErrProvider) PullMessages(_ context.Context, _ string, _ PullMessagesParams) (PullMessagesResult, error) {
	return PullMessagesResult{}, errTestProviderBoom
}
func (testErrProvider) SetSynchronizationPoint(_ context.Context, _ string) error {
	return errTestProviderBoom
}
func (testErrProvider) Renew(_ context.Context, _ string, _ RenewParams) (RenewResult, error) {
	return RenewResult{}, errTestProviderBoom
}
func (testErrProvider) Unsubscribe(_ context.Context, _ string) error { return errTestProviderBoom }

func soapRequestEvent(op, inner string) string {
	return `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Body><tev:` + op + ` xmlns:tev="` + EventsNamespace + `">` + inner + `</tev:` + op + `></s:Body></s:Envelope>`
}

func doEventRequest(t *testing.T, h *EventServiceHandler, op string) *httptest.ResponseRecorder {
	t.Helper()
	body := soapRequestEvent(op, "")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, EventServicePath,
		bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func soapBodyRootName(t *testing.T, body []byte) xml.Name {
	t.Helper()
	var env struct {
		Body struct {
			Inner []byte `xml:",innerxml"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	dec := xml.NewDecoder(bytes.NewReader(env.Body.Inner))
	for {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("read body token: %v", err)
		}
		if s, ok := tok.(xml.StartElement); ok {
			return s.Name
		}
	}
}

// ---------- tests ----------------------------------------------------------------

func TestEventService_GetServiceCapabilities(t *testing.T) {
	h := NewEventServiceHandler(testStubProvider{})
	rec := doEventRequest(t, h, "GetServiceCapabilities")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	name := soapBodyRootName(t, rec.Body.Bytes())
	if name.Local != "GetServiceCapabilitiesResponse" {
		t.Fatalf("root element = %q", name.Local)
	}
	if !strings.Contains(rec.Body.String(), "WSPullPointSupport") {
		t.Fatalf("response missing WSPullPointSupport: %s", rec.Body.String())
	}
}

func TestEventService_GetEventProperties(t *testing.T) {
	h := NewEventServiceHandler(testStubProvider{})
	rec := doEventRequest(t, h, "GetEventProperties")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	name := soapBodyRootName(t, rec.Body.Bytes())
	if name.Local != "GetEventPropertiesResponse" {
		t.Fatalf("root element = %q", name.Local)
	}
	if !strings.Contains(rec.Body.String(), "FixedTopicSet") {
		t.Fatalf("response missing FixedTopicSet: %s", rec.Body.String())
	}
}

func TestEventService_CreatePullPointSubscription(t *testing.T) {
	h := NewEventServiceHandler(testStubProvider{},
		WithSubscriptionManagerAddr("http://127.0.0.1:8080/onvif/subscription_manager"),
	)
	rec := doEventRequest(t, h, "CreatePullPointSubscription")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	name := soapBodyRootName(t, rec.Body.Bytes())
	if name.Local != "CreatePullPointSubscriptionResponse" {
		t.Fatalf("root element = %q", name.Local)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "?id=sub-001") {
		t.Fatalf("SubscriptionReference missing subscription ID: %s", body)
	}
	if !strings.Contains(body, "TerminationTime") {
		t.Fatalf("response missing TerminationTime: %s", body)
	}
}

func TestEventService_CreatePullPointSubscription_FallbackAddr(t *testing.T) {
	h := NewEventServiceHandler(testStubProvider{})
	rec := doEventRequest(t, h, "CreatePullPointSubscription")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), SubscriptionManagerPath) {
		t.Fatalf("expected fallback path %q in EPR: %s", SubscriptionManagerPath, rec.Body.String())
	}
}

func TestEventService_UnsupportedOperation(t *testing.T) {
	h := NewEventServiceHandler(testStubProvider{})
	rec := doEventRequest(t, h, "UnknownOp")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d want 501; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Sender") {
		t.Fatalf("expected Sender fault: %s", rec.Body.String())
	}
}

func TestEventService_MethodNotAllowed(t *testing.T) {
	h := NewEventServiceHandler(testStubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, EventServicePath, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d want 405", rec.Code)
	}
}

func TestEventService_InvalidEnvelope(t *testing.T) {
	h := NewEventServiceHandler(testStubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, EventServicePath,
		bytes.NewBufferString("not xml"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestEventService_WrongNamespace(t *testing.T) {
	h := NewEventServiceHandler(testStubProvider{})
	body := `<?xml version="1.0"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Body><wsnt:PullMessages xmlns:wsnt="` + WSNBaseNotificationNS + `"/></s:Body></s:Envelope>`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, EventServicePath,
		bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestEventService_ProviderError(t *testing.T) {
	h := NewEventServiceHandler(testErrProvider{})

	rec := doEventRequest(t, h, "GetServiceCapabilities")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d want 500; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Receiver") {
		t.Fatalf("expected Receiver fault: %s", rec.Body.String())
	}
}

func TestEventService_SizeCap(t *testing.T) {
	h := NewEventServiceHandler(testStubProvider{})
	big := bytes.Repeat([]byte("x"), maxSOAPBodySize+1)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, EventServicePath,
		bytes.NewReader(big))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d want 413", rec.Code)
	}
}

func TestEventService_AuthHookRejects(t *testing.T) {
	h := NewEventServiceHandler(testStubProvider{},
		WithEventAuthHook(AuthFunc(func(context.Context, string, *http.Request) error {
			return io.EOF
		})),
	)
	rec := doEventRequest(t, h, "GetServiceCapabilities")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", rec.Code)
	}
}

func TestEventServiceHandler_PanicsOnNilProvider(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil provider")
		}
	}()
	NewEventServiceHandler(nil)
}

func TestWithEventAuthHookNilIsNoop(t *testing.T) {
	h := NewEventServiceHandler(testStubProvider{}, WithEventAuthHook(nil))
	if h.auth == nil {
		t.Fatal("auth should not be nil after WithEventAuthHook(nil)")
	}
}

func TestParseEventOperation_RejectsWrongNamespace(t *testing.T) {
	body := `<?xml version="1.0"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Body><op xmlns="http://example.com/bad"/></s:Body></s:Envelope>`
	_, _, err := parseEventOperation([]byte(body))
	if err == nil {
		t.Fatal("expected errInvalidNamespace, got nil")
	}
}

func TestParseEventOperation_AcceptsEventsNamespace(t *testing.T) {
	body := soapRequestEvent("GetServiceCapabilities", "")
	_, op, err := parseEventOperation([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op != "GetServiceCapabilities" {
		t.Fatalf("operation = %q", op)
	}
}

// ---------- Subscribe (WS-BaseNotification push) --------------------------------

// subscribeCapturingProvider records the SubscribeParams passed in so tests
// can assert that the consumer EPR and ReferenceParameters round-trip
// through the SOAP decoder verbatim.
type subscribeCapturingProvider struct {
	testStubProvider
	captured SubscribeParams
	out      SubscriptionInfo
	err      error
}

func (p *subscribeCapturingProvider) Subscribe(_ context.Context, params SubscribeParams) (SubscriptionInfo, error) {
	p.captured = params
	if p.err != nil {
		return SubscriptionInfo{}, p.err
	}
	return p.out, nil
}

func soapRequestSubscribe(consumerInner, filter, initialTermination string) string {
	return `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Body>` +
		`<wsnt:Subscribe xmlns:wsnt="` + WSNBaseNotificationNS + `" xmlns:wsa="` + WSAddressingNamespace + `">` +
		`<wsnt:ConsumerReference>` + consumerInner + `</wsnt:ConsumerReference>` +
		filter +
		initialTermination +
		`</wsnt:Subscribe></s:Body></s:Envelope>`
}

func TestEventService_Subscribe(t *testing.T) {
	p := &subscribeCapturingProvider{
		out: SubscriptionInfo{SubscriptionID: "sub-xyz"},
	}
	h := NewEventServiceHandler(p,
		WithSubscriptionManagerAddr("http://127.0.0.1:8080/onvif/subscription_manager"))

	body := soapRequestSubscribe(
		`<wsa:Address>http://consumer.example/sink</wsa:Address>`,
		`<wsnt:Filter><wsnt:TopicExpression Dialect="http://docs.oasis-open.org/wsn/t-1/TopicExpression/Concrete">tns1:VideoSource/MotionAlarm</wsnt:TopicExpression></wsnt:Filter>`,
		`<wsnt:InitialTerminationTime>PT1H</wsnt:InitialTerminationTime>`,
	)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, EventServicePath,
		bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	name := soapBodyRootName(t, rec.Body.Bytes())
	if name.Local != "SubscribeResponse" {
		t.Fatalf("root element = %q", name.Local)
	}
	resp := rec.Body.String()
	for _, want := range []string{
		"?id=sub-xyz",
		"<TerminationTime>",
		"<CurrentTime>",
	} {
		if !strings.Contains(resp, want) {
			t.Errorf("response missing %q: %s", want, resp)
		}
	}
	if p.captured.ConsumerAddress != "http://consumer.example/sink" {
		t.Errorf("ConsumerAddress = %q", p.captured.ConsumerAddress)
	}
	if p.captured.Filter != "tns1:VideoSource/MotionAlarm" {
		t.Errorf("Filter = %q", p.captured.Filter)
	}
	if p.captured.InitialTerminationTime != "PT1H" {
		t.Errorf("InitialTerminationTime = %q", p.captured.InitialTerminationTime)
	}
}

func TestEventService_Subscribe_EchoesReferenceParameters(t *testing.T) {
	p := &subscribeCapturingProvider{
		out: SubscriptionInfo{SubscriptionID: "sub-rp"},
	}
	h := NewEventServiceHandler(p)
	body := soapRequestSubscribe(
		`<wsa:Address>http://consumer/x</wsa:Address>`+
			`<wsa:ReferenceParameters><wsa:MyId>k</wsa:MyId></wsa:ReferenceParameters>`,
		"", "",
	)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, EventServicePath,
		bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(p.captured.ConsumerReferenceParams, "<wsa:MyId>k</wsa:MyId>") &&
		!strings.Contains(p.captured.ConsumerReferenceParams, "<MyId") {
		t.Errorf("ReferenceParameters not captured: %q", p.captured.ConsumerReferenceParams)
	}
}

var errTestConsumerRequired = errors.New("ConsumerReference Address required")

func TestEventService_Subscribe_MissingConsumerAddressReturnsFault(t *testing.T) {
	p := &subscribeCapturingProvider{
		err: errors.Join(ErrInvalidArgs, errTestConsumerRequired),
	}
	h := NewEventServiceHandler(p)
	body := soapRequestSubscribe(``, "", "")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, EventServicePath,
		bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Sender") {
		t.Errorf("expected Sender fault: %s", rec.Body.String())
	}
}

func TestParseEventOperation_AcceptsWSNNamespaceForSubscribe(t *testing.T) {
	body := soapRequestSubscribe(`<wsa:Address>http://c/x</wsa:Address>`, "", "")
	_, op, err := parseEventOperation([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op != "Subscribe" {
		t.Fatalf("operation = %q", op)
	}
}
