package client

import (
	"context"
	"log/slog"
	"math"
	"time"
)

// Backoff constants per CON-001 §3.1 and SPR-003B T-010B.
const (
	// InitialBackoff is the first retry delay.
	InitialBackoff = 1 * time.Second
	// MaxBackoff is the cap on the exponential backoff.
	MaxBackoff = 30 * time.Second
	// BackoffMultiplier is the exponential growth factor.
	BackoffMultiplier = 2.0
)

// ConnectWithRetry attempts to connect to the server, retrying with
// exponential backoff (1s, 2s, 4s, 8s, 16s, 30s, 30s...) until
// the context is cancelled. On success the backoff resets.
//
// The onConnect callback is called after each successful connection.
// If onConnect returns an error, the connection is closed and retried.
//
// ConnectWithRetry blocks until the context is cancelled.
func (c *Connector) ConnectWithRetry(ctx context.Context, onConnect func() error) error {
	backoff := InitialBackoff
	attempt := 0

	for {
		attempt++

		// Try to connect.
		err := c.Connect(ctx)
		if err != nil {
			c.logger.Info("connection failed, will retry",
				slog.Int("attempt", attempt),
				slog.Int64("backoff_ms", backoff.Milliseconds()),
				slog.String("error", err.Error()),
			)

			if sleepErr := c.sleepWithContext(ctx, backoff); sleepErr != nil {
				return sleepErr // context cancelled
			}
			backoff = nextBackoff(backoff)
			continue
		}

		// Connection succeeded — reset backoff.
		backoff = InitialBackoff
		attempt = 0

		c.logger.Info("connected, running onConnect callback")

		// Run the post-connect callback (e.g., open control channel, register tunnels).
		if onConnect != nil {
			if cbErr := onConnect(); cbErr != nil {
				c.logger.Warn("onConnect callback failed",
					slog.String("error", cbErr.Error()),
				)
				c.Close()
				continue
			}
		}

		// Wait for disconnection.
		c.waitForDisconnect(ctx)

		// If context was cancelled, exit cleanly.
		if ctx.Err() != nil {
			c.Close()
			return ctx.Err()
		}

		// Disconnected — clean up and retry.
		c.logger.Info("disconnected, will reconnect",
			slog.Int64("backoff_ms", backoff.Milliseconds()),
		)
		c.Close()

		if sleepErr := c.sleepWithContext(ctx, backoff); sleepErr != nil {
			return sleepErr
		}
		backoff = nextBackoff(backoff)
	}
}

// nextBackoff calculates the next backoff duration, capped at MaxBackoff.
func nextBackoff(current time.Duration) time.Duration {
	next := time.Duration(math.Round(float64(current) * BackoffMultiplier))
	if next > MaxBackoff {
		return MaxBackoff
	}
	return next
}

// sleepWithContext sleeps for d or until ctx is cancelled, whichever comes first.
// Returns ctx.Err() if the context was cancelled.
func (c *Connector) sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// keepaliveInterval is how often the client sends a keepalive probe.
const keepaliveInterval = 5 * time.Second

// waitForDisconnect blocks until the SSH client connection is closed
// or the context is cancelled. It uses both passive Wait and active
// keepalive probes (SSH "keepalive@fonzygrok" requests) to reliably
// detect server-side connection drops.
func (c *Connector) waitForDisconnect(ctx context.Context) {
	c.mu.RLock()
	cl := c.client
	c.mu.RUnlock()

	if cl == nil {
		return
	}

	// Channel for passive disconnect detection.
	waitDone := make(chan struct{})
	go func() {
		cl.Conn.Wait()
		close(waitDone)
	}()

	// Active keepalive ticker.
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-waitDone:
			// SSH connection closed (detected passively).
			c.mu.Lock()
			c.connected = false
			c.client = nil
			c.mu.Unlock()
			return
		case <-ctx.Done():
			// Context cancelled.
			return
		case <-ticker.C:
			// Active keepalive probe.
			_, _, err := cl.Conn.SendRequest("keepalive@fonzygrok", true, nil)
			if err != nil {
				c.logger.Info("keepalive failed, connection lost",
					slog.String("error", err.Error()),
				)
				c.mu.Lock()
				c.connected = false
				c.client = nil
				c.mu.Unlock()
				return
			}
		}
	}
}

// BackoffSequence returns the backoff durations for n attempts.
// Exported for testing.
func BackoffSequence(n int) []time.Duration {
	seq := make([]time.Duration, n)
	b := InitialBackoff
	for i := 0; i < n; i++ {
		seq[i] = b
		b = nextBackoff(b)
	}
	return seq
}
