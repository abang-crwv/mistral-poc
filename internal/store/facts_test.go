package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// upsertSeedRun inserts a row in `runs` so the facts FK constraint is
// satisfied for the upsert tests.
func upsertSeedRun(t *testing.T, s *Store, runID string) {
	t.Helper()
	if _, err := s.DB().ExecContext(t.Context(),
		`INSERT INTO runs (id, template_id, created_at, updated_at, status, rack, created_by)
		 VALUES (?, 't', 0, 0, 'pending', '', 'wpena')`, runID); err != nil {
		t.Fatalf("seed run: %v", err)
	}
}

// insertSeedEvent inserts a stub event so source_event_id FK constraint
// is satisfied. Returns the event's autoincrement id.
func insertSeedEvent(t *testing.T, s *Store, runID, kind string) int64 {
	t.Helper()
	res, err := s.DB().ExecContext(t.Context(),
		`INSERT INTO events (run_id, seq, kind, payload, occurred_at)
		 VALUES (?, 1, ?, '{}', 0)`, runID, kind)
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestUpsertFact_Insert(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000A"
	upsertSeedRun(t, s, runID)
	evtID := insertSeedEvent(t, s, runID, "RunCreated")

	if err := s.UpsertFact(t.Context(), Fact{
		RunID: runID, Scope: "run", Key: "bundle_tag",
		Value: json.RawMessage(`"gb200-fw-2026-05-canary-3"`), Source: "operator",
		SourceEventID: evtID, DiscoveredAt: 1748000000,
	}); err != nil {
		t.Fatalf("UpsertFact: %v", err)
	}

	got, err := s.ListFacts(t.Context(), runID, "", "")
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(got) != 1 || got[0].Key != "bundle_tag" {
		t.Fatalf("got %+v, want one bundle_tag fact", got)
	}
}

func TestUpsertFact_Overwrite(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000B"
	upsertSeedRun(t, s, runID)
	evt1 := insertSeedEvent(t, s, runID, "RunCreated")

	first := Fact{RunID: runID, Scope: "run", Key: "bundle_tag", Value: json.RawMessage(`"v1"`), Source: "operator", SourceEventID: evt1, DiscoveredAt: 100}
	if err := s.UpsertFact(t.Context(), first); err != nil {
		t.Fatalf("first UpsertFact: %v", err)
	}

	// New event, same (run, scope, key) — must overwrite.
	evt2 := insertSeedEvent2(t, s, runID, "FactsDiscovered", 2)
	second := Fact{RunID: runID, Scope: "run", Key: "bundle_tag", Value: json.RawMessage(`"v2"`), Source: "operator", SourceEventID: evt2, DiscoveredAt: 200}
	if err := s.UpsertFact(t.Context(), second); err != nil {
		t.Fatalf("second UpsertFact: %v", err)
	}

	got, err := s.ListFacts(t.Context(), runID, "", "")
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (overwrite, not duplicate)", len(got))
	}
	if string(got[0].Value) != `"v2"` {
		t.Fatalf("value = %q, want %q (overwrite)", got[0].Value, `"v2"`)
	}
	if got[0].SourceEventID != evt2 {
		t.Fatalf("source_event_id = %d, want %d", got[0].SourceEventID, evt2)
	}
}

// insertSeedEvent2 is identical to insertSeedEvent but allows a custom
// seq so subsequent events on the same run don't violate UNIQUE(run_id, seq).
func insertSeedEvent2(t *testing.T, s *Store, runID, kind string, seq int) int64 {
	t.Helper()
	res, err := s.DB().ExecContext(t.Context(),
		`INSERT INTO events (run_id, seq, kind, payload, occurred_at)
		 VALUES (?, ?, ?, '{}', 0)`, runID, seq, kind)
	if err != nil {
		t.Fatalf("seed event seq=%d: %v", seq, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestListFacts_NoFilter(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000C"
	upsertSeedRun(t, s, runID)
	evt := insertSeedEvent(t, s, runID, "RunCreated")

	for i, f := range []Fact{
		{RunID: runID, Scope: "run", Key: "bundle_tag", Value: json.RawMessage(`"x"`), Source: "operator", SourceEventID: evt, DiscoveredAt: 1},
		{RunID: runID, Scope: "rack:dh3-r012-us-east-01a", Key: "instance_type", Value: json.RawMessage(`"gb200-4x"`), Source: "inventory", SourceEventID: evt, DiscoveredAt: 2},
	} {
		if err := s.UpsertFact(t.Context(), f); err != nil {
			t.Fatalf("UpsertFact[%d]: %v", i, err)
		}
	}

	got, err := s.ListFacts(t.Context(), runID, "", "")
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestListFacts_ScopeFilter(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000D"
	upsertSeedRun(t, s, runID)
	evt := insertSeedEvent(t, s, runID, "RunCreated")

	rackScope := "rack:dh3-r012-us-east-01a"
	for _, f := range []Fact{
		{RunID: runID, Scope: "run", Key: "bundle_tag", Value: json.RawMessage(`"x"`), Source: "operator", SourceEventID: evt, DiscoveredAt: 1},
		{RunID: runID, Scope: rackScope, Key: "instance_type", Value: json.RawMessage(`"gb200-4x"`), Source: "inventory", SourceEventID: evt, DiscoveredAt: 2},
		{RunID: runID, Scope: rackScope, Key: "sku", Value: json.RawMessage(`"nvl72"`), Source: "inventory", SourceEventID: evt, DiscoveredAt: 3},
	} {
		if err := s.UpsertFact(t.Context(), f); err != nil {
			t.Fatalf("UpsertFact: %v", err)
		}
	}

	got, err := s.ListFacts(t.Context(), runID, rackScope, "")
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (only rack-scoped)", len(got))
	}
	for _, f := range got {
		if f.Scope != rackScope {
			t.Errorf("scope = %q, want %q", f.Scope, rackScope)
		}
	}
}

func TestListFacts_SourceFilter(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000E"
	upsertSeedRun(t, s, runID)
	evt := insertSeedEvent(t, s, runID, "RunCreated")

	for _, f := range []Fact{
		{RunID: runID, Scope: "run", Key: "bundle_tag", Value: json.RawMessage(`"x"`), Source: "operator", SourceEventID: evt, DiscoveredAt: 1},
		{RunID: runID, Scope: "rack:dh3-r012-us-east-01a", Key: "instance_type", Value: json.RawMessage(`"gb200-4x"`), Source: "inventory", SourceEventID: evt, DiscoveredAt: 2},
	} {
		if err := s.UpsertFact(t.Context(), f); err != nil {
			t.Fatalf("UpsertFact: %v", err)
		}
	}

	got, err := s.ListFacts(t.Context(), runID, "", "operator")
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(got) != 1 || got[0].Source != "operator" {
		t.Fatalf("got %+v, want one operator-sourced fact", got)
	}
}
