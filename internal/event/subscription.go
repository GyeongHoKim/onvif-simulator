package event

import (
	"strings"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
)

// subscription represents one active pull-point subscription.
// Fields must be accessed with Broker.mu held.
type subscription struct {
	id string
	// address is the full subscription manager URL including ?id=<id>.
	// Set by the handler layer; used as SubscriptionReference in notifications.
	address string
	// filter is the raw TopicExpression from CreatePullPointSubscription.
	// Empty means "accept all topics".
	filter          string
	terminationTime time.Time
	queue           []eventsvc.NotificationMessage
}

// enqueue appends msg to the subscription queue. When the queue is at capacity
// the oldest message is dropped to make room.
func (s *subscription) enqueue(msg eventsvc.NotificationMessage) {
	if len(s.queue) >= defaultQueueDepth {
		// Drop oldest.
		s.queue = s.queue[1:]
	}
	s.queue = append(s.queue, msg)
}

// drain removes and returns up to limit messages. limit <= 0 means all.
func (s *subscription) drain(limit int) []eventsvc.NotificationMessage {
	if len(s.queue) == 0 {
		return nil
	}
	n := len(s.queue)
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]eventsvc.NotificationMessage, n)
	copy(out, s.queue[:n])
	s.queue = s.queue[n:]
	return out
}

// matchesTopic returns true when the subscription should receive an event for
// the given topic. An empty filter matches everything. Otherwise the filter
// must be a prefix of (or equal to) the topic string.
func (s *subscription) matchesTopic(topic string) bool {
	if s.filter == "" {
		return true
	}
	return topic == s.filter || strings.HasPrefix(topic, s.filter+"/")
}
