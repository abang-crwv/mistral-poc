package engine

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"qac/internal/agent"
	"qac/internal/probe"
	"qac/internal/store"
	"qac/internal/verdict"
)

func probeClientsZero() probe.Clients { return probe.Clients{} }

// mockAgent is a controllable Agent for engine tests.
type mockAgent struct {
	ref string
	run func(ctx context.Context, c agent.Clients, sc agent.StepContext) (agent.Output, error)
}

func (m *mockAgent) Type() string      { return "mock_agent" }
func (m *mockAgent) Purpose() string   { return "test" }
func (m *mockAgent) PromptRef() string { return m.ref }
func (m *mockAgent) Run(ctx context.Context, c agent.Clients, sc agent.StepContext) (agent.Output, error) {
	return m.run(ctx, c, sc)
}

func seedAIAssessTemplate(t *testing.T, s *store.Store) {
	t.Helper()
	yamlBody := "schema: qac.template/v1\nid: ai-test\n"
	parsed := `{
		"id":"ai-test","version":1,"title":"AI test","target":{"scope":"rack"},
		"inputs":[{"id":"canary_racks","type":"multi_text","required":true}],
		"steps":[
			{"id":"ai_summary","kind":"ai_assess","title":"AI assessment","prompt_ref":"test.v1","context_steps":["seed"],"expected_verdict":["pass","fail","needs_review"]},
			{"id":"operator_signoff","kind":"signoff","title":"Sign off"}
		]
	}`
	if err := s.UpsertTemplate(t.Context(), "ai-test", 1, yamlBody, parsed); err != nil {
		t.Fatalf("UpsertTemplate: %v", err)
	}
}

func seedAIRun(t *testing.T, s *store.Store, runID string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"template_id":      "ai-test",
		"template_version": 1,
		"inputs":           map[string]any{"canary_racks": []string{"rack-a"}},
		"created_by":       "wpena",
	})
	if err := s.AppendEvent(t.Context(), runID, "RunCreated", payload); err != nil {
		t.Fatalf("RunCreated: %v", err)
	}
}

func TestEngine_AIAssess_RunsAgent_PersistsEvidence_EmitsMappedVerdict(t *testing.T) {
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "qac.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	seedAIAssessTemplate(t, s)

	eng := New(s, nil, probeClientsZero())
	var gotBundle string
	reg := agent.NewRegistry()
	reg.Register(&mockAgent{
		ref: "test.v1",
		run: func(_ context.Context, _ agent.Clients, sc agent.StepContext) (agent.Output, error) {
			gotBundle = sc.EvidenceBundle
			return agent.Output{Verdict: "fail", Confidence: "high", Reasoning: "seed failed", Sources: []string{"mock"}}, nil
		},
	})
	eng.RegisterAgents(reg, agent.Clients{})

	seedAIRun(t, s, "run-ai")
	// Now the run row exists — insert the context-step evidence.
	if err := s.PutEvidence(t.Context(), store.Evidence{
		ID: "ev-seed", RunID: "run-ai", StepID: "seed",
		ContentType: "application/json", Payload: []byte(`{"outcome":"failed"}`),
	}); err != nil {
		t.Fatalf("PutEvidence(seed): %v", err)
	}

	eng.Kick(context.Background(), "run-ai")
	waitFor(t, eng.Wait("run-ai"), 5*time.Second)

	// The agent saw the seed evidence.
	if gotBundle == "" || !strings.Contains(gotBundle, "failed") {
		t.Errorf("agent did not receive the seed evidence bundle; got %q", gotBundle)
	}

	// StepCompleted for ai_summary carries the mapped verdict.
	evs, err := s.LoadEvents(t.Context(), "run-ai")
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	var completedVerdict string
	var haltedAtSignoff = true
	for _, e := range evs {
		if e.Kind == "StepStarted" && stepIDOf(e) == "operator_signoff" {
			haltedAtSignoff = false
		}
		if e.Kind == "StepCompleted" && stepIDOf(e) == "ai_summary" {
			var p struct {
				Verdict string `json:"verdict"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			completedVerdict = p.Verdict
		}
	}
	if completedVerdict != verdict.Failed {
		t.Errorf("ai_summary StepCompleted verdict = %q, want %q", completedVerdict, verdict.Failed)
	}
	if !haltedAtSignoff {
		t.Errorf("walk should halt at operator_signoff, not start it")
	}

	// The assessment evidence row was persisted with the distinctive content type.
	rows, err := s.ListEvidence(t.Context(), "run-ai", "ai_summary")
	if err != nil {
		t.Fatalf("ListEvidence: %v", err)
	}
	if len(rows) != 1 || rows[0].ContentType != "application/vnd.qac.assessment+json" {
		t.Fatalf("assessment evidence not persisted with expected content type; rows=%+v", rows)
	}
}

func stepIDOf(e store.Event) string {
	var p struct {
		StepID string `json:"step_id"`
	}
	_ = json.Unmarshal(e.Payload, &p)
	return p.StepID
}
