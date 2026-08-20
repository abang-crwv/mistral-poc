package store

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestAppendEvent_FactsDiscovered_ProjectsToFacts(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000A"
	// First, append RunCreated so the run row exists.
	rcPayload, _ := json.Marshal(map[string]any{
		"template_id": "firmware-release-canary",
		"inputs":      map[string]any{"bundle_tag": "x", "canary_racks": []string{"dh3-r012-us-east-01a"}},
		"created_by":  "wpena",
	})
	if err := s.AppendEvent(t.Context(), runID, "RunCreated", rcPayload); err != nil {
		t.Fatalf("RunCreated AppendEvent: %v", err)
	}

	// Append FactsDiscovered with two facts at rack scope.
	fdPayload, _ := json.Marshal(map[string]any{
		"scope":  "rack:dh3-r012-us-east-01a",
		"source": "inventory",
		"facts": map[string]any{
			"instance_type": "gb200-4x",
			"sku":           "nvl72",
		},
	})
	if err := s.AppendEvent(t.Context(), runID, "FactsDiscovered", fdPayload); err != nil {
		t.Fatalf("FactsDiscovered AppendEvent: %v", err)
	}

	got, err := s.ListFacts(t.Context(), runID, "rack:dh3-r012-us-east-01a", "")
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// Spot-check encoding: value is JSON-encoded string.
	var v string
	for _, f := range got {
		if f.Key == "instance_type" {
			if err := json.Unmarshal(f.Value, &v); err != nil {
				t.Fatalf("unmarshal value: %v", err)
			}
			if v != "gb200-4x" {
				t.Fatalf("value = %q, want gb200-4x", v)
			}
		}
	}

	// Verify source_event_id stitching: every fact must point to the
	// FactsDiscovered event row, not RunCreated and not 0.
	var fdEventID int64
	if err := s.DB().QueryRowContext(t.Context(),
		`SELECT id FROM events WHERE run_id = ? AND kind = 'FactsDiscovered'`, runID).Scan(&fdEventID); err != nil {
		t.Fatalf("query FactsDiscovered event id: %v", err)
	}
	for _, f := range got {
		if f.SourceEventID != fdEventID {
			t.Errorf("fact %q source_event_id = %d, want %d", f.Key, f.SourceEventID, fdEventID)
		}
	}
}

func TestAppendEvent_FactsDiscoveryFailed_SetsRunStatusFailed(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000B"
	rcPayload, _ := json.Marshal(map[string]any{
		"template_id": "firmware-release-canary",
		"inputs":      map[string]any{"bundle_tag": "x", "canary_racks": []string{"dh3-r012-us-east-01a"}},
		"created_by":  "wpena",
	})
	if err := s.AppendEvent(t.Context(), runID, "RunCreated", rcPayload); err != nil {
		t.Fatalf("RunCreated: %v", err)
	}

	fdfPayload, _ := json.Marshal(map[string]any{
		"scope":      "run",
		"source":     "inventory",
		"error":      "racks not found in inventory",
		"unresolved": []string{"dh99-r999-us-east-99z"},
	})
	if err := s.AppendEvent(t.Context(), runID, "FactsDiscoveryFailed", fdfPayload); err != nil {
		t.Fatalf("FactsDiscoveryFailed: %v", err)
	}

	runs, err := s.ListRuns(t.Context())
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	var status string
	for _, r := range runs {
		if r.ID == runID {
			status = r.Status
			break
		}
	}
	if status != "failed" {
		t.Fatalf("status = %q, want failed", status)
	}
}

func TestAppendEvent_FactsDiscovered_MalformedPayload(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000C"
	rcPayload, _ := json.Marshal(map[string]any{
		"template_id": "firmware-release-canary",
		"inputs":      map[string]any{"bundle_tag": "x", "canary_racks": []string{"dh3-r012-us-east-01a"}},
		"created_by":  "wpena",
	})
	if err := s.AppendEvent(t.Context(), runID, "RunCreated", rcPayload); err != nil {
		t.Fatalf("RunCreated: %v", err)
	}

	if err := s.AppendEvent(t.Context(), runID, "FactsDiscovered", []byte(`{not json`)); err == nil {
		t.Fatal("AppendEvent FactsDiscovered with bad payload: want error, got nil")
	}

	// Confirm no facts row landed (transaction rolled back).
	got, _ := s.ListFacts(t.Context(), runID, "", "")
	if len(got) != 0 {
		t.Fatalf("len facts = %d, want 0 (rollback expected)", len(got))
	}
}

func TestAppendEvent_StepStarted_FlipsRunningAndDecodesPayload(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000STA1"
	rcPayload, _ := json.Marshal(map[string]any{
		"template_id": "firmware-release-canary",
		"inputs":      map[string]any{"bundle_tag": "x", "canary_racks": []string{"dh3-r012-us-east-01a"}},
		"created_by":  "wpena",
	})
	if err := s.AppendEvent(t.Context(), runID, "RunCreated", rcPayload); err != nil {
		t.Fatalf("RunCreated: %v", err)
	}

	ssPayload, _ := json.Marshal(map[string]any{
		"step_id":    "alert_baseline",
		"step_type":  "alert_probe",
		"racks":      []string{"dh3-r012-us-east-01a"},
		"started_at": 1700000000,
	})
	if err := s.AppendEvent(t.Context(), runID, "StepStarted", ssPayload); err != nil {
		t.Fatalf("StepStarted: %v", err)
	}

	runs, _ := s.ListRuns(t.Context())
	if len(runs) != 1 || runs[0].ID != runID {
		t.Fatalf("ListRuns: %+v", runs)
	}
	if runs[0].Status != "running" {
		t.Errorf("status = %q, want running", runs[0].Status)
	}
}

func TestAppendEvent_StepStarted_MalformedPayloadRollsBack(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000STA2"
	rcPayload, _ := json.Marshal(map[string]any{"template_id": "t", "inputs": map[string]any{"bundle_tag": "x", "canary_racks": []string{"dh3-r012-us-east-01a"}}})
	_ = s.AppendEvent(t.Context(), runID, "RunCreated", rcPayload)

	if err := s.AppendEvent(t.Context(), runID, "StepStarted", []byte(`{not json`)); err == nil {
		t.Fatal("AppendEvent StepStarted with bad payload: nil, want error")
	}

	// Confirm no StepStarted event landed and status stayed pending.
	evs, _ := s.LoadEvents(t.Context(), runID)
	for _, ev := range evs {
		if ev.Kind == "StepStarted" {
			t.Errorf("StepStarted event landed despite malformed payload")
		}
	}
	runs, _ := s.ListRuns(t.Context())
	if runs[0].Status != "pending" {
		t.Errorf("status = %q, want pending (rollback)", runs[0].Status)
	}
}

func TestAppendEvent_StepCompleted_Gatherer_NoVerdict(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000CMP1"
	rcPayload, _ := json.Marshal(map[string]any{"template_id": "t", "inputs": map[string]any{"bundle_tag": "x", "canary_racks": []string{"dh3-r012-us-east-01a"}}})
	_ = s.AppendEvent(t.Context(), runID, "RunCreated", rcPayload)

	ssPayload, _ := json.Marshal(map[string]any{"step_id": "alert_baseline", "step_type": "alert_probe"})
	_ = s.AppendEvent(t.Context(), runID, "StepStarted", ssPayload)

	scPayload, _ := json.Marshal(map[string]any{
		"step_id":      "alert_baseline",
		"evidence_id":  "01HZXEVIDENCE00000000001",
		"duration_ms":  42,
		"completed_at": 1700000001,
		"sources":      []string{"alertmanager-stub"},
	})
	if err := s.AppendEvent(t.Context(), runID, "StepCompleted", scPayload); err != nil {
		t.Fatalf("StepCompleted: %v", err)
	}

	runs, _ := s.ListRuns(t.Context())
	if runs[0].Status != "running" {
		t.Errorf("status = %q, want running (gatherer leaves run running)", runs[0].Status)
	}
}

func TestAppendEvent_StepFailed_SetsRunStatusFailed(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000FAI1"
	rcPayload, _ := json.Marshal(map[string]any{"template_id": "t", "inputs": map[string]any{"bundle_tag": "x", "canary_racks": []string{"dh3-r012-us-east-01a"}}})
	_ = s.AppendEvent(t.Context(), runID, "RunCreated", rcPayload)

	sfPayload, _ := json.Marshal(map[string]any{
		"step_id":     "alert_baseline",
		"error":       "alertclient: source unavailable",
		"duration_ms": 7,
		"failed_at":   1700000002,
	})
	if err := s.AppendEvent(t.Context(), runID, "StepFailed", sfPayload); err != nil {
		t.Fatalf("StepFailed: %v", err)
	}

	runs, _ := s.ListRuns(t.Context())
	if runs[0].Status != "failed" {
		t.Errorf("status = %q, want failed", runs[0].Status)
	}
}

func TestAppendEvent_StepCompleted_MalformedPayloadRollsBack(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000CMP2"
	rcPayload, _ := json.Marshal(map[string]any{"template_id": "t", "inputs": map[string]any{"bundle_tag": "x", "canary_racks": []string{"dh3-r012-us-east-01a"}}})
	_ = s.AppendEvent(t.Context(), runID, "RunCreated", rcPayload)

	// Land a StepStarted so status is "running" before the malformed StepCompleted.
	ssPayload, _ := json.Marshal(map[string]any{"step_id": "alert_baseline"})
	_ = s.AppendEvent(t.Context(), runID, "StepStarted", ssPayload)

	if err := s.AppendEvent(t.Context(), runID, "StepCompleted", []byte(`{not json`)); err == nil {
		t.Fatal("AppendEvent StepCompleted with bad payload: nil, want error")
	}

	// No StepCompleted event landed; status stays running.
	evs, _ := s.LoadEvents(t.Context(), runID)
	for _, ev := range evs {
		if ev.Kind == "StepCompleted" {
			t.Errorf("StepCompleted event landed despite malformed payload")
		}
	}
	runs, _ := s.ListRuns(t.Context())
	if runs[0].Status != "running" {
		t.Errorf("status = %q, want running (rollback)", runs[0].Status)
	}
}

func TestAppendEvent_StepFailed_MalformedPayloadRollsBack(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000FAI2"
	rcPayload, _ := json.Marshal(map[string]any{"template_id": "t", "inputs": map[string]any{"bundle_tag": "x", "canary_racks": []string{"dh3-r012-us-east-01a"}}})
	_ = s.AppendEvent(t.Context(), runID, "RunCreated", rcPayload)

	if err := s.AppendEvent(t.Context(), runID, "StepFailed", []byte(`{not json`)); err == nil {
		t.Fatal("AppendEvent StepFailed with bad payload: nil, want error")
	}

	// No StepFailed event landed; status stays pending.
	evs, _ := s.LoadEvents(t.Context(), runID)
	for _, ev := range evs {
		if ev.Kind == "StepFailed" {
			t.Errorf("StepFailed event landed despite malformed payload")
		}
	}
	runs, _ := s.ListRuns(t.Context())
	if runs[0].Status != "pending" {
		t.Errorf("status = %q, want pending (rollback)", runs[0].Status)
	}
}

func TestAppendEvent_StepFailed_AfterSignoff_DoesNotOverride(t *testing.T) {
	// The store-level projection must respect operator signoff just like
	// engine.Project does. A StepFailed event arriving after a signoff
	// must not flip status away from signed_off.
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000FAI3"
	rcPayload, _ := json.Marshal(map[string]any{"template_id": "t", "inputs": map[string]any{"bundle_tag": "x", "canary_racks": []string{"dh3-r012-us-east-01a"}}})
	_ = s.AppendEvent(t.Context(), runID, "RunCreated", rcPayload)

	// Force the runs row to signed_off directly. (iter-5i added the
	// OperatorAction signoff case to the projection; we set the state
	// directly here so the test exercises the StepFailed SQL guard in
	// isolation, independent of the OperatorAction path.)
	if _, err := s.DB().ExecContext(t.Context(),
		`UPDATE runs SET status = 'signed_off' WHERE id = ?`, runID); err != nil {
		t.Fatalf("seed signed_off: %v", err)
	}

	sfPayload, _ := json.Marshal(map[string]any{"step_id": "x", "error": "boom"})
	if err := s.AppendEvent(t.Context(), runID, "StepFailed", sfPayload); err != nil {
		t.Fatalf("StepFailed: %v", err)
	}

	runs, _ := s.ListRuns(t.Context())
	if runs[0].Status != "signed_off" {
		t.Errorf("status = %q, want signed_off (StepFailed must not override signoff)", runs[0].Status)
	}

	// Confirm the StepFailed event was still recorded (only the projection is guarded).
	evs, _ := s.LoadEvents(t.Context(), runID)
	hasSF := false
	for _, ev := range evs {
		if ev.Kind == "StepFailed" {
			hasSF = true
		}
	}
	if !hasSF {
		t.Errorf("StepFailed event was not recorded; expected event row to persist even when projection is guarded")
	}
}

func TestAppendEvent_BMNDiscovered_ProjectsRackDeviceslots(t *testing.T) {
	s := openTestStore(t)
	runID := mustSeedRun(t, s)
	payload, _ := json.Marshal(map[string]any{
		"rack":          "dh3-r012-us-east-01a",
		"zone":          "us-east-01a",
		"source":        "inventory",
		"discovered_at": int64(1709337600),
		"bmns": []map[string]any{
			{"deviceslot": "dh3-r012-node-01-us-east-01a", "bmn_name": "s90txs51", "ct_position": 1},
			{"deviceslot": "dh3-r012-node-02-us-east-01a", "bmn_name": "s90txs52", "ct_position": 2},
		},
	})
	if err := s.AppendEvent(context.Background(), runID, "BMNDiscovered", payload); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	row := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM rack_deviceslots WHERE run_id = ?`, runID)
	var n int
	_ = row.Scan(&n)
	if n != 2 {
		t.Errorf("rack_deviceslots count = %d, want 2", n)
	}
}

func TestAppendEvent_BMNIgnored_ProjectsBmnIgnores(t *testing.T) {
	s := openTestStore(t)
	runID := mustSeedRun(t, s)
	payload, _ := json.Marshal(map[string]any{
		"rack":       "dh3-r012-us-east-01a",
		"deviceslot": "dh3-r012-node-07-us-east-01a",
		"source":     "operator",
		"reason":     "known bad GPU",
		"marked_at":  int64(1709337600),
	})
	if err := s.AppendEvent(context.Background(), runID, "BMNIgnored", payload); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	row := s.db.QueryRowContext(context.Background(),
		`SELECT source, reason FROM bmn_ignores WHERE run_id = ? AND deviceslot = ?`,
		runID, "dh3-r012-node-07-us-east-01a")
	var source, reason string
	if err := row.Scan(&source, &reason); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if source != "operator" {
		t.Errorf("source = %q, want operator", source)
	}
	if reason != "known bad GPU" {
		t.Errorf("reason = %q, want 'known bad GPU'", reason)
	}
}

func TestAppendEvent_BMNIgnored_DualSourceCoexists(t *testing.T) {
	// Same (run_id, deviceslot) can carry both operator + rlcc rows.
	s := openTestStore(t)
	runID := mustSeedRun(t, s)
	slot := "dh3-r012-node-07-us-east-01a"
	for _, src := range []string{"operator", "rlcc"} {
		payload, _ := json.Marshal(map[string]any{
			"rack":       "dh3-r012-us-east-01a",
			"deviceslot": slot,
			"source":     src,
			"marked_at":  int64(1709337600),
		})
		if err := s.AppendEvent(context.Background(), runID, "BMNIgnored", payload); err != nil {
			t.Fatalf("AppendEvent (%s): %v", src, err)
		}
	}
	row := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM bmn_ignores WHERE run_id = ? AND deviceslot = ?`, runID, slot)
	var n int
	_ = row.Scan(&n)
	if n != 2 {
		t.Errorf("bmn_ignores count = %d, want 2 (operator + rlcc)", n)
	}
}

func TestAppendEvent_BMNIgnored_SameSourceUpserts(t *testing.T) {
	// Re-emitting the same (run, deviceslot, source) updates marked_at.
	s := openTestStore(t)
	runID := mustSeedRun(t, s)
	slot := "dh3-r012-node-07-us-east-01a"
	for _, ts := range []int64{1709337600, 1709337900} {
		payload, _ := json.Marshal(map[string]any{
			"rack":       "dh3-r012-us-east-01a",
			"deviceslot": slot,
			"source":     "rlcc",
			"marked_at":  ts,
		})
		if err := s.AppendEvent(context.Background(), runID, "BMNIgnored", payload); err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
	}
	row := s.db.QueryRowContext(context.Background(),
		`SELECT marked_at FROM bmn_ignores WHERE run_id = ? AND deviceslot = ? AND source = 'rlcc'`,
		runID, slot)
	var ts int64
	_ = row.Scan(&ts)
	if ts != 1709337900 {
		t.Errorf("marked_at = %d, want 1709337900 (later wins)", ts)
	}
}

func TestAppendEvent_BMNDiscoveryFailed_MarksRunFailed(t *testing.T) {
	s := openTestStore(t)
	runID := mustSeedRun(t, s)
	payload, _ := json.Marshal(map[string]any{
		"rack":    "dh3-r012-us-east-01a",
		"source":  "inventory",
		"message": "no such rack",
	})
	if err := s.AppendEvent(context.Background(), runID, "BMNDiscoveryFailed", payload); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	row := s.db.QueryRowContext(context.Background(),
		`SELECT status FROM runs WHERE id = ?`, runID)
	var status string
	_ = row.Scan(&status)
	if status != "failed" {
		t.Errorf("status = %q, want failed", status)
	}
}

func TestAppendEvent_FactsDiscovered_PreservesUnknownKinds(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000D"
	rcPayload, _ := json.Marshal(map[string]any{
		"template_id": "firmware-release-canary",
		"inputs":      map[string]any{"bundle_tag": "x", "canary_racks": []string{"dh3-r012-us-east-01a"}},
		"created_by":  "wpena",
	})
	if err := s.AppendEvent(t.Context(), runID, "RunCreated", rcPayload); err != nil {
		t.Fatalf("RunCreated: %v", err)
	}

	// Unknown kind — projection ignores; event still inserts.
	if err := s.AppendEvent(t.Context(), runID, "SomeFutureKind", []byte(`{"k":"v"}`)); err != nil {
		t.Fatalf("unknown kind AppendEvent: %v", err)
	}
	evs, err := s.LoadEvents(t.Context(), runID)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("len events = %d, want 2", len(evs))
	}
}

func TestRunCreatedPayload_LegacyRoundTrip(t *testing.T) {
	// A pre-iter-5d payload (no rlcc_workflow field) must decode cleanly
	// with the new struct shape — RLCCWorkflow stays nil.
	body := []byte(`{
		"template_id": "firmware-release-canary",
		"bundle": "gb200-fw-2026-05-canary-3",
		"rack": "dh3-r012-us-east-01a",
		"created_by": "wpena",
		"inputs": {"bundle_tag":"gb200-fw-2026-05-canary-3","canary_racks":["dh3-r012-us-east-01a"]}
	}`)
	var p runCreatedPayload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("Unmarshal legacy: %v", err)
	}
	if p.TemplateID != "firmware-release-canary" {
		t.Errorf("TemplateID = %q, want firmware-release-canary", p.TemplateID)
	}
	if p.RLCCWorkflow != nil {
		t.Errorf("RLCCWorkflow = %v, want nil (legacy payload)", p.RLCCWorkflow)
	}
}

// mustAppend marshals payload and appends an event, fataling on error.
func mustAppend(t *testing.T, s *Store, ctx context.Context, runID, kind string, payload map[string]any) {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := s.AppendEvent(ctx, runID, kind, b); err != nil {
		t.Fatalf("AppendEvent(%s): %v", kind, err)
	}
}

// runStatus returns the status of the run with the given ID, fataling if not found.
func runStatus(t *testing.T, s *Store, ctx context.Context, runID string) string {
	t.Helper()
	runs, err := s.ListRuns(ctx)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	for _, r := range runs {
		if r.ID == runID {
			return r.Status
		}
	}
	t.Fatalf("run %q not found in ListRuns", runID)
	return ""
}

func TestProjection_StepCompletedVerdict_SetsStatus(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	runID := "01HZX000000000000000VRD1"
	mustAppend(t, s, ctx, runID, "RunCreated", map[string]any{"template_id": "t", "created_by": "x"})
	mustAppend(t, s, ctx, runID, "StepStarted", map[string]any{"step_id": "s1"})
	mustAppend(t, s, ctx, runID, "StepCompleted", map[string]any{"step_id": "s0"}) // empty verdict
	if got := runStatus(t, s, ctx, runID); got != "running" {
		t.Errorf("empty-verdict StepCompleted: status=%q want running", got)
	}
	mustAppend(t, s, ctx, runID, "StepCompleted", map[string]any{"step_id": "s1", "verdict": "passed"})
	if got := runStatus(t, s, ctx, runID); got != "passed" {
		t.Errorf("passed: status=%q want passed", got)
	}
	mustAppend(t, s, ctx, runID, "StepCompleted", map[string]any{"step_id": "s2", "verdict": "failed"})
	if got := runStatus(t, s, ctx, runID); got != "failed" {
		t.Errorf("failed: status=%q want failed", got)
	}
	mustAppend(t, s, ctx, runID, "StepCompleted", map[string]any{"step_id": "s3", "verdict": "passed"})
	if got := runStatus(t, s, ctx, runID); got != "failed" {
		t.Errorf("worst-wins broken: status=%q want failed", got)
	}
}

func TestProjection_OperatorSignoff_SetsSignedOff(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	runID := "01HZX000000000000000SGN1"
	mustAppend(t, s, ctx, runID, "RunCreated", map[string]any{"template_id": "t", "created_by": "x"})
	mustAppend(t, s, ctx, runID, "OperatorAction", map[string]any{"action": "signoff"})
	if got := runStatus(t, s, ctx, runID); got != "signed_off" {
		t.Errorf("signoff: status=%q want signed_off", got)
	}
	mustAppend(t, s, ctx, runID, "StepFailed", map[string]any{"step_id": "s9", "error": "x"})
	if got := runStatus(t, s, ctx, runID); got != "signed_off" {
		t.Errorf("signed_off not terminal: status=%q", got)
	}
}

func TestRunCreatedPayload_Iter5dRoundTrip(t *testing.T) {
	// An iter-5d payload (with rlcc_workflow) must decode cleanly and
	// preserve the raw bytes — store doesn't parse the snapshot, it
	// just passes it through.
	body := []byte(`{
		"template_id": "firmware-release-canary",
		"created_by": "wpena",
		"inputs": {"bundle_tag":"x","canary_racks":["dh3-r012-us-east-01a"],"rlcc_workflow":"gb200-rack-bringup-v4"},
		"rlcc_workflow": {"name":"gb200-rack-bringup-v4","source_sha":"abc","actions":[{"name":"a1","handler":"hlcc-state"}]}
	}`)
	var p runCreatedPayload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("Unmarshal iter-5d: %v", err)
	}
	if p.RLCCWorkflow == nil {
		t.Fatal("RLCCWorkflow = nil, want non-nil")
	}
	// passthrough: the raw bytes should include the workflow name.
	if !bytes.Contains(*p.RLCCWorkflow, []byte("gb200-rack-bringup-v4")) {
		t.Errorf("RLCCWorkflow raw bytes missing workflow name: %s", string(*p.RLCCWorkflow))
	}
}
