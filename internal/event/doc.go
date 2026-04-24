// Package event implements the EventBroker, which is the concrete
// eventsvc.Provider for the ONVIF simulator.
//
// # Architecture
//
// EventBroker manages pull-point subscriptions and an in-memory per-subscription
// event queue. It satisfies the eventsvc.Provider interface so it can be plugged
// directly into eventsvc.EventServiceHandler and
// eventsvc.SubscriptionManagerHandler without any adapters.
//
// GUI/TUI front-ends (and future test harnesses) call Broker.Publish to inject
// events into every active subscription that has a matching topic filter. The
// handler layer then drains queues on PullMessages calls.
//
// # Topic support
//
// The set of advertised topics is driven by the config.EventsConfig.Topics list.
// Publish accepts any topic string; whether it appears in GetEventProperties is
// controlled by the Enabled flag in the config.
//
// # Subscription lifecycle
//
// CreatePullPointSubscription allocates a new subscription with a UUID token and
// a configurable termination time. Expired subscriptions are cleaned up lazily on
// the next PullMessages or Renew call for that token, and proactively by a
// background goroutine started with Broker.Start.
package event
