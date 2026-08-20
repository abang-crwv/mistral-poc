package facts

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"qac/internal/store"
)

func openStoreWithRun(t *testing.T, runID string) *store.Store {
	t.Helper()
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	rc, _ := json.Marshal(map[string]any{
		"template_id": "firmware-release-canary",
		"inputs":      map[string]any{"bundle_tag": "x", "canary_racks": []string{"dh3-r012-us-east-01a"}},
		"created_by":  "wpena",
	})
	if err := s.AppendEvent(t.Context(), runID, "RunCreated", rc); err != nil {
		t.Fatalf("seed RunCreated: %v", err)
	}
	return s
}

func TestEmit_HappyPath(t *testing.T) {
	runID := "01HZX0000000000000000000A"
	s := openStoreWithRun(t, runID)

	if err := Emit(context.Background(), s, runID, "run", "operator", map[Key]any{
		KeyBundleTag: "gb200-fw-2026-05-canary-3",
		KeyRequester: "alice",
	}); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	facts, err := s.ListFacts(t.Context(), runID, "run", "")
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("len = %d, want 2", len(facts))
	}
}

func TestEmit_UnknownKey(t *testing.T) {
	runID := "01HZX0000000000000000000B"
	s := openStoreWithRun(t, runID)

	err := Emit(context.Background(), s, runID, "run", "operator", map[Key]any{
		Key("ghost_key"): "x",
	})
	if !errors.Is(err, ErrUnknownFactKey) {
		t.Fatalf("err = %v, want ErrUnknownFactKey", err)
	}

	// No facts landed.
	facts, _ := s.ListFacts(t.Context(), runID, "", "")
	if len(facts) != 0 {
		t.Fatalf("len facts = %d, want 0", len(facts))
	}
}

func TestEmit_ReEmitOverwrites(t *testing.T) {
	runID := "01HZX0000000000000000000C"
	s := openStoreWithRun(t, runID)

	if err := Emit(context.Background(), s, runID, "run", "operator", map[Key]any{
		KeyBundleTag: "v1",
	}); err != nil {
		t.Fatalf("first Emit: %v", err)
	}
	if err := Emit(context.Background(), s, runID, "run", "operator", map[Key]any{
		KeyBundleTag: "v2",
	}); err != nil {
		t.Fatalf("second Emit: %v", err)
	}

	facts, _ := s.ListFacts(t.Context(), runID, "run", "")
	if len(facts) != 1 {
		t.Fatalf("len = %d, want 1 (overwrite)", len(facts))
	}
	var v string
	_ = json.Unmarshal(facts[0].Value, &v)
	if v != "v2" {
		t.Fatalf("value = %q, want v2", v)
	}
}

func TestEmit_JSONEncodingRoundTrip(t *testing.T) {
	runID := "01HZX0000000000000000000D"
	s := openStoreWithRun(t, runID)

	if err := Emit(context.Background(), s, runID, "run", "operator", map[Key]any{
		KeyBundleTag: "string-value",
	}); err != nil {
		t.Fatalf("Emit string: %v", err)
	}

	facts, _ := s.ListFacts(t.Context(), runID, "run", "")
	if len(facts) != 1 {
		t.Fatalf("len = %d, want 1", len(facts))
	}
	var s1 string
	if err := json.Unmarshal(facts[0].Value, &s1); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s1 != "string-value" {
		t.Fatalf("got %q", s1)
	}
}

func TestEmit_EmptyBatchIsNoOp(t *testing.T) {
	runID := "01HZX0000000000000000000E"
	s := openStoreWithRun(t, runID)

	beforeEvents, _ := s.LoadEvents(t.Context(), runID)

	if err := Emit(context.Background(), s, runID, "run", "operator", map[Key]any{}); err != nil {
		t.Fatalf("Emit empty: %v", err)
	}

	afterEvents, _ := s.LoadEvents(t.Context(), runID)
	if len(afterEvents) != len(beforeEvents) {
		t.Fatalf("events grew from %d to %d on empty Emit; want no-op",
			len(beforeEvents), len(afterEvents))
	}
}

func TestEmit_PartialFailureRollsBack(t *testing.T) {
	runID := "01HZX0000000000000000000F"
	s := openStoreWithRun(t, runID)

	// All keys must be valid for the batch to succeed; one bad key kills
	// the whole batch (no facts landed, no event landed).
	err := Emit(context.Background(), s, runID, "run", "operator", map[Key]any{
		KeyBundleTag:     "good-value",
		Key("not_a_key"): "bad-key",
	})
	if !errors.Is(err, ErrUnknownFactKey) {
		t.Fatalf("err = %v, want ErrUnknownFactKey", err)
	}

	facts, _ := s.ListFacts(t.Context(), runID, "", "")
	if len(facts) != 0 {
		t.Fatalf("len facts = %d, want 0 (rollback)", len(facts))
	}
}
