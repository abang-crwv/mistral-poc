package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// ptrString returns a pointer to s. Convenience helper for tests that
// construct Evidence.Deviceslot inline without a separate var.
func ptrString(s string) *string { return &s }

// mustSeedRun inserts a RunCreated event (and its runs projection) so
// that evidence rows satisfy the FK constraint on runs(id). The run id
// is returned so callers can use it directly.
func mustSeedRun(t *testing.T, s *Store) string {
	t.Helper()
	payload := []byte(`{"template_id":"firmware-release-canary","template_version":4,"inputs":{}}`)
	runID := "run-test-evidence"
	if err := s.AppendEvent(context.Background(), runID, "RunCreated", payload); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	return runID
}

func TestPutGetEvidence_NilDeviceslotRoundTrips(t *testing.T) {
	s := openTestStore(t)
	runID := mustSeedRun(t, s)
	want := Evidence{
		ID:          "ev-1",
		RunID:       runID,
		StepID:      "s1",
		ContentType: "application/json",
		Payload:     []byte(`{}`),
		Deviceslot:  nil,
	}
	if err := s.PutEvidence(context.Background(), want); err != nil {
		t.Fatalf("PutEvidence: %v", err)
	}
	got, err := s.GetEvidence(context.Background(), "ev-1")
	if err != nil {
		t.Fatalf("GetEvidence: %v", err)
	}
	if got.Deviceslot != nil {
		t.Errorf("Deviceslot = %v, want nil", got.Deviceslot)
	}
}

func TestPutGetEvidence_DeviceslotRoundTrips(t *testing.T) {
	s := openTestStore(t)
	runID := mustSeedRun(t, s)
	want := Evidence{
		ID:          "ev-2",
		RunID:       runID,
		StepID:      "s1",
		ContentType: "application/json",
		Payload:     []byte(`{}`),
		Deviceslot:  ptrString("dh1-r037-node-07-us-east-04b"),
	}
	if err := s.PutEvidence(context.Background(), want); err != nil {
		t.Fatalf("PutEvidence: %v", err)
	}
	got, err := s.GetEvidence(context.Background(), "ev-2")
	if err != nil {
		t.Fatalf("GetEvidence: %v", err)
	}
	if got.Deviceslot == nil {
		t.Fatal("Deviceslot is nil, want non-nil")
	}
	if *got.Deviceslot != "dh1-r037-node-07-us-east-04b" {
		t.Errorf("Deviceslot = %q, want dh1-r037-node-07-us-east-04b", *got.Deviceslot)
	}
}

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestPutEvidence_GetEvidence_RoundTrip(t *testing.T) {
	s := openStore(t)
	ctx := t.Context()

	// PutEvidence FK-references runs(id); insert a run row first.
	runID := "01HZX0000000000000000RUN1"
	if _, err := s.DB().ExecContext(ctx, `
		INSERT INTO runs (id, template_id, bundle, created_at, updated_at, status, rack, created_by, inputs)
		VALUES (?, ?, '', 0, 0, 'pending', '', 'wpena', '{}')`,
		runID, "firmware-release-canary"); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	in := Evidence{
		ID:          "01HZXEVIDENCE00000000001",
		RunID:       runID,
		StepID:      "alert_baseline",
		ContentType: "application/json",
		Payload:     []byte(`{"alerts_clear":true}`),
	}
	if err := s.PutEvidence(ctx, in); err != nil {
		t.Fatalf("PutEvidence: %v", err)
	}

	got, err := s.GetEvidence(ctx, in.ID)
	if err != nil {
		t.Fatalf("GetEvidence: %v", err)
	}
	if got.RunID != in.RunID || got.StepID != in.StepID || got.ContentType != in.ContentType {
		t.Errorf("got = %+v, want subset of %+v", got, in)
	}
	if string(got.Payload) != string(in.Payload) {
		t.Errorf("payload = %s, want %s", got.Payload, in.Payload)
	}
	if got.CreatedAt == 0 {
		t.Errorf("created_at = 0, want unix timestamp")
	}
}

func TestGetEvidence_NotFound(t *testing.T) {
	s := openStore(t)
	_, err := s.GetEvidence(t.Context(), "01HZXDOESNOTEXIST00000001")
	if !errors.Is(err, ErrEvidenceNotFound) {
		t.Fatalf("err = %v, want ErrEvidenceNotFound", err)
	}
}

func TestPutEvidence_ForeignKeyViolation(t *testing.T) {
	s := openStore(t)
	in := Evidence{
		ID:          "01HZXEVIDENCE00000000002",
		RunID:       "no-such-run",
		StepID:      "x",
		ContentType: "application/json",
		Payload:     []byte(`{}`),
	}
	if err := s.PutEvidence(t.Context(), in); err == nil {
		t.Fatal("PutEvidence: nil, want FK constraint violation")
	} else if !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Errorf("err = %v, want FK violation", err)
	}
}

func TestEvidence_IdempotentMigration(t *testing.T) {
	// Open on the same path twice; second open must succeed (no
	// "table evidence already exists" error). Also confirm the
	// evidence table is queryable so a regression that drops the
	// CREATE block from schemaSQL would fail loudly.
	path := filepath.Join(t.TempDir(), "fwqual.db")
	s1, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open #1: %v", err)
	}
	_ = s1.Close()
	s2, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open #2 (idempotent migration): %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	if _, err := s2.DB().ExecContext(t.Context(),
		`SELECT id FROM evidence LIMIT 0`); err != nil {
		t.Fatalf("evidence table missing after idempotent migration: %v", err)
	}
}

func TestPutEvidence_DuplicateID_ReturnsError(t *testing.T) {
	// Evidence is immutable in iter-4b: a second PutEvidence with the
	// same id surfaces a PK conflict rather than silently overwriting.
	s := openStore(t)
	ctx := t.Context()
	runID := "01HZX0000000000000000RUN1"
	if _, err := s.DB().ExecContext(ctx, `
		INSERT INTO runs (id, template_id, bundle, created_at, updated_at, status, rack, created_by, inputs)
		VALUES (?, ?, '', 0, 0, 'pending', '', 'wpena', '{}')`,
		runID, "firmware-release-canary"); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	in := Evidence{
		ID:          "01HZXEVIDENCE00000000DUP1",
		RunID:       runID,
		StepID:      "alert_baseline",
		ContentType: "application/json",
		Payload:     []byte(`{"alerts_clear":true}`),
	}
	if err := s.PutEvidence(ctx, in); err != nil {
		t.Fatalf("first PutEvidence: %v", err)
	}
	if err := s.PutEvidence(ctx, in); err == nil {
		t.Fatal("second PutEvidence: nil, want PK conflict")
	}
}

func TestPutEvidence_EmptyID_Rejected(t *testing.T) {
	s := openStore(t)
	in := Evidence{
		ID:          "",
		RunID:       "any",
		StepID:      "x",
		ContentType: "application/json",
		Payload:     []byte(`{}`),
	}
	err := s.PutEvidence(t.Context(), in)
	if err == nil {
		t.Fatal("PutEvidence with empty id: nil, want error")
	}
	if !strings.Contains(err.Error(), "empty id") {
		t.Errorf("err = %v, want mention of empty id", err)
	}
}

func TestPutEvidence_EmptyRunID_Rejected(t *testing.T) {
	s := openStore(t)
	in := Evidence{
		ID:          "01HZXEVIDENCE00000000EMPT",
		RunID:       "",
		StepID:      "x",
		ContentType: "application/json",
		Payload:     []byte(`{}`),
	}
	err := s.PutEvidence(t.Context(), in)
	if err == nil {
		t.Fatal("PutEvidence with empty run_id: nil, want error")
	}
	if !strings.Contains(err.Error(), "empty run_id") {
		t.Errorf("err = %v, want mention of empty run_id", err)
	}
}

func TestListEvidence(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Seed two runs so PutEvidence FK constraints are satisfied.
	seedPayload := func(runID string) []byte {
		return []byte(`{"template_id":"firmware-release-canary","inputs":{"bundle_tag":"b","canary_racks":["r1"]},"created_by":"wpena"}`)
	}
	for _, runID := range []string{"run1", "run2"} {
		if err := s.AppendEvent(ctx, runID, "RunCreated", seedPayload(runID)); err != nil {
			t.Fatalf("seed run %s: %v", runID, err)
		}
	}

	ds := "rack-a-node-01-z"
	put := func(id, run, step string, dsp *string) {
		t.Helper()
		if err := s.PutEvidence(ctx, Evidence{ID: id, RunID: run, StepID: step, ContentType: "application/json", Payload: []byte(`{"k":1}`), Deviceslot: dsp}); err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
	}
	put("e1", "run1", "l11", nil)
	put("e2", "run1", "l11", &ds)
	put("e3", "run1", "other", nil)
	put("e4", "run2", "l11", nil)

	all, err := s.ListEvidence(ctx, "run1", "")
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("run1 all = %d, want 3", len(all))
	}

	step, err := s.ListEvidence(ctx, "run1", "l11")
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if len(step) != 2 {
		t.Errorf("run1 l11 = %d, want 2", len(step))
	}

	empty, err := s.ListEvidence(ctx, "nope", "")
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("unknown run = %d, want 0", len(empty))
	}
}
