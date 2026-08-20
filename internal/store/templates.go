package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// TemplateRow is the full row for a stored template. BodyYAML preserves
// the original source; ParsedJSON is json.Marshal of the parsed
// template.Template so handlers can serve it without re-parsing.
type TemplateRow struct {
	ID         string          `json:"id"`
	Version    int             `json:"version"`
	BodyYAML   string          `json:"-"`
	ParsedJSON json.RawMessage `json:"parsed"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// TemplateSummary is the list-view projection (id, version, title).
// Title is decoded out of ParsedJSON to avoid storing it twice.
type TemplateSummary struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Title   string `json:"title"`
}

// UpsertTemplate inserts a template or replaces an existing one by id.
// Updates touch updated_at; inserts also set created_at.
func (s *Store) UpsertTemplate(ctx context.Context, id string, version int, bodyYAML, parsedJSON string) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO templates (id, version, body_yaml, parsed_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  version     = excluded.version,
		  body_yaml   = excluded.body_yaml,
		  parsed_json = excluded.parsed_json,
		  updated_at  = excluded.updated_at`,
		id, version, bodyYAML, parsedJSON, now, now,
	)
	if err != nil {
		return fmt.Errorf("upsert template %s: %w", id, err)
	}
	return nil
}

// ListTemplates returns a summary row per template, ordered by id ASC.
func (s *Store) ListTemplates(ctx context.Context) ([]TemplateSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, version, parsed_json FROM templates ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query templates: %w", err)
	}
	defer rows.Close()

	var out []TemplateSummary
	for rows.Next() {
		var sum TemplateSummary
		var parsed string
		if err := rows.Scan(&sum.ID, &sum.Version, &parsed); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		var lite struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal([]byte(parsed), &lite); err != nil {
			return nil, fmt.Errorf("decode parsed_json for %s: %w", sum.ID, err)
		}
		sum.Title = lite.Title
		out = append(out, sum)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iter: %w", err)
	}
	return out, nil
}

// GetTemplate returns one row by id. The bool is true when found.
// A row that exists but has malformed parsed_json returns an error.
func (s *Store) GetTemplate(ctx context.Context, id string) (TemplateRow, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, version, body_yaml, parsed_json, created_at, updated_at
		FROM templates
		WHERE id = ?`, id)

	var (
		tr                   TemplateRow
		parsed               string
		createdAt, updatedAt int64
	)
	if err := row.Scan(&tr.ID, &tr.Version, &tr.BodyYAML, &parsed, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TemplateRow{}, false, nil
		}
		return TemplateRow{}, false, fmt.Errorf("scan template %s: %w", id, err)
	}
	tr.ParsedJSON = json.RawMessage(parsed)
	tr.CreatedAt = time.Unix(createdAt, 0).UTC()
	tr.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return tr, true, nil
}
