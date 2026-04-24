package client

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

// TestBackoffProgression verifies the exponential backoff sequence:
// 1s, 2s, 4s, 8s, 16s, 30s, 30s...
func TestBackoffProgression(t *testing.T) {
	expected := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second,
		30 * time.Second,
		30 * time.Second,
	}

	seq := BackoffSequence(len(expected))
	for i, want := range expected {
		if seq[i] != want {
			t.Errorf("backoff[%d] = %v, want %v", i, seq[i], want)
		}
	}
}

// TestConnectWithRetrySuccess verifies that ConnectWithRetry connects
// and calls onConnect on first attempt when the server is available.
func TestConnectWithRetrySuccess(t *testing.T) {
	addr, cleanup := startTestSSHServer(t, testToken)
	defer cleanup()

	c := NewConnector(ClientConfig{
		ServerAddr: addr,
		Token:      testToken,
		Insecure:   true,
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var callbackCalled atomic.Int32

	// Run ConnectWithRetry in a goroutine since it blocks.
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.ConnectWithRetry(ctx, func() error {
			callbackCalled.Add(1)
			// Cancel after callback runs to break the loop.
			cancel()
			return nil
		})
	}()

	err := <-errCh
	// Should return context.Canceled since we cancelled.
	if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		t.Fatalf("ConnectWithRetry() unexpected error: %v", err)
	}

	if callbackCalled.Load() != 1 {
		t.Errorf("onConnect called %d times, want 1", callbackCalled.Load())
	}
}

// TestConnectWithRetryContextCancel verifies that cancelling the context
// stops the retry loop immediately.
func TestConnectWithRetryContextCancel(t *testing.T) {
	// Point at a port that will never accept.
	c := NewConnector(ClientConfig{
		ServerAddr: "127.0.0.1:1",
		Token:      testToken,
		Insecure:   true,
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.ConnectWithRetry(ctx, nil)
	}()

	// Let it attempt once, then cancel.
	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ConnectWithRetry did not exit after context cancel")
	}
}

// TestConnectWithRetryReconnect verifies that the client reconnects
// after the server goes away and comes back. This test uses a different
// approach: it closes the server to trigger disconnect detection,
// then starts a new server on the same address.
func TestConnectWithRetryReconnect(t *testing.T) {
	// Start the server.
	addr, cleanup := startTestSSHServer(t, testToken)

	c := NewConnector(ClientConfig{
		ServerAddr: addr,
		Token:      testToken,
		Insecure:   true,
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var connectCount atomic.Int32
	firstConnected := make(chan struct{})

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.ConnectWithRetry(ctx, func() error {
			n := connectCount.Add(1)
			t.Logf("onConnect called, count=%d", n)
			if n == 1 {
				close(firstConnected)
			}
			if n >= 2 {
				// Second connect: we've reconnected, exit.
				cancel()
			}
			return nil
		})
	}()

	// Wait for first connect to complete.
	select {
	case <-firstConnected:
		t.Log("first connection established")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first connection")
	}

	// Kill the server to force a disconnect.
	t.Log("killing server to trigger disconnect")
	cleanup()

	// Give the client time to detect the disconnect.
	time.Sleep(500 * time.Millisecond)

	// Start a new server on the SAME address.
	t.Logf("starting new server on %s", addr)
	_, newCleanup := startTestSSHServerOnAddr(t, testToken, addr)
	defer newCleanup()

	// Wait for the retry loop to exit.
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			t.Fatalf("ConnectWithRetry() unexpected error: %v", err)
		}
	case <-time.After(30 * time.Second):
		cancel()
		t.Fatal("ConnectWithRetry did not reconnect in time")
	}

	finalCount := connectCount.Load()
	t.Logf("total connects: %d", finalCount)
	if finalCount < 2 {
		t.Errorf("expected at least 2 connects (reconnect), got %d", finalCount)
	}
}

// TestConnectWithRetryLifecycleCallbacksReportFailuresAndRetries verifies that
// clients can surface connection failure and retry lifecycle events.
func TestConnectWithRetryLifecycleCallbacksReportFailuresAndRetries(t *testing.T) {
	c := NewConnector(ClientConfig{
		ServerAddr: "127.0.0.1:1",
		Token:      testToken,
		Insecure:   true,
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	failed := make(chan time.Duration, 1)
	retrying := make(chan time.Duration, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.ConnectWithRetryHooks(ctx, ConnectHooks{
			OnConnectionFailed: func(err error, attempt int, backoff time.Duration) {
				if attempt != 1 {
					t.Errorf("attempt = %d, want 1", attempt)
				}
				failed <- backoff
			},
			OnRetrying: func(backoff time.Duration) {
				retrying <- backoff
			},
		})
	}()

	select {
	case got := <-failed:
		if got != InitialBackoff {
			t.Fatalf("OnConnectionFailed backoff = %v, want %v", got, InitialBackoff)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for OnConnectionFailed")
	}

	select {
	case got := <-retrying:
		if got != InitialBackoff {
			t.Fatalf("OnRetrying backoff = %v, want %v", got, InitialBackoff)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for OnRetrying")
	}

	cancel()
	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ConnectWithRetryHooks did not exit after cancel")
	}
}

func TestConnectWithRetryMonitorsConnectionWhenOnConnectedBlocks(t *testing.T) {
	addr, cleanup := startTestSSHServer(t, testToken)

	c := NewConnector(ClientConfig{
		ServerAddr: addr,
		Token:      testToken,
		Insecure:   true,
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	connected := make(chan struct{})
	disconnected := make(chan struct{})
	releaseOnConnected := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.ConnectWithRetryHooks(ctx, ConnectHooks{
			OnConnected: func(connCtx context.Context) error {
				close(connected)
				<-releaseOnConnected
				return nil
			},
			OnDisconnected: func(err error, backoff time.Duration) {
				close(disconnected)
				cancel()
			},
		})
	}()

	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for initial connection")
	}

	cleanup()

	select {
	case <-disconnected:
	case <-time.After(10 * time.Second):
		close(releaseOnConnected)
		t.Fatal("disconnect was not observed while OnConnected was blocked")
	}

	close(releaseOnConnected)
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ConnectWithRetryHooks did not exit")
	}
}

// TestConnectWithRetryCallbackError verifies that a failing onConnect
// callback causes a retry.
func TestConnectWithRetryCallbackError(t *testing.T) {
	addr, cleanup := startTestSSHServer(t, testToken)
	defer cleanup()

	c := NewConnector(ClientConfig{
		ServerAddr: addr,
		Token:      testToken,
		Insecure:   true,
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var callCount atomic.Int32

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.ConnectWithRetry(ctx, func() error {
			n := callCount.Add(1)
			if n == 1 {
				return fmt.Errorf("simulated callback failure")
			}
			// Second call succeeds, then cancel.
			cancel()
			return nil
		})
	}()

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("timed out waiting for retry after callback failure")
	}

	if callCount.Load() < 2 {
		t.Errorf("expected at least 2 callback calls, got %d", callCount.Load())
	}
}
