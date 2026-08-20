package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
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
	"qac/internal/seed"
	"qac/internal/store"
)

// newActionsTestServer wires a server against the real on-disk canonical
// template, seeds all 18 CTs terminal so l11_fielddiag drains on the first
// poll, and creates one run that the engine walks up to the evidence_review
// halt. It returns the server, store, engine, and the created run id so the
// actions tests can POST /api/runs/{id}/actions against a live gated walk.
func newActionsTestServer(t *testing.T) (*httptest.Server, *store.Store, *engine.Engine, string) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	raw, err := os.ReadFile(filepath.Join("..", "..", "templates", "firmware-release-canary.yaml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := strings.Replace(string(raw), "poll_interval: 30s", "poll_interval: 5ms", 1)
	if !strings.Contains(body, "poll_interval: 5ms") {
		t.Fatal("expected to rewrite poll_interval: 30s → 5ms; template shape changed?")
	}
	fsys := fstest.MapFS{
		"templates/firmware-release-canary.yaml": &fstest.MapFile{Data: []byte(body)},
	}
	if err := seed.LoadEmbeddedTemplates(ctx, s, fsys); err != nil {
		t.Fatalf("LoadEmbeddedTemplates: %v", err)
	}

	resolver := inventoryclient.NewMapResolverWithBMNs(inventoryclient.SeedDemoFixtures(), inventoryclient.SeedDemoBMNs())
	lifeC := lifecycleclient.NewMapClient(nil)

	// Seed all 18 CTs terminal-pass so l11_fielddiag drains and the walk
	// reaches the evidence_review gate.
	bmns, _ := resolver.ResolveBMNs(ctx, "dh3-r012-us-east-01a")
	for _, b := range bmns {
		lifeC.SetFLCCObservation(b.Deviceslot, lifecycleclient.FLCCObservation{State: "ready", PrevState: "l11-fielddiag"})
	}

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
		c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = eng.Shutdown(c)
	})
	t.Cleanup(srv.Close)

	reqBody := `{
    "template_id": "firmware-release-canary",
    "inputs": {
        "bundle_tag":    "gb200-fw-2026-05-canary-3",
        "canary_racks":  ["dh3-r012-us-east-01a"],
        "rlcc_workflow": "gb200-rack-bringup-v4"
    }
}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /api/runs: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 201 {
		var env map[string]any
		_ = json.NewDecoder(res.Body).Decode(&env)
		t.Fatalf("create run status = %d, want 201; body = %+v", res.StatusCode, env)
	}
	var out struct {
		Run struct{ ID string } `json:"run"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode create run: %v", err)
	}

	// Wait for the walk to settle at the evidence_review halt.
	waitSettled(t, eng, out.Run.ID)
	return srv, s, eng, out.Run.ID
}

// waitSettled blocks until the engine's walk for runID has paused (channel
// closes when the runner exits, which it does at every operator-gated halt).
func waitSettled(t *testing.T, eng *engine.Engine, runID string) {
	t.Helper()
	select {
	case <-eng.Wait(runID):
	case <-time.After(5 * time.Second):
		t.Fatalf("engine walk for %s did not settle within 5s", runID)
	}
}

// postAction is a small helper for POST /api/runs/{id}/actions.
func postAction(t *testing.T, srvURL, runID string, req map[string]any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(req)
	res, err := http.Post(srvURL+"/api/runs/"+runID+"/actions", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST actions: %v", err)
	}
	return res
}

func TestOperatorAction_AdvanceCurrentGate_200_ResumesWalk(t *testing.T) {
	srv, s, eng, runID := newActionsTestServer(t)

	res := postAction(t, srv.URL, runID, map[string]any{
		"step_id": "evidence_review",
		"action":  "advance",
	})
	defer res.Body.Close()
	if res.StatusCode != 200 {
		var env map[string]any
		_ = json.NewDecoder(res.Body).Decode(&env)
		t.Fatalf("status = %d, want 200; body = %+v", res.StatusCode, env)
	}

	// Walk resumes and halts at the next gate (ai_summary). Confirm the
	// OperatorAction event for evidence_review landed.
	waitSettled(t, eng, runID)
	evs, err := s.LoadEvents(context.Background(), runID)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if !hasOperatorAction(t, evs, "evidence_review") {
		t.Fatalf("no OperatorAction event for evidence_review")
	}
}

func TestOperatorAction_SignoffAfterAdvancingGates_200_StatusSignedOff(t *testing.T) {
	srv, s, eng, runID := newActionsTestServer(t)

	// Advance evidence_review, then ai_summary, then sign off.
	r1 := postAction(t, srv.URL, runID, map[string]any{"step_id": "evidence_review", "action": "advance"})
	r1.Body.Close()
	if r1.StatusCode != 200 {
		t.Fatalf("advance evidence_review status = %d, want 200", r1.StatusCode)
	}
	waitSettled(t, eng, runID)

	r2 := postAction(t, srv.URL, runID, map[string]any{"step_id": "ai_summary", "action": "advance"})
	r2.Body.Close()
	if r2.StatusCode != 200 {
		t.Fatalf("advance ai_summary status = %d, want 200", r2.StatusCode)
	}
	waitSettled(t, eng, runID)

	r3 := postAction(t, srv.URL, runID, map[string]any{
		"step_id":     "operator_signoff",
		"action":      "signoff",
		"verdict":     "pass",
		"signer_name": "Wallace Pena",
		"signer_role": "FRO lead",
	})
	defer r3.Body.Close()
	if r3.StatusCode != 200 {
		var env map[string]any
		_ = json.NewDecoder(r3.Body).Decode(&env)
		t.Fatalf("signoff status = %d, want 200; body = %+v", r3.StatusCode, env)
	}

	runs, _ := s.ListRuns(context.Background())
	var status string
	for _, rn := range runs {
		if rn.ID == runID {
			status = rn.Status
		}
	}
	if status != engine.StatusSignedOff {
		t.Fatalf("run status = %q, want %q", status, engine.StatusSignedOff)
	}
}

func TestOperatorAction_UnknownRun_404(t *testing.T) {
	srv, _, _, _ := newActionsTestServer(t)

	res := postAction(t, srv.URL, "does-not-exist", map[string]any{"step_id": "evidence_review", "action": "advance"})
	defer res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
}

func TestOperatorAction_StepNotGated_422(t *testing.T) {
	srv, _, _, runID := newActionsTestServer(t)

	// l11_fielddiag is a probe step, not gated.
	res := postAction(t, srv.URL, runID, map[string]any{"step_id": "l11_fielddiag", "action": "advance"})
	defer res.Body.Close()
	if res.StatusCode != 422 {
		t.Fatalf("status = %d, want 422", res.StatusCode)
	}
}

func TestOperatorAction_NotCurrentGate_422(t *testing.T) {
	srv, _, _, runID := newActionsTestServer(t)

	// operator_signoff is gated but not the current (first un-actioned) gate;
	// evidence_review is.
	res := postAction(t, srv.URL, runID, map[string]any{
		"step_id":     "operator_signoff",
		"action":      "signoff",
		"verdict":     "pass",
		"signer_name": "x",
		"signer_role": "y",
	})
	defer res.Body.Close()
	if res.StatusCode != 422 {
		t.Fatalf("status = %d, want 422", res.StatusCode)
	}
}

func TestOperatorAction_SignoffActionOnReviewStep_422(t *testing.T) {
	srv, _, _, runID := newActionsTestServer(t)

	// Current gate is evidence_review (kind review), which accepts advance.
	res := postAction(t, srv.URL, runID, map[string]any{
		"step_id":     "evidence_review",
		"action":      "signoff",
		"signer_name": "x",
		"signer_role": "y",
	})
	defer res.Body.Close()
	if res.StatusCode != 422 {
		t.Fatalf("status = %d, want 422", res.StatusCode)
	}
}

func TestOperatorAction_AdvanceActionOnSignoffStep_422(t *testing.T) {
	srv, _, eng, runID := newActionsTestServer(t)

	// Drive to operator_signoff, then try advance (signoff steps require signoff).
	r1 := postAction(t, srv.URL, runID, map[string]any{"step_id": "evidence_review", "action": "advance"})
	r1.Body.Close()
	waitSettled(t, eng, runID)
	r2 := postAction(t, srv.URL, runID, map[string]any{"step_id": "ai_summary", "action": "advance"})
	r2.Body.Close()
	waitSettled(t, eng, runID)

	res := postAction(t, srv.URL, runID, map[string]any{"step_id": "operator_signoff", "action": "advance"})
	defer res.Body.Close()
	if res.StatusCode != 422 {
		t.Fatalf("status = %d, want 422", res.StatusCode)
	}
}

func TestOperatorAction_SignoffMissingSignerName_422(t *testing.T) {
	srv, _, eng, runID := newActionsTestServer(t)

	r1 := postAction(t, srv.URL, runID, map[string]any{"step_id": "evidence_review", "action": "advance"})
	r1.Body.Close()
	waitSettled(t, eng, runID)
	r2 := postAction(t, srv.URL, runID, map[string]any{"step_id": "ai_summary", "action": "advance"})
	r2.Body.Close()
	waitSettled(t, eng, runID)

	res := postAction(t, srv.URL, runID, map[string]any{
		"step_id":     "operator_signoff",
		"action":      "signoff",
		"verdict":     "pass",
		"signer_role": "FRO lead",
	})
	defer res.Body.Close()
	if res.StatusCode != 422 {
		t.Fatalf("status = %d, want 422", res.StatusCode)
	}
}

func TestOperatorAction_SignoffVerdictNotAllowed_422(t *testing.T) {
	srv, _, eng, runID := newActionsTestServer(t)

	r1 := postAction(t, srv.URL, runID, map[string]any{"step_id": "evidence_review", "action": "advance"})
	r1.Body.Close()
	waitSettled(t, eng, runID)
	r2 := postAction(t, srv.URL, runID, map[string]any{"step_id": "ai_summary", "action": "advance"})
	r2.Body.Close()
	waitSettled(t, eng, runID)

	res := postAction(t, srv.URL, runID, map[string]any{
		"step_id":     "operator_signoff",
		"action":      "signoff",
		"verdict":     "maybe", // not in final_verdict_options [pass, fail]
		"signer_name": "Wallace Pena",
		"signer_role": "FRO lead",
	})
	defer res.Body.Close()
	if res.StatusCode != 422 {
		t.Fatalf("status = %d, want 422", res.StatusCode)
	}
}

func hasOperatorAction(t *testing.T, evs []store.Event, stepID string) bool {
	t.Helper()
	for _, ev := range evs {
		if ev.Kind != engine.KindOperatorAction {
			continue
		}
		var p struct {
			StepID string `json:"step_id"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			continue
		}
		if p.StepID == stepID {
			return true
		}
	}
	return false
}
