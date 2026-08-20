// Package agent defines the agent abstraction the engine drives for ai_assess
// steps, plus a registry keyed by prompt_ref. It mirrors internal/probe so the
// two subsystems are learnable together; future agent purposes register here.
package agent

import (
	"context"
	"sort"
	"sync"

	"qac/internal/llmclient"
	"qac/internal/verdict"
)

// Clients aggregates the backend ports an Agent.Run may need. Grows additively
// (iteration 2 adds a read-only tool registry for tool-using agents).
type Clients struct {
	LLM llmclient.Client
}

// StepContext is the per-ai_assess-step input to Agent.Run. EvidenceBundle is
// the assembled text of the context_steps' evidence rows. The cite/suppression
// flags come from the template step and are honored best-effort in iteration 1.
type StepContext struct {
	RunID               string
	StepID              string
	EvidenceBundle      string
	ExpectedVerdict     []string
	RespectSuppressions bool
	CitePatterns        bool
	CiteGlean           bool
	Inputs              map[string]any
}

// Output is the canonical agent-result envelope, persisted as an evidence row.
// Verdict is the template vocabulary ("pass" | "needs_review" | "fail"); the
// engine maps it to the run-verdict vocabulary via MapVerdict.
type Output struct {
	Verdict      string           `json:"verdict"`
	Confidence   string           `json:"confidence"`
	Reasoning    string           `json:"reasoning"`
	RankedCauses []llmclient.Cause `json:"ranked_causes,omitempty"`
	LikelyOwner  string           `json:"likely_owner,omitempty"`
	Sources      []string         `json:"sources,omitempty"`
}

// Agent is the contract every agent type satisfies. Keyed by PromptRef in the
// registry so the engine can dispatch a template's ai_assess step by its
// prompt_ref.
type Agent interface {
	Type() string
	Purpose() string
	PromptRef() string
	Run(ctx context.Context, clients Clients, sc StepContext) (Output, error)
}

// MapVerdict converts the agent/template verdict vocabulary to the engine's.
// Unknown values map to warning so a human reviews them.
func MapVerdict(raw string) string {
	switch raw {
	case "pass":
		return verdict.Passed
	case "needs_review":
		return verdict.Warning
	case "fail":
		return verdict.Failed
	default:
		return verdict.Warning
	}
}

// Registry maps prompt_ref → Agent. Last Register wins. Concurrent-safe.
type Registry struct {
	mu    sync.RWMutex
	byRef map[string]Agent
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry { return &Registry{byRef: map[string]Agent{}} }

// Register stores a under its PromptRef() key.
func (r *Registry) Register(a Agent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byRef[a.PromptRef()] = a
}

// Get returns the agent registered for promptRef. The bool is false if none.
func (r *Registry) Get(promptRef string) (Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.byRef[promptRef]
	return a, ok
}

// Info is a registered agent's public metadata, for the /api/agents surface.
type Info struct {
	Type      string `json:"type"`
	Purpose   string `json:"purpose"`
	PromptRef string `json:"prompt_ref"`
}

// List returns Info for every registered agent, sorted by Type.
func (r *Registry) List() []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Info, 0, len(r.byRef))
	for _, a := range r.byRef {
		out = append(out, Info{Type: a.Type(), Purpose: a.Purpose(), PromptRef: a.PromptRef()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}
