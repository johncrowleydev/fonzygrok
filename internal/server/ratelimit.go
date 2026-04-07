package server

import (
	"sync"

	"golang.org/x/time/rate"
)

// RateLimiter provides per-tunnel token bucket rate limiting.
// Each tunnel gets its own rate.Limiter instance, auto-created on
// first request. Default rate and burst are configurable.
type RateLimiter struct {
	limiters sync.Map // tunnelID → *rate.Limiter
	rate     float64  // default requests per second
	burst    int      // default burst size
}

// NewRateLimiter creates a RateLimiter with the given defaults.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	return &RateLimiter{
		rate:  rps,
		burst: burst,
	}
}

// Allow checks whether a request to the given tunnel is permitted.
// Auto-creates a limiter on first call for a new tunnelID.
func (rl *RateLimiter) Allow(tunnelID string) bool {
	lim := rl.getOrCreate(tunnelID)
	return lim.Allow()
}

// SetLimit sets a custom rate limit for a specific tunnel, replacing the
// default. Existing tokens in the bucket are preserved.
func (rl *RateLimiter) SetLimit(tunnelID string, rps float64, burst int) {
	lim := rl.getOrCreate(tunnelID)
	lim.SetLimit(rate.Limit(rps))
	lim.SetBurst(burst)
}

// GetLimit returns the current rate limit (rps, burst) for a tunnel.
// Returns the defaults if no custom limit is set.
func (rl *RateLimiter) GetLimit(tunnelID string) (rps float64, burst int) {
	v, ok := rl.limiters.Load(tunnelID)
	if !ok {
		return rl.rate, rl.burst
	}
	lim := v.(*rate.Limiter)
	return float64(lim.Limit()), lim.Burst()
}

// Remove deletes the limiter for a tunnel, freeing memory.
// Called when the tunnel is deregistered.
func (rl *RateLimiter) Remove(tunnelID string) {
	rl.limiters.Delete(tunnelID)
}

// getOrCreate returns the existing limiter or creates a new one with defaults.
func (rl *RateLimiter) getOrCreate(tunnelID string) *rate.Limiter {
	v, loaded := rl.limiters.Load(tunnelID)
	if loaded {
		return v.(*rate.Limiter)
	}

	lim := rate.NewLimiter(rate.Limit(rl.rate), rl.burst)
	actual, _ := rl.limiters.LoadOrStore(tunnelID, lim)
	return actual.(*rate.Limiter)
}
