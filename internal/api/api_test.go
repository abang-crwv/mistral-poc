package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"qac/internal/alertcategoryclient"
	"qac/internal/alertclient"
	"qac/internal/engine"
	"qac/internal/firmwareclient"
	"qac/internal/flccclient"
	"qac/internal/inventoryclient"
	"qac/internal/lifecycleclient"
	"qac/internal/probe"
	"qac/internal/probe/alertprobe"
	"qac/internal/probe/firmwareinventoryprobe"
	"qac/internal/probe/rlccactionprobe"
	"qac/internal/rlccclient"
	"qac/internal/store"
)

// newTestServer is the shared test harness. iter-4b grows it to
// construct a fully-wired Engine (alertprobe registered, MapAlertClient
// + InventoryResolver injected) and return it as the third value so
// tests can drive Wait/Shutdown directly.
func newTestServer(t *testing.T) (*httptest.Server, *store.Store, *engine.Engine) {
	t.Helper()
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	resolver := inventoryclient.NewMapResolverWithBMNs(inventoryclient.SeedDemoFixtures(), inventoryclient.SeedDemoBMNs())
	lifeC := lifecycleclient.NewMapClient(nil)
	reg := probe.NewRegistry()
	reg.Register(alertprobe.New())
	reg.Register(firmwareinventoryprobe.New())
	reg.Register(rlccactionprobe.New())
	eng := engine.New(s, reg, probe.Clients{
		AlertClient:         alertclient.NewMapAlertClient(alertclient.SeedDemoAlerts(), nil),
		AlertCategoryClient: alertcategoryclient.NewMapClient(alertcategoryclient.SeedDemoCategories()),
		FirmwareClient:      firmwareclient.NewMapClient(firmwareclient.SeedDemoFirmware()),
		InventoryResolver:   resolver,
		LifecycleClient:     lifeC,
		EvidenceWriter:      s,
		EventEmitter:        probe.NewStoreEmitter(s),
	})
	srv := httptest.NewServer(Router(s, "test", resolver, eng, rlccclient.NewMapClient(), flccclient.NewMapClient(), false, func() {}, lifeC))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = eng.Shutdown(ctx)
	})
	t.Cleanup(srv.Close)
	return srv, s, eng
}

// upsertCanonicalCanaryTemplate inserts a minimal valid fw-release-canary
// template into the store so POST /api/runs can find it. The shape mirrors
// the canonical inputs (bundle_tag + canary_racks) so the legacy {bundle,
// rack} → inputs normalization in createRunHandler can validate inputs.
func upsertCanonicalCanaryTemplate(t *testing.T, s *store.Store) {
	t.Helper()
	yamlBody := `schema: qac.template/v1
id: fw-release-canary
version: 1
title: "FW release canary"
target:
  scope: rack
  instance_types: [gb200-4x]
inputs:
  - id: bundle_tag
    label: "FW bundle tag"
    type: text
    required: true
  - id: canary_racks
    label: "Canary racks"
    type: multi_text
    required: true
    validate: "^dh\\d+-r\\d{3}-[a-z]+-[a-z]+-\\d{2}[a-z]$"
steps:
  - id: noop
    kind: probe
    title: "no-op"
    action:
      tool: metrics
      timeout: 5m
    checkpoint:
      targets: per_rack
      success_when:
        stdout_contains: "ok"
`
	parsed := `{
		"id": "firmware-release-canary",
		"version": 1,
		"title": "FW release canary",
		"inputs": [
			{"id":"bundle_tag","label":"FW bundle tag","type":"text","required":true},
			{"id":"canary_racks","label":"Canary racks","type":"multi_text","required":true,"validate":"^dh\\d+-r\\d{3}-[a-z]+-[a-z]+-\\d{2}[a-z]$"}
		]
	}`
	if err := s.UpsertTemplate(context.Background(), "firmware-release-canary", 1, yamlBody, parsed); err != nil {
		t.Fatalf("UpsertTemplate: %v", err)
	}
}

func TestGetHealth_OK(t *testing.T) {
	srv, _, _ := newTestServer(t)

	res, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
		DBPath  string `json:"db_path"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.OK {
		t.Fatal("ok = false")
	}
}

func TestGetRuns_EmptyWhenNoRuns(t *testing.T) {
	srv, _, _ := newTestServer(t)

	res, err := http.Get(srv.URL + "/api/runs")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct {
		Runs []any `json:"runs"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Runs) != 0 {
		t.Fatalf("got %d runs, want 0", len(body.Runs))
	}
}

func TestGetRuns_ReturnsSeededList(t *testing.T) {
	srv, s, _ := newTestServer(t)
	payload, _ := json.Marshal(map[string]string{
		"template_id": "firmware-release-canary",
		"rack":        "as1-r07",
		"created_by":  "wpena",
	})
	if err := s.AppendEvent(context.Background(), "01HZX0000000000000000000A", "RunCreated", payload); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	res, err := http.Get(srv.URL + "/api/runs")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

func TestGetRunByID_ReturnsRunAndEvents(t *testing.T) {
	srv, s, _ := newTestServer(t)
	runID := "01HZX0000000000000000000A"
	payload, _ := json.Marshal(map[string]string{"template_id": "t", "rack": "as1-r07", "created_by": "wpena"})
	if err := s.AppendEvent(context.Background(), runID, "RunCreated", payload); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	res, err := http.Get(srv.URL + "/api/runs/" + runID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

func TestGetRunByID_NotFound(t *testing.T) {
	srv, _, _ := newTestServer(t)

	res, err := http.Get(srv.URL + "/api/runs/does-not-exist")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
}

// === POST /api/runs — iter-3a evolution ===

func TestCreateRun_LegacyShape_HappyPath(t *testing.T) {
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)

	body := `{"bundle":"gb200-fw-2026-05-canary-3","rack":"dh3-r012-us-east-01a"}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	var out struct {
		Run struct {
			ID, Bundle, Rack, Status string
			Inputs                   json.RawMessage `json:"inputs"`
		} `json:"run"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Run.Bundle != "gb200-fw-2026-05-canary-3" {
		t.Fatalf("bundle = %q", out.Run.Bundle)
	}
	if out.Run.Rack != "dh3-r012-us-east-01a" {
		t.Fatalf("rack = %q", out.Run.Rack)
	}
	if !strings.Contains(string(out.Run.Inputs), "bundle_tag") {
		t.Fatalf("inputs should mention bundle_tag; got %s", string(out.Run.Inputs))
	}
}

func TestCreateRun_LegacyShape_BadRack(t *testing.T) {
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)

	body := `{"bundle":"b","rack":"as1-r07"}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestCreateRun_LegacyShape_EmptyBundle(t *testing.T) {
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)

	body := `{"bundle":"","rack":"dh3-r012-us-east-01a"}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestCreateRun_NewShape_HappyPath(t *testing.T) {
	srv, s, _ := newTestServer(t)
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
}

func TestCreateRun_NewShape_UnknownTemplate(t *testing.T) {
	srv, _, _ := newTestServer(t)

	body := `{"template_id":"ghost","inputs":{}}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 422 {
		t.Fatalf("status = %d, want 422", res.StatusCode)
	}
	var env struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error.Code != "template_not_found" {
		t.Fatalf("code = %q, want template_not_found", env.Error.Code)
	}
}

func TestCreateRun_NewShape_MissingRequiredInput(t *testing.T) {
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)

	body := `{
		"template_id": "firmware-release-canary",
		"inputs": { "bundle_tag": "x" }
	}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
	var env struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(env.Error.Message, "canary_racks") {
		t.Fatalf("message = %q, want mention of canary_racks", env.Error.Message)
	}
}

func TestCreateRun_NewShape_WrongInputType(t *testing.T) {
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)

	body := `{
		"template_id": "firmware-release-canary",
		"inputs": { "bundle_tag": 42, "canary_racks": ["dh3-r012-us-east-01a"] }
	}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestCreateRun_NewShape_BadRackRegex(t *testing.T) {
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)

	body := `{
		"template_id": "firmware-release-canary",
		"inputs": { "bundle_tag": "x", "canary_racks": ["not-a-rack"] }
	}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestCreateRun_BothShapesPresent_400(t *testing.T) {
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)

	body := `{
		"bundle": "x",
		"rack": "dh3-r012-us-east-01a",
		"template_id": "firmware-release-canary",
		"inputs": {}
	}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestCreateRun_MalformedJSON(t *testing.T) {
	srv, _, _ := newTestServer(t)

	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(`{not json`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

// === iter-4a — fact discovery on POST /api/runs ===

func TestCreateRun_EmitsOperatorAndInventoryFacts(t *testing.T) {
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)

	body := `{"bundle":"gb200-fw-2026-05-canary-3","rack":"dh3-r012-us-east-01a"}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	var out struct {
		Run struct{ ID string } `json:"run"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	facts, err := s.ListFacts(context.Background(), out.Run.ID, "", "")
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	// Expect: 1 operator (bundle_tag) + 6 inventory (instance_type, sku,
	// variant, gb_generation, region, cluster) = 7
	if len(facts) != 7 {
		t.Fatalf("len facts = %d, want 7", len(facts))
	}

	// Spot-check: one operator fact for bundle_tag.
	opFacts, _ := s.ListFacts(context.Background(), out.Run.ID, "run", "operator")
	if len(opFacts) != 1 || opFacts[0].Key != "bundle_tag" {
		t.Errorf("op facts = %+v, want one bundle_tag", opFacts)
	}

	// Spot-check: six rack facts.
	rackFacts, _ := s.ListFacts(context.Background(), out.Run.ID, "rack:dh3-r012-us-east-01a", "inventory")
	if len(rackFacts) != 6 {
		t.Errorf("rack facts = %d, want 6", len(rackFacts))
	}
}

func TestCreateRun_UnresolvedRack_Returns422AndFailsRun(t *testing.T) {
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
	var env struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error.Code != "inventory_unresolved" {
		t.Fatalf("code = %q, want inventory_unresolved", env.Error.Code)
	}
	unresolved, ok := env.Error.Details["unresolved"].([]any)
	if !ok || len(unresolved) != 1 || unresolved[0] != "dh99-r999-us-east-99z" {
		t.Fatalf("details.unresolved = %v, want [dh99-r999-us-east-99z]", env.Error.Details["unresolved"])
	}

	// Confirm run row was preserved with status='failed'.
	runs, _ := s.ListRuns(context.Background())
	if len(runs) != 1 {
		t.Fatalf("len runs = %d, want 1", len(runs))
	}
	if runs[0].Status != "failed" {
		t.Errorf("status = %q, want failed", runs[0].Status)
	}

	// Confirm event trail: RunCreated, operator FactsDiscovered, FactsDiscoveryFailed.
	evs, _ := s.LoadEvents(context.Background(), runs[0].ID)
	if len(evs) != 3 {
		t.Fatalf("len events = %d, want 3", len(evs))
	}
	wantKinds := []string{"RunCreated", "FactsDiscovered", "FactsDiscoveryFailed"}
	for i, ev := range evs {
		if ev.Kind != wantKinds[i] {
			t.Errorf("event[%d] = %q, want %q", i, ev.Kind, wantKinds[i])
		}
	}

	// Confirm NO rack-scope facts (all-or-nothing on inventory).
	rackFacts, _ := s.ListFacts(context.Background(), runs[0].ID, "rack:*", "")
	if len(rackFacts) != 0 {
		t.Errorf("rack facts = %d, want 0 (rollback semantics)", len(rackFacts))
	}
}

func TestCreateRun_MixedResolvedAndUnresolved_AllOrNothing(t *testing.T) {
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)

	// Two racks: one resolves, one does not. The all-or-nothing rule says
	// neither lands rack-scope facts.
	body := `{
		"template_id": "firmware-release-canary",
		"inputs": {
			"bundle_tag": "x",
			"canary_racks": ["dh3-r012-us-east-01a", "dh99-r999-us-east-99z"]
		}
	}`
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
	if runs[0].Status != "failed" {
		t.Errorf("status = %q, want failed", runs[0].Status)
	}

	// No rack-scope facts for EITHER rack.
	rackFacts, _ := s.ListFacts(context.Background(), runs[0].ID, "rack:*", "")
	if len(rackFacts) != 0 {
		t.Errorf("rack facts = %d, want 0 (all-or-nothing)", len(rackFacts))
	}
}
