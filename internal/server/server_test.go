package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServerStartStop(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping (requires PostgreSQL)")
	}

	tmpDir := t.TempDir()

	config := ServerConfig{
		DataDir:     tmpDir,
		DatabaseURL: dsn,
		Domain:      "test.com",
		SSH: SSHConfig{
			Addr:        "127.0.0.1:0",
			HostKeyPath: filepath.Join(tmpDir, "host_key"),
		},
		Edge: EdgeConfig{
			Addr: "127.0.0.1:0",
		},
		Admin: AdminConfig{
			Addr: "127.0.0.1:0",
		},
	}

	logger := testLogger()
	srv, err := NewServer(config, logger)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Wait for server to start.
	time.Sleep(200 * time.Millisecond)

	// Trigger shutdown.
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within 5s")
	}
}

func TestServerCreatesHostKey(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping (requires PostgreSQL)")
	}

	tmpDir := t.TempDir()

	config := ServerConfig{
		DataDir:     tmpDir,
		DatabaseURL: dsn,
		Domain:      "test.com",
		SSH: SSHConfig{
			Addr:        "127.0.0.1:0",
			HostKeyPath: filepath.Join(tmpDir, "host_key"),
		},
		Edge: EdgeConfig{
			Addr: "127.0.0.1:0",
		},
		Admin: AdminConfig{
			Addr: "127.0.0.1:0",
		},
	}

	srv, err := NewServer(config, testLogger())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	// Host key should exist.
	if _, err := os.Stat(filepath.Join(tmpDir, "host_key")); err != nil {
		t.Errorf("host key not created: %v", err)
	}

	// Store should be accessible.
	if srv.Store() == nil {
		t.Error("expected non-nil Store()")
	}

	// Clean up.
	srv.Stop()
}

func TestServerStoreAccess(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping (requires PostgreSQL)")
	}

	tmpDir := t.TempDir()

	config := ServerConfig{
		DataDir:     tmpDir,
		DatabaseURL: dsn,
		Domain:      "test.com",
		SSH: SSHConfig{
			Addr:        "127.0.0.1:0",
			HostKeyPath: filepath.Join(tmpDir, "host_key"),
		},
		Edge: EdgeConfig{
			Addr: "127.0.0.1:0",
		},
		Admin: AdminConfig{
			Addr: "127.0.0.1:0",
		},
	}

	srv, err := NewServer(config, testLogger())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Stop()

	// Should be able to create a token via the store.
	tok, raw, err := srv.Store().CreateToken("test-token")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if tok.ID == "" || raw == "" {
		t.Error("expected non-empty token ID and raw token")
	}
}
