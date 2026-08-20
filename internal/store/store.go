// Package store owns the SQLite persistence layer for fwqual.
// The events table is the source of truth; the runs table is a denormalized
// projection kept current in the same transaction as event appends.
package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Store wraps a *sql.DB. Safe for concurrent use.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path, applies migrations,
// and enables WAL mode.
func Open(ctx context.Context, path string) (*Store, error) {
	// _journal_mode=WAL and _busy_timeout improve concurrency for the
	// single-process server case. _fk=1 enforces foreign keys.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close releases the underlying DB pool.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the underlying *sql.DB. Used by tests; production code
// should call methods on Store instead.
func (s *Store) DB() *sql.DB { return s.db }

// RawDB returns the underlying *sql.DB. Used by tests that need to
// count rows or peek at the database without going through the
// typed accessors. Production code should use the typed methods.
func (s *Store) RawDB() *sql.DB { return s.db }
