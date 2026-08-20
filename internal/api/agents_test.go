package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"qac/internal/agent"
	"qac/internal/agent/canaryassessor"
	"qac/internal/engine"
	"qac/internal/llmclient"
	"qac/internal/probe"
	"qac/internal/store"
)

func TestListAgentsHandler(t *testing.T) {
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "qac.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	eng := engine.New(s, probe.NewRegistry(), probe.Clients{})
	reg := agent.NewRegistry()
	reg.Register(canaryassessor.New())
	eng.RegisterAgents(reg, agent.Clients{LLM: llmclient.NewFixtureClient()})

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()
	listAgentsHandler(eng).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Agents []struct {
			Type      string `json:"type"`
			Purpose   string `json:"purpose"`
			PromptRef string `json:"prompt_ref"`
			Model     string `json:"model"`
			Live      bool   `json:"live"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Agents) != 1 {
		t.Fatalf("agents = %d, want 1", len(body.Agents))
	}
	a := body.Agents[0]
	if a.Type != "canary_assessor" || a.PromptRef != "fw-release-canary.summary.v1" {
		t.Errorf("agent identity wrong: %+v", a)
	}
	if a.Purpose != canaryassessor.New().Purpose() {
		t.Errorf("purpose = %q, want %q", a.Purpose, canaryassessor.New().Purpose())
	}
	if a.Model != "fixture" || a.Live {
		t.Errorf("agent backend = %q live=%v, want fixture/false", a.Model, a.Live)
	}
}
