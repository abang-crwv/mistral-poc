package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"qac/internal/alertclient"
	"qac/internal/probe"
	"qac/internal/store"
)

// mockProbe is a controllable Probe for engine tests — does not depend
// on alertprobe so engine tests stay focused on the engine.
type mockProbe struct {
	typeName string
	cat      probe.Category
	run      func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error)
}

func (m *mockProbe) Type() string             { return m.typeName }
func (m *mockProbe) Category() probe.Category { return m.cat }
func (m *mockProbe) Run(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
	return m.run(ctx, c, sc)
}

func newTestEngine(t *testing.T, probes ...probe.Probe) (*Engine, *store.Store) {
	t.Helper()
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	reg := probe.NewRegistry()
	for _, p := range probes {
		reg.Register(p)
	}
	eng := New(s, reg, probe.Clients{})

	// Seed the iter-4b canonical template into the store so the engine
	// can find a runnable probe_type step.
	seedAlertProbeTemplate(t, s)

	return eng, s
}

// seedAlertProbeTemplate writes a minimal canary template into store
// whose first step is a probe_type=alert_probe step.
func seedAlertProbeTemplate(t *testing.T, s *store.Store) {
	t.Helper()
	yamlBody := `schema: qac.template/v1
id: fw-release-canary
version: 3
title: "FW release canary"
target:
  scope: rack
inputs:
  - id: bundle_tag
    type: text
    required: true
  - id: canary_racks
    type: multi_text
    required: true
steps:
  - id: alert_baseline
    kind: probe
    probe_type: alert_probe
    title: "Pre-zap alert baseline"
`
	parsed := `{
		"id": "firmware-release-canary",
		"version": 3,
		"title": "FW release canary",
		"target": {"scope": "rack"},
		"inputs": [
			{"id":"bundle_tag","type":"text","required":true},
			{"id":"canary_racks","type":"multi_text","required":true}
		],
		"steps": [
			{"id":"alert_baseline","kind":"probe","title":"Pre-zap alert baseline","probe_type":"alert_probe"}
		]
	}`
	if err := s.UpsertTemplate(t.Context(), "firmware-release-canary", 3, yamlBody, parsed); err != nil {
		t.Fatalf("UpsertTemplate: %v", err)
	}
}

// seedRun appends RunCreated for runID and returns when done.
func seedRun(t *testing.T, s *store.Store, runID string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"template_id":      "firmware-release-canary",
		"template_version": 3,
		"inputs": map[string]any{
			"bundle_tag":   "x",
			"canary_racks": []string{"rack-a"},
		},
		"created_by": "wpena",
	})
	if err := s.AppendEvent(t.Context(), runID, "RunCreated", payload); err != nil {
		t.Fatalf("RunCreated: %v", err)
	}
}

func waitFor(t *testing.T, ch <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("engine: timed out after %s waiting for runner", timeout)
	}
}

func TestEngine_Kick_RunsProbe_PersistsEvidence_EmitsCompleted(t *testing.T) {
	mp := &mockProbe{
		typeName: "alert_probe",
		cat:      probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			return probe.Output{
				StructuredData: json.RawMessage(`{"alerts_clear":true}`),
				Sources:        []string{"alertmanager-stub"},
				ProbedAt:       1700000000,
			}, nil
		},
	}
	eng, s := newTestEngine(t, mp)
	runID := "01HZX0000000000000000RUNX"
	seedRun(t, s, runID)

	eng.Kick(t.Context(), runID)
	waitFor(t, eng.Wait(runID), 2*time.Second)

	evs, _ := s.LoadEvents(t.Context(), runID)
	kinds := make([]string, len(evs))
	for i, e := range evs {
		kinds[i] = e.Kind
	}
	// Single probe step walks to completion, so the tail is StepStarted,
	// StepCompleted, then the RunCompleted finalizer.
	wantTail := []string{"StepStarted", "StepCompleted", "RunCompleted"}
	n := len(kinds)
	if n < 3 || kinds[n-3] != wantTail[0] || kinds[n-2] != wantTail[1] || kinds[n-1] != wantTail[2] {
		t.Fatalf("event tail = %v, want %v", kinds, wantTail)
	}

	// Decode the StepCompleted (now second-to-last) to find evidence_id, then GetEvidence.
	var sc StepCompletedPayload
	if err := json.Unmarshal(evs[len(evs)-2].Payload, &sc); err != nil {
		t.Fatalf("decode StepCompleted: %v", err)
	}
	if sc.EvidenceID == "" {
		t.Fatal("StepCompleted.EvidenceID empty")
	}
	ev, err := s.GetEvidence(t.Context(), sc.EvidenceID)
	if err != nil {
		t.Fatalf("GetEvidence: %v", err)
	}
	if string(ev.Payload) != `{"alerts_clear":true}` {
		t.Errorf("evidence payload = %s, want {\"alerts_clear\":true}", ev.Payload)
	}
}

func TestEngine_Kick_ProbeError_EmitsStepFailed_NoEvidence(t *testing.T) {
	mp := &mockProbe{
		typeName: "alert_probe",
		cat:      probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			return probe.Output{}, alertclient.ErrSourceUnavailable
		},
	}
	eng, s := newTestEngine(t, mp)
	runID := "01HZX0000000000000000RUNY"
	seedRun(t, s, runID)

	eng.Kick(t.Context(), runID)
	waitFor(t, eng.Wait(runID), 2*time.Second)

	evs, _ := s.LoadEvents(t.Context(), runID)
	// The probe errors → StepFailed; the walk then ends → RunCompleted
	// finalizer. Find the StepFailed event rather than assume it's last.
	foundSF := false
	for _, e := range evs {
		if e.Kind != "StepFailed" {
			continue
		}
		foundSF = true
		var sf StepFailedPayload
		if err := json.Unmarshal(e.Payload, &sf); err != nil {
			t.Fatalf("decode StepFailed: %v", err)
		}
		if sf.Error == "" {
			t.Error("StepFailed.Error empty")
		}
	}
	if !foundSF {
		t.Fatalf("no StepFailed event in run history")
	}

	// No evidence rows.
	var n int
	_ = s.DB().QueryRowContext(t.Context(), `SELECT count(*) FROM evidence WHERE run_id = ?`, runID).Scan(&n)
	if n != 0 {
		t.Errorf("evidence rows = %d, want 0", n)
	}
}

func TestEngine_Kick_ProbePanic_EmitsStepFailed(t *testing.T) {
	mp := &mockProbe{
		typeName: "alert_probe",
		cat:      probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			panic("boom")
		},
	}
	eng, s := newTestEngine(t, mp)
	runID := "01HZX0000000000000000RUNZ"
	seedRun(t, s, runID)

	eng.Kick(t.Context(), runID)
	waitFor(t, eng.Wait(runID), 2*time.Second)

	evs, _ := s.LoadEvents(t.Context(), runID)
	last := evs[len(evs)-1]
	if last.Kind != "StepFailed" {
		t.Fatalf("last event = %q, want StepFailed", last.Kind)
	}
	var sf StepFailedPayload
	_ = json.Unmarshal(last.Payload, &sf)
	if sf.Error == "" || sf.Error[:5] != "panic" {
		t.Errorf("StepFailed.Error = %q, want panic-prefixed", sf.Error)
	}
}

func TestEngine_Kick_UnknownProbeType_EmitsStepFailed(t *testing.T) {
	// Empty registry — first probe step references "alert_probe", which is unknown.
	eng, s := newTestEngine(t /* no probes registered */)
	runID := "01HZX0000000000000000RUN1"
	seedRun(t, s, runID)

	eng.Kick(t.Context(), runID)
	waitFor(t, eng.Wait(runID), 2*time.Second)

	evs, _ := s.LoadEvents(t.Context(), runID)
	// Unknown probe → StepFailed; the walk then ends → RunCompleted
	// finalizer. Assert a StepFailed is present rather than last.
	foundSF := false
	for _, e := range evs {
		if e.Kind == "StepFailed" {
			foundSF = true
			break
		}
	}
	if !foundSF {
		t.Fatalf("no StepFailed event in run history")
	}
}

func TestEngine_Wait_UnknownRunID_ReturnsPreClosedChannel(t *testing.T) {
	eng, _ := newTestEngine(t)
	ch := eng.Wait("01HZXNOSUCHRUNXXXXXXXXXXX")
	select {
	case <-ch:
		// pre-closed: read succeeds immediately
	case <-time.After(50 * time.Millisecond):
		t.Fatal("Wait(unknownRunID): blocked, want pre-closed channel")
	}
}

func TestEngine_Wait_ClosesAfterRunnerCompletes(t *testing.T) {
	// Slow probe so Wait clearly observes a transition from blocked to closed.
	released := make(chan struct{})
	mp := &mockProbe{
		typeName: "alert_probe",
		cat:      probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			<-released
			return probe.Output{StructuredData: json.RawMessage(`{}`), ProbedAt: 1}, nil
		},
	}
	eng, s := newTestEngine(t, mp)
	runID := "01HZX0000000000000000WAIT"
	seedRun(t, s, runID)

	eng.Kick(t.Context(), runID)
	ch := eng.Wait(runID)
	select {
	case <-ch:
		t.Fatal("Wait closed before runner finished")
	case <-time.After(50 * time.Millisecond):
		// expected: still in-flight
	}
	close(released)
	waitFor(t, ch, 2*time.Second)
}

func TestEngine_Kick_Idempotent(t *testing.T) {
	// A second Kick for the same runID must not spawn another goroutine.
	var seen int32
	var mu sync.Mutex
	mp := &mockProbe{
		typeName: "alert_probe",
		cat:      probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			mu.Lock()
			seen++
			mu.Unlock()
			return probe.Output{StructuredData: json.RawMessage(`{}`), ProbedAt: 1}, nil
		},
	}
	eng, s := newTestEngine(t, mp)
	runID := "01HZX0000000000000000IDEM"
	seedRun(t, s, runID)

	eng.Kick(t.Context(), runID)
	eng.Kick(t.Context(), runID) // should be no-op
	waitFor(t, eng.Wait(runID), 2*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if seen != 1 {
		t.Fatalf("probe.Run invocations = %d, want 1", seen)
	}
}

func TestEngine_Shutdown_WaitsForInflight(t *testing.T) {
	released := make(chan struct{})
	mp := &mockProbe{
		typeName: "alert_probe",
		cat:      probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			<-released
			return probe.Output{StructuredData: json.RawMessage(`{}`), ProbedAt: 1}, nil
		},
	}
	eng, s := newTestEngine(t, mp)
	runID := "01HZX0000000000000000SHUT"
	seedRun(t, s, runID)

	eng.Kick(t.Context(), runID)

	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		shutdownDone <- eng.Shutdown(ctx)
	}()

	// Confirm Shutdown is still pending.
	select {
	case <-shutdownDone:
		t.Fatal("Shutdown returned before runner released")
	case <-time.After(50 * time.Millisecond):
	}

	close(released)
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not return after runner released")
	}
}

func TestEngine_Shutdown_TimeoutReturnsDeadlineExceeded(t *testing.T) {
	// A probe that never returns; Shutdown with a 50ms ctx must
	// surface context.DeadlineExceeded so callers using errors.Is can
	// distinguish "drained cleanly" from "timed out".
	stuck := make(chan struct{})
	defer close(stuck) // unblock the goroutine when the test exits so it doesn't leak
	mp := &mockProbe{
		typeName: "alert_probe",
		cat:      probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			<-stuck
			return probe.Output{StructuredData: json.RawMessage(`{}`), ProbedAt: 1}, nil
		},
	}
	eng, s := newTestEngine(t, mp)
	runID := "01HZX0000000000000000TIMO"
	seedRun(t, s, runID)

	eng.Kick(t.Context(), runID)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := eng.Shutdown(ctx)
	if err == nil {
		t.Fatal("Shutdown returned nil, want context.DeadlineExceeded")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want errors.Is(err, context.DeadlineExceeded)", err)
	}
}

func TestEngine_Kick_AfterRunnerCompletes_StartsFreshRunner(t *testing.T) {
	// iter-4c semantics: the walk's seenSteps filter (driven by events
	// already in the log) means a re-Kick after a completed run spawns
	// a new goroutine that immediately exits — every runnable step is
	// already in `seen`. This is the desired idempotency property.
	// (iter-4b's looser semantics — where re-Kick re-ran the probe —
	// were a single-step accident, not a contract.)
	var seen int
	var mu sync.Mutex
	mp := &mockProbe{
		typeName: "alert_probe",
		cat:      probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			mu.Lock()
			seen++
			mu.Unlock()
			return probe.Output{StructuredData: json.RawMessage(`{}`), ProbedAt: 1}, nil
		},
	}
	eng, s := newTestEngine(t, mp)
	runID := "01HZX0000000000000000RKIK"
	seedRun(t, s, runID)

	eng.Kick(t.Context(), runID)
	waitFor(t, eng.Wait(runID), 2*time.Second)

	// Second Kick after completion — spawns a goroutine that finds no
	// unrun steps and exits.
	eng.Kick(t.Context(), runID)
	waitFor(t, eng.Wait(runID), 2*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if seen != 1 {
		t.Fatalf("probe.Run invocations = %d, want 1 (re-Kick must not re-run completed steps)", seen)
	}
}

// multiStepTemplate seeds a synthetic template into the store whose
// steps are all probe_type. Returns nothing; the caller seeds a run
// referencing templateID afterward. The caller chooses how many
// probe_type steps and what probe_type each one uses (so unknown-
// probe-type tests can mix in a non-registered type, and halt tests
// can mix in operator-gated kinds).
func multiStepTemplate(t *testing.T, s *store.Store, templateID string, stepDefs []struct{ ID, ProbeType, Kind string }) {
	t.Helper()
	yamlSteps := ""
	for _, sd := range stepDefs {
		kind := sd.Kind
		if kind == "" {
			kind = "probe"
		}
		if sd.ProbeType != "" {
			yamlSteps += fmt.Sprintf(
				"  - id: %s\n    kind: %s\n    probe_type: %s\n    title: %q\n",
				sd.ID, kind, sd.ProbeType, sd.ID)
		} else {
			switch kind {
			case "review":
				yamlSteps += fmt.Sprintf(
					"  - id: %s\n    kind: review\n    title: %q\n    sources: [%s]\n",
					sd.ID, sd.ID, stepDefs[0].ID)
			case "ai_assess":
				yamlSteps += fmt.Sprintf(
					"  - id: %s\n    kind: ai_assess\n    title: %q\n    prompt_ref: stub\n",
					sd.ID, sd.ID)
			case "signoff":
				yamlSteps += fmt.Sprintf(
					"  - id: %s\n    kind: signoff\n    title: %q\n    final_verdict_options: [pass, fail]\n",
					sd.ID, sd.ID)
			}
		}
	}
	yamlBody := fmt.Sprintf(`schema: qac.template/v1
id: %s
version: 1
title: "multi-step test"
target:
  scope: rack
inputs:
  - id: bundle_tag
    type: text
    required: true
  - id: canary_racks
    type: multi_text
    required: true
steps:
%s`, templateID, yamlSteps)

	// Hand-build the parsed JSON to avoid invoking the YAML parser here.
	type ps struct {
		ID                  string   `json:"id"`
		Kind                string   `json:"kind"`
		Title               string   `json:"title"`
		ProbeType           string   `json:"probe_type,omitempty"`
		Sources             []string `json:"sources,omitempty"`
		PromptRef           string   `json:"prompt_ref,omitempty"`
		FinalVerdictOptions []string `json:"final_verdict_options,omitempty"`
	}
	stepsJSON := make([]ps, 0, len(stepDefs))
	for _, sd := range stepDefs {
		kind := sd.Kind
		if kind == "" {
			kind = "probe"
		}
		p := ps{ID: sd.ID, Kind: kind, Title: sd.ID, ProbeType: sd.ProbeType}
		switch kind {
		case "review":
			p.Sources = []string{stepDefs[0].ID}
		case "ai_assess":
			p.PromptRef = "stub"
		case "signoff":
			p.FinalVerdictOptions = []string{"pass", "fail"}
		}
		stepsJSON = append(stepsJSON, p)
	}
	parsed, _ := json.Marshal(map[string]any{
		"id": templateID, "version": 1, "title": "multi-step test",
		"target": map[string]any{"scope": "rack"},
		"inputs": []map[string]any{
			{"id": "bundle_tag", "type": "text", "required": true},
			{"id": "canary_racks", "type": "multi_text", "required": true},
		},
		"steps": stepsJSON,
	})
	if err := s.UpsertTemplate(t.Context(), templateID, 1, yamlBody, string(parsed)); err != nil {
		t.Fatalf("UpsertTemplate: %v", err)
	}
}

func seedRunWithTemplate(t *testing.T, s *store.Store, runID, templateID string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"template_id":      templateID,
		"template_version": 1,
		"inputs": map[string]any{
			"bundle_tag":   "x",
			"canary_racks": []string{"rack-a"},
		},
		"created_by": "wpena",
	})
	if err := s.AppendEvent(t.Context(), runID, "RunCreated", payload); err != nil {
		t.Fatalf("RunCreated: %v", err)
	}
}

func TestEngine_Walk_MultipleStepsRunSequentially(t *testing.T) {
	// Three probe_type steps all using the mock probe. Walk should fire
	// StepStarted/StepCompleted for each in template order.
	mp := &mockProbe{
		typeName: "p_walk",
		cat:      probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			return probe.Output{StructuredData: json.RawMessage(`{}`), ProbedAt: 1}, nil
		},
	}
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	reg := probe.NewRegistry()
	reg.Register(mp)
	eng := New(s, reg, probe.Clients{})

	multiStepTemplate(t, s, "tpl-walk", []struct{ ID, ProbeType, Kind string }{
		{"step_a", "p_walk", "probe"},
		{"step_b", "p_walk", "probe"},
		{"step_c", "p_walk", "probe"},
	})
	runID := "01HZX0000000000000000WLK1"
	seedRunWithTemplate(t, s, runID, "tpl-walk")

	eng.Kick(t.Context(), runID)
	waitFor(t, eng.Wait(runID), 2*time.Second)

	evs, _ := s.LoadEvents(t.Context(), runID)
	wantTail := []string{
		"StepStarted", "StepCompleted",
		"StepStarted", "StepCompleted",
		"StepStarted", "StepCompleted",
		"RunCompleted", // walk reached the end with no operator gate
	}
	if len(evs) < len(wantTail)+1 {
		t.Fatalf("event count = %d, want at least %d", len(evs), len(wantTail)+1)
	}
	tail := evs[len(evs)-len(wantTail):]
	for i, want := range wantTail {
		if tail[i].Kind != want {
			t.Errorf("event tail[%d] = %q, want %q", i, tail[i].Kind, want)
		}
	}
}

func TestEngine_Walk_ContinuesPastStepFailed(t *testing.T) {
	// Middle step errors. Walk should still emit StepStarted for it,
	// then a StepFailed, then continue to step_c which completes.
	var which int
	var mu sync.Mutex
	mp := &mockProbe{
		typeName: "p_mixed",
		cat:      probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			mu.Lock()
			which++
			n := which
			mu.Unlock()
			if n == 2 {
				return probe.Output{}, errors.New("middle step intentionally broken")
			}
			return probe.Output{StructuredData: json.RawMessage(`{}`), ProbedAt: 1}, nil
		},
	}
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	reg := probe.NewRegistry()
	reg.Register(mp)
	eng := New(s, reg, probe.Clients{})
	multiStepTemplate(t, s, "tpl-mixed", []struct{ ID, ProbeType, Kind string }{
		{"step_a", "p_mixed", "probe"},
		{"step_b", "p_mixed", "probe"},
		{"step_c", "p_mixed", "probe"},
	})
	runID := "01HZX0000000000000000WLK2"
	seedRunWithTemplate(t, s, runID, "tpl-mixed")

	eng.Kick(t.Context(), runID)
	waitFor(t, eng.Wait(runID), 2*time.Second)

	evs, _ := s.LoadEvents(t.Context(), runID)
	var started, completed, failed int
	var lastByStep = map[string]string{}
	for _, e := range evs {
		switch e.Kind {
		case "StepStarted":
			started++
		case "StepCompleted":
			completed++
		case "StepFailed":
			failed++
		}
		if e.Kind == "StepCompleted" || e.Kind == "StepFailed" {
			var p struct{ StepID string `json:"step_id"` }
			_ = json.Unmarshal(e.Payload, &p)
			lastByStep[p.StepID] = e.Kind
		}
	}
	if started != 3 || completed != 2 || failed != 1 {
		t.Fatalf("event counts: started=%d completed=%d failed=%d; want 3/2/1", started, completed, failed)
	}
	if lastByStep["step_b"] != "StepFailed" {
		t.Errorf("step_b final = %q, want StepFailed", lastByStep["step_b"])
	}
	if lastByStep["step_c"] != "StepCompleted" {
		t.Errorf("step_c final = %q, want StepCompleted", lastByStep["step_c"])
	}
}

func TestEngine_Walk_HaltsAtOperatorGatedStep(t *testing.T) {
	// Template: 2 probe_type steps + 1 review step. Walk runs the two
	// probes, then halts at the review step (no event emitted for it).
	mp := &mockProbe{
		typeName: "p_halt",
		cat:      probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			return probe.Output{StructuredData: json.RawMessage(`{}`), ProbedAt: 1}, nil
		},
	}
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	reg := probe.NewRegistry()
	reg.Register(mp)
	eng := New(s, reg, probe.Clients{})
	multiStepTemplate(t, s, "tpl-halt", []struct{ ID, ProbeType, Kind string }{
		{"step_a", "p_halt", "probe"},
		{"step_b", "p_halt", "probe"},
		{"review_step", "", "review"},
	})
	runID := "01HZX0000000000000000WLK3"
	seedRunWithTemplate(t, s, runID, "tpl-halt")

	eng.Kick(t.Context(), runID)
	waitFor(t, eng.Wait(runID), 2*time.Second)

	evs, _ := s.LoadEvents(t.Context(), runID)
	for _, e := range evs {
		if e.Kind == "StepStarted" || e.Kind == "StepCompleted" || e.Kind == "StepFailed" {
			var p struct{ StepID string `json:"step_id"` }
			_ = json.Unmarshal(e.Payload, &p)
			if p.StepID == "review_step" {
				t.Errorf("emitted %q event for review_step; walk should have halted before it", e.Kind)
			}
		}
	}
	var started int
	for _, e := range evs {
		if e.Kind == "StepStarted" {
			started++
		}
	}
	if started != 2 {
		t.Errorf("StepStarted count = %d, want 2 (review step should not start)", started)
	}
}

func TestEngine_Walk_UnknownProbeType_RestContinue(t *testing.T) {
	// Middle step references an unregistered probe type. Walk records a
	// StepFailed for it and continues to step_c.
	mp := &mockProbe{
		typeName: "p_known",
		cat:      probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			return probe.Output{StructuredData: json.RawMessage(`{}`), ProbedAt: 1}, nil
		},
	}
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	reg := probe.NewRegistry()
	reg.Register(mp)
	eng := New(s, reg, probe.Clients{})
	multiStepTemplate(t, s, "tpl-unk", []struct{ ID, ProbeType, Kind string }{
		{"step_a", "p_known", "probe"},
		{"step_b", "p_ghost", "probe"}, // unregistered
		{"step_c", "p_known", "probe"},
	})
	runID := "01HZX0000000000000000WLK4"
	seedRunWithTemplate(t, s, runID, "tpl-unk")

	eng.Kick(t.Context(), runID)
	waitFor(t, eng.Wait(runID), 2*time.Second)

	evs, _ := s.LoadEvents(t.Context(), runID)
	lastByStep := map[string]string{}
	for _, e := range evs {
		if e.Kind == "StepCompleted" || e.Kind == "StepFailed" {
			var p struct{ StepID string `json:"step_id"` }
			_ = json.Unmarshal(e.Payload, &p)
			lastByStep[p.StepID] = e.Kind
		}
	}
	if lastByStep["step_a"] != "StepCompleted" {
		t.Errorf("step_a final = %q, want StepCompleted", lastByStep["step_a"])
	}
	if lastByStep["step_b"] != "StepFailed" {
		t.Errorf("step_b final = %q, want StepFailed (unknown probe type)", lastByStep["step_b"])
	}
	if lastByStep["step_c"] != "StepCompleted" {
		t.Errorf("step_c final = %q, want StepCompleted (walk continued past unknown)", lastByStep["step_c"])
	}
}

func TestEngine_Kick_StepTimeoutBackstop_iter5g(t *testing.T) {
	mp := &mockProbe{
		typeName: "rlcc_action_probe",
		cat:      probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			<-ctx.Done() // never completes on its own; relies on the step timeout
			return probe.Output{StructuredData: json.RawMessage(`{"timed_out":true}`), ProbedAt: 1}, nil
		},
	}
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	reg := probe.NewRegistry()
	reg.Register(mp)
	eng := New(s, reg, probe.Clients{})

	// Single probe_type step with a 50ms timeout.
	parsed := `{"id":"t","version":1,"title":"t","target":{"scope":"rack"},
		"inputs":[{"id":"canary_racks","type":"multi_text","required":true}],
		"steps":[{"id":"s1","kind":"probe","title":"s1","probe_type":"rlcc_action_probe","timeout":"50ms",
		          "racks":"{{ inputs.canary_racks }}"}]}`
	if err := s.UpsertTemplate(t.Context(), "t", 1, "schema: qac.template/v1\n", parsed); err != nil {
		t.Fatalf("UpsertTemplate: %v", err)
	}
	runID := "01HZX0000000000000000TMOX"
	rc, _ := json.Marshal(map[string]any{"template_id": "t", "created_by": "wpena",
		"inputs": map[string]any{"canary_racks": []string{"rack-a"}}})
	if err := s.AppendEvent(t.Context(), runID, "RunCreated", rc); err != nil {
		t.Fatalf("RunCreated: %v", err)
	}

	eng.Kick(t.Context(), runID)
	select {
	case <-eng.Wait(runID):
	case <-time.After(3 * time.Second):
		t.Fatal("step did not time out within 3s; backstop not applied")
	}

	evs, _ := s.LoadEvents(t.Context(), runID)
	var sawCompleted bool
	for _, e := range evs {
		if e.Kind == "StepCompleted" {
			sawCompleted = true
		}
	}
	if !sawCompleted {
		t.Error("want StepCompleted (probe returned partial Output after ctx timeout)")
	}
}

func TestEngine_Kick_StepVerdict_iter5h(t *testing.T) {
	mp := &mockProbe{
		typeName: "rlcc_action_probe", cat: probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			return probe.Output{StructuredData: json.RawMessage(`{}`), ProbedAt: 1, Verdict: "failed"}, nil
		},
	}
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil { t.Fatalf("Open: %v", err) }
	t.Cleanup(func() { _ = s.Close() })
	reg := probe.NewRegistry(); reg.Register(mp)
	eng := New(s, reg, probe.Clients{})
	parsed := `{"id":"t","version":1,"title":"t","target":{"scope":"rack"},
		"inputs":[{"id":"canary_racks","type":"multi_text","required":true}],
		"steps":[{"id":"s1","kind":"probe","title":"s1","probe_type":"rlcc_action_probe","racks":"{{ inputs.canary_racks }}"}]}`
	if err := s.UpsertTemplate(t.Context(), "t", 1, "schema: qac.template/v1\n", parsed); err != nil { t.Fatalf("UpsertTemplate: %v", err) }
	runID := "01HZX0000000000000000VRDT"
	rc, _ := json.Marshal(map[string]any{"template_id": "t", "created_by": "wpena", "inputs": map[string]any{"canary_racks": []string{"rack-a"}}})
	if err := s.AppendEvent(t.Context(), runID, "RunCreated", rc); err != nil { t.Fatalf("RunCreated: %v", err) }
	eng.Kick(t.Context(), runID)
	<-eng.Wait(runID)
	evs, _ := s.LoadEvents(t.Context(), runID)
	var got string
	for _, e := range evs {
		if e.Kind == "StepCompleted" {
			var p struct{ Verdict string `json:"verdict"` }
			_ = json.Unmarshal(e.Payload, &p)
			got = p.Verdict
		}
	}
	if got != "failed" { t.Errorf("StepCompleted.Verdict = %q, want failed", got) }
}

func TestEngine_Kick_PopulatesStepContext_iter5e(t *testing.T) {
	var (
		mu      sync.Mutex
		gotSC   probe.StepContext
		gotSeen bool
	)
	mp := &mockProbe{
		typeName: "rlcc_action_probe",
		cat:      probe.CategoryGatherer,
		run: func(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
			mu.Lock()
			gotSC = sc
			gotSeen = true
			mu.Unlock()
			return probe.Output{StructuredData: json.RawMessage(`{}`), ProbedAt: 1700000000}, nil
		},
	}

	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	reg := probe.NewRegistry()
	reg.Register(mp)
	eng := New(s, reg, probe.Clients{})

	// Template with a probe_type step that also carries action.args.
	parsed := `{
		"id":"firmware-release-canary","version":3,"title":"t",
		"target":{"scope":"rack"},
		"inputs":[{"id":"bundle_tag","type":"text","required":true},
		          {"id":"canary_racks","type":"multi_text","required":true}],
		"steps":[{"id":"l11","kind":"probe","title":"l11","probe_type":"rlcc_action_probe",
		          "racks":"{{ inputs.canary_racks }}",
		          "action":{"tool":"rlcc","args":{"match_handler":"l11-fielddiag","target_state":"l11-fielddiag","poll_interval":"1ms"}}}]
	}`
	if err := s.UpsertTemplate(t.Context(), "firmware-release-canary", 3, "schema: qac.template/v1\nid: firmware-release-canary\n", parsed); err != nil {
		t.Fatalf("UpsertTemplate: %v", err)
	}

	runID := "01HZX0000000000000000RUNX"
	// RunCreated as generic JSON: the store decoder reads inputs.canary_racks
	// (for resolveRacks) and rlcc_workflow from the raw payload. engine's
	// RunCreatedPayload has no Inputs field, so the map form is required here.
	rc, _ := json.Marshal(map[string]any{
		"template_id": "firmware-release-canary",
		"created_by":  "wpena",
		"inputs":      map[string]any{"bundle_tag": "x", "canary_racks": []string{"rack-a"}},
		"rlcc_workflow": map[string]any{
			"name": "gb200-canary", "source_sha": "abc123",
			"actions": []map[string]any{{"name": "fd", "handler": "l11-fielddiag"}},
		},
	})
	if err := s.AppendEvent(t.Context(), runID, "RunCreated", rc); err != nil {
		t.Fatalf("RunCreated: %v", err)
	}
	// Operator-ignored CT.
	ign, _ := json.Marshal(BMNIgnoredPayload{Rack: "rack-a", Deviceslot: "rack-a-node-03-z", Source: "operator", MarkedAt: 1700000000})
	if err := s.AppendEvent(t.Context(), runID, "BMNIgnored", ign); err != nil {
		t.Fatalf("BMNIgnored: %v", err)
	}

	eng.Kick(t.Context(), runID)
	waitFor(t, eng.Wait(runID), 2*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if !gotSeen {
		t.Fatal("probe was never run")
	}
	if gotSC.RLCCWorkflow == nil || gotSC.RLCCWorkflow.Name != "gb200-canary" {
		t.Errorf("sc.RLCCWorkflow = %+v, want gb200-canary", gotSC.RLCCWorkflow)
	}
	if gotSC.Config["match_handler"] != "l11-fielddiag" {
		t.Errorf("sc.Config[match_handler] = %v, want l11-fielddiag", gotSC.Config["match_handler"])
	}
	if len(gotSC.IgnoredDeviceslots) != 1 || gotSC.IgnoredDeviceslots[0] != "rack-a-node-03-z" {
		t.Errorf("sc.IgnoredDeviceslots = %v, want [rack-a-node-03-z]", gotSC.IgnoredDeviceslots)
	}
}
