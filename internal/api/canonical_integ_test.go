package api

import (
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
	"qac/internal/probe/failcauseprobe"
	"qac/internal/probe/firmwareinventoryprobe"
	"qac/internal/probe/rlccactionprobe"
	"qac/internal/rlccclient"
	"qac/internal/seed"
	"qac/internal/store"
	"qac/internal/template"
)

// TestCreateRun_CanonicalCanaryTemplate_LegacyShape exercises the full
// integration path: real templates/firmware-release-canary.yaml seeded via
// seed.LoadEmbeddedTemplates, then POST /api/runs with the iter-2 legacy
// {bundle, rack} shape. After 201, asserts the iter-4a facts projection
// landed end-to-end (1 operator bundle_tag + 6 inventory rack facts).
func TestCreateRun_CanonicalCanaryTemplate_LegacyShape(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	body, err := os.ReadFile(filepath.Join("..", "..", "templates", "firmware-release-canary.yaml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	fsys := fstest.MapFS{
		"templates/firmware-release-canary.yaml": &fstest.MapFile{Data: body},
	}
	if err := seed.LoadEmbeddedTemplates(ctx, s, fsys); err != nil {
		t.Fatalf("LoadEmbeddedTemplates: %v", err)
	}

	resolver := inventoryclient.NewMapResolverWithBMNs(inventoryclient.SeedDemoFixtures(), inventoryclient.SeedDemoBMNs())
	reg := probe.NewRegistry()
	reg.Register(alertprobe.New())
	reg.Register(firmwareinventoryprobe.New())
	eng := engine.New(s, reg, probe.Clients{
		AlertClient:         alertclient.NewMapAlertClient(alertclient.SeedDemoAlerts(), nil),
		AlertCategoryClient: alertcategoryclient.NewMapClient(alertcategoryclient.SeedDemoCategories()),
		FirmwareClient:      firmwareclient.NewMapClient(firmwareclient.SeedDemoFirmware()),
		InventoryResolver:   resolver,
	})
	// Seed one RLCC ignore so BMNIgnored(source=rlcc) lands in the events table.
	lifeC := lifecycleclient.NewMapClient(map[lifecycleclient.RackKey][]lifecycleclient.IgnoredBMN{
		{Rack: "dh3-r012-us-east-01a", Zone: "us-east-01a"}: {
			{Deviceslot: "dh3-r012-node-03-us-east-01a", BMNName: "s90txs53"},
		},
	})
	srv := httptest.NewServer(Router(s, "test", resolver, eng, rlccclient.NewMapClient(), flccclient.NewMapClient(), false, func() {}, lifeC))
	t.Cleanup(func() {
		ctx, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		_ = eng.Shutdown(ctx)
	})
	t.Cleanup(srv.Close)

	// iter-5d: canonical body shape. After Task 8 flips
	// firmware-release-canary's rlcc_workflow input to required:true,
	// the legacy {bundle, rack} body would fail validation. Send the
	// canonical {template_id, inputs} body instead.
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
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 201 {
		var env map[string]any
		_ = json.NewDecoder(res.Body).Decode(&env)
		t.Fatalf("status = %d, want 201; body = %+v", res.StatusCode, env)
	}

	// iter-4a: facts projection populated end-to-end against the live
	// on-disk canonical template.
	var out struct {
		Run struct{ ID string } `json:"run"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	allFacts, err := s.ListFacts(ctx, out.Run.ID, "", "")
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(allFacts) != 7 {
		t.Fatalf("len facts = %d, want 7 (1 operator + 6 inventory)", len(allFacts))
	}

	// iter-4b: wait for the engine to finish alert_baseline and assert a
	// StepCompleted event + evidence row landed.
	select {
	case <-eng.Wait(out.Run.ID):
	case <-time.After(5 * time.Second):
		t.Fatal("engine did not finish canary run within 5s")
	}

	evs, err := s.LoadEvents(ctx, out.Run.ID)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}

	// iter-4c: walk runs both alert_baseline and firmware_baseline; we
	// expect at least two StepCompleted events with non-empty
	// evidence_ids.
	type completedRef struct {
		stepID     string
		evidenceID string
	}
	var completed []completedRef
	for _, ev := range evs {
		if ev.Kind != "StepCompleted" {
			continue
		}
		var sc struct {
			StepID     string `json:"step_id"`
			EvidenceID string `json:"evidence_id"`
		}
		if err := json.Unmarshal(ev.Payload, &sc); err != nil {
			t.Fatalf("decode StepCompleted: %v", err)
		}
		completed = append(completed, completedRef{stepID: sc.StepID, evidenceID: sc.EvidenceID})
	}
	if len(completed) < 2 {
		var kinds []string
		for _, ev := range evs {
			kinds = append(kinds, ev.Kind)
		}
		t.Fatalf("len StepCompleted = %d, want >= 2; events = %v", len(completed), kinds)
	}

	// Verify each StepCompleted has a non-empty evidence id and the
	// evidence is GET-able.
	var alertEvidenceID, firmwareEvidenceID string
	for _, c := range completed {
		if c.evidenceID == "" {
			t.Errorf("StepCompleted(step_id=%s) has empty evidence_id", c.stepID)
			continue
		}
		ev, err := s.GetEvidence(ctx, c.evidenceID)
		if err != nil {
			t.Errorf("GetEvidence(%s): %v", c.evidenceID, err)
			continue
		}
		switch c.stepID {
		case "alert_baseline":
			alertEvidenceID = c.evidenceID
			if !strings.Contains(string(ev.Payload), "categories") {
				t.Errorf("alert_baseline evidence payload missing categories: %s", ev.Payload)
			}
		case "firmware_baseline":
			firmwareEvidenceID = c.evidenceID
			if !strings.Contains(string(ev.Payload), "all_resolved") {
				t.Errorf("firmware_baseline evidence payload missing all_resolved: %s", ev.Payload)
			}
		}
	}
	if alertEvidenceID == "" {
		t.Error("no StepCompleted for alert_baseline observed in events")
	}
	if firmwareEvidenceID == "" {
		t.Error("no StepCompleted for firmware_baseline observed in events")
	}

	// iter-5b: assert per-CT discovery landed end-to-end.
	runID := out.Run.ID

	// One BMNDiscovered event per resolved rack.
	bmnDiscRow := s.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE run_id = ? AND kind = 'BMNDiscovered'`, runID)
	var nDisc int
	if err := bmnDiscRow.Scan(&nDisc); err != nil {
		t.Fatalf("query BMNDiscovered count: %v", err)
	}
	if nDisc != 1 {
		t.Errorf("BMNDiscovered count = %d, want 1 (one rack)", nDisc)
	}

	// rack_deviceslots has 18 rows for the demo rack.
	dsRow := s.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM rack_deviceslots WHERE run_id = ?`, runID)
	var nDS int
	if err := dsRow.Scan(&nDS); err != nil {
		t.Fatalf("query rack_deviceslots count: %v", err)
	}
	if nDS != 18 {
		t.Errorf("rack_deviceslots count = %d, want 18", nDS)
	}

	// GET /api/runs/{id} response includes racks[] with ct_count.
	resp, err := http.Get(srv.URL + "/api/runs/" + runID)
	if err != nil {
		t.Fatalf("GET /api/runs/%s: %v", runID, err)
	}
	defer resp.Body.Close()
	var detail struct {
		Racks []struct {
			Rack    string `json:"rack"`
			CTCount int    `json:"ct_count"`
		} `json:"racks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode GET run detail: %v", err)
	}
	if len(detail.Racks) != 1 || detail.Racks[0].CTCount != 18 {
		t.Errorf("GET racks = %v, want one rack with ct_count=18", detail.Racks)
	}

	// BMNIgnored(source=rlcc) emitted for the MapClient-seeded RLCC ignore.
	ignRow := s.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bmn_ignores WHERE run_id = ? AND source = 'rlcc'`, runID)
	var nIgn int
	if err := ignRow.Scan(&nIgn); err != nil {
		t.Fatalf("query bmn_ignores count: %v", err)
	}
	if nIgn != 1 {
		t.Errorf("rlcc-source bmn_ignores count = %d, want 1", nIgn)
	}

	// iter-5d: assert the workflow snapshot landed in RunCreated.payload.
	getRes, err := http.Get(srv.URL + "/api/runs/" + out.Run.ID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer getRes.Body.Close()
	var run struct {
		Events []struct {
			Kind    string          `json:"kind"`
			Payload json.RawMessage `json:"payload"`
		} `json:"events"`
	}
	if err := json.NewDecoder(getRes.Body).Decode(&run); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if len(run.Events) == 0 || run.Events[0].Kind != "RunCreated" {
		t.Fatalf("events[0] = %+v, want RunCreated first", run.Events)
	}
	var rc struct {
		RLCCWorkflow *struct {
			Name      string `json:"name"`
			SourceSHA string `json:"source_sha"`
			Actions   []any  `json:"actions"`
		} `json:"rlcc_workflow"`
	}
	if err := json.Unmarshal(run.Events[0].Payload, &rc); err != nil {
		t.Fatalf("unmarshal RunCreated.payload: %v", err)
	}
	if rc.RLCCWorkflow == nil {
		t.Fatal("RunCreated.payload.rlcc_workflow is nil; want inlined snapshot")
	}
	if rc.RLCCWorkflow.Name != "gb200-rack-bringup-v4" {
		t.Errorf("Name = %q, want gb200-rack-bringup-v4", rc.RLCCWorkflow.Name)
	}
	if rc.RLCCWorkflow.SourceSHA == "" {
		t.Error("SourceSHA = empty; want non-empty (pinned)")
	}
	if len(rc.RLCCWorkflow.Actions) == 0 {
		t.Error("len(Actions) = 0; want > 0")
	}
}

// TestCanonicalTemplate_L11UsesRLCCActionProbe pins the iter-5g wiring: the
// on-disk canonical template's l11_fielddiag step is a probe_type step that
// carries its config via action.args (map form), and the template version is
// bumped to 9.
func TestCanonicalTemplate_L11UsesRLCCActionProbe(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "templates", "firmware-release-canary.yaml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	tpl, err := template.Parse(body)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tpl.Version != 11 {
		t.Errorf("template version = %d, want 11", tpl.Version)
	}
	var l11 *template.Step
	for i := range tpl.Steps {
		if tpl.Steps[i].ID == "l11_fielddiag" {
			l11 = &tpl.Steps[i]
			break
		}
	}
	if l11 == nil {
		t.Fatal("no l11_fielddiag step found")
	}
	if l11.ProbeType != "rlcc_action_probe" {
		t.Errorf("l11_fielddiag.ProbeType = %q, want rlcc_action_probe", l11.ProbeType)
	}
	if l11.Action == nil {
		t.Fatal("l11_fielddiag.Action is nil; want action carrying config args")
	}
	args, ok := l11.Action.Args.(map[string]any)
	if !ok {
		t.Fatalf("l11_fielddiag.Action.Args = %T, want map[string]any", l11.Action.Args)
	}
	if args["match_handler"] != "l11-fielddiag" {
		t.Errorf("args[match_handler] = %v, want l11-fielddiag", args["match_handler"])
	}
	if args["target_state"] != "l11-fielddiag" {
		t.Errorf("args[target_state] = %v, want l11-fielddiag", args["target_state"])
	}
	if args["poll_interval"] != "30s" {
		t.Errorf("args[poll_interval] = %v, want 30s", args["poll_interval"])
	}
	if args["evaluation_unit"] != "nvlink-domain" {
		t.Errorf("args[evaluation_unit] = %v, want nvlink-domain", args["evaluation_unit"])
	}
}

// TestCanonicalCanary_RLCCActionProbe_EndToEnd is the first real exercise of
// rlcc_action_probe end-to-end with the prev_state verdict model (iter-5h).
// All 18 CTs are seeded terminal on the first poll: 16 pass (PrevState ==
// target_state), 1 fail (State:"fail"), 1 ignorable (State:"rma" + broken-
// collect workflow). The run must drain its pool and emit StepCompleted with
// Verdict:"failed" (1 fail under all-must-pass).
func TestCanonicalCanary_RLCCActionProbe_EndToEnd(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Load the real on-disk template, then shrink the probe poll interval so
	// the seeded observations are observed on the first tick. All 18 CTs are
	// terminal from the first poll, so the run drains quickly.
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
	lifeC := lifecycleclient.NewMapClient(nil) // no RLCC ignores → all 18 CTs walked

	// Seed all 18 CTs terminal under the prev_state model: bmns[0] fails,
	// bmns[1] is ignorable (rma + broken-collect workflow), the rest pass
	// (PrevState == target_state "l11-fielddiag" + State:"ready"). All are
	// terminal on the first poll so the pool drains without waiting.
	bmns, _ := resolver.ResolveBMNs(ctx, "dh3-r012-us-east-01a")
	var failDS string // the deviceslot we mark FLCC "fail" — fail_cause must explain it (iter-5l)
	for i, b := range bmns {
		switch i {
		case 0:
			failDS = b.Deviceslot
			lifeC.SetFLCCObservation(b.Deviceslot, lifecycleclient.FLCCObservation{State: "fail", PrevStep: "gb200-l11-fielddiag"})
		case 1:
			lifeC.SetFLCCObservation(b.Deviceslot, lifecycleclient.FLCCObservation{State: "rma", Workflow: "broken-collect"})
		default:
			lifeC.SetFLCCObservation(b.Deviceslot, lifecycleclient.FLCCObservation{State: "ready", PrevState: "l11-fielddiag"})
		}
	}
	reg := probe.NewRegistry()
	reg.Register(alertprobe.New())
	reg.Register(firmwareinventoryprobe.New())
	reg.Register(rlccactionprobe.New())
	reg.Register(failcauseprobe.New())
	eng := engine.New(s, reg, probe.Clients{
		// iter-5l: seed a critical node-scoped alert on the failed tray's
		// deviceslot so fail_cause can correlate a cause to it.
		AlertClient: alertclient.NewMapAlertClient(alertclient.SeedDemoAlerts(), map[string][]alertclient.Alert{
			failDS: {{ID: "a-fail", Name: "NVLinkLinkDown", Severity: "critical", Scope: "dh3-r012-us-east-01a", Deviceslot: failDS}},
		}),
		AlertCategoryClient: alertcategoryclient.NewMapClient(alertcategoryclient.SeedDemoCategories()),
		FirmwareClient:      firmwareclient.NewMapClient(firmwareclient.SeedDemoFirmware()),
		InventoryResolver:   resolver,
		LifecycleClient:     lifeC,
		EvidenceWriter:      s,
		EvidenceReader:      s,
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
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 201 {
		var env map[string]any
		_ = json.NewDecoder(res.Body).Decode(&env)
		t.Fatalf("status = %d, want 201; body = %+v", res.StatusCode, env)
	}
	var out struct {
		Run struct{ ID string } `json:"run"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	runID := out.Run.ID

	// Walk runs alert_baseline + firmware_baseline + l11_fielddiag, then halts at
	// evidence_review (a review step). Wait for the walk to finish.
	select {
	case <-eng.Wait(runID):
	case <-time.After(5 * time.Second):
		t.Fatal("engine did not finish canary run within 5s")
	}

	// l11_fielddiag must have reached StepCompleted (the probe is a gatherer;
	// it returns a non-error Output).
	evs, err := s.LoadEvents(ctx, runID)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	var l11Completed bool
	for _, ev := range evs {
		if ev.Kind != "StepCompleted" {
			continue
		}
		var sc struct {
			StepID string `json:"step_id"`
		}
		if err := json.Unmarshal(ev.Payload, &sc); err != nil {
			t.Fatalf("decode StepCompleted payload: %v", err)
		}
		if sc.StepID == "l11_fielddiag" {
			l11Completed = true
		}
	}
	if !l11Completed {
		t.Fatal("no StepCompleted for l11_fielddiag; the probe did not run end-to-end")
	}

	// 18 per-CT evidence rows (deviceslot set) for the 18-CT demo rack.
	var nPerCT int
	if err := s.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM evidence WHERE run_id = ? AND step_id = 'l11_fielddiag' AND deviceslot IS NOT NULL`,
		runID).Scan(&nPerCT); err != nil {
		t.Fatalf("query per-CT evidence: %v", err)
	}
	if nPerCT != 18 {
		t.Errorf("per-CT evidence rows = %d, want 18", nPerCT)
	}

	// 1 aggregate Output row (deviceslot NULL) written by the engine from the
	// probe's returned Output.
	var nAgg int
	if err := s.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM evidence WHERE run_id = ? AND step_id = 'l11_fielddiag' AND deviceslot IS NULL`,
		runID).Scan(&nAgg); err != nil {
		t.Fatalf("query aggregate evidence: %v", err)
	}
	if nAgg != 1 {
		t.Errorf("aggregate evidence rows = %d, want 1", nAgg)
	}

	// The aggregate Output reports 16 success / 1 fail / 1 ignorable buckets
	// (iter-5h prev_state model: 16 PrevState="l11-fielddiag" pass, 1 fail, 1 rma).
	var aggPayload string
	if err := s.DB().QueryRowContext(ctx,
		`SELECT payload FROM evidence WHERE run_id = ? AND step_id = 'l11_fielddiag' AND deviceslot IS NULL`,
		runID).Scan(&aggPayload); err != nil {
		t.Fatalf("scan aggregate payload: %v", err)
	}
	var agg struct {
		CountActive    int    `json:"count_active"`
		CountSuccess   int    `json:"count_success"`
		CountFailed    int    `json:"count_failed"`
		CountIgnorable int    `json:"count_ignorable"`
		TimedOut       bool   `json:"timed_out"`
		EvaluationUnit string `json:"evaluation_unit"`
	}
	if err := json.Unmarshal([]byte(aggPayload), &agg); err != nil {
		t.Fatalf("unmarshal aggregate payload %q: %v", aggPayload, err)
	}
	if agg.CountActive != 18 || agg.CountSuccess != 16 || agg.CountFailed != 1 || agg.CountIgnorable != 1 {
		t.Errorf("buckets = %+v, want active18 success16 failed1 ignorable1", agg)
	}
	if agg.TimedOut {
		t.Error("aggregate timed_out = true, want false (all CTs completed)")
	}
	if agg.EvaluationUnit != "nvlink-domain" {
		t.Errorf("evaluation_unit = %q, want nvlink-domain", agg.EvaluationUnit)
	}

	// iter-5h: the StepCompleted event for l11_fielddiag must carry
	// Verdict:"failed" — 1 fail under all-must-pass policy.
	var l11Verdict string
	for _, ev := range evs {
		if ev.Kind != "StepCompleted" {
			continue
		}
		var p struct {
			StepID  string `json:"step_id"`
			Verdict string `json:"verdict"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("decode StepCompleted: %v", err)
		}
		if p.StepID == "l11_fielddiag" {
			l11Verdict = p.Verdict
		}
	}
	if l11Verdict != "failed" {
		t.Errorf("l11_fielddiag verdict = %q, want failed (1 fail, all-must-pass)", l11Verdict)
	}

	// iter-5i: verdict must reach the API-served run status via the store
	// projection (internal/store/events.go StepCompleted worst-wins rollup).
	statusRes, err := http.Get(srv.URL + "/api/runs/" + runID)
	if err != nil {
		t.Fatalf("GET run status: %v", err)
	}
	defer statusRes.Body.Close()
	var detail struct {
		Run struct {
			Status string `json:"status"`
		} `json:"run"`
	}
	if err := json.NewDecoder(statusRes.Body).Decode(&detail); err != nil {
		t.Fatalf("decode run detail: %v", err)
	}
	if detail.Run.Status != engine.StatusFailed {
		t.Errorf("run status = %q, want %q (verdict reaches run status)", detail.Run.Status, engine.StatusFailed)
	}

	// iter-5j: GET /api/runs/{id}/evidence?step=l11_fielddiag returns all
	// 19 rows (18 per-CT with deviceslot set + 1 aggregate with deviceslot
	// nil). The per-CT count matches the seeded 18-CT demo rack (16 pass +
	// 1 fail + 1 ignorable — rlcc_action_probe calls writeCTEvidence for
	// all terminal trays including ignorable ones).
	evRes, err := http.Get(srv.URL + "/api/runs/" + runID + "/evidence?step=l11_fielddiag")
	if err != nil {
		t.Fatalf("GET evidence list: %v", err)
	}
	defer evRes.Body.Close()
	var evList struct {
		Evidence []struct {
			Deviceslot *string `json:"deviceslot"`
		} `json:"evidence"`
	}
	if err := json.NewDecoder(evRes.Body).Decode(&evList); err != nil {
		t.Fatalf("decode evidence list: %v", err)
	}
	var listPerCT, listAgg int
	for _, e := range evList.Evidence {
		if e.Deviceslot != nil {
			listPerCT++
		} else {
			listAgg++
		}
	}
	// 18 per-CT (one per tray: 16 success + 1 fail + 1 ignorable) + 1 aggregate.
	if listPerCT != 18 || listAgg != 1 {
		t.Errorf("evidence list: perCT=%d agg=%d, want 18 + 1", listPerCT, listAgg)
	}

	// iter-5l: fail_cause ran (unconditionally) after l11_fielddiag, before the
	// evidence_review halt. It reads l11_fielddiag's failed trays and correlates
	// a deviceslot-scoped alert to each. With 1 failed tray (failDS) carrying a
	// seeded critical alert, expect 1 per-CT cause row (deviceslot set) + the
	// aggregate row, and the cause row must carry that critical alert.
	fcRes, err := http.Get(srv.URL + "/api/runs/" + runID + "/evidence?step=fail_cause")
	if err != nil {
		t.Fatalf("GET fail_cause evidence: %v", err)
	}
	defer fcRes.Body.Close()
	var fcList struct {
		Evidence []struct {
			Deviceslot *string         `json:"deviceslot"`
			Payload    json.RawMessage `json:"payload"`
		} `json:"evidence"`
	}
	if err := json.NewDecoder(fcRes.Body).Decode(&fcList); err != nil {
		t.Fatalf("decode fail_cause evidence: %v", err)
	}
	var fcCause, fcAgg int
	var failRowHasAlert bool
	for _, e := range fcList.Evidence {
		if e.Deviceslot == nil {
			fcAgg++
			continue
		}
		fcCause++
		if *e.Deviceslot == failDS {
			var p struct {
				Alerts []alertclient.Alert `json:"alerts"`
			}
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				t.Fatalf("decode fail_cause cause row: %v", err)
			}
			if len(p.Alerts) == 1 && p.Alerts[0].Severity == "critical" {
				failRowHasAlert = true
			}
		}
	}
	if fcAgg != 1 {
		t.Errorf("fail_cause aggregate rows = %d, want 1", fcAgg)
	}
	if fcCause != 1 {
		t.Errorf("fail_cause per-CT cause rows = %d, want 1 (the single failed tray)", fcCause)
	}
	if !failRowHasAlert {
		t.Errorf("failed deviceslot %s did not carry its seeded critical alert", failDS)
	}

	// iter-5k: drive the full operator sequence ON TOP of the real
	// probe-driven canary (which carries a real failed verdict from the FLCC
	// fail tray). The run is halted at evidence_review; advance through the
	// two review/ai_assess gates, then sign off, and confirm the run reaches
	// signed_off via the live HTTP API.
	//
	// The actions handler enforces current-gate ordering (a POST to a gate
	// that isn't the first un-actioned one returns 422 — see
	// TestOperatorAction_NotCurrentGate_422). So a 200 on each step below is
	// itself proof the walk had halted at exactly that gate; we additionally
	// confirm the OperatorAction event landed for each.

	// 1) evidence_review must be the current gate: advancing it returns 200.
	//    After the walk settles it halts at ai_summary (proven by step 2's 200).
	a1 := postAction(t, srv.URL, runID, map[string]any{"step_id": "evidence_review", "action": "advance"})
	a1.Body.Close()
	if a1.StatusCode != 200 {
		t.Fatalf("advance evidence_review status = %d, want 200", a1.StatusCode)
	}
	waitSettled(t, eng, runID)

	// 2) ai_summary is now the current gate: advancing it returns 200 (a 422
	//    here would mean the walk did not halt at ai_summary). It then halts
	//    at operator_signoff (proven by step 3's 200).
	a2 := postAction(t, srv.URL, runID, map[string]any{"step_id": "ai_summary", "action": "advance"})
	a2.Body.Close()
	if a2.StatusCode != 200 {
		t.Fatalf("advance ai_summary status = %d, want 200 (run did not halt at ai_summary)", a2.StatusCode)
	}
	waitSettled(t, eng, runID)

	// Confirm both advance OperatorAction events landed.
	advEvs, err := s.LoadEvents(ctx, runID)
	if err != nil {
		t.Fatalf("LoadEvents after advances: %v", err)
	}
	if !hasOperatorAction(t, advEvs, "evidence_review") {
		t.Fatal("no OperatorAction event for evidence_review")
	}
	if !hasOperatorAction(t, advEvs, "ai_summary") {
		t.Fatal("no OperatorAction event for ai_summary")
	}

	// 3) operator_signoff is now the current gate. Sign off with verdict
	//    "fail" (a value in final_verdict_options [pass, fail], and apt since
	//    a tray failed) → run reaches signed_off. A 200 here proves the walk
	//    had halted at operator_signoff.
	a3 := postAction(t, srv.URL, runID, map[string]any{
		"step_id":     "operator_signoff",
		"action":      "signoff",
		"verdict":     "fail",
		"signer_name": "op",
		"signer_role": "FRO",
	})
	defer a3.Body.Close()
	if a3.StatusCode != 200 {
		var env map[string]any
		_ = json.NewDecoder(a3.Body).Decode(&env)
		t.Fatalf("signoff status = %d, want 200; body = %+v", a3.StatusCode, env)
	}
	waitSettled(t, eng, runID)

	signRes, err := http.Get(srv.URL + "/api/runs/" + runID)
	if err != nil {
		t.Fatalf("GET run after signoff: %v", err)
	}
	defer signRes.Body.Close()
	var signed struct {
		Run struct {
			Status string `json:"status"`
		} `json:"run"`
	}
	if err := json.NewDecoder(signRes.Body).Decode(&signed); err != nil {
		t.Fatalf("decode signed run: %v", err)
	}
	if signed.Run.Status != engine.StatusSignedOff {
		t.Errorf("run status = %q, want %q (signed_off after operator sequence)", signed.Run.Status, engine.StatusSignedOff)
	}
}
