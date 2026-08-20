package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Run is the projection row for a single run. Inputs is the raw JSON
// the user supplied (or that the createRunHandler normalized from the
// legacy {bundle, rack} shape).
type Run struct {
	ID         string          `json:"id"`
	Bundle     string          `json:"bundle"`
	TemplateID string          `json:"template_id"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	Status     string          `json:"status"`
	Rack       string          `json:"rack"`
	CreatedBy  string          `json:"created_by"`
	Inputs     json.RawMessage `json:"inputs"`
}

// Event is a single event log entry.
type Event struct {
	Seq        int64           `json:"seq"`
	Kind       string          `json:"kind"`
	Payload    json.RawMessage `json:"payload"`
	OccurredAt time.Time       `json:"occurred_at"`
}

// ListRuns returns all runs sorted by updated_at desc.
func (s *Store) ListRuns(ctx context.Context) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, bundle, template_id, created_at, updated_at, status, rack, created_by, inputs
		FROM runs
		ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query runs: %w", err)
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		var r Run
		var createdAt, updatedAt int64
		var inputs string
		if err := rows.Scan(&r.ID, &r.Bundle, &r.TemplateID, &createdAt, &updatedAt, &r.Status, &r.Rack, &r.CreatedBy, &inputs); err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		r.CreatedAt = time.Unix(createdAt, 0).UTC()
		r.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		r.Inputs = json.RawMessage(inputs)
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iter: %w", err)
	}
	return runs, nil
}

// LoadEvents returns the event log for a run, ordered by seq ascending.
func (s *Store) LoadEvents(ctx context.Context, runID string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT seq, kind, payload, occurred_at
		FROM events
		WHERE run_id = ?
		ORDER BY seq ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var payload string
		var occurredAt int64
		if err := rows.Scan(&e.Seq, &e.Kind, &payload, &occurredAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		e.Payload = json.RawMessage(payload)
		e.OccurredAt = time.Unix(occurredAt, 0).UTC()
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iter: %w", err)
	}
	return events, nil
}
