package auth

import (
	"sync"
	"time"
)

// NonceCache tracks recently seen nonces so the same (nonce,created) pair
// cannot be replayed within the WS-UsernameToken clock-skew window.
type NonceCache struct {
	mu   sync.Mutex
	ttl  time.Duration
	seen map[string]time.Time
	// Size bound: any entry older than ttl is pruned on each Add. An
	// additional safety cap stops unbounded growth if the clock freezes.
	maxSize int
}

// NewNonceCache returns a cache with the given TTL. Entries older than ttl
// are pruned opportunistically on Add.
func NewNonceCache(ttl time.Duration) *NonceCache {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &NonceCache{
		ttl:     ttl,
		seen:    make(map[string]time.Time),
		maxSize: 4096,
	}
}

// Add records key as seen at now. Returns false if key was already seen
// within the TTL window (replay), true if this is a first sighting.
func (c *NonceCache) Add(key string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(now)
	if _, ok := c.seen[key]; ok {
		return false
	}
	if len(c.seen) >= c.maxSize {
		// Hard cap reached — drop the oldest entry to make room. O(n) but
		// only triggered under pathological conditions.
		var oldestKey string
		var oldestAt time.Time
		for k, v := range c.seen {
			if oldestKey == "" || v.Before(oldestAt) {
				oldestKey = k
				oldestAt = v
			}
		}
		delete(c.seen, oldestKey)
	}
	c.seen[key] = now
	return true
}

func (c *NonceCache) pruneLocked(now time.Time) {
	cutoff := now.Add(-c.ttl)
	for k, v := range c.seen {
		if v.Before(cutoff) {
			delete(c.seen, k)
		}
	}
}
