package rlccactionprobe

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"qac/internal/inventoryclient"
	"qac/internal/lifecycleclient"
	"qac/internal/probe"
	"qac/internal/rlccclient"
	"qac/internal/store"
)

// --- fakes for the three ports ---

type fakeLifecycle struct {
	mu               sync.Mutex
	actionCalls      int
	completeAfter    int // QueryFLCCState reports ready once actionCalls >= this
	rlccCalls        int
	rlccIgnoreOnCall int    // 0 = never; otherwise return rlccSlot once rlccCalls >= this
	rlccSlot         string
	flcc             map[string]lifecycleclient.FLCCObservation // explicit per-deviceslot seeding
}

// QueryFLCCState satisfies the LifecycleClient interface. If per-deviceslot
// observations are seeded via the flcc map, those are returned directly.
// Otherwise, completeAfter drives tick-based semantics: returns
// {State:"ready", PrevState:"l11-fielddiag"} for all requested deviceslots
// once actionCalls >= completeAfter (completeAfter must be >0); returns
// zero-value (in-progress) before that. PrevState:"l11-fielddiag" signals
// "moved past the diagnostic" to the diagnostic-state classifier.
func (f *fakeLifecycle) QueryFLCCState(_ context.Context, dss []string) (map[string]lifecycleclient.FLCCObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actionCalls++
	out := make(map[string]lifecycleclient.FLCCObservation, len(dss))
	if f.flcc != nil {
		// explicit seeding: return per-slot observations (zero-value for unseeded)
		for _, ds := range dss {
			out[ds] = f.flcc[ds]
		}
		return out, nil
	}
	// tick-based: return "moved past diagnostic" once completeAfter is reached
	done := f.completeAfter > 0 && f.actionCalls >= f.completeAfter
	for _, ds := range dss {
		if done {
			out[ds] = lifecycleclient.FLCCObservation{State: "ready", PrevState: "l11-fielddiag"}
		} else {
			out[ds] = lifecycleclient.FLCCObservation{}
		}
	}
	return out, nil
}

// QueryActionState is a no-op stub retained so fakeLifecycle still satisfies
// the LifecycleClient interface. The iter-5g probe uses QueryFLCCState instead.
func (f *fakeLifecycle) QueryActionState(_ context.Context, dss []string, _ string) (map[string]bool, error) {
	out := make(map[string]bool, len(dss))
	return out, nil
}

func (f *fakeLifecycle) QueryRLCCIgnored(_ context.Context, _ lifecycleclient.RackKey) ([]lifecycleclient.IgnoredBMN, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rlccCalls++
	if f.rlccIgnoreOnCall > 0 && f.rlccCalls >= f.rlccIgnoreOnCall {
		return []lifecycleclient.IgnoredBMN{{Deviceslot: f.rlccSlot}}, nil
	}
	return nil, nil
}

type fakeEvidence struct {
	mu   sync.Mutex
	rows []store.Evidence
}

func (f *fakeEvidence) PutEvidence(_ context.Context, ev store.Evidence) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, ev)
	return nil
}
func (f *fakeEvidence) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.rows) }

type fakeEmitter struct {
	mu      sync.Mutex
	ignored []probe.BMNIgnoredPayload
}

func (f *fakeEmitter) EmitBMNIgnored(_ context.Context, _ string, p probe.BMNIgnoredPayload) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ignored = append(f.ignored, p)
	return nil
}
func (f *fakeEmitter) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.ignored) }

// --- test helpers ---

// bmns returns n canonical BMNs for rack "rack-a" zone "z".
func bmns(n int) []inventoryclient.BMN {
	out := make([]inventoryclient.BMN, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, inventoryclient.BMN{
			Deviceslot: deviceslot(i),
			BMNName:    "bmn",
			Rack:       "rack-a",
			Zone:       "z",
			CTPosition: i,
		})
	}
	return out
}

func deviceslot(i int) string {
	// canonical-ish; parseActionLabels is not exercised by the fakes.
	if i < 10 {
		return "rack-a-node-0" + string(rune('0'+i)) + "-z"
	}
	return "rack-a-node-1" + string(rune('0'+(i-10))) + "-z"
}

func newWorkflow() *rlccclient.WorkflowSnapshot {
	return &rlccclient.WorkflowSnapshot{
		Name: "gb200-canary", SourceSHA: "abc123",
		Actions: []rlccclient.Action{
			{Name: "power", Handler: rlccclient.HandlerPowerOperation},
			{Name: "field diag", Handler: rlccclient.HandlerL11FieldDiag},
		},
	}
}

// baseConfig returns a minimal config for the diagnostic-state model.
// target_state drives classifyFLCC: PrevState=="l11-fielddiag" → outcomeSuccess.
func baseConfig() map[string]any {
	return map[string]any{
		"match_handler": "l11-fielddiag",
		"target_state":  "l11-fielddiag",
		"poll_interval": "1ms",
	}
}

func clientsWith(fl *fakeLifecycle, fe *fakeEvidence, fem *fakeEmitter, n int) probe.Clients {
	return probe.Clients{
		InventoryResolver: inventoryclient.NewMapResolverWithBMNs(nil, map[string][]inventoryclient.BMN{"rack-a": bmns(n)}),
		LifecycleClient:   fl,
		EvidenceWriter:    fe,
		EventEmitter:      fem,
	}
}

func TestProbe_TypeAndCategory(t *testing.T) {
	p := New()
	if p.Type() != "rlcc_action_probe" {
		t.Errorf("Type() = %q, want rlcc_action_probe", p.Type())
	}
	if p.Category() != probe.CategoryGatherer {
		t.Errorf("Category() = %q, want gatherer", p.Category())
	}
}

func TestProbe_MissingSnapshot(t *testing.T) {
	_, err := New().Run(context.Background(),
		clientsWith(&fakeLifecycle{}, &fakeEvidence{}, &fakeEmitter{}, 1),
		probe.StepContext{RunID: "r", StepID: "s", Racks: []string{"rack-a"}, Config: baseConfig(), RLCCWorkflow: nil})
	if err == nil {
		t.Fatal("want error for nil RLCCWorkflow")
	}
}

func TestProbe_NoMatchingAction(t *testing.T) {
	cfg := baseConfig()
	cfg["match_handler"] = "enable-cks"
	_, err := New().Run(context.Background(),
		clientsWith(&fakeLifecycle{completeAfter: 1}, &fakeEvidence{}, &fakeEmitter{}, 1),
		probe.StepContext{RunID: "r", StepID: "s", Racks: []string{"rack-a"}, Config: cfg, RLCCWorkflow: newWorkflow()})
	if err == nil {
		t.Fatal("want error for unmatched handler")
	}
}

func TestProbe_MissingConfigKeys(t *testing.T) {
	cases := map[string]map[string]any{
		"no match_handler": {"target_state": "x", "poll_interval": "1ms"},
		"no target_state":  {"match_handler": "l11-fielddiag", "poll_interval": "1ms"},
		"bad poll":         {"match_handler": "l11-fielddiag", "target_state": "x", "poll_interval": "nonsense"},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := New().Run(context.Background(),
				clientsWith(&fakeLifecycle{completeAfter: 1}, &fakeEvidence{}, &fakeEmitter{}, 1),
				probe.StepContext{RunID: "r", StepID: "s", Racks: []string{"rack-a"}, Config: cfg, RLCCWorkflow: newWorkflow()})
			if err == nil {
				t.Fatalf("%s: want error", name)
			}
		})
	}
}

func TestBuildPool_SubtractsOperatorAndRLCCIgnores(t *testing.T) {
	fl := &fakeLifecycle{} // QueryRLCCIgnored returns nil (no initial RLCC ignores)
	c := clientsWith(fl, &fakeEvidence{}, &fakeEmitter{}, 18)
	sc := probe.StepContext{
		RunID: "r", StepID: "s", Racks: []string{"rack-a"},
		IgnoredDeviceslots: []string{deviceslot(3), deviceslot(4)},
	}
	pool, rackKeys, err := buildPool(context.Background(), c, sc)
	if err != nil {
		t.Fatalf("buildPool: %v", err)
	}
	if len(pool) != 16 {
		t.Errorf("pool size = %d, want 16 (18 - 2 operator-ignored)", len(pool))
	}
	if len(rackKeys) != 1 || rackKeys[0].Rack != "rack-a" || rackKeys[0].Zone != "z" {
		t.Errorf("rackKeys = %+v, want one {rack-a,z}", rackKeys)
	}
	for _, ct := range pool {
		if ct.deviceslot == deviceslot(3) || ct.deviceslot == deviceslot(4) {
			t.Errorf("operator-ignored %s leaked into pool", ct.deviceslot)
		}
	}
}

func TestBuildPool_SubtractsInitialRLCCIgnore(t *testing.T) {
	fl := &fakeLifecycle{rlccIgnoreOnCall: 1, rlccSlot: deviceslot(5)} // ignored from the first query
	c := clientsWith(fl, &fakeEvidence{}, &fakeEmitter{}, 18)
	pool, _, err := buildPool(context.Background(), c,
		probe.StepContext{RunID: "r", StepID: "s", Racks: []string{"rack-a"}})
	if err != nil {
		t.Fatalf("buildPool: %v", err)
	}
	if len(pool) != 17 {
		t.Errorf("pool size = %d, want 17 (18 - 1 RLCC-ignored at start)", len(pool))
	}
}

// TestProbe_Walk_AllCompleteAfterThreeTicks verifies that all 18 CTs transition
// to the success bucket after three QueryFLCCState ticks (using the tick-based
// completeAfter=3 mechanism; fakeLifecycle returns PrevState:"l11-fielddiag"
// once actionCalls >= 3, so prev_state matches the step's target_state and the
// tray classifies as success under the iter-5h prev_state model).
func TestProbe_Walk_AllCompleteAfterThreeTicks(t *testing.T) {
	fl := &fakeLifecycle{completeAfter: 3} // returns State:"ready" on the 3rd QueryFLCCState
	fe := &fakeEvidence{}
	fem := &fakeEmitter{}
	out, err := New().Run(context.Background(),
		clientsWith(fl, fe, fem, 18),
		probe.StepContext{RunID: "r", StepID: "s", Racks: []string{"rack-a"}, Config: baseConfig(), RLCCWorkflow: newWorkflow()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fe.count() != 18 {
		t.Errorf("per-CT evidence rows = %d, want 18", fe.count())
	}
	if fem.count() != 0 {
		t.Errorf("emitted ignores = %d, want 0", fem.count())
	}

	var body map[string]any
	if err := json.Unmarshal(out.StructuredData, &body); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if body["action_name"] != "field diag" {
		t.Errorf("action_name = %v, want 'field diag'", body["action_name"])
	}
	if int(body["count_success"].(float64)) != 18 {
		t.Errorf("count_success = %v, want 18", body["count_success"])
	}
	if int(body["count_active"].(float64)) != 18 {
		t.Errorf("count_active = %v, want 18", body["count_active"])
	}
	if body["timed_out"].(bool) {
		t.Error("timed_out = true, want false")
	}
	if len(out.Sources) != 1 || out.Sources[0] != "lifecycleclient" {
		t.Errorf("sources = %v, want [lifecycleclient]", out.Sources)
	}
	if out.Verdict != "passed" {
		t.Errorf("verdict = %q, want passed (all CTs succeeded)", out.Verdict)
	}
}

func TestProbe_Walk_EmptyPool(t *testing.T) {
	fl := &fakeLifecycle{completeAfter: 1}
	fe := &fakeEvidence{}
	out, err := New().Run(context.Background(),
		clientsWith(fl, fe, &fakeEmitter{}, 18),
		probe.StepContext{RunID: "r", StepID: "s", Racks: []string{"rack-a"}, Config: baseConfig(),
			RLCCWorkflow:       newWorkflow(),
			IgnoredDeviceslots: allDeviceslots(18)})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fe.count() != 0 {
		t.Errorf("evidence rows = %d, want 0 for empty pool", fe.count())
	}
	var body map[string]any
	_ = json.Unmarshal(out.StructuredData, &body)
	if int(body["count_active"].(float64)) != 0 {
		t.Errorf("count_active = %v, want 0", body["count_active"])
	}
	if int(body["duration_ticks"].(float64)) != 0 {
		t.Errorf("duration_ticks = %v, want 0", body["duration_ticks"])
	}
	if out.Verdict != "passed" {
		t.Errorf("verdict = %q, want passed (empty pool, nothing failed)", out.Verdict)
	}
}

func allDeviceslots(n int) []string {
	out := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, deviceslot(i))
	}
	return out
}

// TestProbe_Walk_TimesOutPartial verifies that a probe times out gracefully
// when CTs never reach a terminal FLCC state within the deadline.
// completeAfter=1000 means QueryFLCCState never returns State:"ready" in
// practice; timed_out=true and count_success=0.
func TestProbe_Walk_TimesOutPartial(t *testing.T) {
	fl := &fakeLifecycle{completeAfter: 1000} // never completes within the deadline
	fe := &fakeEvidence{}
	cfg := baseConfig()
	cfg["poll_interval"] = "20ms"

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	out, err := New().Run(ctx,
		clientsWith(fl, fe, &fakeEmitter{}, 4),
		probe.StepContext{RunID: "r", StepID: "s", Racks: []string{"rack-a"}, Config: cfg, RLCCWorkflow: newWorkflow()})
	if err != nil {
		t.Fatalf("Run: %v", err) // ctx timeout is a normal Output, not an error
	}
	var body map[string]any
	if err := json.Unmarshal(out.StructuredData, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body["timed_out"].(bool) {
		t.Error("timed_out = false, want true")
	}
	if int(body["count_success"].(float64)) != 0 {
		t.Errorf("count_success = %v, want 0", body["count_success"])
	}
	if int(body["count_active"].(float64)) != 4 {
		t.Errorf("count_active = %v, want 4", body["count_active"])
	}
	if int(body["count_in_progress"].(float64)) != 4 {
		t.Errorf("count_in_progress = %v, want 4 (all CTs still pending at timeout)", body["count_in_progress"])
	}
	if out.Verdict != "warning" {
		t.Errorf("verdict = %q, want warning (timed out with in-progress CTs)", out.Verdict)
	}
}

// TestProbe_Walk_RLCCIgnoreMidRun verifies the mid-run RLCC-ignore path.
// Slot 7 becomes RLCC-ignored after the 2nd QueryRLCCIgnored call (the first
// is in buildPool and returns nothing, so slot 7 starts in the pool). With
// completeAfter=3, the remaining 17 CTs move past the diagnostic on tick 3.
// Result: 17 evidence rows, count_success=17, count_ignorable=1.
func TestProbe_Walk_RLCCIgnoreMidRun(t *testing.T) {
	fl := &fakeLifecycle{completeAfter: 3, rlccIgnoreOnCall: 2, rlccSlot: deviceslot(7)}
	fe := &fakeEvidence{}
	fem := &fakeEmitter{}
	out, err := New().Run(context.Background(),
		clientsWith(fl, fe, fem, 18),
		probe.StepContext{RunID: "r", StepID: "s", Racks: []string{"rack-a"}, Config: baseConfig(), RLCCWorkflow: newWorkflow()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fem.count() != 1 {
		t.Fatalf("emitted ignores = %d, want 1", fem.count())
	}
	if fem.ignored[0].Source != "rlcc" || fem.ignored[0].Deviceslot != deviceslot(7) {
		t.Errorf("emitted = %+v, want source=rlcc deviceslot=%s", fem.ignored[0], deviceslot(7))
	}
	if fe.count() != 17 {
		t.Errorf("per-CT evidence rows = %d, want 17 (slot 7 dropped before completion)", fe.count())
	}
	var body map[string]any
	_ = json.Unmarshal(out.StructuredData, &body)
	if int(body["count_success"].(float64)) != 17 {
		t.Errorf("count_success = %v, want 17", body["count_success"])
	}
	if int(body["count_ignorable"].(float64)) != 1 {
		t.Errorf("count_ignorable = %v, want 1", body["count_ignorable"])
	}
	if out.Verdict != "passed" {
		t.Errorf("verdict = %q, want passed (17 success, no fails)", out.Verdict)
	}
}

// TestProbe_PerCTEvidenceShape verifies that each terminal CT produces an
// evidence row with the expected FLCC-model keys. Uses completeAfter=1 so
// both CTs terminate on the first tick (moved past diagnostic).
func TestProbe_PerCTEvidenceShape(t *testing.T) {
	fl := &fakeLifecycle{completeAfter: 1}
	fe := &fakeEvidence{}
	_, err := New().Run(context.Background(),
		clientsWith(fl, fe, &fakeEmitter{}, 2),
		probe.StepContext{RunID: "run-1", StepID: "step-1", Racks: []string{"rack-a"}, Config: baseConfig(), RLCCWorkflow: newWorkflow()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fe.count() != 2 {
		t.Fatalf("evidence rows = %d, want 2", fe.count())
	}
	row := fe.rows[0]
	if row.Deviceslot == nil || *row.Deviceslot == "" {
		t.Error("evidence row missing deviceslot")
	}
	if row.RunID != "run-1" || row.StepID != "step-1" {
		t.Errorf("evidence row run/step = %s/%s, want run-1/step-1", row.RunID, row.StepID)
	}
	var d map[string]any
	if err := json.Unmarshal(row.Payload, &d); err != nil {
		t.Fatalf("decode evidence payload: %v", err)
	}
	for _, k := range []string{"outcome", "flcc_state", "observed_at_tick", "ct_position", "deviceslot", "evaluation_unit"} {
		if _, ok := d[k]; !ok {
			t.Errorf("evidence payload missing key %q", k)
		}
	}
}

// TestProbe_Walk_FLCCBuckets verifies that a single-tick walk classifies
// three CTs into the correct buckets: success (PrevState==diagnosticState),
// failed (State="fail"), and ignorable (State="rma"). All three are terminal
// so the pool drains immediately and timed_out=false.
func TestProbe_Walk_FLCCBuckets(t *testing.T) {
	fl := &fakeLifecycle{flcc: map[string]lifecycleclient.FLCCObservation{
		deviceslot(1): {State: "ready", PrevState: "l11-fielddiag"}, // success (moved past diagnostic)
		deviceslot(2): {State: "fail", Step: "fielddiag"},           // failed
		deviceslot(3): {State: "rma"},                               // ignorable
	}}
	fe := &fakeEvidence{}
	fem := &fakeEmitter{}
	c := clientsWith(fl, fe, fem, 3)

	sc := probe.StepContext{
		RunID: "01HZX0000000000000000RUNX", StepID: "l11", Racks: []string{"rack-a"},
		RLCCWorkflow: newWorkflow(),
		Config: map[string]any{
			"match_handler": "l11-fielddiag", "target_state": "l11-fielddiag",
			"poll_interval": "1ms", "evaluation_unit": "nvlink-domain",
		},
	}
	out, err := (&Probe{}).Run(context.Background(), c, sc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var agg struct {
		CountActive    int    `json:"count_active"`
		CountSuccess   int    `json:"count_success"`
		CountFailed    int    `json:"count_failed"`
		CountIgnorable int    `json:"count_ignorable"`
		EvaluationUnit string `json:"evaluation_unit"`
		TimedOut       bool   `json:"timed_out"`
	}
	if err := json.Unmarshal(out.StructuredData, &agg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if agg.CountActive != 3 || agg.CountSuccess != 1 || agg.CountFailed != 1 || agg.CountIgnorable != 1 {
		t.Errorf("counts = %+v, want active3 success1 failed1 ignorable1", agg)
	}
	if agg.EvaluationUnit != "nvlink-domain" {
		t.Errorf("evaluation_unit = %q, want nvlink-domain", agg.EvaluationUnit)
	}
	if agg.TimedOut {
		t.Error("timed_out = true, want false (all terminal)")
	}
	if fe.count() != 3 {
		t.Errorf("per-CT evidence rows = %d, want 3", fe.count())
	}
	if out.Verdict != "failed" {
		t.Errorf("verdict = %q, want failed (one CT in fail state)", out.Verdict)
	}
}

// TestProbe_Walk_Verdict exercises all three verdict outcomes: passed (all CTs
// succeed), failed (at least one CT in fail state), and warning (timed out with
// in-progress CTs — covered by TestProbe_Walk_TimesOutPartial).
func TestProbe_Walk_Verdict(t *testing.T) {
	const diag = "l11-fielddiag"
	mkSC := func() probe.StepContext {
		return probe.StepContext{
			RunID: "01HZX0000000000000000RUNX", StepID: "l11", Racks: []string{"rack-a"},
			RLCCWorkflow: newWorkflow(),
			Config:       map[string]any{"match_handler": "l11-fielddiag", "target_state": diag, "poll_interval": "1ms", "evaluation_unit": "nvlink-domain"},
		}
	}
	// all pass → passed
	flPass := &fakeLifecycle{flcc: map[string]lifecycleclient.FLCCObservation{
		deviceslot(1): {State: "ready", PrevState: diag},
		deviceslot(2): {State: "ready", PrevState: diag},
		deviceslot(3): {State: "ready", PrevState: diag},
	}}
	out, err := (&Probe{}).Run(context.Background(), clientsWith(flPass, &fakeEvidence{}, &fakeEmitter{}, 3), mkSC())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Verdict != "passed" {
		t.Errorf("all-pass verdict = %q, want passed", out.Verdict)
	}

	// one fail → failed
	flFail := &fakeLifecycle{flcc: map[string]lifecycleclient.FLCCObservation{
		deviceslot(1): {State: "ready", PrevState: diag},
		deviceslot(2): {State: "fail", PrevStep: "gb200-l11-fielddiag"},
		deviceslot(3): {State: "ready", PrevState: diag},
	}}
	out, err = (&Probe{}).Run(context.Background(), clientsWith(flFail, &fakeEvidence{}, &fakeEmitter{}, 3), mkSC())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Verdict != "failed" {
		t.Errorf("one-fail verdict = %q, want failed", out.Verdict)
	}
}
