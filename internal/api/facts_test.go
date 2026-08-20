package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"qac/internal/store"
)

// seedRunWithFacts is a test helper that creates a run via direct
// store calls (bypassing the HTTP layer so we don't depend on the
// createRunHandler discovery logic from Task 12). It appends RunCreated,
// then FactsDiscovered for one run-scope op fact and two rack-scope
// inventory facts.
func seedRunWithFacts(t *testing.T, s *store.Store, runID string) {
	t.Helper()
	rcPayload, _ := json.Marshal(map[string]any{
		"template_id": "firmware-release-canary",
		"inputs":      map[string]any{"bundle_tag": "x", "canary_racks": []string{"dh3-r012-us-east-01a"}},
		"created_by":  "wpena",
	})
	if err := s.AppendEvent(context.Background(), runID, "RunCreated", rcPayload); err != nil {
		t.Fatalf("seed RunCreated: %v", err)
	}

	opPayload, _ := json.Marshal(map[string]any{
		"scope":  "run",
		"source": "operator",
		"facts":  map[string]any{"bundle_tag": "gb200-fw-2026-05-canary-3"},
	})
	if err := s.AppendEvent(context.Background(), runID, "FactsDiscovered", opPayload); err != nil {
		t.Fatalf("seed operator FactsDiscovered: %v", err)
	}

	invPayload, _ := json.Marshal(map[string]any{
		"scope":  "rack:dh3-r012-us-east-01a",
		"source": "inventory",
		"facts": map[string]any{
			"instance_type": "gb200-4x",
			"sku":           "nvl72",
		},
	})
	if err := s.AppendEvent(context.Background(), runID, "FactsDiscovered", invPayload); err != nil {
		t.Fatalf("seed inventory FactsDiscovered: %v", err)
	}
}

func TestGetFacts_Happy(t *testing.T) {
	srv, s, _ := newTestServer(t)
	runID := "01HZX0000000000000000000A"
	seedRunWithFacts(t, s, runID)

	res, err := http.Get(srv.URL + "/api/runs/" + runID + "/facts")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct {
		Facts []struct {
			Scope         string          `json:"scope"`
			Key           string          `json:"key"`
			Value         json.RawMessage `json:"value"`
			Source        string          `json:"source"`
			SourceEventID int64           `json:"source_event_id"`
			DiscoveredAt  int64           `json:"discovered_at"`
		} `json:"facts"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Facts) != 3 {
		t.Fatalf("len = %d, want 3 (1 operator + 2 inventory)", len(body.Facts))
	}
}

func TestGetFacts_UnknownRun(t *testing.T) {
	srv, _, _ := newTestServer(t)
	res, err := http.Get(srv.URL + "/api/runs/01HZX9999999999999999999Z/facts")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
	var env struct {
		Error struct{ Code string } `json:"error"`
	}
	_ = json.NewDecoder(res.Body).Decode(&env)
	if env.Error.Code != "not_found" {
		t.Fatalf("code = %q, want not_found", env.Error.Code)
	}
}

func TestGetFacts_ScopeFilter(t *testing.T) {
	srv, s, _ := newTestServer(t)
	runID := "01HZX0000000000000000000B"
	seedRunWithFacts(t, s, runID)

	res, err := http.Get(srv.URL + "/api/runs/" + runID + "/facts?scope=run")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	var body struct {
		Facts []struct{ Scope string } `json:"facts"`
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	if len(body.Facts) != 1 {
		t.Fatalf("len = %d, want 1 (run-scope only)", len(body.Facts))
	}
	if body.Facts[0].Scope != "run" {
		t.Errorf("scope = %q, want run", body.Facts[0].Scope)
	}
}

func TestGetFacts_SourceFilter(t *testing.T) {
	srv, s, _ := newTestServer(t)
	runID := "01HZX0000000000000000000C"
	seedRunWithFacts(t, s, runID)

	res, err := http.Get(srv.URL + "/api/runs/" + runID + "/facts?source=inventory")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	var body struct {
		Facts []struct {
			Source string
		} `json:"facts"`
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	if len(body.Facts) != 2 {
		t.Fatalf("len = %d, want 2 (inventory-only)", len(body.Facts))
	}
}

func TestGetFacts_RackWildcardScope(t *testing.T) {
	srv, s, _ := newTestServer(t)
	runID := "01HZX0000000000000000000D"
	seedRunWithFacts(t, s, runID)

	u := srv.URL + "/api/runs/" + runID + "/facts?" + url.Values{"scope": {"rack:*"}}.Encode()
	res, err := http.Get(u)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	var body struct {
		Facts []struct{ Scope string } `json:"facts"`
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	if len(body.Facts) != 2 {
		t.Fatalf("len = %d, want 2 (rack-scope only)", len(body.Facts))
	}
}
