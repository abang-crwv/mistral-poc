# AI-reasoned verdict — design

Date: 2026-07-05
Status: design (approved for planning)
Related: `templates/firmware-release-canary.yaml` (`ai_summary` step), `internal/engine/runner.go`, `internal/probe/`, `internal/verdict/`, memory `qac-current-state`

## Motivation

qac walks a canary template's steps; gatherer probes accumulate per-rack evidence, but nothing yet produces a **verdict** — the run walks to the end and finalizes with an empty verdict (`runner.go` `worstVerdictFromEvents`). The template already declares an `ai_assess` step (`ai_summary`) with `prompt_ref`, `context_steps`, and `expected_verdict: [pass, fail, needs_review]`, but the runner currently just **halts** at it as a manual operator gate (`runner.go:109`). There is no LLM client in the repo.

We want the verdict to be **AI-reasoned**: an agent reasons over the evidence the probes gathered and emits an advisory verdict with confidence and ranked likely causes, which the operator then signs off on. This is the first real "does the canary pass" signal in the tool.

## Guiding principle (the agent contract)

The agent follows the operator's "ZachGPT" contract for how fleet-ops agents should behave:

- **Read-only reasoning.** The probes already gathered the evidence; the agent reasons over it, it does not mutate fleet state.
- **States confidence.** Every assessment carries a confidence level and its reasoning.
- **Narrows and ranks.** It ranks likely causes and names a likely owning team rather than asserting a single answer.
- **Agents narrow, humans decide.** The verdict is **advisory**. The existing `operator_signoff` step is the human decision and can finalize the run.

## Scope

### In scope (iteration 1)

- A **single-shot** structured LLM call that reads accumulated evidence and returns a verdict.
- A **general agent framework** (registry + interface + LLM port + prompt registry) that other agent purposes (the stub `agents`/`reports`/etc. surfaces) can later plug into — with one concrete agent, the canary assessor, wired to the `ai_summary` step.
- Auto-run at `ai_assess`: the verdict rolls up into run status; the walk continues to `operator_signoff`.
- Fixture-backed default (no `ANTHROPIC_API_KEY` → deterministic canned assessment); live via `.env`.
- `GET /api/agents` + a real Agents page (replaces the `StubPage`).

### Out of scope (future requests)

- **Tool-using agent loop** (agent re-verifies against live inventory/o11y like ZachGPT). Iteration 1 reasons only over already-gathered evidence; the LLM port is the seam where a loop replaces the single-shot call.
- **Per-step assertion probes / hybrid** (an AI verdict per check step gating the walk mid-flight). Iteration 1 is one end-of-run assessment.
- Streaming the assessment to the UI; the Agents page as a full agent-management console.

## Architecture

### New packages

```
internal/llmclient/     LLM port. Client interface + fixture default + live Anthropic impl
                        (github.com/anthropics/anthropic-sdk-go). Selected by ANTHROPIC_API_KEY.
internal/agent/         Agent interface + Registry (mirrors internal/probe/probe.go) + AgentOutput.
  canaryassessor/       First concrete agent, keyed to prompt_ref "fw-release-canary.summary.v1".
internal/prompt/        Embedded prompt registry keyed by prompt_ref (//go:embed prompts/*.md).
```

The `agent` package mirrors `probe` deliberately so the two subsystems are learnable together and future agent purposes register the same way.

- `agent.Agent` interface:
  ```go
  type Agent interface {
      Type() string                 // e.g. "canary_assessor"
      Purpose() string              // one line, for /api/agents
      PromptRef() string            // the prompt_ref it services
      Run(ctx, AgentClients, AgentContext) (AgentOutput, error)
  }
  ```
- `AgentContext`: `{ RunID, StepID, ContextSteps []string, Inputs map[string]any, RespectSuppressions, CitePatterns, CiteGlean, ExpectedVerdict []string }` — assembled by the runner from the step + run.
- `AgentOutput`:
  ```go
  type AgentOutput struct {
      Verdict     string       // "pass" | "needs_review" | "fail" (template vocabulary)
      Confidence  string       // "high" | "medium" | "low"
      Reasoning   string
      RankedCauses []Cause     // {Summary, LikelyOwner, Confidence}
      LikelyOwner string
      Sources     []string     // provenance (e.g. "anthropic:claude-opus-4-8", or "fixture")
  }
  ```
  Persisted as an evidence row (JSON) on the step.
- `agent.Registry`: `Register`/`Get(promptRef)`/`List() []Info` — same shape as `probe.Registry`.
- `AgentClients`: aggregate of ports the agent needs (iteration 1: `LLMClient`, `EvidenceReader`). Grows additively, like `probe.Clients`.

### LLM port

`llmclient.Client`:
```go
type Client interface {
    Assess(ctx context.Context, req AssessRequest) (Assessment, error)
}
```
- `AssessRequest`: `{ SystemPrompt string, EvidenceBundle string, ExpectedVerdict []string }`.
- `Assessment`: `{ Verdict, Confidence, Reasoning, RankedCauses, LikelyOwner, Model string }`.

**Live implementation** (`AnthropicClient`), using `github.com/anthropics/anthropic-sdk-go`:
- Model `claude-opus-4-8` (default; overridable via env later).
- Adaptive thinking (`thinking: {type: "adaptive"}`), `output_config.effort: "high"` — a judgment task where correctness matters.
- **Structured output** via a strict tool `record_assessment` (`strict: true`, `additionalProperties: false`) whose schema is the `Assessment` shape and whose enum for `verdict` is the step's `expected_verdict`; force `tool_choice` to that tool. (Equivalently `output_config.format` json_schema — the plan picks one; strict tool is the primary approach for the Go SDK.)
- **Prompt caching**: the system prompt (rubric + instructions, from the prompt registry) is the stable prefix and carries a `cache_control` breakpoint; the per-run evidence bundle follows it (volatile, uncached). Stable across runs → cache reads on the rubric.
- Streaming for the request (large evidence + thinking) via the SDK's accumulate-final-message pattern.

**Fixture implementation** (`FixtureClient`): returns a deterministic assessment derived from the evidence bundle (e.g. `fail` if the bundle contains a failed-tray marker, else `pass`, confidence `low`, reasoning noting it is a fixture). Used when `ANTHROPIC_API_KEY` is absent. Keeps tests hermetic and the flow offline-capable, and sets the degraded signal (below).

### Engine change (auto-run at ai_assess)

`runner.go` currently halts at `review`/`ai_assess`/`signoff` (`runner.go:109`). Change: `ai_assess` no longer halts. On an `ai_assess` step the runner:

1. Loads evidence rows for each `step.ContextSteps` via `store.ListEvidence(runID, stepID)`; assembles a text **evidence bundle** (per-step, per-rack). If `respect_suppressions`, filter suppressed rows out of the bundle.
2. Looks up the agent in the **agent registry** by `step.PromptRef` (falls back to `StepFailed` with a clear message if none registered — mirrors the missing-probe path at `runner.go:128`).
3. Runs the agent → `AgentOutput`.
4. Persists `AgentOutput` as an evidence row (`store.PutEvidence`), like a probe.
5. Emits `StepCompleted` with the **mapped** verdict (below), reusing `emitStepCompleted(..., verdict)` — the verdict then flows through the existing `worstVerdictFromEvents` → `RunCompleted` rollup with **no new event kinds**.

`review` and `signoff` steps continue to halt as operator gates. So the walk is: gatherers → `ai_assess` (auto, advisory verdict rolls up) → halt at `operator_signoff` (human decision, can finalize to `signed_off`, which is terminal and overrides per `verdict.go`).

### Verdict vocabulary

Two vocabularies exist: engine/`verdict.go` uses `passed`/`warning`/`failed`; the template's `ai_assess`/`signoff` use `pass`/`fail`/`needs_review`. The agent emits the template vocabulary (its structured-output enum is the step's `expected_verdict`); a single mapping function converts to engine vocabulary before `emitStepCompleted`:

| Agent verdict | Engine verdict |
|---|---|
| `pass` | `passed` |
| `needs_review` | `warning` |
| `fail` | `failed` |

`needs_review → warning` is deliberate: warning is non-terminal and signals "human should look", which is exactly `needs_review`'s intent, and it rolls up correctly (a `fail` on any step still dominates via `verdict.Worse`).

### HTTP + frontend

- **`GET /api/agents`** (registered in `internal/api/api.go`, mirrors `listProbesHandler`): returns each registered agent's `{ type, purpose, prompt_ref, model, live }`. `live` reflects whether the LLM client is the live Anthropic impl (key present) or fixture.
- **Agents page**: replace the `agents` → `StubPage` route in `web/src/app/routes.tsx` with a real page (`web/src/features/agents/`) listing agents from `/api/agents`, showing purpose, model, and a live/fixture badge. Scope: read-only list, matching the existing `probes` feature.
- **Run detail**: the run-detail view already renders per-step evidence. The `ai_summary` step's evidence renders verdict + **confidence** + reasoning + ranked causes + likely owner, badged **"advisory — operator decides."** No new run-detail route; a renderer for the assessment evidence shape.

### Config

- `ANTHROPIC_API_KEY` read from the environment; loaded from `.env` via the existing `cmd/qac/dotenv.go` path. Add `ANTHROPIC_API_KEY` (and a `CLAUDE_API_KEY` alias) to `applyEnvAliases` so bare names resolve. Documented in `README.md` alongside the other creds.
- No key → `FixtureClient`; key present → `AnthropicClient`. The choice is wired at server boot (`cmd/qac/serve.go`), like the other live-vs-fixture backends, and surfaced through the `X-Qac-Degraded` header convention already used for fixture-backed responses.

### External dependency

Adds `github.com/anthropics/anthropic-sdk-go` to `go.mod`. This is a normal external module (fetched via the Go proxy), consistent with the existing `modernc.org/sqlite`, cobra, and ulid dependencies. It does **not** violate the repo's hard constraints: no `replace` directive, and it is not a sibling argo project (the "port, don't import" rule is about `~/Documents/argo/*`).

## Data flow

```
operator creates run
  → engine walks steps
    → gatherer probes write evidence rows (existing)
    → ai_summary (kind: ai_assess):
        runner assembles evidence bundle from context_steps' evidence rows
        → agent.Registry.Get("fw-release-canary.summary.v1")
        → canaryassessor.Run:
            build system prompt (prompt registry, prompt_ref) + evidence bundle
            → llmclient.Assess → Assessment {verdict, confidence, reasoning, ranked_causes, likely_owner}
        → AgentOutput persisted as evidence row
        → StepCompleted(verdict = map(agent verdict))   ← existing plumbing
    → worstVerdictFromEvents rolls up run status (passed/warning/failed)
    → halt at operator_signoff (human gate)
operator signs off → run finalizes (signed_off, terminal)
```

## Error handling

- **Missing agent** for a `prompt_ref` → `StepFailed` with `agent type %q not registered` (mirrors missing-probe path). Walk continues.
- **LLM call fails** (network, 4xx/5xx after SDK retries, refusal, timeout) → `StepFailed` with the error; the run does not get a verdict from this step and the operator sees the failure on the run. (Fixture path never errors.)
- **Structured-output / schema mismatch** → the SDK's strict-tool validation forces a retry; a persistent mismatch surfaces as `StepFailed`.
- Consistent with the existing "never 500 after a successful side-effect" rule: evidence is persisted before `StepCompleted` is emitted, and an orphaned-evidence path already exists in `runner.go`.

## Testing

- **Hermetic engine test**: an `ai_assess` step end-to-end with the `FixtureClient`, asserting evidence is persisted, `StepCompleted` carries the mapped verdict, and the run rolls up to the expected status — no network.
- **Canonical-template integration test** (following `internal/api/canonical_integ_test.go`): load `templates/firmware-release-canary.yaml`, drive a run through `ai_summary`, assert the verdict maps and rolls up, and that the walk then halts at `operator_signoff`.
- **Unit tests**: verdict vocabulary mapping (`pass|needs_review|fail` → `passed|warning|failed`), evidence-bundle assembly (including suppression filtering), and `AgentOutput` JSON round-trip.
- **Live path** guarded behind `ANTHROPIC_API_KEY` (skipped when absent), asserting the live client returns a well-formed `Assessment` for a small fixture bundle.
- Frontend: an MSW-backed test for the Agents page and the assessment evidence renderer.

## Risks / open questions

- **Advisory verdict rolling up to run status.** A `fail` from the AI sets the run `failed` before the operator signs off. This is intended (the canary's assessment is the run outcome; sign-off finalizes/overrides), but the UI must make the advisory nature unmistakable to avoid the TARS failure mode.
- **Cost/latency.** Opus 4.8 at `high` effort on a large evidence bundle is a minutes-scale call; the engine runs it inline during the walk. Acceptable for iteration 1 (canary runs are not latency-sensitive); a future async/streaming path can improve UX.
- **Prompt quality is load-bearing.** The rubric in the prompt registry determines verdict quality. Iteration 1 ships a first `fw-release-canary.summary.v1` rubric; it will need tuning against real runs. `cite_patterns`/`cite_glean` are honored best-effort (there is no live pattern/Glean enrichment feeding the bundle yet).

## Iteration boundary

This is iteration 1 of the agent surface. The seams for later work:
- **Tools as integrations (defined next iteration).** See below.
- Per-step / hybrid AI verdicts register additional agents against other `prompt_ref`s / step kinds; the registry and evidence/verdict plumbing are unchanged.
- Other stub surfaces (agents/reports) consume the same `agent` framework.

### Tools as integrations (iteration 2, out of scope here)

Agents will need **read-only tools** to reason well — re-verifying against live state instead of trusting the point-in-time evidence bundle (the ZachGPT "live data over memory" rule). Iteration 1 does not build tools, but its structure is chosen so they slot in without reshaping the agent surface:

- **The seam is `AgentClients` + `Agent.Run`.** In iteration 1, `AgentClients` carries `LLMClient` + `EvidenceReader`, and `canaryassessor.Run` makes one `llmclient.Assess` call. In iteration 2, `AgentClients` grows a **tool/integration registry**, and `Run` becomes a reason→call-read-only-tool→refine loop (the SDK's tool-runner / manual agentic loop). The `Agent` interface, the registry, the `AgentOutput` envelope, and the engine/evidence/verdict plumbing are all unchanged — only the body of `Run` and the clients aggregate grow.
- **Tools are built as integrations**, keyed to the `integrations` surface (today a `StubPage`). Each integration is a **read-only** port exposed to the agent as a tool: `cw-fleet-tools` wrappers (racks/rdesc/rocebr/bmns), `cw-plugins` skills (triage-sweep, ib-bw-check, node-net-test), o11yops/Grafana, `kubectl`, `git log`. qac already has ported read-only backend clients (`inventoryclient`, `lifecycleclient`, `lokiclient`, `rlccclient`, …) that these tools wrap — the integration layer adapts them into agent-callable tools rather than adding new fleet access.
- **Contract carries over.** Tools are read-only (the agent names a mutating command but never runs it); every tool call is logged/attributable as provenance on the `AgentOutput.Sources`; the operator sign-off remains the decision.

A separate spec + plan will design the tool/integration registry, the per-tool schemas, and the loop. This section only fixes the iteration-1 seam so that work is additive.
