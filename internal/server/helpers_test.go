package server

import (
	"os"
	"testing"

	"github.com/fonzygrok/fonzygrok/internal/store"
)

// newTestStore creates a test PostgreSQL store, dropping and recreating all tables.
// Skips the test if TEST_DATABASE_URL is not set.
// Uses an advisory lock to prevent concurrent test packages from racing.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping (requires PostgreSQL)")
	}
	st, err := store.New(dsn)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	// Acquire advisory lock to prevent cross-package migration races.
	// Lock ID 42 is arbitrary but must match across all test helpers.
	st.DB().Exec("SELECT pg_advisory_lock(42)")

	// Drop and recreate for isolation.
	st.DB().Exec("DROP TABLE IF EXISTS connection_log CASCADE")
	st.DB().Exec("DROP TABLE IF EXISTS tunnels CASCADE")
	st.DB().Exec("DROP TABLE IF EXISTS invite_codes CASCADE")
	st.DB().Exec("DROP TABLE IF EXISTS tokens CASCADE")
	st.DB().Exec("DROP TABLE IF EXISTS users CASCADE")
	if err := st.Migrate(); err != nil {
		st.DB().Exec("SELECT pg_advisory_unlock(42)")
		st.Close()
		t.Fatalf("store.Migrate: %v", err)
	}

	// Release the lock after setup completes.
	st.DB().Exec("SELECT pg_advisory_unlock(42)")
	return st
}
