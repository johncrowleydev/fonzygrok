// Package store provides persistent storage for fonzygrok using SQLite.
// It manages database initialization, migrations, and provides CRUD
// operations for tokens, tunnels, and connection logs.
package store

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/fonzygrok/fonzygrok/migrations"

	// Pure-Go SQLite driver — no CGo required.
	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database connection and provides methods for
// persisting fonzygrok state.
type Store struct {
	db *sql.DB
}

// New opens a SQLite database at the given path and configures WAL mode.
// Use ":memory:" for in-memory testing databases.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("store: open database %q: %w", dbPath, err)
	}

	// Enable WAL mode for concurrent reads per GOV-008.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: enable WAL mode: %w", err)
	}

	// Enable foreign keys.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: enable foreign keys: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB returns the underlying *sql.DB for use by other store packages.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Migrate runs all embedded SQL migration files in order.
// Migrations are idempotent (use IF NOT EXISTS).
func (s *Store) Migrate() error {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("store: read migrations dir: %w", err)
	}

	// Sort by filename to ensure execution order.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := migrations.FS.ReadFile(entry.Name())
		if err != nil {
			return fmt.Errorf("store: read migration %s: %w", entry.Name(), err)
		}
		if _, err := s.db.Exec(string(data)); err != nil {
			return fmt.Errorf("store: execute migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}
