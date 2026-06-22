package web

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	rl := newRateLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		allowed, retry := rl.allow("key1")
		if !allowed {
			t.Errorf("expected attempt %d to be allowed, retry=%v", i+1, retry)
		}
		if retry != 0 {
			t.Errorf("expected retry=0 on allowed attempt, got %v", retry)
		}
	}
}

func TestRateLimiter_RejectsOverLimit(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)

	rl.allow("key1")
	rl.allow("key1")

	allowed, retry := rl.allow("key1")
	if allowed {
		t.Error("expected third attempt to be rejected")
	}
	if retry <= 0 {
		t.Errorf("expected positive retry duration, got %v", retry)
	}
}

func TestRateLimiter_DifferentKeysIndependent(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)

	rl.allow("key1")
	rl.allow("key1")

	// key2 should still be allowed even though key1 is exhausted.
	allowed, _ := rl.allow("key2")
	if !allowed {
		t.Error("expected key2 to be independent of key1")
	}
}

func TestRateLimiter_ResetClearsCounter(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)

	rl.allow("key1")
	rl.allow("key1")

	// Should be at limit.
	if allowed, _ := rl.allow("key1"); allowed {
		t.Error("expected key1 to be at limit before reset")
	}

	// Reset and try again.
	rl.reset("key1")
	allowed, _ := rl.allow("key1")
	if !allowed {
		t.Error("expected key1 to be allowed after reset")
	}
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	rl := newRateLimiter(2, 50*time.Millisecond)

	rl.allow("key1")
	rl.allow("key1")
	if allowed, _ := rl.allow("key1"); allowed {
		t.Error("expected key1 to be at limit")
	}

	time.Sleep(60 * time.Millisecond)

	allowed, _ := rl.allow("key1")
	if !allowed {
		t.Error("expected key1 to be allowed after window expiry")
	}
}

func TestRateLimitKey_ExtractsIP(t *testing.T) {
	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "192.168.1.100:54321"
	key := rateLimitKey(req, "admin")
	if key != "192.168.1.100|admin" {
		t.Errorf("expected '192.168.1.100|admin', got %q", key)
	}
}

func TestRateLimitKey_NoPort(t *testing.T) {
	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "10.0.0.1"
	key := rateLimitKey(req, "user")
	// When SplitHostPort fails, the raw RemoteAddr is used.
	if key != "10.0.0.1|user" {
		t.Errorf("expected '10.0.0.1|user', got %q", key)
	}
}
