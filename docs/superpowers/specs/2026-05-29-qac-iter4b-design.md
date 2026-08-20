# qac iter-4b — engine + first probe (alert_probe) end-to-end

**Status:** draft
**Branch:** `wp/add-qac` (continues iter-4a)
**Predecessor:** [iter-4a spec](2026-05-29-qac-iter4a-design.md), iter-4a HEAD at `5d0333e`
**PR posture:** this iteration is the **PR-opener**. iter-4a was held local; iter-4b opens the first PR because the canary verifies something end-to-end (probe runs, produces evidence, operator reads it back).

## 1. Purpose

Land the probe execution loop. After iter-4a's metadata model, iter-4b ships the engine that drives a run forward, the **probe-type abstraction** (with three categories: gatherer, assertion, action), the **first concrete probe** (`alert_probe`, a gatherer), an **evidence store** for probe outputs, and the HTTP endpoint to retrieve evidence. After this iteration, the loop is: operator POSTs a run → engine kicks off a goroutine → probe runs against a stubbed backend → evidence lands → operator polls or follows the evidence URL.

## 2. Scope decisions (from brainstorming)

| Decision | Value | Why |
|---|---|---|
| Engine drive model | **Async goroutine + poll** | Sets up iter-4c's long-running probes (60m/18h) without rework. Operator polls `GET /api/runs/{id}`. |
| First probe | **`alert_probe`** | Atomic, stub-friendly, useful standalone and as a baseline component. User flagged it earlier. |
| Probe verdict model | **None for gatherers** | Probes split into three categories — gatherer / assertion / action. iter-4b ships a gatherer; no checkpoint expression evaluator needed. Verdict-at-step belongs to assertion probes (iter-4c+). |
| Output storage | **New `evidence` table** | FK to events. Scales to multi-MB blobs in later iterations. Retrieved via new `GET /api/runs/{id}/evidence/{eid}` endpoint. |
| Alert source backing | **Stubbed `AlertClient` interface** | Mirrors iter-4a's `inventoryclient` port pattern. `MapAlertClient` + `SeedDemoAlerts()` fixtures. Real Alertmanager/awxctl integration deferred to iter-4c. |
| Step state | **Computed from events** | No new projection table for steps in iter-4b. Events are the source of truth; consumers re-derive on read. Add a projection table later if/when query patterns demand it. |
| Multi-rack handling | **One probe call covers all racks** | alert_probe takes `sc.Racks = [...]` and produces one `Output` with a `per_rack` map. One StepCompleted event covers the whole multi-rack call. |
| Engine walks template? | **No — stops after first step** | iter-4b ships one step end-to-end. iter-4c adds the multi-step walk (probably via a re-Kick on StepCompleted). |
| Frontend | **No new UI** | Existing run-detail page already lists events; new event kinds render via their `kind` string. Evidence is a manual `curl` for iter-4b; iter-4c adds the step rail + evidence renderer. |
| Checkpoint expression evaluator | **Deferred to iter-4c** | First assertion probe needs it. Gatherers don't. |
| Probe timeout (template's `action.timeout`) | **Read but not enforced** | alert_probe with stubbed client returns instantly. iter-4c wires `context.WithTimeout` when the first real backend lands. |

## 3. Probe categories (codified)

| Category | Role | Verdict-at-step? | iter-4b examples | Future examples |
|---|---|---|---|---|
| `gatherer` | Captures state, produces evidence | **No** — engine just records `StepCompleted` on success | `alert_probe` | `fact_probe`, `inventory_probe`, baseline components |
| `assertion` | Definitively passes / fails | Yes (iter-4c onward via checkpoint expr) | none yet | `l_test_probe`, `fielddiag_probe`, `seatrial_probe` |
| `action` | Performs an operation; reports completion | Yes (action-success vs action-failure) | none yet | `node_zap_probe` |

The category is metadata on the probe type, not config. iter-4b only exercises gatherer semantics.

## 4. New + extended packages

```
internal/probe/                    # NEW
  probe.go                           Probe interface, Category, Clients, StepContext, Output, Registry
  probe_test.go                      Registry get/set, Category constants, Output JSON round-trip

internal/alertclient/              # NEW — port pattern, mirrors inventoryclient
  alertclient.go                     Alert struct, Client interface, ErrSourceUnavailable
  alertclient_test.go                — (most tests live in map_client_test.go)
  map_client.go                      MapAlertClient + NewMapAlertClient + SeedDemoAlerts
  map_client_test.go                 Happy lookup, miss → empty list, ErrSourceUnavailable simulator
  fixture_coverage_test.go           Asserts every rack in canary template's example racks has a fixture entry

internal/probe/alertprobe/         # NEW — first concrete probe
  alertprobe.go                      AlertProbe (Type, Category, Run)
  alertprobe_test.go                 Multi-rack happy, no-alerts, transport error bubbles up, output schema

internal/engine/                   # EXTEND
  engine.go                          Engine struct, New, Kick, Wait, Shutdown
  runner.go                          Internal goroutine body (loadTemplate → findNextStep → emit StepStarted → probe.Run → persist evidence → emit StepCompleted/Failed)
  engine_test.go                     Runner happy path + error paths + Wait semantics + Shutdown
  project.go                         (existing) — extended to recognize new event kinds (StepStarted/Completed/Failed) in the in-memory projection

internal/store/
  evidence.go                        Evidence struct, PutEvidence, GetEvidence
  evidence_test.go                   Round-trip, FK constraint, ID uniqueness
  migrations.go                      Append evidence table to schemaSQL
  events.go                          New cases for StepStarted / StepCompleted / StepFailed (decode-and-validate; no projection write)
  events_test.go                     New tests for the three event kinds

internal/api/
  evidence.go                        GET /api/runs/{id}/evidence/{eid} handler
  evidence_test.go                   Happy, 404 unknown eid, 403 eid-belongs-to-different-run
  api.go                             Register evidence route; Router grows engine param
  runs.go                            createRunHandler tail-calls engine.Kick after successful 201 path

cmd/qac/serve.go                # EXTEND
                                     Construct AlertClient + Probe registry + Engine; pass to server.New

templates/firmware-release-canary.yaml   # MODIFY
                                     Add alert_probe as first step (before baseline_pre); bump version to 3
```

## 5. Core types

### 5.1 probe.Probe

```go
package probe

type Category string
const (
    CategoryGatherer  Category = "gatherer"
    CategoryAssertion Category = "assertion"
    CategoryAction    Category = "action"
)

type Output struct {
    StructuredData json.RawMessage  // probe-shaped JSON output
    Sources        []string          // provenance, e.g., ["alertmanager-stub"]
    ProbedAt       int64             // unix seconds
}

type Clients struct {
    AlertClient       alertclient.Client
    InventoryResolver inventoryclient.Resolver  // already from iter-4a
    // iter-4c+: MetricsClient, FLCCClient, RLCCClient, ...
}

type StepContext struct {
    RunID  string
    StepID string
    Racks  []string         // from run.inputs.canary_racks
    Inputs map[string]any   // operator inputs (bundle_tag, etc.)
    Config map[string]any   // step's config block from the template
}

type Probe interface {
    Type() string                  // "alert_probe"
    Category() Category            // CategoryGatherer for AlertProbe
    Run(ctx context.Context, clients Clients, sc StepContext) (Output, error)
}

type Registry struct{ /* map[string]Probe */ }
func NewRegistry() *Registry
func (r *Registry) Register(p Probe)
func (r *Registry) Get(typeName string) (Probe, bool)
```

### 5.2 alertclient

```go
package alertclient

type Alert struct {
    ID       string   `json:"id"`
    Name     string   `json:"name"`
    Severity string   `json:"severity"`   // "critical" | "warning" | "info"
    FiredAt  int64    `json:"fired_at"`   // unix seconds
    Summary  string   `json:"summary"`
    Scope    string   `json:"scope"`      // rack id
}

type Client interface {
    ListByRack(ctx context.Context, rackID string) ([]Alert, error)
}

var ErrSourceUnavailable = errors.New("alertclient: source unavailable")

// MapAlertClient is the deterministic in-memory backend for iter-4b.
type MapAlertClient struct{ /* table map[string][]Alert */ }
func NewMapAlertClient(table map[string][]Alert) *MapAlertClient
func (m *MapAlertClient) ListByRack(ctx, rackID) ([]Alert, error)

// SeedDemoAlerts returns deterministic fixtures covering the rack ids
// in seed_demo.go and firmware-release-canary.yaml. Coverage enforced by
// fixture_coverage_test.go.
func SeedDemoAlerts() map[string][]Alert
```

Fixture coverage (4+ racks from iter-4a's `inventoryclient.SeedDemoFixtures`):
- `dh3-r012-us-east-01a`: empty list (clean rack — primary happy-path)
- `dh3-r012-us-east-02a`: one `warning` alert (`GPUTempHigh`)
- `dh4-r020-us-west-01b`: empty list
- `dh4-r020-us-west-02b`: one `critical` alert (`NVLinkLinkDown`)
- `dh9-r999-ambig-test-01a`: empty list (parallel to iter-4a's ambiguous inventory fixture, so AlertClient-only unit tests can exercise the same id without crossing into discovery)

### 5.3 alertprobe

```go
package alertprobe

type AlertProbe struct{}
func New() *AlertProbe

func (*AlertProbe) Type() string             { return "alert_probe" }
func (*AlertProbe) Category() probe.Category { return probe.CategoryGatherer }

func (*AlertProbe) Run(ctx context.Context, clients probe.Clients, sc probe.StepContext) (probe.Output, error) {
    // Per-rack ListByRack → roll up into structured output
    // Returns probe.Output with StructuredData = JSON-encoded:
    //   {
    //     "probed_at": <unix>,
    //     "source": "alertmanager-stub",      // single source identifier
    //     "per_rack": {
    //       "<rack_id>": {
    //         "alerts": [{id, name, severity, fired_at, summary}],
    //         "max_severity": "warning" | "critical" | "info" | "",
    //         "count_by_severity": {"critical": 0, "warning": 1, "info": 0}
    //       }
    //     },
    //     "max_severity": "warning",          // worst across all racks
    //     "alerts_clear": false                // true iff every per_rack has empty alerts
    //   }
    //
    // Bubbles up clients.AlertClient.ListByRack errors.
}
```

### 5.4 engine

```go
package engine

type Engine struct {
    store    *store.Store
    probes   *probe.Registry
    clients  probe.Clients

    mu       sync.Mutex
    inflight map[string]chan struct{}   // runID → done channel; registered in Kick before goroutine spawns

    wg       sync.WaitGroup             // tracks in-flight goroutines for Shutdown
    sem      chan struct{}              // cap = 8 concurrent runs
}

func New(s *store.Store, probes *probe.Registry, clients probe.Clients) *Engine

// Kick is fire-and-forget. Registers the done channel synchronously
// before spawning the goroutine, so Wait(runID) immediately after Kick
// is race-safe. Idempotent: a second Kick for an already-inflight run
// is a no-op.
func (e *Engine) Kick(ctx context.Context, runID string)

// Wait returns a channel that closes when the goroutine for runID
// finishes. Returns a pre-closed channel if runID is not in-flight.
func (e *Engine) Wait(runID string) <-chan struct{}

// Shutdown blocks on wg.Wait() with ctx as the deadline.
func (e *Engine) Shutdown(ctx context.Context) error
```

### 5.5 store.Evidence

```go
package store

type Evidence struct {
    ID          string   // ULID
    RunID       string
    StepID      string
    ContentType string   // "application/json" for iter-4b
    Payload     []byte
    CreatedAt   int64
}

func (s *Store) PutEvidence(ctx context.Context, e Evidence) error
func (s *Store) GetEvidence(ctx context.Context, id string) (Evidence, error)
```

DB schema (idempotent migration appended to `schemaSQL`):

```sql
CREATE TABLE IF NOT EXISTS evidence (
  id           TEXT PRIMARY KEY,
  run_id       TEXT NOT NULL REFERENCES runs(id),
  step_id      TEXT NOT NULL,
  content_type TEXT NOT NULL,
  payload      BLOB NOT NULL,
  created_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_evidence_run ON evidence(run_id);
```

## 6. Event kinds (iter-4b additions)

| Kind | Payload | Projection effect |
|---|---|---|
| `StepStarted` | `{step_id, step_type, racks, started_at}` | None (no projection table for steps in iter-4b) |
| `StepCompleted` | `{step_id, evidence_id, duration_ms, completed_at, sources}` | None |
| `StepFailed` | `{step_id, error, duration_ms, failed_at}` | None |

The store-level `AppendEvent` switch grows three new cases. Each decodes the payload to validate shape, then falls through to the standard tail INSERT (no DB-side projection beyond storing the event itself). The in-memory `engine.Project()` is also extended to recognize these kinds so a future `GET /api/runs/{id}/state` can return a derived view.

## 7. HTTP surface

| Method | Path | Behavior |
|---|---|---|
| `POST /api/runs` | (unchanged from iter-4a) | After successful 201 path (RunCreated + facts emitted), tail-calls `engine.Kick(ctx, runID)`. 422 on inventory_unresolved skips the Kick. |
| `GET /api/runs/{id}` | (unchanged) | Returns run + events. New event kinds appear in the events list. |
| `GET /api/runs/{id}/facts` | (unchanged from iter-4a) | — |
| `GET /api/runs/{id}/evidence/{eid}` | **NEW** | Returns evidence body. 404 if eid unknown, 403 if eid's run_id doesn't match path id. Content-Type set from evidence row (always `application/json` for iter-4b). |

## 8. Data flow (happy path)

```
1. POST /api/runs
2. iter-4a path: RunCreated → facts.Emit(operator) → ResolveRack per rack → FactsDiscovered events
3. (NEW) createRunHandler tail-calls engine.Kick(ctx, runID); returns 201
4. ─── async, in goroutine ───
5. Engine loads template (cached by id+version)
6. Engine finds next runnable step (first kind:probe step whose StepStarted has not been emitted)
7. Engine looks up probe type in registry; if missing → emit StepFailed, return
8. Engine builds StepContext{RunID, StepID, Racks=inputs.canary_racks, Inputs=run.inputs, Config=step.config}
9. Engine emits StepStarted{step_id, step_type, racks, started_at}
10. probe.Run(ctx, clients, sc):
    a. alertprobe iterates sc.Racks
    b. Calls clients.AlertClient.ListByRack(ctx, rack) per rack
    c. Builds per_rack rollup + max_severity + alerts_clear
    d. Returns Output{StructuredData, Sources=["alertmanager-stub"], ProbedAt}
11. Engine writes evidence row (ULID id, content_type=application/json, payload=output.StructuredData)
12. Engine emits StepCompleted{step_id, evidence_id, duration_ms, completed_at, sources}
13. Engine goroutine returns; done channel closes; Wait callers unblock
14. Operator polls GET /api/runs/{id} → sees StepStarted + StepCompleted events
15. Operator follows: GET /api/runs/{id}/evidence/{eid} → reads alert state JSON
```

## 9. Error paths

| Error class | Detection point | Event emitted | Run status |
|---|---|---|---|
| Probe.Run returns err | engine runner | `StepFailed{step_id, error, duration_ms, failed_at}` | unchanged (`pending`) |
| AlertClient transport err (`ErrSourceUnavailable`) | inside probe.Run | bubbles up → StepFailed | unchanged |
| Probe type not in registry | engine runner pre-Run | StepFailed with `error: "probe type 'X' not registered"` | unchanged |
| Template lookup fails | engine runner pre-Run | StepFailed | unchanged |
| Evidence PutEvidence fails post-Run | engine runner | StepFailed with `error: "persist evidence: ..."` | unchanged |
| AppendEvent for StepFailed itself fails | engine runner | logged via `slog.Error`; goroutine returns | unchanged (stuck-run; operator-visible via missing terminal event) |
| Context canceled mid-probe | inside probe.Run | StepFailed with `error: "context canceled"` | unchanged |
| Goroutine panic | top-level `defer recover()` | StepFailed with `error: "panic: <msg>"` | unchanged |

**Gatherer semantics rule:** "Found N critical alerts" is **not** a probe error — it's a successful run that produced evidence. The probe returns `nil`; StepCompleted fires; alerts_clear=false is recorded as data, not a failure.

## 10. Template change

`templates/firmware-release-canary.yaml` bumps `version: 2 → 3` and gains a new first step:

```yaml
steps:
  - id: alert_baseline
    kind: probe
    probe_type: alert_probe
    title: "Pre-zap alert baseline"
    racks: "{{ inputs.canary_racks }}"
    timeout: 30s              # read but not enforced in iter-4b
  # ... existing baseline_pre, zap_canary, etc. follow ...
```

**Note on schema shift:** the existing template steps use `action.tool` + `action.args` (tool-based dispatch). The new step uses `probe_type` (probe-based dispatch — the user's reframe). Both shapes coexist in the template for iter-4b — only `probe_type`-shaped steps are runnable; tool-based steps stay as YAML for future migration. iter-4c will migrate the remaining steps to `probe_type` as their probes ship.

## 11. Wiring (cmd/qac/serve.go)

```go
inventoryResolver := inventoryclient.NewMapResolver(inventoryclient.SeedDemoFixtures())
alertC := alertclient.NewMapAlertClient(alertclient.SeedDemoAlerts())
clients := probe.Clients{AlertClient: alertC, InventoryResolver: inventoryResolver}
probes := probe.NewRegistry()
probes.Register(alertprobe.New())
eng := engine.New(s, probes, clients)
srv := server.New(s, addr, dbPath, distFS, inventoryResolver, eng)
// signal handler calls eng.Shutdown(ctx) before exiting
```

## 12. Testing strategy

### 12.1 Unit tests per package

| Package | Tests |
|---|---|
| `internal/probe` | Registry register/get, Category constants, Output JSON round-trip |
| `internal/alertclient` | MapAlertClient happy lookup, empty-list lookup, ErrSourceUnavailable bubbles, SeedDemoAlerts fixture coverage |
| `internal/probe/alertprobe` | Multi-rack happy path, no-alerts path, ErrSourceUnavailable from injected failing client, output schema (per_rack + max_severity + alerts_clear) |
| `internal/store/evidence` | PutEvidence + GetEvidence round-trip, idempotent migration, foreign-key relationship to `runs(id)` matches the iter-4a pattern used by `facts(run_id)` |
| `internal/store/events` | New cases decode payload without projection writes |
| `internal/engine` | Runner happy path (mocked probe via interface), runner errors (probe returns err → StepFailed; probe panics → StepFailed via recover), Wait semantics (returns pre-closed for unknown runID; closes on goroutine completion), Shutdown waits for in-flight |

### 12.2 Integration tests

| Test | Path |
|---|---|
| `TestEndToEnd_AlertProbe_HappyPath` | POST iter-4a happy → wait on engine.Wait(runID) → assert StepStarted + StepCompleted in events → GET evidence returns expected per_rack JSON |
| `TestEndToEnd_AlertProbe_ProbeError` | newTestServer injects a failing AlertClient → POST → wait → assert StepFailed event; no evidence row |
| `TestEndToEnd_InventoryFailureSkipsKick` | POST with unresolved rack → 422 → no StepStarted event landed (engine.Kick not called on the iter-4a 422 path) |
| `TestEngine_GracefulShutdown_FinishesInflight` | Slow mock probe → call Shutdown(ctx, 5s) → assert no goroutines pending |
| `TestEngine_KickIdempotent` | Kick called twice for same runID → only one goroutine spawned |
| `TestCanonicalCanaryTemplate_AlertBaselineRunsEndToEnd` | Real on-disk firmware-release-canary.yaml v3 → POST → wait → assert evidence body has `alerts_clear` field |

### 12.3 The wait idiom

```go
runID := postRun(t, srv)
select {
case <-engine.Wait(runID):
    // goroutine done; assert on events / facts / evidence
case <-time.After(5 * time.Second):
    t.Fatal("engine did not finish run")
}
```

`newTestServer` extends to expose the engine so tests can call Wait. No polling loops. iter-4c's SSE work plugs into the same primitive.

### 12.4 Frontend

No new frontend code. Existing run-detail page picks up the new event kinds via its existing event list. Evidence link is a manual `curl` for iter-4b — iter-4c adds the step rail and evidence renderer.

## 13. Acceptance criteria

| # | Check |
|---|---|
| A1 | `make build` clean |
| A2 | `make lint` clean (go vet + yarn lint) |
| A3 | `make test` green; no regressions in iter-4a's existing test count |
| A4 | Fresh-DB boot creates `evidence` table; iter-3a/4a DBs upgrade cleanly (idempotent migration) |
| A5 | POST happy path (iter-4a behavior preserved) + StepStarted + StepCompleted events land within 2s |
| A6 | `GET /api/runs/{id}/evidence/{eid}` returns 200 with JSON body matching the alert_probe schema |
| A7 | 404 on unknown evidence id; 403 if eid's run_id doesn't match path id |
| A8 | POST with unresolved rack → 422 (iter-4a behavior) + zero step events for that run |
| A9 | Inject failing AlertClient → POST → StepFailed event lands; no evidence row |
| A10 | Engine.Shutdown(ctx) waits for in-flight goroutines and returns nil on clean shutdown |
| A11 | Engine.Wait(unknownRunID) returns a pre-closed channel |
| A12 | Canonical-integ: real on-disk firmware-release-canary.yaml v3 drives the loop end-to-end |
| A13 | Template `version: 3` reported by `qac template load` |
| A14 | iter-4a's existing tests still pass (no regressions in facts / discovery / inventory) |

## 14. Risks + open questions

- **Goroutine leak on stuck-run path:** if AppendEvent for StepFailed itself fails (DB down), the goroutine logs and exits but the in-flight registration in `inflight` map gets cleared (defer in Kick closes the done channel regardless). No leak. Acceptable.
- **Template schema split:** `probe_type`-shaped steps coexist with `action.tool`-shaped steps in the same YAML during iter-4b. Engine's "find next runnable step" must skip tool-based steps for now. Migration of remaining steps is iter-4c's first task as their probes ship.
- **Race between Kick and Wait:** mitigated by Kick registering the done channel synchronously before goroutine spawns. Documented in the Engine type's API contract.
- **Concurrent Kicks for same runID:** Kick checks `inflight[runID]` under mu; second call is a no-op. Tests cover.
- **What about the operator wanting to re-run a step?** Out of scope for iter-4b. iter-4c adds an explicit `POST /api/runs/{id}/advance` (or similar) for retry.

## 15. Forward compatibility hooks

- The `Category` field on Probe lets the engine branch on iter-4c's first assertion probe (verdict comes from checkpoint expression evaluator).
- The `Clients` struct grows new fields as new clients are added (MetricsClient, FLCCClient, etc.) — additive.
- `Engine.Wait` is the SSE primitive — iter-4c's `/api/runs/{id}/events/stream` subscribes per-run instead of polling.
- The split between gatherer/assertion/action probes makes the eventual ai_assess step's job simpler: it reads gatherer evidence, makes a judgment, emits a verdict event.

## 16. Non-goals

- Multi-step orchestration (engine stops after first step in iter-4b)
- Real Alertmanager/awxctl integration (stubbed only)
- Checkpoint expression evaluator (iter-4c, when first assertion probe lands)
- Probe timeout enforcement (iter-4c)
- Frontend changes (iter-4c)
- SSE event streaming (iter-4c)
- Run-resume after crash (iter-4c)
- Step retry endpoint (iter-4c)
- Per-node probes (probe model supports node:<id> scope via iter-4a's facts; no node-level probe in iter-4b)
