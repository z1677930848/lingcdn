package server

import (
	"net/http"
	"sync"
	"time"
)

// ipRateLimiter is a lightweight, in-memory token-bucket rate limiter keyed by
// client IP. It is intended to throttle sensitive endpoints (login, register,
// password reset, email code requests) against brute-force attempts. The
// implementation is intentionally dependency-free; it is not a replacement for
// a distributed limiter but covers the single-control-plane deployment target.
type ipRateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*tokenBucket
	capacity  float64       // max tokens the bucket can hold
	refillPer time.Duration // time to add one token
	lastSweep time.Time
}

type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
	lastSeen   time.Time
}

// newIPRateLimiter creates a limiter that allows up to `capacity` requests in
// a burst and refills one token every `refillPer`. For example,
// `newIPRateLimiter(10, time.Minute)` permits 10 requests immediately and then
// 1 additional request per minute thereafter.
func newIPRateLimiter(capacity int, refillPer time.Duration) *ipRateLimiter {
	if capacity <= 0 {
		capacity = 1
	}
	if refillPer <= 0 {
		refillPer = time.Second
	}
	return &ipRateLimiter{
		buckets:   make(map[string]*tokenBucket),
		capacity:  float64(capacity),
		refillPer: refillPer,
		lastSweep: time.Now(),
	}
}

// allow reports whether the given key (typically a client IP) may proceed. It
// consumes a single token on success. Empty keys are always allowed so callers
// don't need to special-case missing IPs; in that case limiting is a no-op.
func (l *ipRateLimiter) allow(key string) bool {
	if key == "" {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	// Periodic sweep: drop buckets inactive longer than 10 minutes to bound memory.
	if now.Sub(l.lastSweep) > 5*time.Minute {
		cutoff := now.Add(-10 * time.Minute)
		for k, b := range l.buckets {
			if b.lastSeen.Before(cutoff) {
				delete(l.buckets, k)
			}
		}
		l.lastSweep = now
	}

	b, ok := l.buckets[key]
	if !ok {
		b = &tokenBucket{
			tokens:     l.capacity - 1, // consume one for this request
			lastRefill: now,
			lastSeen:   now,
		}
		l.buckets[key] = b
		return true
	}

	// Refill based on elapsed time.
	elapsed := now.Sub(b.lastRefill)
	if elapsed > 0 {
		add := float64(elapsed) / float64(l.refillPer)
		if add > 0 {
			b.tokens += add
			if b.tokens > l.capacity {
				b.tokens = l.capacity
			}
			b.lastRefill = now
		}
	}
	b.lastSeen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens -= 1
	return true
}

// rateLimit is a middleware convenience wrapper that applies the limiter to an
// HTTP handler using the request's client IP as the key.
func rateLimit(limiter *ipRateLimiter, errMsg string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if limiter != nil && !limiter.allow(getRequestIP(r)) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": errMsg})
			return
		}
		next(w, r)
	}
}
