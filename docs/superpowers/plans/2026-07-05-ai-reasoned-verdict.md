# AI-reasoned verdict — Implementation Plan (iteration 1)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An agent reasons over the evidence qac's probes gathered and emits an advisory verdict (with confidence and ranked causes) at the `ai_assess` step; the verdict rolls up into run status and the operator signs off.

**Architecture:** A general agent framework (`internal/agent` registry + interface, mirroring `internal/probe`), an `internal/llmclient` port (fixture default + live Anthropic via the official Go SDK), and an `internal/prompt` embedded-rubric registry. The engine's runner stops halting at `ai_assess` and instead dispatches to the agent registry, persisting the agent's structured output as evidence and emitting `StepCompleted` with the mapped verdict — reusing the existing verdict/rollup plumbing (no new event kinds). A read-only `GET /api/agents` + a real Agents page replace the stub.

**Tech Stack:** Go 1.26 (module path bare `qac`), `github.com/anthropics/anthropic-sdk-go`, `modernc.org/sqlite`, cobra; React 18 + Vite + TS + TanStack Query + MSW.

## Global Constraints

- **Branch first.** Current branch is `main`. Create `feat/ai-reasoned-verdict` before Task 1; commit per task; **do not open a PR** (project holds a local branch until MVPs complete — memory `qac-current-state`).
- **Self-contained.** Module path is bare `qac`; **no `replace` directives**; do not import sibling argo projects. `github.com/anthropics/anthropic-sdk-go` is a normal external dependency (fetched via the Go proxy) and is allowed.
- **Verdict vocabularies.** Agent/template vocabulary is `pass | needs_review | fail`; engine vocabulary (`internal/verdict`) is `passed | warning | failed`. Map `pass→passed`, `needs_review→warning`, `fail→failed`.
- **Cloud Console copy guide** for all user-facing strings (CLI output, UI text, API descriptions): sentence case, "and" not "&", no exclamation points, preserve technical identifiers verbatim.
- **Model default** `claude-opus-4-8`, adaptive thinking, effort left at the SDK default (`high`). Do not set sampling params or `budget_tokens` (they 400 on Opus 4.8).
- **Assessment evidence content type:** `application/vnd.qac.assessment+json` (the frontend keys off this).

---

### Task 0: Create the feature branch

- [ ] **Step 1: Branch off main**

```bash
cd /Users/wpena/coreweave/qac
git checkout -b feat/ai-reasoned-verdict
```

- [ ] **Step 2: Confirm build is green before starting**

Run: `go build ./... && go test ./internal/engine/ -count=1`
Expected: builds; engine tests PASS.

---

### Task 1: Prompt registry (`internal/prompt`)

**Files:**
- Create: `internal/prompt/prompt.go`
- Create: `internal/prompt/prompts/fw-release-canary.summary.v1.md`
- Test: `internal/prompt/prompt_test.go`

**Interfaces:**
- Produces: `prompt.Get(ref string) (string, error)` — returns the embedded prompt/rubric body for a `prompt_ref`.

- [ ] **Step 1: Write the failing test**

Create `internal/prompt/prompt_test.go`:

```go
package prompt

import (
	"strings"
	"testing"
)

func TestGet_CanaryRubric(t *testing.T) {
	body, err := Get("fw-release-canary.summary.v1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(body, "record_assessment") {
		t.Errorf("rubric should instruct the model to call record_assessment; got:\n%s", body)
	}
}

func TestGet_Unknown(t *testing.T) {
	if _, err := Get("does.not.exist"); err == nil {
		t.Fatal("expected error for unknown prompt_ref")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/prompt/ -run TestGet -v`
Expected: FAIL (package does not compile — `Get` undefined).

- [ ] **Step 3: Write the prompt registry**

Create `internal/prompt/prompt.go`:

```go
// Package prompt is the embedded registry of agent prompts/rubrics, keyed by
// the template's prompt_ref. Storing prompts as files (not Go string literals)
// keeps them reviewable and lets an operator diff rubric versions.
package prompt

import (
	"embed"
	"fmt"
)

//go:embed prompts/*.md
var fs embed.FS

// Get returns the prompt body registered under ref (a template prompt_ref,
// e.g. "fw-release-canary.summary.v1"). Returns an error when no prompt is
// registered for ref.
func Get(ref string) (string, error) {
	b, err := fs.ReadFile("prompts/" + ref + ".md")
	if err != nil {
		return "", fmt.Errorf("prompt %q not found: %w", ref, err)
	}
	return string(b), nil
}
```

- [ ] **Step 4: Write the canary rubric**

Create `internal/prompt/prompts/fw-release-canary.summary.v1.md`:

```markdown
You are a fleet-operations reviewer assessing a firmware-release canary run for CoreWeave.

You are given the evidence that automated probes gathered for the canary racks: alert history, firmware inventory and bundle convergence, HPC-verification status, GPU performance, and AWX zap-job results. You did not run the fleet; you reason over what the probes captured.

Your job is to produce one advisory verdict for the run, following these rules:

- Reason from the evidence only. Do not assume state the evidence does not show.
- State your confidence (high, medium, or low) and the reasoning behind it.
- Narrow the field: rank the most likely causes of any problem you find, and name the team most likely to own each, rather than asserting a single certain answer.
- You do not decide the outcome. A human operator signs off after reading your assessment; your verdict is advisory.

Choose the verdict:
- pass — the evidence shows the release landed cleanly and the racks are healthy.
- fail — the evidence shows a clear problem attributable to this release.
- needs_review — the evidence is ambiguous, incomplete, or shows something a human should look at before deciding.

Report your assessment by calling the record_assessment tool with your verdict, confidence, reasoning, ranked likely causes, and the likely owning team. Do not answer in prose; call the tool.
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/prompt/ -run TestGet -v`
Expected: PASS (both tests).

- [ ] **Step 6: Commit**

```bash
git add internal/prompt
git commit -m "feat(agent): embedded prompt registry + canary assessor rubric"
```

---

### Task 2: LLM port + fixture client (`internal/llmclient`)

**Files:**
- Create: `internal/llmclient/llmclient.go`
- Test: `internal/llmclient/llmclient_test.go`

**Interfaces:**
- Produces:
  - `llmclient.Assessment struct { Verdict, Confidence, Reasoning string; RankedCauses []Cause; LikelyOwner, Model string }`
  - `llmclient.Cause struct { Summary, LikelyOwner, Confidence string }`
  - `llmclient.AssessRequest struct { SystemPrompt, EvidenceBundle string; ExpectedVerdict []string }`
  - `llmclient.Info struct { Model string; Live bool }`
  - `llmclient.Client interface { Assess(ctx, AssessRequest) (Assessment, error); Info() Info }`
  - `llmclient.NewFixtureClient() *FixtureClient`

- [ ] **Step 1: Write the failing test**

Create `internal/llmclient/llmclient_test.go`:

```go
package llmclient

import (
	"context"
	"testing"
)

func TestFixtureClient_PassWhenNoFailureMarker(t *testing.T) {
	a, err := NewFixtureClient().Assess(context.Background(), AssessRequest{
		EvidenceBundle:  `{"outcome":"ok","rack":"rack-a"}`,
		ExpectedVerdict: []string{"pass", "fail", "needs_review"},
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Verdict != "pass" {
		t.Errorf("verdict = %q, want pass", a.Verdict)
	}
	if a.Model != "fixture" {
		t.Errorf("model = %q, want fixture", a.Model)
	}
}

func TestFixtureClient_FailWhenFailureMarker(t *testing.T) {
	a, err := NewFixtureClient().Assess(context.Background(), AssessRequest{
		EvidenceBundle:  `{"outcome":"failed","rack":"rack-a"}`,
		ExpectedVerdict: []string{"pass", "fail", "needs_review"},
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Verdict != "fail" {
		t.Errorf("verdict = %q, want fail", a.Verdict)
	}
}

func TestFixtureClient_Info(t *testing.T) {
	got := NewFixtureClient().Info()
	if got.Live || got.Model != "fixture" {
		t.Errorf("Info() = %+v, want {fixture false}", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llmclient/ -v`
Expected: FAIL (package does not compile).

- [ ] **Step 3: Write the port + fixture client**

Create `internal/llmclient/llmclient.go`:

```go
// Package llmclient is the LLM port an agent uses to turn an evidence bundle
// into a structured assessment. It has a fixture default (offline, hermetic)
// and a live Anthropic implementation (see anthropic.go), selected by whether
// ANTHROPIC_API_KEY is set at server boot.
package llmclient

import (
	"context"
	"strings"
)

// Cause is one ranked likely cause of a canary problem.
type Cause struct {
	Summary     string `json:"summary"`
	LikelyOwner string `json:"likely_owner,omitempty"`
	Confidence  string `json:"confidence,omitempty"`
}

// Assessment is the structured verdict the LLM produces. Verdict is the
// template vocabulary ("pass" | "needs_review" | "fail"); Model is provenance
// ("claude-opus-4-8" for the live client, "fixture" for the fixture).
type Assessment struct {
	Verdict      string  `json:"verdict"`
	Confidence   string  `json:"confidence"`
	Reasoning    string  `json:"reasoning"`
	RankedCauses []Cause `json:"ranked_causes,omitempty"`
	LikelyOwner  string  `json:"likely_owner,omitempty"`
	Model        string  `json:"model"`
}

// AssessRequest is the input to Assess. ExpectedVerdict is the allowed verdict
// set from the step (the structured-output enum for the live client).
type AssessRequest struct {
	SystemPrompt    string
	EvidenceBundle  string
	ExpectedVerdict []string
}

// Info describes the active LLM backend, for the /api/agents surface.
type Info struct {
	Model string `json:"model"`
	Live  bool   `json:"live"`
}

// Client turns an evidence bundle into an assessment.
type Client interface {
	Assess(ctx context.Context, req AssessRequest) (Assessment, error)
	Info() Info
}

// FixtureClient is the offline default: a deterministic assessment derived
// from the evidence bundle. It exists so the flow works and tests run without
// ANTHROPIC_API_KEY — it is not a real reasoner.
type FixtureClient struct{}

// NewFixtureClient returns the offline fixture LLM client.
func NewFixtureClient() *FixtureClient { return &FixtureClient{} }

// Assess returns "fail" when the evidence bundle contains a failure marker,
// else "pass", always at low confidence. This is a stand-in heuristic, not
// reasoning.
func (FixtureClient) Assess(_ context.Context, req AssessRequest) (Assessment, error) {
	verdict := "pass"
	reasoning := "Fixture assessment: no failure markers found in the gathered evidence."
	if strings.Contains(req.EvidenceBundle, "failed") {
		verdict = "fail"
		reasoning = "Fixture assessment: the gathered evidence contains a failure marker."
	}
	return Assessment{
		Verdict:    verdict,
		Confidence: "low",
		Reasoning:  reasoning,
		Model:      "fixture",
	}, nil
}

// Info reports the fixture backend.
func (FixtureClient) Info() Info { return Info{Model: "fixture", Live: false} }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/llmclient/ -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add internal/llmclient/llmclient.go internal/llmclient/llmclient_test.go
git commit -m "feat(agent): llmclient port + offline fixture client"
```

---

### Task 3: Agent framework (`internal/agent`)

**Files:**
- Create: `internal/agent/agent.go`
- Test: `internal/agent/agent_test.go`

**Interfaces:**
- Consumes: `llmclient.Client`, `llmclient.Cause`; `verdict.Passed/Warning/Failed`.
- Produces:
  - `agent.Agent interface { Type() string; Purpose() string; PromptRef() string; Run(ctx, Clients, StepContext) (Output, error) }`
  - `agent.Clients struct { LLM llmclient.Client }`
  - `agent.StepContext struct { RunID, StepID, EvidenceBundle string; ExpectedVerdict []string; RespectSuppressions, CitePatterns, CiteGlean bool; Inputs map[string]any }`
  - `agent.Output struct { Verdict, Confidence, Reasoning string; RankedCauses []llmclient.Cause; LikelyOwner string; Sources []string }`
  - `agent.Registry` with `NewRegistry()`, `Register(Agent)`, `Get(promptRef string) (Agent, bool)`, `List() []Info`
  - `agent.Info struct { Type, Purpose, PromptRef string }`
  - `agent.MapVerdict(raw string) string`

- [ ] **Step 1: Write the failing test**

Create `internal/agent/agent_test.go`:

```go
package agent

import (
	"context"
	"testing"

	"qac/internal/llmclient"
	"qac/internal/verdict"
)

func TestMapVerdict(t *testing.T) {
	cases := map[string]string{
		"pass":         verdict.Passed,
		"needs_review": verdict.Warning,
		"fail":         verdict.Failed,
		"weird":        verdict.Warning, // unknown → flag for a human
	}
	for raw, want := range cases {
		if got := MapVerdict(raw); got != want {
			t.Errorf("MapVerdict(%q) = %q, want %q", raw, got, want)
		}
	}
}

type stubAgent struct{ ref string }

func (s stubAgent) Type() string      { return "stub" }
func (s stubAgent) Purpose() string   { return "test agent" }
func (s stubAgent) PromptRef() string { return s.ref }
func (stubAgent) Run(context.Context, Clients, StepContext) (Output, error) {
	return Output{}, nil
}

func TestRegistry_RegisterGetList(t *testing.T) {
	r := NewRegistry()
	r.Register(stubAgent{ref: "x.v1"})
	if _, ok := r.Get("x.v1"); !ok {
		t.Fatal("Get(x.v1) not found after Register")
	}
	if _, ok := r.Get("missing"); ok {
		t.Fatal("Get(missing) should be false")
	}
	list := r.List()
	if len(list) != 1 || list[0].PromptRef != "x.v1" {
		t.Fatalf("List() = %+v, want one entry keyed x.v1", list)
	}
}

// Compile-time check that llmclient.Client is what Clients carries.
var _ = Clients{LLM: llmclient.NewFixtureClient()}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/ -v`
Expected: FAIL (package does not compile).

- [ ] **Step 3: Write the agent framework**

Create `internal/agent/agent.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/agent/agent.go internal/agent/agent_test.go
git commit -m "feat(agent): Agent interface, registry, output envelope, verdict mapping"
```

---

### Task 4: Engine dispatch for `ai_assess`

**Files:**
- Modify: `internal/engine/engine.go` (add `agents`/`agentClients` fields, `RegisterAgents`, `Agents`, `AgentLLMInfo`)
- Modify: `internal/engine/runner.go:107-113` (replace the combined halt block)
- Create: `internal/engine/aiassess.go` (`runAIAssess`, `assembleEvidenceBundle`)
- Test: `internal/engine/aiassess_test.go`

**Interfaces:**
- Consumes: `agent.Registry`, `agent.Clients`, `agent.Agent`, `agent.StepContext`, `agent.Output`, `agent.MapVerdict`, `agent.Info`; `llmclient.Info`; existing `emitStepStarted/emitStepCompleted/emitStepFailed`, `store.PutEvidence`, `store.ListEvidence`, `ulid.Make`.
- Produces (on `*Engine`): `RegisterAgents(*agent.Registry, agent.Clients)`, `Agents() []agent.Info`, `AgentLLMInfo() llmclient.Info`.
- Note: `engine.New` signature is **unchanged** — agents are wired via `RegisterAgents`, so existing `engine.New(...)` call sites keep compiling.

- [ ] **Step 1: Write the failing test**

Create `internal/engine/aiassess_test.go`:

```go
package engine

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"qac/internal/agent"
	"qac/internal/store"
	"qac/internal/verdict"
)

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
```

> Note: `probeClientsZero()` is a tiny helper — add it in this test file: `func probeClientsZero() probe.Clients { return probe.Clients{} }` and import `"qac/internal/probe"`. `New(s, nil, ...)` is valid because this template has no `probe_type` steps, so the probe registry is never consulted.

Add to `internal/engine/aiassess_test.go` imports/helpers:

```go
import "qac/internal/probe"

func probeClientsZero() probe.Clients { return probe.Clients{} }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run TestEngine_AIAssess -v`
Expected: FAIL (compile error — `RegisterAgents` undefined, etc.).

- [ ] **Step 3: Add engine fields + wiring methods**

In `internal/engine/engine.go`, add imports `"qac/internal/agent"` and `"qac/internal/llmclient"`. Add fields to the `Engine` struct (after `clients probe.Clients`):

```go
	agents       *agent.Registry
	agentClients agent.Clients
```

Add these methods (e.g. below `Probes()`):

```go
// RegisterAgents wires the agent registry and the ports agents need. Called
// once at server boot after New. Left unset in tests that don't exercise
// ai_assess steps.
func (e *Engine) RegisterAgents(reg *agent.Registry, clients agent.Clients) {
	e.agents = reg
	e.agentClients = clients
}

// Agents returns metadata for every registered agent, for /api/agents. Empty
// when no agents are registered.
func (e *Engine) Agents() []agent.Info {
	if e.agents == nil {
		return nil
	}
	return e.agents.List()
}

// AgentLLMInfo reports the active agent LLM backend (model + live), for
// /api/agents. Reports a "none" backend when agents are unwired.
func (e *Engine) AgentLLMInfo() llmclient.Info {
	if e.agentClients.LLM == nil {
		return llmclient.Info{Model: "none", Live: false}
	}
	return e.agentClients.LLM.Info()
}
```

- [ ] **Step 4: Replace the runner halt block**

In `internal/engine/runner.go`, replace the block at ~lines 107-113:

```go
		// Halt at operator-gated step kinds. The operator-advance
		// endpoint that unblocks these is iter-4d.
		if step.Kind == "review" || step.Kind == "ai_assess" || step.Kind == "signoff" {
			slog.Info("engine: halted at operator-gated step",
				"run_id", runID, "step_id", step.ID, "kind", step.Kind)
			return
		}
```

with:

```go
		// ai_assess runs an agent automatically: it reasons over the
		// context_steps' evidence and emits an advisory verdict that rolls
		// up via the existing StepCompleted plumbing. The walk then
		// continues to the operator gate.
		if step.Kind == "ai_assess" {
			e.runAIAssess(ctx, runID, step, inputs)
			continue
		}
		// Halt at the remaining operator-gated step kinds. The
		// operator-advance endpoint that unblocks these is iter-4d.
		if step.Kind == "review" || step.Kind == "signoff" {
			slog.Info("engine: halted at operator-gated step",
				"run_id", runID, "step_id", step.ID, "kind", step.Kind)
			return
		}
```

- [ ] **Step 5: Add the ai_assess handler**

Create `internal/engine/aiassess.go`:

```go
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"qac/internal/agent"
	"qac/internal/store"
	"qac/internal/template"
)

// assessmentContentType marks an evidence row as an agent assessment so the
// UI can render it distinctly.
const assessmentContentType = "application/vnd.qac.assessment+json"

// runAIAssess dispatches an ai_assess step to the registered agent for the
// step's prompt_ref: it assembles the context_steps' evidence into a bundle,
// runs the agent, persists the agent output as an evidence row, and emits
// StepCompleted with the mapped verdict (which the existing rollup picks up).
// Any failure emits StepFailed and returns so the walk moves on.
func (e *Engine) runAIAssess(ctx context.Context, runID string, step template.Step, inputs map[string]any) {
	startedAt := time.Now()
	if err := e.emitStepStarted(ctx, runID, step.ID, "ai_assess", nil, startedAt.Unix()); err != nil {
		e.emitStepFailed(ctx, runID, step.ID, fmt.Sprintf("emit StepStarted: %v", err), 0)
		return
	}
	if e.agents == nil {
		e.emitStepFailed(ctx, runID, step.ID, "no agent registry configured", 0)
		return
	}
	ag, ok := e.agents.Get(step.PromptRef)
	if !ok {
		e.emitStepFailed(ctx, runID, step.ID,
			fmt.Sprintf("no agent registered for prompt_ref %q", step.PromptRef), 0)
		return
	}
	bundle, err := e.assembleEvidenceBundle(ctx, runID, step.ContextSteps)
	if err != nil {
		e.emitStepFailed(ctx, runID, step.ID, err.Error(), time.Since(startedAt).Milliseconds())
		return
	}
	out, err := ag.Run(ctx, e.agentClients, agent.StepContext{
		RunID:               runID,
		StepID:              step.ID,
		EvidenceBundle:      bundle,
		ExpectedVerdict:     step.ExpectedVerdict,
		RespectSuppressions: step.RespectSuppressions,
		CitePatterns:        step.CitePatterns,
		CiteGlean:           step.CiteGlean,
		Inputs:              inputs,
	})
	if err != nil {
		e.emitStepFailed(ctx, runID, step.ID, err.Error(), time.Since(startedAt).Milliseconds())
		return
	}
	payload, err := json.Marshal(out)
	if err != nil {
		e.emitStepFailed(ctx, runID, step.ID, fmt.Sprintf("marshal assessment: %v", err), time.Since(startedAt).Milliseconds())
		return
	}
	evidenceID := ulid.Make().String()
	if err := e.store.PutEvidence(ctx, store.Evidence{
		ID:          evidenceID,
		RunID:       runID,
		StepID:      step.ID,
		ContentType: assessmentContentType,
		Payload:     payload,
	}); err != nil {
		e.emitStepFailed(ctx, runID, step.ID, fmt.Sprintf("persist assessment: %v", err), time.Since(startedAt).Milliseconds())
		return
	}
	_ = e.emitStepCompleted(ctx, runID, step.ID, evidenceID,
		time.Since(startedAt).Milliseconds(), out.Sources, agent.MapVerdict(out.Verdict))
}

// assembleEvidenceBundle concatenates the evidence rows of contextSteps into a
// text bundle the agent reasons over. Suppression filtering is a documented
// no-op stand-in in iteration 1 (there is no per-row suppression concept yet).
func (e *Engine) assembleEvidenceBundle(ctx context.Context, runID string, contextSteps []string) (string, error) {
	var b strings.Builder
	for _, sid := range contextSteps {
		rows, err := e.store.ListEvidence(ctx, runID, sid)
		if err != nil {
			return "", fmt.Errorf("list evidence for step %s: %w", sid, err)
		}
		for _, row := range rows {
			fmt.Fprintf(&b, "## step %s", sid)
			if row.Deviceslot != nil {
				fmt.Fprintf(&b, " (deviceslot %s)", *row.Deviceslot)
			}
			b.WriteString("\n")
			b.Write(row.Payload)
			b.WriteString("\n\n")
		}
	}
	return b.String(), nil
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/engine/ -run TestEngine_AIAssess -v`
Expected: PASS.

- [ ] **Step 7: Run the whole engine + api suite to confirm nothing regressed**

Run: `go test ./internal/engine/ ./internal/api/ -count=1`
Expected: PASS (existing tests unaffected — `engine.New` signature unchanged).

- [ ] **Step 8: Commit**

```bash
git add internal/engine
git commit -m "feat(engine): ai_assess auto-runs its agent and rolls up the verdict"
```

---

### Task 5: Canary assessor agent (`internal/agent/canaryassessor`)

**Files:**
- Create: `internal/agent/canaryassessor/canaryassessor.go`
- Test: `internal/agent/canaryassessor/canaryassessor_test.go`

**Interfaces:**
- Consumes: `agent.Agent`, `agent.Clients`, `agent.StepContext`, `agent.Output`; `llmclient.AssessRequest`, `llmclient.Assessment`, `llmclient.Client`; `prompt.Get`.
- Produces: `canaryassessor.New() *Agent` (satisfies `agent.Agent`, `PromptRef()=="fw-release-canary.summary.v1"`, `Type()=="canary_assessor"`).

- [ ] **Step 1: Write the failing test**

Create `internal/agent/canaryassessor/canaryassessor_test.go`:

```go
package canaryassessor

import (
	"context"
	"testing"

	"qac/internal/agent"
	"qac/internal/llmclient"
)

// stubLLM returns a canned assessment and records the request it received.
type stubLLM struct {
	got llmclient.AssessRequest
	ret llmclient.Assessment
}

func (s *stubLLM) Assess(_ context.Context, req llmclient.AssessRequest) (llmclient.Assessment, error) {
	s.got = req
	return s.ret, nil
}
func (*stubLLM) Info() llmclient.Info { return llmclient.Info{Model: "stub", Live: false} }

func TestCanaryAssessor_Run_PassesRubricAndTranslatesAssessment(t *testing.T) {
	llm := &stubLLM{ret: llmclient.Assessment{
		Verdict: "fail", Confidence: "high", Reasoning: "bad", Model: "claude-opus-4-8",
		RankedCauses: []llmclient.Cause{{Summary: "bmc", LikelyOwner: "hw"}},
	}}
	out, err := New().Run(context.Background(), agent.Clients{LLM: llm}, agent.StepContext{
		EvidenceBundle:  "some evidence",
		ExpectedVerdict: []string{"pass", "fail", "needs_review"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The rubric was fetched and passed as the system prompt.
	if llm.got.SystemPrompt == "" {
		t.Error("system prompt (rubric) was not passed to the LLM")
	}
	if llm.got.EvidenceBundle != "some evidence" {
		t.Errorf("evidence bundle = %q", llm.got.EvidenceBundle)
	}
	// The assessment is translated into the agent Output verbatim.
	if out.Verdict != "fail" || out.Confidence != "high" || len(out.RankedCauses) != 1 {
		t.Errorf("Output not translated from Assessment: %+v", out)
	}
	// Provenance records the model.
	if len(out.Sources) != 1 || out.Sources[0] != "claude-opus-4-8" {
		t.Errorf("Sources = %v, want [claude-opus-4-8]", out.Sources)
	}
}

func TestCanaryAssessor_Identity(t *testing.T) {
	a := New()
	if a.Type() != "canary_assessor" || a.PromptRef() != "fw-release-canary.summary.v1" {
		t.Errorf("identity = %q / %q", a.Type(), a.PromptRef())
	}
	var _ agent.Agent = a // satisfies the interface
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/canaryassessor/ -v`
Expected: FAIL (package does not compile).

- [ ] **Step 3: Write the agent**

Create `internal/agent/canaryassessor/canaryassessor.go`:

```go
// Package canaryassessor is the first concrete agent: it reasons over a
// firmware-release canary's gathered evidence and emits an advisory verdict.
// Iteration 1 is a single-shot LLM call; the tool-using loop (iteration 2)
// replaces the body of Run behind the same agent.Agent interface.
package canaryassessor

import (
	"context"
	"fmt"

	"qac/internal/agent"
	"qac/internal/llmclient"
	"qac/internal/prompt"
)

// Agent is the canary-assessor agent.
type Agent struct{}

// New returns the canary-assessor agent.
func New() *Agent { return &Agent{} }

// Type is the stable identifier for the /api/agents surface.
func (*Agent) Type() string { return "canary_assessor" }

// Purpose is one-line copy for the Agents page.
func (*Agent) Purpose() string {
	return "Reasons over per-rack canary evidence and emits an advisory verdict with confidence and ranked likely causes."
}

// PromptRef is the template prompt_ref this agent services.
func (*Agent) PromptRef() string { return "fw-release-canary.summary.v1" }

// Run fetches the rubric, asks the LLM to assess the evidence bundle, and
// translates the assessment into an agent Output.
func (a *Agent) Run(ctx context.Context, clients agent.Clients, sc agent.StepContext) (agent.Output, error) {
	if clients.LLM == nil {
		return agent.Output{}, fmt.Errorf("canaryassessor: no LLM client configured")
	}
	system, err := prompt.Get(a.PromptRef())
	if err != nil {
		return agent.Output{}, err
	}
	assessment, err := clients.LLM.Assess(ctx, llmclient.AssessRequest{
		SystemPrompt:    system,
		EvidenceBundle:  sc.EvidenceBundle,
		ExpectedVerdict: sc.ExpectedVerdict,
	})
	if err != nil {
		return agent.Output{}, err
	}
	return agent.Output{
		Verdict:      assessment.Verdict,
		Confidence:   assessment.Confidence,
		Reasoning:    assessment.Reasoning,
		RankedCauses: assessment.RankedCauses,
		LikelyOwner:  assessment.LikelyOwner,
		Sources:      []string{assessment.Model},
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/canaryassessor/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/canaryassessor
git commit -m "feat(agent): canary assessor agent (single-shot, rubric-driven)"
```

---

### Task 6: Live Anthropic LLM client (`internal/llmclient/anthropic.go`)

**Files:**
- Modify: `go.mod`, `go.sum` (add `github.com/anthropics/anthropic-sdk-go`)
- Create: `internal/llmclient/anthropic.go`
- Test: `internal/llmclient/anthropic_test.go` (guarded by `ANTHROPIC_API_KEY`)

**Interfaces:**
- Produces: `llmclient.NewAnthropicClient() *AnthropicClient` (satisfies `llmclient.Client`).

- [ ] **Step 1: Add the SDK dependency**

Run:
```bash
go get github.com/anthropics/anthropic-sdk-go@latest
```
Expected: `go.mod`/`go.sum` updated with the module.

- [ ] **Step 2: Write the guarded live test**

Create `internal/llmclient/anthropic_test.go`:

```go
package llmclient

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestAnthropicClient_Assess_Live(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY unset; skipping live LLM test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	a, err := NewAnthropicClient().Assess(ctx, AssessRequest{
		SystemPrompt:    "You assess a canary. Call record_assessment with your verdict.",
		EvidenceBundle:  `{"rack":"rack-a","outcome":"ok","firmware":"converged"}`,
		ExpectedVerdict: []string{"pass", "fail", "needs_review"},
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	switch a.Verdict {
	case "pass", "fail", "needs_review":
	default:
		t.Errorf("verdict %q not in expected set", a.Verdict)
	}
	if a.Model == "" {
		t.Error("model provenance not set")
	}
}

func TestAnthropicClient_Info(t *testing.T) {
	got := NewAnthropicClient().Info()
	if !got.Live || got.Model != "claude-opus-4-8" {
		t.Errorf("Info() = %+v, want {claude-opus-4-8 true}", got)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/llmclient/ -run TestAnthropicClient -v`
Expected: FAIL (compile error — `NewAnthropicClient` undefined).

- [ ] **Step 4: Write the live client**

Create `internal/llmclient/anthropic.go`:

```go
package llmclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// modelID is the assessment model. Opus 4.8 with adaptive thinking; effort is
// left at the SDK default (high). Do not set sampling params or budget_tokens
// — they 400 on Opus 4.8.
const modelID = anthropic.ModelClaudeOpus4_8

// AnthropicClient is the live LLM backend. anthropic.NewClient() reads
// ANTHROPIC_API_KEY from the environment.
type AnthropicClient struct {
	client anthropic.Client
}

// NewAnthropicClient returns a live client. The caller selects this over the
// fixture only when ANTHROPIC_API_KEY is present.
func NewAnthropicClient() *AnthropicClient {
	return &AnthropicClient{client: anthropic.NewClient()}
}

// Info reports the live backend.
func (*AnthropicClient) Info() Info { return Info{Model: string(modelID), Live: true} }

// Assess asks the model to reason over the evidence and call the strict
// record_assessment tool, then decodes that tool call into an Assessment.
func (c *AnthropicClient) Assess(ctx context.Context, req AssessRequest) (Assessment, error) {
	tool := anthropic.ToolParam{
		Name:        "record_assessment",
		Description: anthropic.String("Record the canary verdict, confidence, reasoning, ranked likely causes, and the likely owning team."),
		Strict:      anthropic.Bool(true),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"verdict":      map[string]any{"type": "string", "enum": req.ExpectedVerdict},
				"confidence":   map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
				"reasoning":    map[string]any{"type": "string"},
				"likely_owner": map[string]any{"type": "string"},
				"ranked_causes": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"summary":      map[string]any{"type": "string"},
							"likely_owner": map[string]any{"type": "string"},
							"confidence":   map[string]any{"type": "string"},
						},
						"required": []string{"summary"},
					},
				},
			},
			ExtraFields: map[string]any{
				"required":             []string{"verdict", "confidence", "reasoning"},
				"additionalProperties": false,
			},
		},
	}

	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     modelID,
		MaxTokens: 8192,
		Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
		System: []anthropic.TextBlockParam{{
			Text:         req.SystemPrompt,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Tools: []anthropic.ToolUnionParam{{OfTool: &tool}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.EvidenceBundle)),
		},
	})
	if err != nil {
		return Assessment{}, fmt.Errorf("anthropic assess: %w", err)
	}

	for _, block := range resp.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok && tu.Name == "record_assessment" {
			var a Assessment
			if err := json.Unmarshal([]byte(tu.JSON.Input.Raw()), &a); err != nil {
				return Assessment{}, fmt.Errorf("decode assessment: %w", err)
			}
			a.Model = string(resp.Model)
			return a, nil
		}
	}
	return Assessment{}, fmt.Errorf("model did not call record_assessment (stop_reason %q)", resp.StopReason)
}
```

> If the compiler reports a mismatch on any `anthropic.*` symbol (the SDK version resolved by `go get` may differ from these names), run `go doc github.com/anthropics/anthropic-sdk-go <Symbol>` to find the exact name and adjust — do not guess. The documented shapes used here (`NewClient`, `Messages.New`, `MessageNewParams{Model,MaxTokens,Thinking,System,Tools,Messages}`, `ToolParam` with `Strict`+`InputSchema.ExtraFields`, `ToolUnionParam{OfTool:&tool}`, `block.AsAny().(anthropic.ToolUseBlock)`, `tu.JSON.Input.Raw()`, `NewCacheControlEphemeralParam`, `ThinkingConfigParamUnion{OfAdaptive:...}`) are from the anthropic-sdk-go Go reference.

- [ ] **Step 5: Verify it compiles; run tests**

Run: `go build ./... && go test ./internal/llmclient/ -run TestAnthropicClient_Info -v`
Expected: builds; `TestAnthropicClient_Info` PASS. (`_Live` skips without a key.)

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/llmclient/anthropic.go internal/llmclient/anthropic_test.go
git commit -m "feat(agent): live Anthropic llmclient (opus-4-8, adaptive thinking, strict tool)"
```

---

### Task 7: `GET /api/agents` + handler

**Files:**
- Create: `internal/api/agents.go`
- Modify: `internal/api/api.go` (register the route)
- Test: `internal/api/agents_test.go`

**Interfaces:**
- Consumes: `engine.Engine.Agents() []agent.Info`, `engine.Engine.AgentLLMInfo() llmclient.Info`, existing `writeJSON`.
- Produces: `GET /api/agents` → `{"agents":[{type,purpose,prompt_ref,model,live}]}`.

- [ ] **Step 1: Write the failing test**

Create `internal/api/agents_test.go`:

```go
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
	if a.Model != "fixture" || a.Live {
		t.Errorf("agent backend = %q live=%v, want fixture/false", a.Model, a.Live)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestListAgentsHandler -v`
Expected: FAIL (compile error — `listAgentsHandler` undefined).

- [ ] **Step 3: Write the handler**

Create `internal/api/agents.go`:

```go
package api

import (
	"net/http"

	"qac/internal/engine"
)

// listAgentsHandler serves GET /api/agents: every registered agent with its
// purpose and prompt_ref (from the registry) plus the shared LLM backend
// model and whether it is live (fixture when ANTHROPIC_API_KEY is unset).
func listAgentsHandler(eng *engine.Engine) http.HandlerFunc {
	type view struct {
		Type      string `json:"type"`
		Purpose   string `json:"purpose"`
		PromptRef string `json:"prompt_ref"`
		Model     string `json:"model"`
		Live      bool   `json:"live"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		llm := eng.AgentLLMInfo()
		infos := eng.Agents()
		out := make([]view, 0, len(infos))
		for _, a := range infos {
			out = append(out, view{
				Type:      a.Type,
				Purpose:   a.Purpose,
				PromptRef: a.PromptRef,
				Model:     llm.Model,
				Live:      llm.Live,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"agents": out})
	}
}
```

- [ ] **Step 4: Register the route**

In `internal/api/api.go`, add after the `GET /api/probes` line:

```go
	mux.HandleFunc("GET /api/agents", listAgentsHandler(eng))
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestListAgentsHandler -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/agents.go internal/api/api.go internal/api/agents_test.go
git commit -m "feat(api): GET /api/agents lists registered agents and LLM backend"
```

---

### Task 8: Wire agents at server boot + `.env` alias + README

**Files:**
- Modify: `cmd/qac/serve.go` (select LLM client, build agent registry, `RegisterAgents`)
- Modify: `cmd/qac/dotenv.go:82-86` (add `ANTHROPIC_API_KEY` alias)
- Modify: `README.md` (document `ANTHROPIC_API_KEY`)
- Test: `cmd/qac/dotenv_test.go` (extend for the new alias)

**Interfaces:**
- Consumes: `engine.Engine.RegisterAgents`, `agent.NewRegistry`, `canaryassessor.New`, `llmclient.NewAnthropicClient`/`NewFixtureClient`.

- [ ] **Step 1: Add the ANTHROPIC alias with a failing test**

In `cmd/qac/dotenv_test.go`, add:

```go
func TestApplyEnvAliases_Anthropic(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_API_KEY", "sk-test")
	applyEnvAliases()
	if os.Getenv("ANTHROPIC_API_KEY") != "sk-test" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want sk-test (from CLAUDE_API_KEY alias)", os.Getenv("ANTHROPIC_API_KEY"))
	}
}
```

(Ensure `"os"` is imported in the test file.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/qac/ -run TestApplyEnvAliases_Anthropic -v`
Expected: FAIL (alias not applied).

- [ ] **Step 3: Add the alias**

In `cmd/qac/dotenv.go`, add to the `envAliases` map:

```go
	"ANTHROPIC_API_KEY":        {"CLAUDE_API_KEY"},
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/qac/ -run TestApplyEnvAliases_Anthropic -v`
Expected: PASS.

- [ ] **Step 5: Wire the agent registry in serve.go**

In `cmd/qac/serve.go`, add imports:

```go
	"qac/internal/agent"
	"qac/internal/agent/canaryassessor"
	"qac/internal/llmclient"
```

After the `eng := engine.New(...)` block (currently ending at line ~258), add:

```go
				// Agent LLM backend: live Anthropic when ANTHROPIC_API_KEY is
				// set (e.g. via .env), else the offline fixture. Fixture keeps
				// the flow working and hermetic without a key.
				var llmC llmclient.Client
				if os.Getenv("ANTHROPIC_API_KEY") != "" {
					llmC = llmclient.NewAnthropicClient()
					slog.Info("using anthropic backend for agents", "model", "claude-opus-4-8")
				} else {
					llmC = llmclient.NewFixtureClient()
					slog.Warn("ANTHROPIC_API_KEY unset; using fixture assessment for agents (no live AI verdict)")
				}
				agentReg := agent.NewRegistry()
				agentReg.Register(canaryassessor.New())
				eng.RegisterAgents(agentReg, agent.Clients{LLM: llmC})
```

- [ ] **Step 6: Document the key in README**

In `README.md`, in the `.env` code block, add under the existing creds:

```sh
ANTHROPIC_API_KEY=            # AI-reasoned verdict at the ai_assess step; blank → offline fixture assessment
```

And add a sentence after the block: "With `ANTHROPIC_API_KEY` set, the `ai_assess` step reasons over gathered evidence with Claude Opus 4.8 and emits an advisory verdict; without it, a deterministic fixture assessment is used."

- [ ] **Step 7: Build and validate**

Run: `go build ./... && go vet ./...`
Expected: builds; vet clean.

- [ ] **Step 8: Commit**

```bash
git add cmd/qac/serve.go cmd/qac/dotenv.go cmd/qac/dotenv_test.go README.md
git commit -m "feat: wire canary assessor at boot; ANTHROPIC_API_KEY via .env alias"
```

---

### Task 9: Agents page (frontend)

**Files:**
- Create: `web/src/features/agents/types.ts`
- Create: `web/src/features/agents/useAgents.ts`
- Create: `web/src/features/agents/Agents.tsx`
- Create: `web/src/features/agents/Agents.test.tsx`
- Modify: `web/src/app/routes.tsx` (swap the `agents` StubPage for the real page)
- Modify: `web/src/mocks/handlers.ts` (add a default `GET /api/agents` handler)

**Interfaces:**
- Consumes: `apiGet` from `@/lib/api`; the `/api/agents` contract from Task 7.

- [ ] **Step 1: Add the default mock handler**

In `web/src/mocks/handlers.ts`, add alongside the `/api/probes` handler (near line 282):

```ts
  http.get('/api/agents', () =>
    HttpResponse.json({
      agents: [
        {
          type: 'canary_assessor',
          purpose:
            'Reasons over per-rack canary evidence and emits an advisory verdict with confidence and ranked likely causes.',
          prompt_ref: 'fw-release-canary.summary.v1',
          model: 'fixture',
          live: false,
        },
      ],
    }),
  ),
```

- [ ] **Step 2: Write the failing test**

Create `web/src/features/agents/Agents.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { http, HttpResponse } from 'msw';
import { server } from '@/mocks/server';
import Agents from './Agents';

function renderAgents() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Agents />
    </QueryClientProvider>,
  );
}

describe('Agents', () => {
  it('lists agents from /api/agents with a backend badge', async () => {
    renderAgents();
    expect(screen.getByText(/loading agents/i)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText('canary_assessor')).toBeInTheDocument());
    expect(screen.getByText('fw-release-canary.summary.v1')).toBeInTheDocument();
    expect(screen.getByText(/fixture/i)).toBeInTheDocument();
  });

  it('renders an error state when /api/agents fails', async () => {
    server.use(
      http.get('/api/agents', () =>
        HttpResponse.json({ error: { code: 'internal', message: 'boom' } }, { status: 500 }),
      ),
    );
    renderAgents();
    await waitFor(() => expect(screen.getByText(/could not load agents/i)).toBeInTheDocument());
  });
});
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd web && yarn test --run src/features/agents/Agents.test.tsx`
Expected: FAIL (cannot resolve `./Agents`).

- [ ] **Step 4: Write the types + hook**

Create `web/src/features/agents/types.ts`:

```ts
// Agent is one registered agent as served by GET /api/agents. The backend
// registry is the source of truth for type/purpose/prompt_ref; model + live
// describe the shared LLM backend (fixture when no ANTHROPIC_API_KEY).
export interface Agent {
  type: string;
  purpose: string;
  prompt_ref: string;
  model: string;
  live: boolean;
}
```

Create `web/src/features/agents/useAgents.ts`:

```ts
import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import type { Agent } from './types';

// useAgents fetches the registered agents from GET /api/agents.
export function useAgents() {
  return useQuery<Agent[]>({
    queryKey: ['agents'],
    queryFn: async () => (await apiGet<{ agents: Agent[] }>('/agents')).agents,
  });
}
```

> If `apiGet` is not exported from `@/lib/api` (the reference file showed only a private `request`), mirror however `useProbes.ts` imports it — that file uses `import { apiGet } from '@/lib/api'`, so the export exists.

- [ ] **Step 5: Write the page**

Create `web/src/features/agents/Agents.tsx`:

```tsx
import { useAgents } from './useAgents';

const muted = 'var(--cw-gray-700)';

export default function Agents() {
  const { data, isLoading, isError, error } = useAgents();

  return (
    <main
      style={{ flex: 1, overflow: 'auto', padding: '32px 40px', fontFamily: 'var(--font-sans)' }}
    >
      <div
        style={{ fontSize: 12, letterSpacing: '.08em', textTransform: 'uppercase', color: muted }}
      >
        QAgenticCow
      </div>
      <h1
        style={{
          fontFamily: 'var(--font-display)',
          fontSize: 24,
          fontWeight: 700,
          color: 'var(--cw-gray-950)',
          margin: '6px 0 4px',
        }}
      >
        Agents
      </h1>
      <p style={{ color: muted, fontSize: 14, maxWidth: 640 }}>
        The agents that reason over gathered evidence to produce advisory verdicts. Agents narrow
        and rank; the operator decides.
      </p>

      {isLoading && <p style={{ marginTop: 20, color: muted, fontSize: 13.5 }}>Loading agents…</p>}
      {isError && (
        <p style={{ marginTop: 20, color: 'var(--cw-red-700, #b42318)', fontSize: 13.5 }}>
          Could not load agents. {(error as Error)?.message ?? ''}
        </p>
      )}

      {data && (
        <div style={{ marginTop: 20, display: 'grid', gap: 12, maxWidth: 820 }}>
          {data.map((a) => (
            <div
              key={a.type}
              style={{
                border: '1px solid var(--cw-gray-400)',
                borderRadius: 6,
                padding: '14px 16px',
                background: 'var(--cw-white, #fff)',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
                <span style={{ fontWeight: 600, fontSize: 15, color: 'var(--cw-gray-950)' }}>
                  {a.type}
                </span>
                <span
                  style={{
                    fontSize: 11.5,
                    fontWeight: 600,
                    textTransform: 'uppercase',
                    letterSpacing: '.04em',
                    padding: '2px 8px',
                    borderRadius: 999,
                    background: a.live ? 'var(--cw-green-100, #e3f6e8)' : 'var(--cw-gray-200, #eee)',
                    color: a.live ? 'var(--cw-green-800, #1f7a3d)' : 'var(--cw-gray-800, #444)',
                  }}
                >
                  {a.live ? 'live' : 'fixture'}: {a.model}
                </span>
              </div>
              <code
                style={{ fontSize: 12.5, color: muted, fontFamily: 'var(--font-mono, monospace)' }}
              >
                {a.prompt_ref}
              </code>
              <p style={{ margin: '6px 0 0', fontSize: 13.5, color: 'var(--cw-gray-800)' }}>
                {a.purpose}
              </p>
            </div>
          ))}
          {data.length === 0 && (
            <p style={{ color: muted, fontSize: 13.5 }}>No agents are registered.</p>
          )}
        </div>
      )}
    </main>
  );
}
```

- [ ] **Step 6: Swap the route**

In `web/src/app/routes.tsx`: add the import near the `Probes` import:

```tsx
import Agents from '@/features/agents/Agents';
```

and change the `agents` route from:

```tsx
      { path: 'agents', element: <StubPage title="Agents" /> },
```

to:

```tsx
      { path: 'agents', element: <Agents /> },
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd web && yarn test --run src/features/agents/Agents.test.tsx`
Expected: PASS (both tests).

- [ ] **Step 8: Commit**

```bash
git add web/src/features/agents web/src/app/routes.tsx web/src/mocks/handlers.ts
git commit -m "feat(web): Agents page listing registered agents and LLM backend"
```

---

### Task 10: Run-detail assessment card (advisory rendering)

**Files:**
- Create: `web/src/features/canary/AssessmentCard.jsx`
- Modify: `web/src/features/canary/RunDetail.jsx` (render the card for assessment evidence)
- Test: `web/src/features/canary/AssessmentCard.test.tsx`

**Interfaces:**
- Consumes: an evidence row of `content_type === 'application/vnd.qac.assessment+json'` whose parsed payload is the `agent.Output` shape `{ verdict, confidence, reasoning, ranked_causes, likely_owner, sources }`.

- [ ] **Step 1: Read the run-detail evidence rendering**

Read `web/src/features/canary/RunDetail.jsx` and locate where per-step evidence rows are rendered (search for where the run's evidence list / `content_type` / payload is shown). Note the variable holding an evidence row's `content_type` and parsed `payload`.

- [ ] **Step 2: Write the failing test**

Create `web/src/features/canary/AssessmentCard.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import AssessmentCard from './AssessmentCard';

describe('AssessmentCard', () => {
  it('renders verdict, confidence, reasoning, and the advisory badge', () => {
    render(
      <AssessmentCard
        assessment={{
          verdict: 'fail',
          confidence: 'high',
          reasoning: 'Two racks did not converge to the target bundle.',
          ranked_causes: [{ summary: 'BMC firmware stuck', likely_owner: 'hardware' }],
          likely_owner: 'hardware',
          sources: ['claude-opus-4-8'],
        }}
      />,
    );
    expect(screen.getByText(/fail/i)).toBeInTheDocument();
    expect(screen.getByText(/high/i)).toBeInTheDocument();
    expect(screen.getByText(/did not converge/i)).toBeInTheDocument();
    expect(screen.getByText(/advisory — operator decides/i)).toBeInTheDocument();
    expect(screen.getByText(/BMC firmware stuck/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd web && yarn test --run src/features/canary/AssessmentCard.test.tsx`
Expected: FAIL (cannot resolve `./AssessmentCard`).

- [ ] **Step 4: Write the card**

Create `web/src/features/canary/AssessmentCard.jsx`:

```jsx
// AssessmentCard renders an agent's advisory verdict for an ai_assess step.
// `assessment` is the parsed payload of an evidence row whose content_type is
// application/vnd.qac.assessment+json.
const verdictColor = {
  pass: { bg: 'var(--cw-green-100, #e3f6e8)', fg: 'var(--cw-green-800, #1f7a3d)' },
  needs_review: { bg: 'var(--cw-amber-100, #fdf0d5)', fg: 'var(--cw-amber-800, #8a5a00)' },
  fail: { bg: 'var(--cw-red-100, #fde8e6)', fg: 'var(--cw-red-800, #b42318)' },
};

export default function AssessmentCard({ assessment }) {
  const a = assessment || {};
  const c = verdictColor[a.verdict] || {
    bg: 'var(--cw-gray-200, #eee)',
    fg: 'var(--cw-gray-800, #444)',
  };
  return (
    <div
      style={{
        border: '1px solid var(--cw-gray-400)',
        borderRadius: 6,
        padding: '14px 16px',
        background: 'var(--cw-white, #fff)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
        <span
          style={{
            fontSize: 12,
            fontWeight: 700,
            textTransform: 'uppercase',
            letterSpacing: '.04em',
            padding: '3px 10px',
            borderRadius: 999,
            background: c.bg,
            color: c.fg,
          }}
        >
          {a.verdict || 'unknown'}
        </span>
        <span style={{ fontSize: 12.5, color: 'var(--cw-gray-700)' }}>
          confidence: {a.confidence || 'unknown'}
        </span>
        <span
          style={{
            fontSize: 11.5,
            color: 'var(--cw-gray-700)',
            border: '1px solid var(--cw-gray-400)',
            borderRadius: 4,
            padding: '2px 8px',
          }}
        >
          advisory — operator decides
        </span>
      </div>
      {a.reasoning && (
        <p style={{ margin: '10px 0 0', fontSize: 13.5, color: 'var(--cw-gray-900)' }}>
          {a.reasoning}
        </p>
      )}
      {a.likely_owner && (
        <p style={{ margin: '6px 0 0', fontSize: 13, color: 'var(--cw-gray-700)' }}>
          Likely owner: {a.likely_owner}
        </p>
      )}
      {Array.isArray(a.ranked_causes) && a.ranked_causes.length > 0 && (
        <ul style={{ margin: '8px 0 0', paddingLeft: 18, fontSize: 13, color: 'var(--cw-gray-800)' }}>
          {a.ranked_causes.map((cause, i) => (
            <li key={i}>
              {cause.summary}
              {cause.likely_owner ? ` — ${cause.likely_owner}` : ''}
            </li>
          ))}
        </ul>
      )}
      {Array.isArray(a.sources) && a.sources.length > 0 && (
        <p style={{ margin: '8px 0 0', fontSize: 12, color: 'var(--cw-gray-600)' }}>
          Source: {a.sources.join(', ')}
        </p>
      )}
    </div>
  );
}
```

- [ ] **Step 5: Integrate into RunDetail**

In `web/src/features/canary/RunDetail.jsx`, at the evidence-rendering spot found in Step 1, add (adjusting variable names to match that file): when an evidence row's `content_type === 'application/vnd.qac.assessment+json'`, render `<AssessmentCard assessment={JSON.parse(<row payload string>)} />` instead of the default JSON dump. Add the import at the top:

```jsx
import AssessmentCard from './AssessmentCard';
```

- [ ] **Step 6: Run tests**

Run: `cd web && yarn test --run src/features/canary/AssessmentCard.test.tsx && yarn test --run`
Expected: AssessmentCard test PASS; full suite green (no regressions in RunDetail tests).

- [ ] **Step 7: Commit**

```bash
git add web/src/features/canary/AssessmentCard.jsx web/src/features/canary/AssessmentCard.test.tsx web/src/features/canary/RunDetail.jsx
git commit -m "feat(web): advisory assessment card on the run detail"
```

---

### Task 11: Full verification

- [ ] **Step 1: Backend build, vet, tests**

Run: `make build && go vet ./... && go test ./... -count=1`
Expected: `bin/qac` built; vet clean; all Go tests PASS (the live LLM test skips without a key).

- [ ] **Step 2: Frontend lint + tests**

Run: `cd web && yarn lint && yarn test --run`
Expected: lint clean; all Vitest suites PASS.

- [ ] **Step 3: Manual smoke (fixture path)**

Run:
```bash
./bin/qac seed-demo
./bin/qac serve --addr 127.0.0.1:8080 &
curl -s http://127.0.0.1:8080/api/agents | python3 -m json.tool
```
Expected: `/api/agents` lists `canary_assessor` with `"model":"fixture","live":false`. Open `http://127.0.0.1:8080/agents` → the Agents page renders it. Stop the server.

- [ ] **Step 4: Spawn the validator-tester agent**

Per the project's proactive-validation rule, dispatch `validator-tester` to independently confirm: `make build`, `go test ./...`, `cd web && yarn test --run`, and that `qac serve` starts and `/api/agents` returns 200.

---

## Self-review

**1. Spec coverage:**
- New packages (`llmclient`, `agent`, `canaryassessor`, `prompt`) → Tasks 1,2,3,5,6. ✓
- `AgentOutput`/assessment shape → Task 3 (`agent.Output`), Task 2 (`Assessment`). ✓
- Live client (Opus 4.8, adaptive thinking, effort default high, strict tool, cache) → Task 6. ✓
- Fixture default + `X-Qac-Degraded`/live signal → Tasks 2, 7 (`live` flag on `/api/agents`), 8 (boot selection). Note: the spec mentioned surfacing the fixture/live state via the degraded header; this plan surfaces it via the `/api/agents` `live` field and a boot warning. The `X-Qac-Degraded` response header on assessment responses is not separately added (the verdict is delivered as run evidence, not a dedicated endpoint) — acceptable; flagged for the user.
- Engine auto-run at `ai_assess`, roll-up, no new event kinds → Task 4. ✓
- Verdict vocab mapping → Task 3 (`MapVerdict`) + Task 4 test. ✓
- `GET /api/agents` + Agents page → Tasks 7, 9. ✓
- Run-detail advisory rendering → Task 10. ✓
- `ANTHROPIC_API_KEY` via `.env` alias → Task 8. ✓
- Tests: hermetic engine (Task 4), unit (Tasks 1,2,3,5), handler (Task 7), guarded live (Task 6), frontend (Tasks 9,10). The spec's on-disk canonical-template integration test is **not** separately added — the hermetic engine test in Task 4 covers "verdict maps + rolls up + halts at signoff" with a minimal template. Flagged for the user as a scope trim.

**2. Placeholder scan:** No "TBD/implement later"; all code steps carry full code. One deliberate note-to-implementer in Task 6 (resolve any SDK symbol drift via `go doc`) and Task 10 Step 1/5 (locate the evidence-rendering site in `RunDetail.jsx`) — both are honest existing-codebase integration instructions, not placeholders. Fix the flagged non-ASCII in the `AssessmentCard` `fail` color to `#b42318` on write (Task 10 note).

**3. Type consistency:** `agent.Output` fields (Verdict/Confidence/Reasoning/RankedCauses/LikelyOwner/Sources) match `llmclient.Assessment` and the `AssessmentCard` payload keys (verdict/confidence/reasoning/ranked_causes/likely_owner/sources). `MapVerdict` inputs (`pass`/`needs_review`/`fail`) match the fixture and rubric. `assessmentContentType` (`application/vnd.qac.assessment+json`) is identical in Task 4 (engine) and Tasks 9/10 (frontend). `RegisterAgents`/`Agents()`/`AgentLLMInfo()` names are consistent across Tasks 4, 7. `Info` field names (`model`/`live`) consistent across `llmclient.Info` (Task 2), handler view (Task 7), and frontend `Agent` type (Task 9).
