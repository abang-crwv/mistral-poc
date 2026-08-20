package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestOpen_CreatesSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fwqual.db")
	s, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	row := s.DB().QueryRowContext(context.Background(),
		`SELECT name FROM sqlite_master WHERE type='table' AND name='runs'`)
	var name string
	if err := row.Scan(&name); err != nil {
		t.Fatalf("runs table missing: %v", err)
	}
	if name != "runs" {
		t.Fatalf("got table %q, want %q", name, "runs")
	}
}

func TestAppendEvent_RunCreated_UpsertsRun(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000A"
	payload, _ := json.Marshal(map[string]string{
		"template_id": "firmware-release-canary",
		"bundle":      "gb200-fw-2026-05-canary-3",
		"rack":        "dh3-r012-us-east-01a",
		"created_by":  "wpena",
	})
	if err := s.AppendEvent(ctx, runID, "RunCreated", payload); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	var status, rack, bundle string
	row := s.DB().QueryRowContext(ctx,
		`SELECT status, rack, bundle FROM runs WHERE id = ?`, runID)
	if err := row.Scan(&status, &rack, &bundle); err != nil {
		t.Fatalf("scan run: %v", err)
	}
	if status != "pending" {
		t.Fatalf("status = %q, want pending", status)
	}
	if rack != "dh3-r012-us-east-01a" {
		t.Fatalf("rack = %q, want dh3-r012-us-east-01a", rack)
	}
	if bundle != "gb200-fw-2026-05-canary-3" {
		t.Fatalf("bundle = %q, want gb200-fw-2026-05-canary-3", bundle)
	}

	var seq int
	row = s.DB().QueryRowContext(ctx, `SELECT seq FROM events WHERE run_id = ?`, runID)
	if err := row.Scan(&seq); err != nil {
		t.Fatalf("scan event: %v", err)
	}
	if seq != 1 {
		t.Fatalf("seq = %d, want 1", seq)
	}
}

func TestListRuns_OrdersByUpdatedDesc(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Insert two runs; the second is newer by one second via manual UPDATE
	mk := func(id, rack string) {
		p, _ := json.Marshal(map[string]string{"template_id": "t", "rack": rack, "created_by": "wpena"})
		if err := s.AppendEvent(ctx, id, "RunCreated", p); err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
	}
	mk("01HZX0000000000000000000A", "as1-r07")
	mk("01HZX0000000000000000000B", "as2-r11")
	if _, err := s.DB().ExecContext(ctx,
		`UPDATE runs SET updated_at = updated_at + 1 WHERE id = ?`,
		"01HZX0000000000000000000B"); err != nil {
		t.Fatalf("bump: %v", err)
	}

	runs, err := s.ListRuns(ctx)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len = %d, want 2", len(runs))
	}
	if runs[0].ID != "01HZX0000000000000000000B" {
		t.Fatalf("first.ID = %q, want newest first", runs[0].ID)
	}
}

func TestLoadEvents_ReturnsInSeqOrder(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000A"
	p, _ := json.Marshal(map[string]string{"template_id": "t", "rack": "as1-r07", "created_by": "wpena"})
	if err := s.AppendEvent(ctx, runID, "RunCreated", p); err != nil {
		t.Fatalf("AppendEvent RunCreated: %v", err)
	}
	if err := s.AppendEvent(ctx, runID, "StepStarted", []byte(`{"step_id":"capacity_check"}`)); err != nil {
		t.Fatalf("AppendEvent StepStarted: %v", err)
	}

	events, err := s.LoadEvents(ctx, runID)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len = %d, want 2", len(events))
	}
	if events[0].Kind != "RunCreated" || events[1].Kind != "StepStarted" {
		t.Fatalf("kinds = [%q, %q], want [RunCreated, StepStarted]", events[0].Kind, events[1].Kind)
	}
}

func TestMigrate_AddsInputsColumn(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fwqual.db")

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	rows, err := s.DB().QueryContext(ctx, `SELECT name FROM pragma_table_info('runs')`)
	if err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	hasInputs := false
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatalf("scan: %v", err)
		}
		if name == "inputs" {
			hasInputs = true
		}
	}
	rows.Close()
	if !hasInputs {
		t.Fatal("inputs column missing after Open")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen — migration must be idempotent.
	s2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if err := s2.Close(); err != nil {
		t.Fatalf("Close 2: %v", err)
	}
}

func TestMigrate_CreatesTemplatesTable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fwqual.db")

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	row := s.DB().QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name='templates'`)
	var name string
	if err := row.Scan(&name); err != nil {
		t.Fatalf("templates table missing: %v", err)
	}
	if name != "templates" {
		t.Fatalf("got table %q, want templates", name)
	}
}

func TestMigrate_CreatesTemplatesTable_Idempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fwqual.db")

	for i := 0; i < 3; i++ {
		s, err := Open(ctx, path)
		if err != nil {
			t.Fatalf("Open #%d: %v", i, err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("Close #%d: %v", i, err)
		}
	}
}

func TestMigrate_AddsBundleColumn(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fwqual.db")

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Verify bundle column exists after first open.
	rows, err := s.DB().QueryContext(ctx, `SELECT name FROM pragma_table_info('runs')`)
	if err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	hasBundle := false
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatalf("scan: %v", err)
		}
		if name == "bundle" {
			hasBundle = true
		}
	}
	rows.Close()
	if !hasBundle {
		t.Fatal("bundle column missing after Open")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen — migration must be idempotent.
	s2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if err := s2.Close(); err != nil {
		t.Fatalf("Close 2: %v", err)
	}
}

func TestAppendEvent_RunCreated_DerivesBundleAndRackFromInputs(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000C"
	payload := []byte(`{
		"template_id": "firmware-release-canary",
		"inputs": {
			"bundle_tag": "gb200-fw-2026-05-canary-7",
			"canary_racks": ["dh3-r012-us-east-01a", "dh3-r013-us-east-01a"]
		},
		"created_by": "wpena"
	}`)
	if err := s.AppendEvent(ctx, runID, "RunCreated", payload); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	var bundle, rack, inputs string
	row := s.DB().QueryRowContext(ctx,
		`SELECT bundle, rack, inputs FROM runs WHERE id = ?`, runID)
	if err := row.Scan(&bundle, &rack, &inputs); err != nil {
		t.Fatalf("scan run: %v", err)
	}
	if bundle != "gb200-fw-2026-05-canary-7" {
		t.Fatalf("bundle = %q, want gb200-fw-2026-05-canary-7", bundle)
	}
	if rack != "dh3-r012-us-east-01a,dh3-r013-us-east-01a" {
		t.Fatalf("rack = %q, want comma-joined racks", rack)
	}
	if inputs == "" || inputs == "{}" {
		t.Fatalf("inputs = %q, want populated JSON", inputs)
	}
}

func TestAppendEvent_RunCreated_LegacyShapeStillWorks(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000D"
	payload := []byte(`{
		"template_id": "firmware-release-canary",
		"bundle": "legacy-bundle",
		"rack": "dh3-r099-us-east-01a",
		"created_by": "wpena"
	}`)
	if err := s.AppendEvent(ctx, runID, "RunCreated", payload); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	var bundle, rack string
	row := s.DB().QueryRowContext(ctx,
		`SELECT bundle, rack FROM runs WHERE id = ?`, runID)
	if err := row.Scan(&bundle, &rack); err != nil {
		t.Fatalf("scan run: %v", err)
	}
	if bundle != "legacy-bundle" {
		t.Fatalf("bundle = %q, want legacy-bundle", bundle)
	}
	if rack != "dh3-r099-us-east-01a" {
		t.Fatalf("rack = %q, want dh3-r099-us-east-01a", rack)
	}
}

func TestMigrate_CreatesFactsTable(t *testing.T) {
	t.Helper()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	var name string
	row := s.DB().QueryRowContext(t.Context(),
		`SELECT name FROM sqlite_master WHERE type='table' AND name='facts'`)
	if err := row.Scan(&name); err != nil {
		t.Fatalf("facts table missing: %v", err)
	}
	if name != "facts" {
		t.Fatalf("table = %q, want facts", name)
	}
}

func TestMigrate_FactsTableIdempotent(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "fwqual.db")

	// First open creates everything.
	s, err := Open(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	s.Close()

	// Re-open exercises migrate against an already-migrated DB. Should
	// not error.
	s2, err := Open(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s2.Close()

	// facts table still present, no duplicate damage.
	var n int
	if err := s2.DB().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='facts'`).Scan(&n); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if n != 1 {
		t.Fatalf("facts table count = %d, want 1", n)
	}
}
