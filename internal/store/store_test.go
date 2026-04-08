package store

import (
	"fmt"
	"os"
	"testing"
)

// testDatabaseURL returns the database URL for tests.
// Set TEST_DATABASE_URL env var, or defaults to local fonzygrok_test DB.
func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://fonzygrok:fonzygrok@localhost:5432/fonzygrok_test?sslmode=disable"
	}
	return url
}

func TestNew(t *testing.T) {
	s, err := New(testDatabaseURL(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	if s.db == nil {
		t.Fatal("expected non-nil db")
	}
}

func TestMigrate(t *testing.T) {
	s, err := New(testDatabaseURL(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Verify all tables exist.
	tables := []string{"tokens", "tunnels", "connection_log", "users", "invite_codes"}
	for _, table := range tables {
		var exists bool
		err := s.db.QueryRow(
			`SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_name = $1
			)`, table,
		).Scan(&exists)
		if err != nil {
			t.Errorf("check table %q: %v", table, err)
		}
		if !exists {
			t.Errorf("table %q not found after migration", table)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	s, err := New(testDatabaseURL(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	// Run migrations twice — should not error.
	if err := s.Migrate(); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestClose(t *testing.T) {
	s, err := New(testDatabaseURL(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// newTestStore creates a test store with a clean database.
// It drops all tables before migrating to ensure test isolation.
// Uses an advisory lock to prevent cross-package migration races.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(testDatabaseURL(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Acquire advisory lock to prevent cross-package migration races.
	// Lock ID 42 is arbitrary but must match across all test helpers.
	s.db.Exec("SELECT pg_advisory_lock(42)")

	// Clean slate: drop all tables for test isolation.
	tables := []string{"invite_codes", "connection_log", "tunnels", "tokens", "users"}
	for _, table := range tables {
		_, err := s.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table))
		if err != nil {
			s.db.Exec("SELECT pg_advisory_unlock(42)")
			t.Fatalf("drop table %s: %v", table, err)
		}
	}

	if err := s.Migrate(); err != nil {
		s.db.Exec("SELECT pg_advisory_unlock(42)")
		s.Close()
		t.Fatalf("Migrate: %v", err)
	}

	// Release the lock after setup completes.
	s.db.Exec("SELECT pg_advisory_unlock(42)")
	return s
}
