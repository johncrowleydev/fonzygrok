// Package store provides persistent storage for fonzygrok using PostgreSQL.
// It manages database initialization, migrations, and provides CRUD
// operations for tokens, tunnels, users, and invite codes.
package store

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/fonzygrok/fonzygrok/migrations"

	// Pure-Go PostgreSQL driver.
	_ "github.com/lib/pq"
)

// Store wraps a PostgreSQL database connection and provides methods for
// persisting fonzygrok state.
type Store struct {
	db *sql.DB
}

// New opens a PostgreSQL database using the given connection string.
// Example: "postgres://user:pass@host:5432/dbname?sslmode=disable"
func New(connStr string) (*Store, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("store: open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: ping database: %w", err)
	}

	// Sensible connection pool defaults for a small app.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

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
// Migrations are idempotent (use IF NOT EXISTS / CREATE OR REPLACE).
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

	// Post-migration: add user_id column to tokens if missing.
	if err := s.addColumnIfMissing("tokens", "user_id", "TEXT REFERENCES users(id)"); err != nil {
		return fmt.Errorf("store: add user_id to tokens: %w", err)
	}

	return nil
}

// addColumnIfMissing adds a column to a table only if it doesn't already exist.
// Uses information_schema instead of SQLite's PRAGMA table_info.
func (s *Store) addColumnIfMissing(table, column, columnDef string) error {
	var exists bool
	err := s.db.QueryRow(
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = $1 AND column_name = $2
		)`, table, column,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check column %s.%s: %w", table, column, err)
	}

	if exists {
		return nil
	}

	_, err = s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, columnDef))
	return err
}
