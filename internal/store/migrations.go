package store

import (
	"context"
	"fmt"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS runs (
  id            TEXT PRIMARY KEY,
  template_id   TEXT NOT NULL,
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL,
  status        TEXT NOT NULL,
  rack          TEXT NOT NULL,
  created_by    TEXT NOT NULL DEFAULT 'wpena'
);

CREATE TABLE IF NOT EXISTS events (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id        TEXT NOT NULL REFERENCES runs(id),
  seq           INTEGER NOT NULL,
  kind          TEXT NOT NULL,
  payload       TEXT NOT NULL,
  occurred_at   INTEGER NOT NULL,
  UNIQUE(run_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_events_run ON events(run_id, seq);
CREATE INDEX IF NOT EXISTS idx_runs_updated ON runs(updated_at DESC);

CREATE TABLE IF NOT EXISTS templates (
  id          TEXT PRIMARY KEY,
  version     INTEGER NOT NULL,
  body_yaml   TEXT NOT NULL,
  parsed_json TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS facts (
  run_id          TEXT    NOT NULL REFERENCES runs(id),
  scope           TEXT    NOT NULL,
  key             TEXT    NOT NULL,
  value           TEXT    NOT NULL,
  source          TEXT    NOT NULL,
  source_event_id INTEGER NOT NULL REFERENCES events(id),
  discovered_at   INTEGER NOT NULL,
  PRIMARY KEY (run_id, scope, key)
);

CREATE INDEX IF NOT EXISTS idx_facts_run ON facts(run_id);
CREATE INDEX IF NOT EXISTS idx_facts_scope ON facts(run_id, scope);

CREATE TABLE IF NOT EXISTS evidence (
  id           TEXT    PRIMARY KEY,
  run_id       TEXT    NOT NULL REFERENCES runs(id),
  step_id      TEXT    NOT NULL,
  content_type TEXT    NOT NULL,
  payload      BLOB    NOT NULL,
  created_at   INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_evidence_run ON evidence(run_id);

CREATE TABLE IF NOT EXISTS rack_deviceslots (
  run_id        TEXT    NOT NULL REFERENCES runs(id),
  rack          TEXT    NOT NULL,
  deviceslot    TEXT    NOT NULL,
  bmn_name      TEXT    NOT NULL,
  zone          TEXT    NOT NULL,
  ct_position   INTEGER,
  discovered_at INTEGER NOT NULL,
  PRIMARY KEY (run_id, rack, deviceslot)
);

CREATE INDEX IF NOT EXISTS idx_rack_deviceslots_run ON rack_deviceslots(run_id);

CREATE TABLE IF NOT EXISTS bmn_ignores (
  run_id     TEXT    NOT NULL REFERENCES runs(id),
  rack       TEXT    NOT NULL,
  deviceslot TEXT    NOT NULL,
  source     TEXT    NOT NULL,
  reason     TEXT,
  marked_at  INTEGER NOT NULL,
  PRIMARY KEY (run_id, deviceslot, source)
);

CREATE INDEX IF NOT EXISTS idx_bmn_ignores_run ON bmn_ignores(run_id);
`

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("apply base schema: %w", err)
	}
	if err := s.addBundleColumnIfMissing(ctx); err != nil {
		return fmt.Errorf("add bundle column: %w", err)
	}
	if err := s.addInputsColumnIfMissing(ctx); err != nil {
		return fmt.Errorf("add inputs column: %w", err)
	}
	if err := s.addDeviceslotColumnIfMissing(ctx); err != nil {
		return fmt.Errorf("add deviceslot column: %w", err)
	}
	if err := s.addWorstVerdictColumnIfMissing(ctx); err != nil {
		return fmt.Errorf("add worst_verdict column: %w", err)
	}
	return nil
}

// addBundleColumnIfMissing adds the `bundle` column to the `runs` table
// when it isn't already present.
func (s *Store) addBundleColumnIfMissing(ctx context.Context) error {
	row := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('runs') WHERE name = 'bundle'`)
	var n int
	if err := row.Scan(&n); err != nil {
		return fmt.Errorf("introspect: %w", err)
	}
	if n > 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		`ALTER TABLE runs ADD COLUMN bundle TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("alter: %w", err)
	}
	return nil
}

// addInputsColumnIfMissing adds the `inputs` JSON-text column to the `runs`
// table when it isn't already present. Defaults to '{}' so iter-2 rows
// projecting under iter-3a code see an empty inputs map instead of NULL.
func (s *Store) addInputsColumnIfMissing(ctx context.Context) error {
	row := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('runs') WHERE name = 'inputs'`)
	var n int
	if err := row.Scan(&n); err != nil {
		return fmt.Errorf("introspect: %w", err)
	}
	if n > 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		`ALTER TABLE runs ADD COLUMN inputs TEXT NOT NULL DEFAULT '{}'`); err != nil {
		return fmt.Errorf("alter: %w", err)
	}
	return nil
}

// addWorstVerdictColumnIfMissing adds the `worst_verdict` TEXT column to the
// `runs` table when it isn't already present. Tracks the worst step verdict
// seen so far (passed < warning < failed); drives the denormalized status.
func (s *Store) addWorstVerdictColumnIfMissing(ctx context.Context) error {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('runs') WHERE name = 'worst_verdict'`).Scan(&n); err != nil {
		return fmt.Errorf("introspect worst_verdict column: %w", err)
	}
	if n > 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		`ALTER TABLE runs ADD COLUMN worst_verdict TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add worst_verdict column: %w", err)
	}
	return nil
}

// addDeviceslotColumnIfMissing adds the nullable `deviceslot` TEXT column
// to evidence so iter-5e's rlcc_action_probe can attribute per-CT
// evidence rows. Pre-iter5 rows scan as NULL; existing callers (alertprobe,
// firmwareinventoryprobe) leave the column unset.
func (s *Store) addDeviceslotColumnIfMissing(ctx context.Context) error {
	row := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('evidence') WHERE name = 'deviceslot'`)
	var n int
	if err := row.Scan(&n); err != nil {
		return fmt.Errorf("introspect: %w", err)
	}
	if n > 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		`ALTER TABLE evidence ADD COLUMN deviceslot TEXT`); err != nil {
		return fmt.Errorf("alter: %w", err)
	}
	return nil
}
