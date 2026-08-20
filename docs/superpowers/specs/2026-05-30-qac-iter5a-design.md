# qac iter-5a — RLCC + FLCC clients backed by Sourcegraph

**Status:** draft
**Branch:** `wp/add-qac` (continues iter-4c)
**Predecessor:** [iter-4c spec](2026-05-30-qac-iter4c-design.md), iter-4c HEAD at `186d54f`
**PR posture:** still local. iter-4c continued the local-only posture; iter-5a stays on `wp/add-qac` until the canary verifies something end-to-end against a real RLCC workflow (iter-5d territory).

## 1. Purpose

Stop hand-authoring canary steps. The `gb200-rack-bringup-v4` walk is already defined upstream — in `rack-lifecycle-controller/chart/rack-lifecycle-controller/values.yaml` (17 RLCC workflows) and `fleet-lifecycle-controller/chart/values.yaml` (~25 FLCC workflows). iter-5a ports those two YAMLs into qac as typed Go data via a Sourcegraph-backed read client, with an offline fixture as the test backend and `QAC_RLCC_BACKEND=map` fallback.

After iter-5a there is **no wizard change, no engine change, no run-create change** — only two new internal packages, one shared HTTP helper, and three new read-only API routes. iter-5b consumes them in the wizard; iter-5c materializes a Template from them at run-create; iter-5d adds an `rlcc_action_probe` that polls per-rack progress against the materialized spine.

This is the foundation step — it converts "discovery" from a slogan into a typed read surface.

## 2. Scope decisions (from brainstorming)

| Decision | Value | Why |
|---|---|---|
| Auth | **`AWXCTL_SOURCEGRAPH_TOKEN` from env, sourced via Doppler/`source_passkeys`** | qac reads the env var; how it gets there is the launcher's contract. Matches every other CoreWeave Go tool. No Doppler SDK, no shell-out to `doppler`. |
| Pinning | **Per-chart SHA captured at wizard render, re-fetched at run-create** | Run is immutable against subsequent chart releases. Same SHAs two months apart → bit-identical Template. Pinning lives in `RunCreated.payload`; iter-5a only surfaces the SHA in the API response — actual snapshot embed is iter-5c. |
| Workflow breadth | **Surface all RLCC entries (17 workflows + 4 no-op state-movers = 21)** | State-movers expose `action_count: 0` so the UI can group/grey them. Hiding decisions live in the UI, not the API. Symmetric with how the YAML stores them. |
| FLCC scope | **Lands in iter-5a alongside RLCC** | The step rail in iter-5b needs FLCC terminal-state resolution (`components.computetray.workflow` → "waits for `l10-test-loop`"). Building FLCC separately would defer that resolution by an iter. Same shape, same backend, ~20 % more code. |
| Caching | **5-minute TTL, stale-while-revalidate, in-memory map keyed by `(repo, path, ref)`** | Wizard render never blocks on Sourcegraph. Admin endpoint `POST /api/admin/sourcegraph/refresh` for "I just merged a chart change and want it now." |
| SHA source | **`X-Sourcegraph-Resolved-Revision` response header** with a one-call GraphQL fallback | Probed against CoreWeave's Sourcegraph during iter-5a-01; falls back to GraphQL `query { repository(name) { commit(rev) { oid } } }` if the header is absent. Both paths live inside `internal/sourcegraph/`. |
| Fixture strategy | **`//go:generate` from vendored `testdata/*.yaml`** | Mechanical regeneration via `cmd/genrlccfixture`; manual refresh cadence; pinned upstream tag in a header comment. Re-parsing testdata must equal the committed `fixture_gen.go` — drift becomes a test failure. |
| Offline override | **`QAC_RLCC_BACKEND=map`** | One env var flips both `rlccclient` and `flccclient` to MapClient. Honored by `cmd/qac/serve.go`. The launcher does not need to set `AWXCTL_SOURCEGRAPH_TOKEN` when this is set. |
| `workflowOverrides` | **Out of scope** | Per-cluster FLCC phase overrides. iter-5a parses the field shape but does not apply it. Documented gap; revisit if a real run shows incorrect terminal-state resolution. |
| Trigger block | **Out of scope** | The sibling top-level `triggers:` section in the RLCC chart encodes priority-ordered workflow-selection rules. iter-5a parses workflows only. |
| Engine integration | **Deferred to iter-5c** | iter-5a only exposes read APIs. The engine continues to walk `templates/firmware-release-canary.yaml` until iter-5c rips it out. |
| Wizard changes | **Deferred to iter-5b** | NewRunModal, newRunSchema, MSW handlers all stay on the iter-2 shape until 5b. |

## 3. Discovered YAML shape (from recon, locked in)

### 3.1 RLCC (rack-lifecycle-controller v3.87.0)

```yaml
workflows:
  - name: gb200-rack-bringup-v4
    description: Rack Bringup V4 for GB200.
    lifecycle: stable                          # stable | deprecated
    current-states: [checked-in, onboarding]   # empty = any
    desired-state: ready
    actions:
      - name: await-rack-provision-v4
        handler: await-checkpoint
        description: Wait for all rack provisioning to complete
        arguments:
          checkpoint: rack                     # rack | power-operation
          components:
            computetray:
              workflow: gb200-rack-provision-v4
              flcc-end-state: l10-test-loop    # the BMN state RLCC waits for
              checks: [count, workflow]
            nvlink:
              workflow: Provision
              checks: [provisionable, workflow, count, healthy, init]
```

Key shape facts (worth memorizing):

- Actions are a **strictly linear ordered list**. No branches, no parallel forks. Per-component fan-out inside one action is intra-step parallelism, not graph branching.
- Sequencing is encoded by alternating doer-actions with `await-checkpoint` actions. The doer references its paired awaiter via `arguments.checkpoint-args-step`.
- **No `timeout`, `failState`, `awaitState`, `requeue` at the RLCC level.** Those live in controller code, not YAML.
- Five component kinds: `coolantdistributionunit`, `computetray`, `nvlink`, `powershelf`, `rack`.
- Eight handler kinds: `power-operation`, `await-checkpoint`, `hlcc-state`, `provision`, `l11-fielddiag`, `hpc-verification`, `flcc-workflow`, `enable-cks`.
- `current-states: []` (empty list) means "triggerable from any state."

### 3.2 FLCC (fleet-lifecycle-controller develop @ 2026-05-29)

```yaml
workflows:
  - name: gb200-rack-provision-v4
    class: rack-provision                      # rack-provision | node-provision
    description: |
      Node+DPU vaultify, seatrial then zap both dpu and host.
    phases:
      - name: node-vaultify
        state: node-vaultify
        failState: fail
        timeout: 1h
      - name: zap-seatrial
        state: zap-seatrial
        failState: fail
        timeout: 1h
        parameters:
          duration: 30m
          startbuffer: 20m
      - name: l10-test-loop                    # terminal — no failState, no timeout
        state: l10-test-loop
```

Key shape facts:

- Phases are an **ordered list**. The last phase is the workflow's terminal state — the BMN parks there until RLCC dispatches the next FLCC workflow.
- Terminal phases omit `failState` and `timeout`.
- `timeout` is a **stuck-state watchdog**, not a work-budget estimate. The YAML file's own comments call this out explicitly.
- `class` is `rack-provision` (RLCC-dispatched, runs per-CT during rack bringup) or `node-provision` (single-node lifecycle).
- **No `awaitState` or `rlcc_workflow` field anywhere.** The FLCC↔RLCC contract is implicit: RLCC's await-checkpoint waits for the terminal phase of the FLCC workflow it just dispatched. qac resolves this via `FLCCClient.TerminalPhase(name)`.

## 4. New packages

```
internal/sourcegraph/                # NEW — shared low-level HTTP client
  client.go                            type Client struct { token, http, cache }
                                       Fetch(ctx, repo, path, ref) (FetchResult, error)
  client_test.go                       httptest harness; header path + GraphQL fallback;
                                       cache hit/miss; missing-token → ErrNoToken

internal/rlccclient/                 # NEW
  types.go                             WorkflowSummary, WorkflowGraph, Action,
                                       ActionArgs, ComponentSpec, HandlerKind +
                                       ComponentKind consts
  client.go                            type Client interface {
                                         ListWorkflows(ctx) ([]WorkflowSummary, SourceRef, error)
                                         GetWorkflow(ctx, name) (WorkflowGraph, SourceRef, error)
                                       }
                                       SourceRef carries {ChartTag, SHA}
  parse.go                             ParseValuesYAML(io.Reader) (map[string]WorkflowGraph, error)
                                       Computes RackKind from name prefix
                                       (gb200-/b300-/gbx00-/bx00-/b200-/"")
                                       Computes ActionCount per workflow
  sourcegraph_client.go                Wraps internal/sourcegraph.Client
                                       Path: chart/rack-lifecycle-controller/values.yaml
                                       Repo: github.com/coreweave/rack-lifecycle-controller
  map_client.go                        Backed by Fixture; Source returns
                                       {ChartTag: "v3.87.0-fixture", SHA: "vendored"}
  fixture_gen.go                       Code-generated; DO NOT EDIT
  testdata/
    rlcc-values.yaml                   vendored slice; header pins
                                       "image.tag v3.87.0, updated 2026-05-29"
  client_test.go                       Round-trip: ParseValuesYAML(testdata) == Fixture
                                       Catalog has 21 entries
                                       gb200-rack-bringup-v4 has 12 actions
                                       action[3].Arguments.Components[computetray].Workflow ==
                                         "gb200-rack-provision-v4"
                                       action[4].Arguments.Components[computetray].FLCCEndState ==
                                         "l10-test-loop"
                                       4 state-mover workflows have ActionCount == 0

internal/flccclient/                 # NEW — mirrors rlccclient
  types.go                             FLCCWorkflowSummary, FLCCWorkflowGraph, FLCCPhase
  client.go                            type Client interface {
                                         ListWorkflows(ctx) ([]FLCCWorkflowSummary, SourceRef, error)
                                         GetWorkflow(ctx, name) (FLCCWorkflowGraph, SourceRef, error)
                                         TerminalPhase(ctx, name) (string, error)
                                       }
  parse.go                             ParseValuesYAML — computes PhaseCount,
                                       TerminalPhase = last phase's State
  sourcegraph_client.go                Path: chart/values.yaml
                                       Repo: github.com/coreweave/fleet-lifecycle-controller
  map_client.go                        Backed by Fixture
  fixture_gen.go                       Code-generated
  testdata/
    flcc-values.yaml                   vendored slice; pinned tag in header
  client_test.go                       gb200-rack-provision-v4 has 14 phases
                                       TerminalPhase("gb200-rack-provision-v4") == "l10-test-loop"
                                       TerminalPhase("gb200-rack-power-cycle") == "hold"
                                       TerminalPhase("gb200-rack-hpc-verification-v4") == "l12-test-loop"
                                       TerminalPhase("ready") == "ready"

internal/api/                        # EXTEND
  rlcc.go                              GET /api/rlcc/workflows
                                       GET /api/rlcc/workflows/{name}
                                       Both responses include
                                         source: {chart_tag, sha}
  flcc.go                              GET /api/flcc/workflows/{name}
                                       (lookup-by-name only; iter-5b's step rail
                                       fetches one at a time per RLCC handoff)
  admin.go                             POST /api/admin/sourcegraph/refresh
                                       (invalidates both client caches)

cmd/genrlccfixture/                  # NEW
  main.go                              Reads testdata/{rlcc,flcc}-values.yaml,
                                       writes
                                       internal/rlccclient/fixture_gen.go +
                                       internal/flccclient/fixture_gen.go

cmd/qac/serve.go                  # EXTEND
                                       At startup:
                                         - Read AWXCTL_SOURCEGRAPH_TOKEN.
                                         - Read QAC_RLCC_BACKEND (empty | "map").
                                         - If backend == "map" OR token == "":
                                             wire MapClient pair.
                                           Else:
                                             wire SourcegraphClient pair.
                                         - Log which backend won.
```

## 5. Go types (authoritative)

```go
// internal/sourcegraph/client.go
type Client struct {
    token   string
    httpDo  func(*http.Request) (*http.Response, error)
    cache   *ttlCache
    baseURL string // https://sourcegraph.coreweave.com
}

type SourceRef struct {
    ChartTag string `json:"chart_tag"`
    SHA      string `json:"sha"`
}

type FetchResult struct {
    Body []byte
    SHA  string
}

// Fetch returns the raw file body at repo@ref/path. SHA is resolved via the
// X-Sourcegraph-Resolved-Revision response header, with a one-call GraphQL
// fallback if the header is absent. Cache key is (repo, path, ref).
func (c *Client) Fetch(ctx context.Context, repo, path, ref string) (FetchResult, error)

// ErrNoToken means the env var is unset; callers should fall back to MapClient.
var ErrNoToken = errors.New("AWXCTL_SOURCEGRAPH_TOKEN unset")
```

```go
// internal/rlccclient/types.go
type HandlerKind string
const (
    HandlerPowerOperation  HandlerKind = "power-operation"
    HandlerAwaitCheckpoint HandlerKind = "await-checkpoint"
    HandlerHLCCState       HandlerKind = "hlcc-state"
    HandlerProvision       HandlerKind = "provision"
    HandlerL11FieldDiag    HandlerKind = "l11-fielddiag"
    HandlerHPCVerification HandlerKind = "hpc-verification"
    HandlerFLCCWorkflow    HandlerKind = "flcc-workflow"
    HandlerEnableCKS       HandlerKind = "enable-cks"
)

type ComponentKind string
const (
    ComponentCDU         ComponentKind = "coolantdistributionunit"
    ComponentComputeTray ComponentKind = "computetray"
    ComponentNVLink      ComponentKind = "nvlink"
    ComponentPowerShelf  ComponentKind = "powershelf"
    ComponentRack        ComponentKind = "rack"
)

type WorkflowSummary struct {
    Name          string   `yaml:"name"           json:"name"`
    Description   string   `yaml:"description"    json:"description"`
    Lifecycle     string   `yaml:"lifecycle"      json:"lifecycle"`
    CurrentStates []string `yaml:"current-states" json:"current_states"`
    DesiredState  string   `yaml:"desired-state"  json:"desired_state"`
    RackKind      string   `yaml:"-"              json:"rack_kind"`
    ActionCount   int      `yaml:"-"              json:"action_count"`
}

type WorkflowGraph struct {
    WorkflowSummary `yaml:",inline"`
    Actions         []Action `yaml:"actions" json:"actions"`
}

type Action struct {
    Name          string      `yaml:"name"                    json:"name"`
    Handler       HandlerKind `yaml:"handler"                 json:"handler"`
    Description   string      `yaml:"description"             json:"description"`
    Force         bool        `yaml:"force,omitempty"         json:"force,omitempty"`
    SkipOnFailure bool        `yaml:"skipOnFailure,omitempty" json:"skip_on_failure,omitempty"`
    Arguments     ActionArgs  `yaml:"arguments"               json:"arguments"`
}

type ActionArgs struct {
    Checkpoint         string                          `yaml:"checkpoint,omitempty"            json:"checkpoint,omitempty"`
    CheckpointArgsStep string                          `yaml:"checkpoint-args-step,omitempty"  json:"checkpoint_args_step,omitempty"`
    Power              string                          `yaml:"power,omitempty"                 json:"power,omitempty"`
    Components         map[ComponentKind]ComponentSpec `yaml:"components,omitempty"            json:"components,omitempty"`
}

type ComponentSpec struct {
    State        string   `yaml:"state,omitempty"           json:"state,omitempty"`
    Workflow     string   `yaml:"workflow,omitempty"        json:"workflow,omitempty"`
    FLCCEndState string   `yaml:"flcc-end-state,omitempty"  json:"flcc_end_state,omitempty"`
    Checks       []string `yaml:"checks,omitempty"          json:"checks,omitempty"`
}
```

```go
// internal/flccclient/types.go
type PhaseClass string
const (
    PhaseClassRackProvision PhaseClass = "rack-provision"
    PhaseClassNodeProvision PhaseClass = "node-provision"
)

type FLCCWorkflowSummary struct {
    Name          string     `yaml:"name"        json:"name"`
    Class         PhaseClass `yaml:"class"       json:"class"`
    Description   string     `yaml:"description" json:"description"`
    PhaseCount    int        `yaml:"-"           json:"phase_count"`
    TerminalPhase string     `yaml:"-"           json:"terminal_phase"`
}

type FLCCWorkflowGraph struct {
    FLCCWorkflowSummary `yaml:",inline"`
    Phases              []FLCCPhase `yaml:"phases" json:"phases"`
}

type FLCCPhase struct {
    Name        string            `yaml:"name"                  json:"name"`
    State       string            `yaml:"state"                 json:"state"`
    Timeout     string            `yaml:"timeout,omitempty"     json:"timeout,omitempty"`
    FailState   string            `yaml:"failState,omitempty"   json:"fail_state,omitempty"`
    RequeueTime string            `yaml:"requeueTime,omitempty" json:"requeue_time,omitempty"`
    Parameters  map[string]string `yaml:"parameters,omitempty"  json:"parameters,omitempty"`
}
```

🟦 **Learning beats for Wallace.** Three Go idioms surface here:
- `type HandlerKind string` + `const` block is Go's "string with closed set of valid values" pattern. No enums in Go. Unknown handler strings still unmarshal (it's still a string); validation that the value is one of the known set lives in a method, not the type system.
- `yaml:",inline"` on an embedded struct flattens its fields into the parent during marshal/unmarshal. So the YAML matches the real chart shape exactly — `name`, `description`, etc. appear at the workflow level, not nested under a `summary` key.
- `,omitempty` on `json` / `yaml` tags means "skip this field on marshal if it's the zero value." Important so the JSON the frontend sees doesn't carry `power: ""` for non-`power-operation` actions.

## 6. HTTP surface

```
GET  /api/rlcc/workflows
     200 OK
     {
       "workflows": [
         { "name": "gb200-rack-bringup-v4", "description": "Rack Bringup V4 for GB200.",
           "lifecycle": "stable", "current_states": ["checked-in","onboarding"],
           "desired_state": "ready", "rack_kind": "gb200", "action_count": 12 },
         ...
       ],
       "source": { "chart_tag": "v3.87.0", "sha": "abc123..." }
     }

GET  /api/rlcc/workflows/{name}
     200 OK
     {
       "workflow": { …WorkflowGraph… },
       "source":   { "chart_tag": "v3.87.0", "sha": "abc123..." }
     }
     404 { "error_code": "not_found", "message": "no RLCC workflow named X" }

GET  /api/flcc/workflows/{name}
     200 OK
     {
       "workflow": { …FLCCWorkflowGraph… },
       "source":   { "chart_tag": "v0.134.1", "sha": "def456..." }
     }
     404 { "error_code": "not_found" }

POST /api/admin/sourcegraph/refresh
     200 OK
     { "invalidated": ["rlcc", "flcc"] }
```

Error codes match the existing qac contract (`not_found`, `invalid_request`, `internal`). Sourcegraph fetch failures with MapClient fallback → 200 with `source.chart_tag == "v3.87.0-fixture"` and an `X-Qac-Degraded: true` response header — same degraded-mode pattern as the projection-unavailable case in `runs.go`.

## 7. Caching

- One `ttlCache` lives in `internal/sourcegraph/client.go`. Keyed by `(repo, path, ref)`.
- TTL = 5 minutes. Cache hit returns instantly; cache miss makes one HTTP call and stores the result with `fetched_at = time.Now()`.
- Stale-while-revalidate: if a cached entry is older than 5 minutes but younger than 30 minutes, return the stale entry and kick off a background refresh. Older than 30 minutes → synchronous refetch.
- `POST /api/admin/sourcegraph/refresh` calls `cache.Purge()`. No partial purge — refresh blasts everything.
- Cache is in-process; restarting qac repopulates from scratch. Acceptable at iter-5a scale.

## 8. Startup wiring (`cmd/qac/serve.go`)

Pseudo-code for the new wiring block:

```go
token := os.Getenv("AWXCTL_SOURCEGRAPH_TOKEN")
backend := os.Getenv("QAC_RLCC_BACKEND") // "" or "map"

var rlccC rlccclient.Client
var flccC flccclient.Client
if backend == "map" || token == "" {
    if backend != "map" {
        slog.Warn("using map backend; AWXCTL_SOURCEGRAPH_TOKEN unset")
    } else {
        slog.Info("using map backend per QAC_RLCC_BACKEND=map")
    }
    rlccC = rlccclient.NewMapClient()
    flccC = flccclient.NewMapClient()
} else {
    sg := sourcegraph.NewClient(token, "https://sourcegraph.coreweave.com")
    rlccC = rlccclient.NewSourcegraphClient(sg)
    flccC = flccclient.NewSourcegraphClient(sg)
    slog.Info("using sourcegraph backend", "token_len", len(token))
}
```

`source_passkeys` is the operator's responsibility — qac does not invoke it.

## 9. Tests

| File | Asserts |
|---|---|
| `internal/sourcegraph/client_test.go` | Token-missing → `ErrNoToken`. Header path returns SHA. GraphQL fallback when header absent. Cache hit count == 1 over 5 calls in 1 second. Stale-while-revalidate path returns stale + refreshes async. |
| `internal/rlccclient/parse_test.go` | ParseValuesYAML on testdata returns 21 entries. gb200-rack-bringup-v4 has 12 actions. RackKind derived correctly for "gb200-...", "b300-...", unknown prefix → empty. action[4] `components.computetray.flcc-end-state` == "l10-test-loop". State-movers (checked-in, onboarding, ready, triage) have ActionCount == 0. |
| `internal/rlccclient/client_test.go` | Round-trip: re-parsing testdata produces a map deep-equal to the committed Fixture. (Drift between testdata and fixture_gen.go is a test failure.) |
| `internal/flccclient/parse_test.go` | gb200-rack-provision-v4 has 14 phases. TerminalPhase("gb200-rack-provision-v4") == "l10-test-loop". TerminalPhase("ready") == "ready". TerminalPhase("nonexistent") returns ErrNotFound. |
| `internal/flccclient/client_test.go` | Round-trip parity. |
| `internal/api/rlcc_test.go` | GET list returns 21 entries + source. GET by name returns full graph + source. Unknown name → 404 with `error_code: "not_found"`. With MapClient: `X-Qac-Degraded` header present. |
| `internal/api/flcc_test.go` | GET by name. Unknown → 404. |
| `internal/api/admin_test.go` | Refresh returns `invalidated: ["rlcc","flcc"]`. |

`go generate ./...` is idempotent: running it twice produces zero diff. CI enforces this via a `git diff --exit-code` step after `go generate` (lands in a follow-up; iter-5a documents the procedure but the check itself is iter-5b territory).

## 10. Refresh procedure (for the README / iter-5a plan)

When a chart bumps upstream and we want qac's offline fixture to reflect it:

```bash
# Pull the latest values.yaml at a pinned tag.
curl -H "Authorization: token $AWXCTL_SOURCEGRAPH_TOKEN" \
  "https://sourcegraph.coreweave.com/.api/repos/github.com/coreweave/rack-lifecycle-controller@v3.88.0/-/raw/chart/rack-lifecycle-controller/values.yaml" \
  > internal/rlccclient/testdata/rlcc-values.yaml

# Same for FLCC.
curl ... > internal/flccclient/testdata/flcc-values.yaml

# Regenerate fixtures.
go generate ./...

# Verify nothing else moved.
go test ./internal/rlccclient/... ./internal/flccclient/...

# Update the header comment in each testdata file with the new pinned tag + date.
```

The procedure is **manual**. Auto-running on every `make build` is rejected — it slows builds, couples the build to network access, and silent drift is exactly what the round-trip test is supposed to surface.

## 11. Risks and open questions

**Risk: Sourcegraph raw API doesn't expose `X-Sourcegraph-Resolved-Revision`.** Mitigation: GraphQL fallback inside `internal/sourcegraph/`. iter-5a-01 probes the real endpoint and picks the path. Documented in the package doc-block.

**Risk: workflowOverrides applied at chart render time would make our terminal-phase resolution wrong on a real cluster.** Mitigation: documented gap; iter-5a parses the field shape so iter-6 can fold it in without a schema change. Real-cluster mismatch would surface as `rlcc_action_probe` (iter-5d) showing a node parked in an unexpected state — visible failure mode, not silent corruption.

**Risk: workflow descriptions are developer notes** (terse, mixed case, marketing capitalization). The picker subtitle will be ugly until someone does an editorial pass. iter-5a accepts that — the descriptions land verbatim in the API; iter-5b can apply a `{workflow_name: display_copy}` overlay if needed.

**Open question:** does the SHA returned by `/api/rlcc/workflows` get passed back on `POST /api/runs` as a top-level field or nested under inputs? **Decided:** top-level `source_sha` field on the request body (iter-5c). iter-5a does not implement run-create changes; this is documented for forward reference.

**Open question:** RackKind regex bucketing — what's the canonical set? **Decided for iter-5a:** `gb200`, `b300`, `gbx00`, `b200`, `bx00` for known prefixes, empty string for everything else. Mapping lives in `parse.go`. Platform team has not formally promised these are stable; the empty-fallback prevents silent miscategorization.

**Open question:** `internal/sourcegraph/` is a generic file-fetcher. Should it move to a sibling repo for reuse? **Decided for iter-5a:** no — qac is self-contained (bare module path, no sibling-repo imports). If `cwf` or `awxctl` later need this, they port it the same way.

## 12. Out of scope (explicit non-goals)

- Wizard rewrite (iter-5b).
- `POST /api/runs` accepting `source_sha` (iter-5c).
- Template materialization at run-create (iter-5c).
- `rlcc_action_probe` (iter-5d).
- PromQL-backed per-rack progress polling (iter-6+).
- Workflow dispatch (`cwctl rlcc workflow run`). qac stays observe-only.
- `workflowOverrides` (FLCC per-cluster phase overrides).
- RLCC `triggers:` block (priority-ordered workflow-selection rules).
- 1Password fallback for the token. Doppler/`source_passkeys` is the iter-5a path.
- CI enforcement of `go generate ./...` idempotency. Procedure documented; check is iter-5b.

## 13. Acceptance criteria

After iter-5a is merged on `wp/add-qac`:

1. `make test` is green; `make lint` is green.
2. `go generate ./...` is idempotent — running it twice from a clean tree produces zero diff.
3. `qac serve` starts cleanly with `QAC_RLCC_BACKEND=map` and no `AWXCTL_SOURCEGRAPH_TOKEN`. Log line confirms the map backend.
4. `qac serve` with a real `AWXCTL_SOURCEGRAPH_TOKEN` exported (operator runs `source_passkeys` first) starts cleanly and the first `GET /api/rlcc/workflows` succeeds end-to-end, returning a non-fixture SHA.
5. `curl localhost:8080/api/rlcc/workflows` returns 21 entries with `action_count: 0` on the four state-mover workflows.
6. `curl localhost:8080/api/rlcc/workflows/gb200-rack-bringup-v4` returns a graph with 12 actions in documented order; `action[3].arguments.components.computetray.workflow == "gb200-rack-provision-v4"`; `action[4].arguments.components.computetray.flcc_end_state == "l10-test-loop"`.
7. `curl localhost:8080/api/flcc/workflows/gb200-rack-provision-v4` returns a graph with 14 phases; the last phase's state is `l10-test-loop`.
8. `curl -X POST localhost:8080/api/admin/sourcegraph/refresh` returns `{"invalidated": ["rlcc","flcc"]}` and a subsequent `GET /api/rlcc/workflows` hits Sourcegraph again (verifiable via debug log).
9. No changes to the wizard, the engine, or the existing canonical canary template are required for iter-5a to ship.

## 14. Forward references

| iter | Adds |
|---|---|
| 5b | Wizard rewrite (`NewRunModal` dropdown driven by `/api/rlcc/workflows`; `newRunSchema.ts` → `{template_id, inputs, source_sha}`; MSW handlers; tests). |
| 5c | `createRunHandler` materialization: re-fetch RLCC + FLCC charts at pinned `source_sha`, splice RLCC spine between qac pre-spine (`alert_baseline`, `fact_baseline`, `baseline_pre`) and post-spine (`baseline_post`, `evidence_review`, `ai_summary`, `operator_signoff`), embed in `RunCreated.payload`. `loadTemplateFromEvents` reads from event payload. `templates/firmware-release-canary.yaml` retires or downgrades. |
| 5d | `internal/probe/rlccprobe/` — `CategoryAssertion`, polls per-rack via PromQL, RLCC status → qac verdict map. Canonical integ test extended to walk the materialized template. |
| 6 | PromQL backend for per-rack progress (port `rack_*lcc_*.promql` from `~/Documents/argo/tiphys/inventory/`). `workflowOverrides` resolution. RLCC `triggers:` block model. Dispatch capability (gated decision). |
