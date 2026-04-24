package eventsvc

import (
	"context"
	"errors"
	"net/http"
	"time"
)

const (
	// EventServicePath is the ONVIF event service endpoint path advertised
	// via Device.GetCapabilities.Events.XAddr.
	EventServicePath = "/onvif/event_service"

	// SubscriptionManagerPath is the base path for the subscription manager
	// endpoint. The full address returned in CreatePullPointSubscription
	// appends ?id=<subscriptionID>.
	SubscriptionManagerPath = "/onvif/subscription_manager"

	// EventsNamespace is the ONVIF events WSDL namespace.
	EventsNamespace = "http://www.onvif.org/ver10/events/wsdl"

	// WSNBaseNotificationNS is the WS-BaseNotification namespace used by
	// Renew and Unsubscribe operations.
	WSNBaseNotificationNS = "http://docs.oasis-open.org/wsn/b-2"

	// WSAddressingNamespace is the WS-Addressing namespace used in EPR elements.
	WSAddressingNamespace = "http://www.w3.org/2005/08/addressing"

	// SchemaNamespace is the ONVIF schema namespace used by tt: elements.
	SchemaNamespace = "http://www.onvif.org/ver10/schema"
)

var (
	errProviderRequired = errors.New("eventsvc: provider is required")
	errUnsupportedOp    = errors.New("eventsvc: unsupported operation")
	errEmptySOAPBody    = errors.New("eventsvc: empty soap body")
	errDecodePayload    = errors.New("eventsvc: malformed request payload")
	errInvalidNamespace = errors.New("eventsvc: unexpected operation namespace")

	// ErrSubscriptionNotFound is returned by Provider when the subscription
	// token does not exist or has expired. The handler maps it to HTTP 400 +
	// SOAP fault code Sender.
	ErrSubscriptionNotFound = errors.New("eventsvc: subscription not found")

	// ErrInvalidArgs is returned by Provider when request argument values are
	// invalid (e.g. negative MessageLimit). Maps to HTTP 400 + Sender.
	ErrInvalidArgs = errors.New("eventsvc: invalid argument value")
)

// ServiceCapabilities is the GetServiceCapabilities payload for eventsvc.
type ServiceCapabilities struct {
	WSSubscriptionPolicySupport                   bool
	WSPullPointSupport                            bool
	WSPausableSubscriptionManagerInterfaceSupport bool
	MaxNotificationProducers                      int
	MaxPullPoints                                 int
	PersistentNotificationStorage                 bool
}

// EventProperties is the GetEventProperties payload.
type EventProperties struct {
	// TopicNamespaceLocation lists URLs to topic namespace schema documents.
	TopicNamespaceLocation []string
	// FixedTopicSet is true when the topic set is static (always true for this simulator).
	FixedTopicSet bool
	// TopicSet is a raw XML fragment for the tns1: topic tree, emitted verbatim.
	TopicSet string
}

// CreatePullPointSubscriptionParams carries the request parameters for
// CreatePullPointSubscription.
type CreatePullPointSubscriptionParams struct {
	// Filter is the optional WS-BaseNotification TopicExpression filter.
	// The simulator accepts any value and emits all configured topics.
	Filter string
	// InitialTerminationTime is the requested ISO 8601 duration or absolute time.
	InitialTerminationTime string
}

// SubscriptionInfo is returned by CreatePullPointSubscription.
type SubscriptionInfo struct {
	// SubscriptionID is the opaque token for this pull-point subscription.
	SubscriptionID  string
	TerminationTime time.Time
	CurrentTime     time.Time
}

// PullMessagesParams carries the request parameters for PullMessages.
type PullMessagesParams struct {
	// Timeout is the ISO 8601 duration the server waits before responding
	// with an empty list. The simulator may respond immediately.
	Timeout string
	// MessageLimit caps the number of NotificationMessage items returned.
	MessageLimit int
}

// NotificationMessage carries one event notification.
type NotificationMessage struct {
	// SubscriptionReference is the EPR address of the subscription.
	SubscriptionReference string
	// Topic is the tns1: event topic expression string.
	Topic string
	// Message is a raw tt:Message XML fragment.
	Message string
}

// PullMessagesResult is the response payload for PullMessages.
type PullMessagesResult struct {
	CurrentTime     time.Time
	TerminationTime time.Time
	Messages        []NotificationMessage
}

// RenewParams carries the request parameters for Renew.
type RenewParams struct {
	TerminationTime string // ISO 8601 duration or absolute datetime
}

// RenewResult is the response payload for Renew.
type RenewResult struct {
	TerminationTime time.Time
	CurrentTime     time.Time
}

// Provider supplies operation data to both the EventServiceHandler and the
// SubscriptionManagerHandler. The concrete implementation is event.Broker.
// Implementations must be safe for concurrent use.
//
// All methods that accept a subscriptionID must return ErrSubscriptionNotFound
// (wrapped) when the token is unknown or expired — the handler maps this to an
// HTTP 400 + wsrf:ResourceUnknown SOAP fault.
type Provider interface {
	// EventServiceCapabilities returns the static capabilities advertised by
	// GetServiceCapabilities.
	EventServiceCapabilities(ctx context.Context) (ServiceCapabilities, error)

	// EventProperties returns the topic set and related metadata advertised by
	// GetEventProperties. TopicSet is a raw XML fragment emitted verbatim.
	EventProperties(ctx context.Context) (EventProperties, error)

	// CreatePullPointSubscription allocates a new pull-point subscription and
	// returns its ID, current time, and termination time.
	CreatePullPointSubscription(ctx context.Context, params CreatePullPointSubscriptionParams) (SubscriptionInfo, error)

	// PullMessages drains up to params.MessageLimit events from the
	// subscription queue.  It must not block; return an empty slice when the
	// queue is empty.
	PullMessages(ctx context.Context, subscriptionID string, params PullMessagesParams) (PullMessagesResult, error)

	// SetSynchronizationPoint is called by a client to force the device to
	// re-emit "Initialized" property events. The broker may respond immediately
	// with an acknowledgement and push events asynchronously via Publish.
	SetSynchronizationPoint(ctx context.Context, subscriptionID string) error

	// Renew extends the termination time of an existing subscription.
	Renew(ctx context.Context, subscriptionID string, params RenewParams) (RenewResult, error)

	// Unsubscribe cancels a subscription immediately.
	Unsubscribe(ctx context.Context, subscriptionID string) error
}

// AuthHook authorizes one request before the operation is executed.
type AuthHook interface {
	Authorize(ctx context.Context, operation string, r *http.Request) error
}

// AuthFunc adapts a function into an AuthHook.
type AuthFunc func(ctx context.Context, operation string, r *http.Request) error

// Authorize executes the auth function.
func (f AuthFunc) Authorize(ctx context.Context, operation string, r *http.Request) error {
	return f(ctx, operation, r)
}
