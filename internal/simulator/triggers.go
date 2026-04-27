package simulator

import (
	"fmt"
	"time"
)

// Motion publishes tns1:VideoSource/MotionAlarm for sourceToken. No-op when
// the topic is disabled in EventsConfig.
func (s *Simulator) Motion(sourceToken string, state bool) {
	if !s.topicEnabled("tns1:VideoSource/MotionAlarm") {
		return
	}
	s.broker.MotionAlarm(sourceToken, state)
	s.recordEvent("tns1:VideoSource/MotionAlarm", sourceToken, fmt.Sprintf("state=%t", state))
}

// ImageTooBlurry publishes tns1:VideoSource/ImageTooBlurry for sourceToken.
func (s *Simulator) ImageTooBlurry(sourceToken string, state bool) {
	if !s.topicEnabled("tns1:VideoSource/ImageTooBlurry") {
		return
	}
	s.broker.ImageTooBlurry(sourceToken, state)
	s.recordEvent("tns1:VideoSource/ImageTooBlurry", sourceToken, fmt.Sprintf("state=%t", state))
}

// ImageTooDark publishes tns1:VideoSource/ImageTooDark for sourceToken.
func (s *Simulator) ImageTooDark(sourceToken string, state bool) {
	if !s.topicEnabled("tns1:VideoSource/ImageTooDark") {
		return
	}
	s.broker.ImageTooDark(sourceToken, state)
	s.recordEvent("tns1:VideoSource/ImageTooDark", sourceToken, fmt.Sprintf("state=%t", state))
}

// ImageTooBright publishes tns1:VideoSource/ImageTooBright for sourceToken.
func (s *Simulator) ImageTooBright(sourceToken string, state bool) {
	if !s.topicEnabled("tns1:VideoSource/ImageTooBright") {
		return
	}
	s.broker.ImageTooBright(sourceToken, state)
	s.recordEvent("tns1:VideoSource/ImageTooBright", sourceToken, fmt.Sprintf("state=%t", state))
}

// DigitalInput publishes tns1:Device/Trigger/DigitalInput.
func (s *Simulator) DigitalInput(inputToken string, logicalState bool) {
	if !s.topicEnabled("tns1:Device/Trigger/DigitalInput") {
		return
	}
	s.broker.DigitalInput(inputToken, logicalState)
	s.recordEvent("tns1:Device/Trigger/DigitalInput", inputToken, fmt.Sprintf("state=%t", logicalState))
}

// SyncProperty re-emits "Initialized" for topic. Mirrors broker.SyncProperty.
func (s *Simulator) SyncProperty(topic, sourceItemName, sourceToken, dataItemName string, state bool) {
	if !s.topicEnabled(topic) {
		return
	}
	s.broker.SyncProperty(topic, sourceItemName, sourceToken, dataItemName, state)
	s.recordEvent(topic, sourceToken, fmt.Sprintf("sync state=%t", state))
}

// PublishRaw is the escape hatch for topics without a typed helper. No-op
// when the topic is not in EventsConfig.Topics or is disabled.
func (s *Simulator) PublishRaw(topic, message string) {
	if !s.topicEnabled(topic) {
		return
	}
	s.broker.Publish(topic, message)
	s.recordEvent(topic, "", "raw")
}

// topicEnabled reports whether topic is in the config and has Enabled=true.
func (s *Simulator) topicEnabled(topic string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.cfg.Events.Topics {
		if t.Name == topic {
			return t.Enabled
		}
	}
	return false
}

// recordEvent appends an EventRecord to the ring and fires OnEvent.
func (s *Simulator) recordEvent(topic, source, payload string) {
	rec := EventRecord{
		Time:    time.Now().UTC(),
		Topic:   topic,
		Source:  source,
		Payload: payload,
	}
	s.ring.push(rec)
	if s.opts.OnEvent != nil {
		s.opts.OnEvent(rec)
	}
}
