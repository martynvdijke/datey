package web

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter is an in-memory fixed-window rate limiter.
// It is intended for login throttling in a single-binary self-hosted app;
// state resets on restart and is not shared across instances.
//
// Spec: security-hardening — Login attempts are rate limited.
type rateLimiter struct {
	mu       sync.Mutex
	counters map[string]*rateCounter
	max      int
	window   time.Duration
}

type rateCounter struct {
	count   int
	resetAt time.Time
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		counters: make(map[string]*rateCounter),
		max:      max,
		window:   window,
	}
}

// allow checks whether a request for the given key is within the limit.
// Returns (allowed, retryAfter) where retryAfter is zero when allowed.
func (rl *rateLimiter) allow(key string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	c, exists := rl.counters[key]
	if !exists || now.After(c.resetAt) {
		rl.counters[key] = &rateCounter{count: 1, resetAt: now.Add(rl.window)}
		return true, 0
	}
	if c.count >= rl.max {
		return false, time.Until(c.resetAt)
	}
	c.count++
	return true, 0
}

// reset clears the counter for the given key (e.g. on successful login).
func (rl *rateLimiter) reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.counters, key)
}

// rateLimitKey builds a key from the request's remote IP and the given username.
func rateLimitKey(r *http.Request, username string) string {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return ip + "|" + username
}
