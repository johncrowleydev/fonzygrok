package server

import (
	"testing"
)

func TestRateLimiter_AllowUnderLimit(t *testing.T) {
	rl := NewRateLimiter(100, 200)

	// Under default limit — should all pass.
	for i := 0; i < 100; i++ {
		if !rl.Allow("tunnel-1") {
			t.Fatalf("request %d should be allowed", i)
		}
	}
}

func TestRateLimiter_BlockOverLimit(t *testing.T) {
	// Very low limit: 1 req/s, burst 2.
	rl := NewRateLimiter(1, 2)

	// First 2 should pass (burst).
	if !rl.Allow("tunnel-1") {
		t.Fatal("first request should be allowed (burst)")
	}
	if !rl.Allow("tunnel-1") {
		t.Fatal("second request should be allowed (burst)")
	}

	// Third should be blocked (burst exhausted, rate too low).
	if rl.Allow("tunnel-1") {
		t.Fatal("third request should be blocked")
	}
}

func TestRateLimiter_CustomPerTunnel(t *testing.T) {
	rl := NewRateLimiter(1000, 2000)

	// Set a very restrictive limit for one tunnel.
	rl.SetLimit("restricted", 1, 1)

	// Restricted tunnel: only 1 allowed.
	if !rl.Allow("restricted") {
		t.Fatal("first request to restricted should pass")
	}
	if rl.Allow("restricted") {
		t.Fatal("second request to restricted should be blocked")
	}

	// Other tunnel uses defaults — should pass freely.
	for i := 0; i < 100; i++ {
		if !rl.Allow("unrestricted") {
			t.Fatalf("request %d to unrestricted should pass", i)
		}
	}
}

func TestRateLimiter_GetLimit(t *testing.T) {
	rl := NewRateLimiter(100, 200)

	// Default.
	rps, burst := rl.GetLimit("tunnel-x")
	if rps != 100 || burst != 200 {
		t.Errorf("default: got rps=%v burst=%d, want 100/200", rps, burst)
	}

	// Custom.
	rl.SetLimit("tunnel-x", 50, 75)
	rps, burst = rl.GetLimit("tunnel-x")
	if rps != 50 || burst != 75 {
		t.Errorf("custom: got rps=%v burst=%d, want 50/75", rps, burst)
	}
}

func TestRateLimiter_RemoveCleanup(t *testing.T) {
	rl := NewRateLimiter(1, 1)

	// Exhaust the bucket.
	rl.Allow("tunnel-1")
	if rl.Allow("tunnel-1") {
		t.Fatal("should be blocked after burst")
	}

	// Remove and re-create — should get a fresh limiter.
	rl.Remove("tunnel-1")
	if !rl.Allow("tunnel-1") {
		t.Fatal("should be allowed after Remove (fresh limiter)")
	}
}

func TestRateLimiter_IndependentTunnels(t *testing.T) {
	rl := NewRateLimiter(1, 2)

	// Exhaust tunnel-1.
	rl.Allow("tunnel-1")
	rl.Allow("tunnel-1")
	if rl.Allow("tunnel-1") {
		t.Fatal("tunnel-1 should be blocked")
	}

	// tunnel-2 should be independent.
	if !rl.Allow("tunnel-2") {
		t.Fatal("tunnel-2 should be allowed (independent limiter)")
	}
}
