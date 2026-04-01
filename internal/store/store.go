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

	// For in-memory databases, limit to a single connection so all
	// queries share the same database instance.
	if dbPath == ":memory:" {
		db.SetMaxOpenConns(1)
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

	// Post-migration: add user_id column to tokens if missing.
	// SQLite lacks ALTER TABLE ... ADD COLUMN IF NOT EXISTS, so we
	// check PRAGMA table_info first. This handles existing v1.1 DBs.
	if err := s.addColumnIfMissing("tokens", "user_id", "TEXT REFERENCES users(id)"); err != nil {
		return fmt.Errorf("store: add user_id to tokens: %w", err)
	}

	return nil
}

// addColumnIfMissing adds a column to a table only if it doesn't already exist.
// DECISION: Use PRAGMA table_info instead of catching ALTER TABLE errors,
// because SQLite error messages for duplicate columns are not standardized.
func (s *Store) addColumnIfMissing(table, column, columnDef string) error {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("pragma table_info(%s): %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan table_info: %w", err)
		}
		if name == column {
			return nil // Column already exists.
		}
	}

	// Column doesn't exist — add it.
	_, err = s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, columnDef))
	return err
}
