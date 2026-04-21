package discovery

import "sync"

// AppSequence tracks d:AppSequence InstanceId and a monotonic MessageNumber (WS-Discovery Appendix I).
type AppSequence struct {
	InstanceID uint32
	mu         sync.Mutex
	next       uint32
}

// NewAppSequence returns a counter starting at 0; the first Next returns 1.
func NewAppSequence(instanceID uint32) *AppSequence {
	return &AppSequence{InstanceID: instanceID}
}

// NextMessageNumber returns the next MessageNumber (>= 1).
func (a *AppSequence) NextMessageNumber() uint32 {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.next++
	return a.next
}
