package eventsvc

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
)

const (
	soapNamespace   = "http://www.w3.org/2003/05/soap-envelope"
	maxSOAPBodySize = 10 << 20

	faultCodeSender   = "Sender"
	faultCodeReceiver = "Receiver"

	bodyLocalName = "Body"
)

var xmlReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&apos;",
)

// EventServiceHandler serves the ONVIF Event Service endpoint.
type EventServiceHandler struct {
	provider                Provider
	auth                    AuthHook
	subscriptionManagerAddr string
}

// EventServiceOption customizes an EventServiceHandler.
type EventServiceOption func(*EventServiceHandler)

// WithEventAuthHook installs a request authorization hook.
func WithEventAuthHook(hook AuthHook) EventServiceOption {
	return func(h *EventServiceHandler) {
		if hook != nil {
			h.auth = hook
		}
	}
}

// WithSubscriptionManagerAddr sets the full URL returned as the
// WS-Addressing Address in CreatePullPointSubscription responses.
// Example: "http://192.168.1.10:8080/onvif/subscription_manager"
func WithSubscriptionManagerAddr(addr string) EventServiceOption {
	return func(h *EventServiceHandler) {
		h.subscriptionManagerAddr = addr
	}
}

// NewEventServiceHandler creates an Event Service HTTP handler.
func NewEventServiceHandler(provider Provider, opts ...EventServiceOption) *EventServiceHandler {
	if provider == nil {
		panic(errProviderRequired)
	}
	h := &EventServiceHandler{
		provider: provider,
		auth:     AuthFunc(func(context.Context, string, *http.Request) error { return nil }),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// ServeHTTP dispatches SOAP Event Service operations.
func (h *EventServiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSOAPBodySize)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeFault(w, http.StatusRequestEntityTooLarge, faultCodeSender, tooLarge.Error())
			return
		}
		writeFault(w, http.StatusBadRequest, faultCodeSender, fmt.Errorf("read request body: %w", err).Error())
		return
	}
	if closeErr := r.Body.Close(); closeErr != nil {
		writeFault(w, http.StatusBadRequest, faultCodeSender, closeErr.Error())
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))

	payload, operation, err := parseEventOperation(raw)
	if err != nil {
		// See devicesvc.ServeHTTP: emit an auth challenge when the body is
		// unparseable and the client supplied no credentials, so 2-pass
		// Digest probes can negotiate.
		if r.Header.Get("Authorization") == "" {
			if challengeErr := h.auth.Authorize(r.Context(), "", r); challengeErr != nil {
				writeAuthFault(w, challengeErr)
				return
			}
		}
		writeFault(w, http.StatusBadRequest, faultCodeSender, err.Error())
		return
	}

	if authErr := h.auth.Authorize(r.Context(), operation, r); authErr != nil {
		writeAuthFault(w, authErr)
		return
	}

	respPayload, err := h.dispatch(r.Context(), operation, payload)
	if err != nil {
		status := http.StatusInternalServerError
		code := faultCodeReceiver
		switch {
		case errors.Is(err, errUnsupportedOp):
			status = http.StatusNotImplemented
			code = faultCodeSender
		case errors.Is(err, errDecodePayload),
			errors.Is(err, ErrInvalidArgs):
			status = http.StatusBadRequest
			code = faultCodeSender
		}
		writeFault(w, status, code, err.Error())
		return
	}
	writeSOAP(w, respPayload)
}

func writeAuthFault(w http.ResponseWriter, authErr error) {
	status := http.StatusUnauthorized
	var challenge *auth.ChallengeError
	if errors.As(authErr, &challenge) {
		if challenge.Status != 0 {
			status = challenge.Status
		}
		for k, vs := range challenge.Headers {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
	}
	if errors.Is(authErr, auth.ErrForbidden) && status == http.StatusUnauthorized {
		status = http.StatusForbidden
	}
	writeFault(w, status, faultCodeSender, authErr.Error())
}

func (h *EventServiceHandler) dispatch(ctx context.Context, operation string, payload []byte) ([]byte, error) {
	switch operation {
	case "GetServiceCapabilities":
		return h.handleGetServiceCapabilities(ctx)
	case "CreatePullPointSubscription":
		return h.handleCreatePullPointSubscription(ctx, payload)
	case "GetEventProperties":
		return h.handleGetEventProperties(ctx, payload)
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedOp, operation)
	}
}

func (h *EventServiceHandler) handleGetServiceCapabilities(ctx context.Context) ([]byte, error) {
	caps, err := h.provider.EventServiceCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getServiceCapabilitiesResponse{
		XMLNS:        EventsNamespace,
		Capabilities: eventServiceCapabilitiesEnvelope(caps),
	})
}

func (h *EventServiceHandler) handleCreatePullPointSubscription(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		Filter                 string `xml:"Filter>TopicExpression"`
		InitialTerminationTime string `xml:"InitialTerminationTime"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("eventsvc: decode CreatePullPointSubscription: %w", err))
	}

	info, err := h.provider.CreatePullPointSubscription(ctx, CreatePullPointSubscriptionParams{
		Filter:                 req.Filter,
		InitialTerminationTime: req.InitialTerminationTime,
	})
	if err != nil {
		return nil, err
	}

	rawAddr := h.subscriptionManagerAddr
	if rawAddr == "" {
		rawAddr = SubscriptionManagerPath
	}
	u, err := url.Parse(rawAddr)
	if err != nil {
		return nil, fmt.Errorf("eventsvc: parse subscription manager addr: %w", err)
	}
	q := u.Query()
	q.Set("id", info.SubscriptionID)
	u.RawQuery = q.Encode()
	addr := u.String()

	return xml.Marshal(createPullPointSubscriptionResponse{
		XMLNS:    EventsNamespace,
		XMLNSWsa: WSAddressingNamespace,
		SubscriptionReference: endpointReferenceEnvelope{
			Address: addr,
		},
		CurrentTime:     formatXSDDateTime(info.CurrentTime),
		TerminationTime: formatXSDDateTime(info.TerminationTime),
	})
}

func (h *EventServiceHandler) handleGetEventProperties(ctx context.Context, _ []byte) ([]byte, error) {
	props, err := h.provider.EventProperties(ctx)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getEventPropertiesResponse{
		XMLNS:                  EventsNamespace,
		TopicNamespaceLocation: props.TopicNamespaceLocation,
		FixedTopicSet:          props.FixedTopicSet,
		TopicSet:               topicSetEnvelope{InnerXML: props.TopicSet},
	})
}

// parseEventOperation extracts the SOAP body inner XML and the operation name,
// validating that the operation element belongs to EventsNamespace.
func parseEventOperation(data []byte) (payload []byte, operation string, err error) {
	var env struct {
		Body struct {
			Inner []byte `xml:",innerxml"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, "", fmt.Errorf("parse soap envelope: %w", err)
	}
	if len(env.Body.Inner) == 0 {
		return nil, "", errEmptySOAPBody
	}

	decoder := xml.NewDecoder(bytes.NewReader(data))
	inBody := false
	for {
		tok, err := decoder.Token()
		if err != nil {
			return nil, "", fmt.Errorf("parse soap body: %w", err)
		}
		switch elem := tok.(type) {
		case xml.StartElement:
			if elem.Name.Local == bodyLocalName && elem.Name.Space == soapNamespace {
				inBody = true
				continue
			}
			if !inBody {
				continue
			}
			if elem.Name.Space != EventsNamespace {
				return nil, "", fmt.Errorf("%w: %s", errInvalidNamespace, elem.Name.Space)
			}
			return env.Body.Inner, elem.Name.Local, nil
		case xml.EndElement:
			if inBody && elem.Name.Local == bodyLocalName && elem.Name.Space == soapNamespace {
				return nil, "", errEmptySOAPBody
			}
		}
	}
}

// ---------- shared SOAP helpers --------------------------------------------------

func writeSOAP(w http.ResponseWriter, payload []byte) {
	envelope := soapEnvelope{
		XMLNSEnv: soapNamespace,
		Body:     soapBody{InnerXML: string(payload)},
	}
	body, err := xml.Marshal(envelope)
	if err != nil {
		writeFault(w, http.StatusInternalServerError, faultCodeReceiver, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return
	}
	if _, err := w.Write(body); err != nil {
		return
	}
}

func writeFault(w http.ResponseWriter, status int, code, reason string) {
	innerXML := fmt.Sprintf(
		"<env:Fault xmlns:env=%q><env:Code><env:Value>env:%s</env:Value></env:Code>"+
			"<env:Reason><env:Text xml:lang=%q>%s</env:Text></env:Reason></env:Fault>",
		soapNamespace,
		xmlEscape(code),
		"en",
		xmlEscape(reason),
	)
	fault := soapEnvelope{
		XMLNSEnv: soapNamespace,
		Body:     soapBody{InnerXML: innerXML},
	}
	body, err := xml.Marshal(fault)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return
	}
	if _, err := w.Write(body); err != nil {
		return
	}
}

func xmlEscape(value string) string {
	return xmlReplacer.Replace(value)
}
