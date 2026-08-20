package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Fact is the projection row for a single (run, scope, key) tuple.
// Value is JSON-encoded so the column can hold strings, numbers,
// booleans, and small lists uniformly.
type Fact struct {
	RunID         string          `json:"-"`
	Scope         string          `json:"scope"`
	Key           string          `json:"key"`
	Value         json.RawMessage `json:"value"`
	Source        string          `json:"source"`
	SourceEventID int64           `json:"source_event_id"`
	DiscoveredAt  int64           `json:"discovered_at"`
}

// UpsertFact inserts or overwrites a fact row by composite key
// (run_id, scope, key). Subsequent calls with the same composite key
// overwrite value/source/source_event_id/discovered_at — last writer
// wins. History is preserved separately in the events table.
func (s *Store) UpsertFact(ctx context.Context, f Fact) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO facts (run_id, scope, key, value, source, source_event_id, discovered_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id, scope, key) DO UPDATE SET
		  value           = excluded.value,
		  source          = excluded.source,
		  source_event_id = excluded.source_event_id,
		  discovered_at   = excluded.discovered_at`,
		f.RunID, f.Scope, f.Key, string(f.Value), f.Source, f.SourceEventID, f.DiscoveredAt,
	)
	if err != nil {
		return fmt.Errorf("upsert fact (%s,%s,%s): %w", f.RunID, f.Scope, f.Key, err)
	}
	return nil
}

// ListFacts returns the projection rows for runID, optionally filtered
// by scope and/or source. Empty filter strings are wildcards. The
// special scope filter "rack:*" matches any scope that starts with
// "rack:" — useful for "give me all rack-scoped facts on this run."
// Results are ordered by (scope, key) for stable test assertions.
func (s *Store) ListFacts(ctx context.Context, runID, scope, source string) ([]Fact, error) {
	conditions := []string{"run_id = ?"}
	args := []any{runID}

	switch {
	case scope == "":
		// no scope filter
	case scope == "rack:*":
		conditions = append(conditions, "scope LIKE 'rack:%'")
	default:
		conditions = append(conditions, "scope = ?")
		args = append(args, scope)
	}

	if source != "" {
		conditions = append(conditions, "source = ?")
		args = append(args, source)
	}

	q := `SELECT scope, key, value, source, source_event_id, discovered_at
	      FROM facts
	      WHERE ` + strings.Join(conditions, " AND ") + `
	      ORDER BY scope, key`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query facts: %w", err)
	}
	defer rows.Close()

	out := []Fact{}
	for rows.Next() {
		var f Fact
		var value string
		if err := rows.Scan(&f.Scope, &f.Key, &value, &f.Source, &f.SourceEventID, &f.DiscoveredAt); err != nil {
			return nil, fmt.Errorf("scan fact: %w", err)
		}
		f.RunID = runID
		f.Value = json.RawMessage(value)
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iter: %w", err)
	}
	return out, nil
}
