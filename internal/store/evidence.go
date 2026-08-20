package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrEvidenceNotFound is returned by GetEvidence when the id has no row.
// Callers branch on errors.Is(err, ErrEvidenceNotFound).
var ErrEvidenceNotFound = errors.New("store: evidence not found")

// Evidence is the persistent record of a probe step's output.
// Payload is opaque bytes; ContentType narrates how to interpret them
// (iter-4b only emits "application/json").
// Deviceslot is nil for run-level evidence; iter-5e's rlcc_action_probe
// sets it to the canonical per-CT deviceslot so evidence can be
// attributed to a specific compute tray.
type Evidence struct {
	ID          string  `json:"id"`
	RunID       string  `json:"run_id"`
	StepID      string  `json:"step_id"`
	ContentType string  `json:"content_type"`
	Payload     []byte  `json:"-"`
	CreatedAt   int64   `json:"created_at"`
	Deviceslot  *string `json:"deviceslot,omitempty"`
}

// PutEvidence inserts a new evidence row. Fails with a foreign-key
// violation if RunID does not exist in runs(id). CreatedAt is stamped
// from time.Now().Unix() if the caller leaves it zero.
func (s *Store) PutEvidence(ctx context.Context, e Evidence) error {
	if e.ID == "" {
		return fmt.Errorf("store: PutEvidence called with empty id")
	}
	if e.RunID == "" {
		return fmt.Errorf("store: PutEvidence called with empty run_id")
	}
	if e.CreatedAt == 0 {
		e.CreatedAt = time.Now().Unix()
	}
	var ds sql.NullString
	if e.Deviceslot != nil {
		ds = sql.NullString{String: *e.Deviceslot, Valid: true}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO evidence (id, run_id, step_id, content_type, payload, created_at, deviceslot)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.RunID, e.StepID, e.ContentType, e.Payload, e.CreatedAt, ds,
	)
	if err != nil {
		return fmt.Errorf("insert evidence %s: %w", e.ID, err)
	}
	return nil
}

// ListEvidence returns a run's evidence rows, ordered by created_at then
// deviceslot (per-CT rows grouped stably under their step). stepID == ""
// returns all rows for the run; otherwise only that step's. Returns an
// empty slice when none.
func (s *Store) ListEvidence(ctx context.Context, runID, stepID string) ([]Evidence, error) {
	q := `SELECT id, run_id, step_id, content_type, payload, created_at, deviceslot
	      FROM evidence WHERE run_id = ?`
	args := []any{runID}
	if stepID != "" {
		q += ` AND step_id = ?`
		args = append(args, stepID)
	}
	q += ` ORDER BY created_at, deviceslot`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list evidence: %w", err)
	}
	defer rows.Close()
	var out []Evidence
	for rows.Next() {
		var e Evidence
		var ds sql.NullString
		if err := rows.Scan(&e.ID, &e.RunID, &e.StepID, &e.ContentType, &e.Payload, &e.CreatedAt, &ds); err != nil {
			return nil, fmt.Errorf("scan evidence: %w", err)
		}
		if ds.Valid {
			v := ds.String
			e.Deviceslot = &v
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetEvidence returns the row with id. Returns ErrEvidenceNotFound when
// no such row exists.
func (s *Store) GetEvidence(ctx context.Context, id string) (Evidence, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, run_id, step_id, content_type, payload, created_at, deviceslot
		FROM evidence
		WHERE id = ?`, id)
	var e Evidence
	var ds sql.NullString
	if err := row.Scan(&e.ID, &e.RunID, &e.StepID, &e.ContentType, &e.Payload, &e.CreatedAt, &ds); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Evidence{}, ErrEvidenceNotFound
		}
		return Evidence{}, fmt.Errorf("scan evidence %s: %w", id, err)
	}
	if ds.Valid {
		v := ds.String
		e.Deviceslot = &v
	}
	return e, nil
}
