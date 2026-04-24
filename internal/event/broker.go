package event

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
)

var errMaxPullPointsReached = errors.New("event: maximum pull points reached")

const (
	// DefaultSubscriptionTimeout is used when config omits subscription_timeout.
	DefaultSubscriptionTimeout = time.Hour

	// DefaultMaxPullPoints is used when config omits max_pull_points.
	DefaultMaxPullPoints = 10

	// defaultQueueDepth is the per-subscription event queue capacity.
	// Oldest events are dropped when the queue is full.
	defaultQueueDepth = 256
)

// TopicConfig mirrors config.TopicConfig to avoid a circular import.
type TopicConfig struct {
	Name    string
	Enabled bool
}

// BrokerConfig holds the runtime configuration for the EventBroker.
// It maps directly from config.EventsConfig.
type BrokerConfig struct {
	MaxPullPoints       int
	SubscriptionTimeout time.Duration
	Topics              []TopicConfig
}

// Broker is the concrete eventsvc.Provider. It is safe for concurrent use.
//
// Zero value is not usable — create instances with New.
type Broker struct {
	cfg BrokerConfig

	mu   sync.Mutex
	subs map[string]*subscription // keyed by subscriptionID

	// stopCh is closed by Stop to terminate the background reaper.
	stopCh chan struct{}
}

// New creates a new Broker with the given configuration.
func New(cfg BrokerConfig) *Broker {
	if cfg.MaxPullPoints <= 0 {
		cfg.MaxPullPoints = DefaultMaxPullPoints
	}
	if cfg.SubscriptionTimeout <= 0 {
		cfg.SubscriptionTimeout = DefaultSubscriptionTimeout
	}
	return &Broker{
		cfg:    cfg,
		subs:   make(map[string]*subscription),
		stopCh: make(chan struct{}),
	}
}

// Start launches the background subscription-reaper goroutine. Call Stop to
// terminate it. Start is idempotent per Broker instance.
func (b *Broker) Start() {
	go b.reapLoop()
}

// Stop signals the background goroutine started by Start to exit.
func (b *Broker) Stop() {
	close(b.stopCh)
}

// Publish injects an event into all active subscriptions that match topic.
// The message argument is a raw tt:Message XML fragment, e.g.:
//
//	`<tt:Message UtcTime="2026-01-01T00:00:00Z" PropertyType="Changed">
//	   <tt:Source><tt:SimpleItem Name="VideoSourceConfigurationToken" Value="vs0"/></tt:Source>
//	   <tt:Data><tt:SimpleItem Name="State" Value="true"/></tt:Data>
//	 </tt:Message>`
//
// Publish is safe for concurrent use and returns immediately.
func (b *Broker) Publish(topic, message string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	for id, sub := range b.subs {
		if now.After(sub.terminationTime) {
			continue
		}
		if !sub.matchesTopic(topic) {
			continue
		}
		sub.enqueue(eventsvc.NotificationMessage{
			SubscriptionReference: sub.address,
			Topic:                 topic,
			Message:               message,
		})
		_ = id
	}
}

// UpdateConfig replaces the broker's runtime configuration. Active
// subscriptions are not affected; the new config takes effect for future
// CreatePullPointSubscription calls and for GetEventProperties.
func (b *Broker) UpdateConfig(cfg BrokerConfig) {
	if cfg.MaxPullPoints <= 0 {
		cfg.MaxPullPoints = DefaultMaxPullPoints
	}
	if cfg.SubscriptionTimeout <= 0 {
		cfg.SubscriptionTimeout = DefaultSubscriptionTimeout
	}
	b.mu.Lock()
	b.cfg = cfg
	b.mu.Unlock()
}

// ---------- eventsvc.Provider implementation ------------------------------------

// EventServiceCapabilities implements eventsvc.Provider.
func (b *Broker) EventServiceCapabilities(_ context.Context) (eventsvc.ServiceCapabilities, error) {
	b.mu.Lock()
	maxPP := b.cfg.MaxPullPoints
	b.mu.Unlock()
	return eventsvc.ServiceCapabilities{
		WSPullPointSupport: true,
		MaxPullPoints:      maxPP,
	}, nil
}

// EventProperties implements eventsvc.Provider.
func (b *Broker) EventProperties(_ context.Context) (eventsvc.EventProperties, error) {
	b.mu.Lock()
	topics := append([]TopicConfig(nil), b.cfg.Topics...)
	b.mu.Unlock()

	topicXML := buildTopicSetXML(topics)
	return eventsvc.EventProperties{
		FixedTopicSet: true,
		TopicSet:      topicXML,
	}, nil
}

// CreatePullPointSubscription implements eventsvc.Provider.
func (b *Broker) CreatePullPointSubscription(
	_ context.Context, params eventsvc.CreatePullPointSubscriptionParams,
) (eventsvc.SubscriptionInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.subs) >= b.cfg.MaxPullPoints {
		return eventsvc.SubscriptionInfo{}, fmt.Errorf("%w (%d)", errMaxPullPointsReached, b.cfg.MaxPullPoints)
	}

	timeout := b.cfg.SubscriptionTimeout
	if d, err := parseISO8601Duration(params.InitialTerminationTime); err == nil && d > 0 {
		timeout = d
	}

	id := newSubscriptionID()
	now := time.Now()
	sub := &subscription{
		id:              id,
		filter:          params.Filter,
		terminationTime: now.Add(timeout),
		queue:           make([]eventsvc.NotificationMessage, 0, defaultQueueDepth),
	}
	b.subs[id] = sub

	return eventsvc.SubscriptionInfo{
		SubscriptionID:  id,
		CurrentTime:     now,
		TerminationTime: sub.terminationTime,
	}, nil
}

// PullMessages implements eventsvc.Provider.
func (b *Broker) PullMessages(
	_ context.Context, subscriptionID string, params eventsvc.PullMessagesParams,
) (eventsvc.PullMessagesResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub, err := b.requireSub(subscriptionID)
	if err != nil {
		return eventsvc.PullMessagesResult{}, err
	}

	msgs := sub.drain(params.MessageLimit)
	return eventsvc.PullMessagesResult{
		CurrentTime:     time.Now(),
		TerminationTime: sub.terminationTime,
		Messages:        msgs,
	}, nil
}

// SetSynchronizationPoint implements eventsvc.Provider.
// For a pull-point subscription, this is a no-op acknowledgement.
func (b *Broker) SetSynchronizationPoint(_ context.Context, subscriptionID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, err := b.requireSub(subscriptionID)
	return err
}

// Renew implements eventsvc.Provider.
func (b *Broker) Renew(
	_ context.Context, subscriptionID string, params eventsvc.RenewParams,
) (eventsvc.RenewResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub, err := b.requireSub(subscriptionID)
	if err != nil {
		return eventsvc.RenewResult{}, err
	}

	timeout := b.cfg.SubscriptionTimeout
	if d, parseErr := parseISO8601Duration(params.TerminationTime); parseErr == nil && d > 0 {
		timeout = d
	}

	now := time.Now()
	sub.terminationTime = now.Add(timeout)
	return eventsvc.RenewResult{
		CurrentTime:     now,
		TerminationTime: sub.terminationTime,
	}, nil
}

// Unsubscribe implements eventsvc.Provider.
func (b *Broker) Unsubscribe(_ context.Context, subscriptionID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.subs[subscriptionID]; !ok {
		return fmt.Errorf("%w: %s", eventsvc.ErrSubscriptionNotFound, subscriptionID)
	}
	delete(b.subs, subscriptionID)
	return nil
}

// ---------- internal helpers ----------------------------------------------------

// requireSub looks up a subscription, checking expiry. Must be called with
// b.mu held.
func (b *Broker) requireSub(id string) (*subscription, error) {
	sub, ok := b.subs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", eventsvc.ErrSubscriptionNotFound, id)
	}
	if time.Now().After(sub.terminationTime) {
		delete(b.subs, id)
		return nil, fmt.Errorf("%w: %s (expired)", eventsvc.ErrSubscriptionNotFound, id)
	}
	return sub, nil
}

// reapLoop periodically removes expired subscriptions.
func (b *Broker) reapLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			b.reap()
		}
	}
}

func (b *Broker) reap() {
	now := time.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, sub := range b.subs {
		if now.After(sub.terminationTime) {
			delete(b.subs, id)
		}
	}
}

// buildTopicSetXML produces the inner XML of a tt:TopicSet element listing all
// enabled topics. The caller emits this verbatim inside GetEventPropertiesResponse.
func buildTopicSetXML(topics []TopicConfig) string {
	if len(topics) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(`<tns1:MessageContentFilter xmlns:tns1="http://www.onvif.org/onvif/ver10/topics">`)
	for _, t := range topics {
		if !t.Enabled {
			continue
		}
		// Emit one wstop:topic element per enabled topic.
		sb.WriteString(`<wstop:topic xmlns:wstop="http://docs.oasis-open.org/wsn/t-1" Final="true">`)
		sb.WriteString(xmlEscape(t.Name))
		sb.WriteString(`</wstop:topic>`)
	}
	sb.WriteString(`</tns1:MessageContentFilter>`)
	return sb.String()
}

func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}
