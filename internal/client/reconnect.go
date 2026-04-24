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

// ConnectHooks receives lifecycle notifications from ConnectWithRetryHooks.
// OnConnected is passed a per-connection context that is cancelled when that
// specific SSH connection is lost or when the parent context is cancelled.
// OnConnected should perform setup and return promptly; if it blocks,
// ConnectWithRetryHooks still monitors the connection and reconnects on loss.
type ConnectHooks struct {
	OnConnecting       func(attempt int)
	OnConnectionFailed func(err error, attempt int, backoff time.Duration)
	OnRetrying         func(backoff time.Duration)
	OnConnected        func(connCtx context.Context) error
	OnDisconnected     func(err error, backoff time.Duration)
}

// ConnectWithRetry attempts to connect to the server, retrying with
// exponential backoff (1s, 2s, 4s, 8s, 16s, 30s, 30s...) until
// the context is cancelled. On success the backoff resets.
//
// The onConnect callback is called after each successful connection.
// If onConnect returns an error, the connection is closed and retried.
//
// ConnectWithRetry blocks until the context is cancelled.
func (c *Connector) ConnectWithRetry(ctx context.Context, onConnect func() error) error {
	return c.ConnectWithRetryHooks(ctx, ConnectHooks{
		OnConnected: func(context.Context) error {
			if onConnect == nil {
				return nil
			}
			return onConnect()
		},
	})
}

// ConnectWithRetryHooks attempts to connect to the server and owns the full
// per-connection lifecycle: connect, setup callback, connection monitoring,
// disconnect notification, cleanup, and retry.
func (c *Connector) ConnectWithRetryHooks(ctx context.Context, hooks ConnectHooks) error {
	backoff := InitialBackoff
	attempt := 0

	for {
		attempt++
		if hooks.OnConnecting != nil {
			hooks.OnConnecting(attempt)
		}

		// Try to connect.
		err := c.Connect(ctx)
		if err != nil {
			c.logger.Info("connection failed, will retry",
				slog.Int("attempt", attempt),
				slog.Int64("backoff_ms", backoff.Milliseconds()),
				slog.String("error", err.Error()),
			)
			if hooks.OnConnectionFailed != nil {
				hooks.OnConnectionFailed(err, attempt, backoff)
			}
			if hooks.OnRetrying != nil {
				hooks.OnRetrying(backoff)
			}
			if sleepErr := c.sleepWithContext(ctx, backoff); sleepErr != nil {
				return sleepErr // context cancelled
			}
			backoff = nextBackoff(backoff)
			continue
		}

		// Connection succeeded — reset backoff for future disconnect retries.
		backoff = InitialBackoff
		attempt = 0

		c.logger.Info("connected, running onConnect callback")

		connCtx, cancelConn := context.WithCancel(ctx)
		setupErr := make(chan error, 1)
		if hooks.OnConnected != nil {
			go func() { setupErr <- hooks.OnConnected(connCtx) }()
		} else {
			setupErr <- nil
		}

		disconnected := make(chan struct{})
		go func() {
			c.waitForDisconnect(connCtx)
			close(disconnected)
		}()

		select {
		case cbErr := <-setupErr:
			if cbErr != nil {
				c.logger.Warn("onConnect callback failed",
					slog.String("error", cbErr.Error()),
				)
				cancelConn()
				c.Close()
				if hooks.OnConnectionFailed != nil {
					hooks.OnConnectionFailed(cbErr, 1, backoff)
				}
				if hooks.OnRetrying != nil {
					hooks.OnRetrying(backoff)
				}
				if sleepErr := c.sleepWithContext(ctx, backoff); sleepErr != nil {
					return sleepErr
				}
				backoff = nextBackoff(backoff)
				continue
			}
			// Setup completed. Continue monitoring until disconnect or cancellation.
			<-disconnected
		case <-disconnected:
			// Setup may still be running; cancel its per-connection context and let
			// lifecycle proceed so reconnect is not blocked by a long-lived callback.
		}

		cancelConn()

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
		if hooks.OnDisconnected != nil {
			hooks.OnDisconnected(nil, backoff)
		}
		if hooks.OnRetrying != nil {
			hooks.OnRetrying(backoff)
		}

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
