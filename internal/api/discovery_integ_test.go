package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestDiscovery_HappyPath_EndToEnd posts a run against a rack present in
// SeedDemoFixtures, then GETs /facts and confirms the event trail +
// fact projection match the spec contract end-to-end.
func TestDiscovery_HappyPath_EndToEnd(t *testing.T) {
	srv, s, eng := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)

	body := `{
		"template_id": "firmware-release-canary",
		"inputs": {
			"bundle_tag": "gb200-fw-2026-05-canary-3",
			"canary_racks": ["dh3-r012-us-east-01a"]
		}
	}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	var created struct {
		Run struct{ ID string } `json:"run"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// iter-4b: engine fires alert_probe after POST. Wait so it doesn't
	// race with the assertions below.
	select {
	case <-eng.Wait(created.Run.ID):
	case <-time.After(2 * time.Second):
		t.Fatal("engine did not finish run")
	}

	// 1. Event trail: RunCreated, operator FactsDiscovered, inventory FactsDiscovered.
	//    iter-4b engine appends StepStarted/StepCompleted after the initial 3 — assert
	//    at least 3 and check the first three kinds only.
	evs, _ := s.LoadEvents(context.Background(), created.Run.ID)
	if len(evs) < 3 {
		t.Fatalf("len events = %d, want at least 3", len(evs))
	}
	if evs[0].Kind != "RunCreated" {
		t.Errorf("event[0] = %q, want RunCreated", evs[0].Kind)
	}
	// 2 FactsDiscovered: one operator, one inventory.
	if evs[1].Kind != "FactsDiscovered" || evs[2].Kind != "FactsDiscovered" {
		t.Errorf("events[1..2] = %q, %q; want both FactsDiscovered", evs[1].Kind, evs[2].Kind)
	}

	// 2. GET /facts returns 7 facts.
	res2, err := http.Get(srv.URL + "/api/runs/" + created.Run.ID + "/facts")
	if err != nil {
		t.Fatalf("GET facts: %v", err)
	}
	defer res2.Body.Close()
	var fb struct {
		Facts []struct {
			Scope, Key, Source string
			Value              json.RawMessage
		} `json:"facts"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&fb); err != nil {
		t.Fatalf("decode facts: %v", err)
	}
	if len(fb.Facts) != 7 {
		t.Fatalf("len facts = %d, want 7", len(fb.Facts))
	}

	// 3. Values arrive as parsed JSON, not double-encoded strings.
	for _, f := range fb.Facts {
		if f.Key == "instance_type" {
			var s string
			if err := json.Unmarshal(f.Value, &s); err != nil {
				t.Fatalf("unmarshal value: %v", err)
			}
			if s != "gb200-4x" {
				t.Errorf("instance_type = %q, want gb200-4x", s)
			}
		}
	}
}

// TestDiscovery_UnresolvedRack_EndToEnd posts a run with an unknown rack,
// asserts the 422 with details, and confirms the run row + event trail
// + (empty) rack-scope facts match the spec.
func TestDiscovery_UnresolvedRack_EndToEnd(t *testing.T) {
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)

	body := `{"bundle":"x","rack":"dh99-r999-us-east-99z"}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 422 {
		t.Fatalf("status = %d, want 422", res.StatusCode)
	}

	runs, _ := s.ListRuns(context.Background())
	if len(runs) != 1 {
		t.Fatalf("len runs = %d, want 1", len(runs))
	}
	run := runs[0]
	if run.Status != "failed" {
		t.Errorf("status = %q, want failed", run.Status)
	}

	evs, _ := s.LoadEvents(context.Background(), run.ID)
	if len(evs) != 3 {
		t.Fatalf("len events = %d, want 3", len(evs))
	}
	wantKinds := []string{"RunCreated", "FactsDiscovered", "FactsDiscoveryFailed"}
	for i, ev := range evs {
		if ev.Kind != wantKinds[i] {
			t.Errorf("event[%d] = %q, want %q", i, ev.Kind, wantKinds[i])
		}
	}

	rackFacts, _ := s.ListFacts(context.Background(), run.ID, "rack:*", "")
	if len(rackFacts) != 0 {
		t.Errorf("rack facts = %d, want 0", len(rackFacts))
	}

	// Operator fact still landed (the bundle_tag at run scope).
	opFacts, _ := s.ListFacts(context.Background(), run.ID, "run", "operator")
	if len(opFacts) != 1 {
		t.Errorf("op facts = %d, want 1 (bundle_tag)", len(opFacts))
	}
}
