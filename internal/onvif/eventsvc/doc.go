// Package eventsvc implements the ONVIF Event Service and the
// WS-BaseNotification SubscriptionManager over SOAP/HTTP.
//
// Two handlers are registered separately:
//
//   - EventServiceHandler serves /onvif/event_service and supports
//     CreatePullPointSubscription, GetEventProperties, and
//     GetServiceCapabilities (namespace http://www.onvif.org/ver10/events/wsdl).
//
//   - SubscriptionManagerHandler serves /onvif/subscription_manager and
//     supports PullMessages and SetSynchronizationPoint (ONVIF events
//     namespace) as well as Renew and Unsubscribe (WS-BaseNotification
//     namespace http://docs.oasis-open.org/wsn/b-2).
//
// Both handlers share the same Provider and AuthHook interfaces.
//
// CreatePullPointSubscription embeds a WS-Addressing EndpointReference in
// its response whose Address is the configured subscription manager URL with
// a ?id= query parameter identifying the new subscription. Subsequent
// PullMessages, Renew, SetSynchronizationPoint, and Unsubscribe calls are
// directed to that address.
package eventsvc
