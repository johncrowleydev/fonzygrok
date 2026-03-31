package store

import (
	"testing"
)

func TestNew(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:): %v", err)
	}
	defer s.Close()

	if s.db == nil {
		t.Fatal("expected non-nil db")
	}
}

func TestMigrate(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:): %v", err)
	}
	defer s.Close()

	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Verify all tables exist.
	tables := []string{"tokens", "tunnels", "connection_log"}
	for _, table := range tables {
		var name string
		err := s.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after migration: %v", table, err)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:): %v", err)
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

func TestWALMode(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:): %v", err)
	}
	defer s.Close()

	var mode string
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	// :memory: databases use "memory" journal mode, which is expected.
	// WAL mode is set but SQLite falls back to "memory" for in-memory DBs.
	if mode != "wal" && mode != "memory" {
		t.Errorf("journal_mode: got %q, want wal or memory", mode)
	}
}

func TestClose(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:): %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Double close should not panic.
	if err := s.Close(); err != nil {
		// Some drivers return an error on double close, that's acceptable.
		t.Logf("double Close returned: %v (acceptable)", err)
	}
}

// newTestStore creates an in-memory store with migrations applied.
// It fails the test if setup fails.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:): %v", err)
	}
	if err := s.Migrate(); err != nil {
		s.Close()
		t.Fatalf("Migrate: %v", err)
	}
	return s
}
