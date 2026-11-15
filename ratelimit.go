package pagetree

import (
	"sync"
	"time"
)

// Параметры in-app лимитов (per API-ключ).
const (
	defaultRPS   = 10
	defaultBurst = 30
	uploadPerMin = 2
	uploadBurst  = 2
)

type rlBucket struct {
	tokens float64
	last   time.Time
}

// rateLimiter — token bucket per key ID, два класса: обычный и upload.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rlBucket
	now     func() time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: make(map[string]*rlBucket), now: time.Now}
}

// allow reports whether a request of the given class may proceed for keyID.
func (rl *rateLimiter) allow(keyID, class string) bool {
	rate, burst := float64(defaultRPS), float64(defaultBurst)
	if class == "upload" {
		rate, burst = uploadPerMin/60.0, float64(uploadBurst)
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := rl.now()
	id := keyID + ":" + class
	b, ok := rl.buckets[id]
	if !ok {
		b = &rlBucket{tokens: burst, last: now}
		rl.buckets[id] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * rate
	if b.tokens > burst {
		b.tokens = burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
