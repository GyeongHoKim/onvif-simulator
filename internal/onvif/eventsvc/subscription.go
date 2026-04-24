package eventsvc

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// SubscriptionManagerHandler serves the ONVIF SubscriptionManager endpoint.
type SubscriptionManagerHandler struct {
	provider Provider
	auth     AuthHook
}

// SubscriptionManagerOption customizes a SubscriptionManagerHandler.
type SubscriptionManagerOption func(*SubscriptionManagerHandler)

// WithSubscriptionManagerAuthHook installs a request authorization hook.
func WithSubscriptionManagerAuthHook(hook AuthHook) SubscriptionManagerOption {
	return func(h *SubscriptionManagerHandler) {
		if hook != nil {
			h.auth = hook
		}
	}
}

// NewSubscriptionManagerHandler creates a Subscription Manager HTTP handler.
func NewSubscriptionManagerHandler(provider Provider, opts ...SubscriptionManagerOption) *SubscriptionManagerHandler {
	if provider == nil {
		panic(errProviderRequired)
	}
	h := &SubscriptionManagerHandler{
		provider: provider,
		auth:     AuthFunc(func(context.Context, string, *http.Request) error { return nil }),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// ServeHTTP dispatches SOAP SubscriptionManager operations.
func (h *SubscriptionManagerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	payload, operation, err := parseSubscriptionManagerOperation(raw)
	if err != nil {
		writeFault(w, http.StatusBadRequest, faultCodeSender, err.Error())
		return
	}

	if authErr := h.auth.Authorize(r.Context(), operation, r); authErr != nil {
		writeAuthFault(w, authErr)
		return
	}

	subscriptionID := r.URL.Query().Get("id")
	if subscriptionID == "" {
		writeFault(w, http.StatusBadRequest, faultCodeSender, "missing subscription id")
		return
	}

	respPayload, err := h.dispatch(r.Context(), subscriptionID, operation, payload)
	if err != nil {
		status := http.StatusInternalServerError
		code := faultCodeReceiver
		switch {
		case errors.Is(err, errUnsupportedOp):
			status = http.StatusNotImplemented
			code = faultCodeSender
		case errors.Is(err, errDecodePayload),
			errors.Is(err, ErrSubscriptionNotFound),
			errors.Is(err, ErrInvalidArgs):
			status = http.StatusBadRequest
			code = faultCodeSender
		}
		writeFault(w, status, code, err.Error())
		return
	}
	writeSOAP(w, respPayload)
}

func (h *SubscriptionManagerHandler) dispatch(
	ctx context.Context, subscriptionID, operation string, payload []byte,
) ([]byte, error) {
	switch operation {
	case "PullMessages":
		return h.handlePullMessages(ctx, subscriptionID, payload)
	case "SetSynchronizationPoint":
		return h.handleSetSynchronizationPoint(ctx, subscriptionID)
	case "Renew":
		return h.handleRenew(ctx, subscriptionID, payload)
	case "Unsubscribe":
		return h.handleUnsubscribe(ctx, subscriptionID)
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedOp, operation)
	}
}

func (h *SubscriptionManagerHandler) handlePullMessages(
	ctx context.Context, subscriptionID string, payload []byte,
) ([]byte, error) {
	var req struct {
		Timeout      string `xml:"Timeout"`
		MessageLimit string `xml:"MessageLimit"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("eventsvc: decode PullMessages: %w", err))
	}
	limit := 0
	if req.MessageLimit != "" {
		v, err := strconv.Atoi(req.MessageLimit)
		if err != nil {
			return nil, fmt.Errorf("%w: MessageLimit must be an integer", ErrInvalidArgs)
		}
		if v < 0 {
			return nil, fmt.Errorf("%w: MessageLimit must be non-negative", ErrInvalidArgs)
		}
		limit = v
	}

	result, err := h.provider.PullMessages(ctx, subscriptionID, PullMessagesParams{
		Timeout:      req.Timeout,
		MessageLimit: limit,
	})
	if err != nil {
		return nil, err
	}

	msgs := make([]notificationMessageEnvelope, len(result.Messages))
	for i, m := range result.Messages {
		msgs[i] = notificationMessageEnvelope{
			SubscriptionReference: endpointReferenceEnvelope{Address: m.SubscriptionReference},
			Topic: topicExpressionEnvelope{
				Dialect: "http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet",
				Value:   m.Topic,
			},
			Message: notificationMessageBody{InnerXML: m.Message},
		}
	}

	return xml.Marshal(pullMessagesResponse{
		XMLNS:               EventsNamespace,
		XMLNSWsa:            WSAddressingNamespace,
		XMLNSTt:             SchemaNamespace,
		CurrentTime:         formatXSDDateTime(result.CurrentTime),
		TerminationTime:     formatXSDDateTime(result.TerminationTime),
		NotificationMessage: msgs,
	})
}

func (h *SubscriptionManagerHandler) handleSetSynchronizationPoint(
	ctx context.Context, subscriptionID string,
) ([]byte, error) {
	if err := h.provider.SetSynchronizationPoint(ctx, subscriptionID); err != nil {
		return nil, err
	}
	return xml.Marshal(setSynchronizationPointResponse{XMLNS: EventsNamespace})
}

func (h *SubscriptionManagerHandler) handleRenew(
	ctx context.Context, subscriptionID string, payload []byte,
) ([]byte, error) {
	var req struct {
		TerminationTime string `xml:"TerminationTime"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("eventsvc: decode Renew: %w", err))
	}

	result, err := h.provider.Renew(ctx, subscriptionID, RenewParams{TerminationTime: req.TerminationTime})
	if err != nil {
		return nil, err
	}

	return xml.Marshal(renewResponse{
		XMLNS:           WSNBaseNotificationNS,
		TerminationTime: formatXSDDateTime(result.TerminationTime),
		CurrentTime:     formatXSDDateTime(result.CurrentTime),
	})
}

func (h *SubscriptionManagerHandler) handleUnsubscribe(
	ctx context.Context, subscriptionID string,
) ([]byte, error) {
	if err := h.provider.Unsubscribe(ctx, subscriptionID); err != nil {
		return nil, err
	}
	return xml.Marshal(unsubscribeResponse{XMLNS: WSNBaseNotificationNS})
}

// parseSubscriptionManagerOperation extracts the SOAP body inner XML and
// operation name, accepting either EventsNamespace or WSNBaseNotificationNS.
func parseSubscriptionManagerOperation(data []byte) (payload []byte, operation string, err error) {
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
			ns := elem.Name.Space
			if ns != EventsNamespace && ns != WSNBaseNotificationNS {
				return nil, "", fmt.Errorf("%w: %s", errInvalidNamespace, ns)
			}
			return env.Body.Inner, elem.Name.Local, nil
		case xml.EndElement:
			if inBody && elem.Name.Local == bodyLocalName && elem.Name.Space == soapNamespace {
				return nil, "", errEmptySOAPBody
			}
		}
	}
}
