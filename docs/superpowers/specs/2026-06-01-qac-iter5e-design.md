# qac iter-5e — rlcc_action_probe as a standalone probe library

**Status:** draft
**Branch:** `wp/add-qac` (continues iter-5d)
**Predecessor:** [iter-5d spec](2026-06-01-qac-iter5d-design.md), iter-5d HEAD at `98f828b`
**PR posture:** still local. iter-5e builds the first probe that consumes the iter-5d workflow snapshot.

## 1. Purpose

iter-5d landed the workflow snapshot in `RunCreated.payload.rlcc_workflow`. The wizard sends it; the handler resolves and pins it; the engine projector decodes the typed shape. **Nobody reads it yet.**

iter-5e ships `rlcc_action_probe` — a standalone probe library that:

- Reads `RLCCWorkflow.Actions[]` from the run's `RunState`
- Picks the matching action by handler kind (e.g., `l11-fielddiag`)
- Walks the action **per compute tray (CT)**, filtered against `BMNIgnored` events
- Polls Prometheus at a configurable cadence (default 60s) to check per-CT completion
- Re-detects RLCC ignores at every tick — newly-ignored CTs emit `BMNIgnored(source=rlcc)` and drop out of polling
- Writes one Evidence row **per CT** (uses iter-5b's `evidence.deviceslot` nullable column)
- Returns one summary Output for the engine's auto-evidence (per-step aggregate)

**Scoped framing:** probes are libraries; wiring them to steps is template configuration. iter-5e ships the library. **No canary template changes.** No step uses the probe yet. Wiring is a future, decoupled concern.

After iter-5e:
- `rlccactionprobe.New()` can be registered in `probe.Registry` (cmd/qac/serve.go + tests)
- The probe is fully unit-tested + integration-tested against `MapClient` lifecycle backends
- The probe is **not** referenced by any canary template step; wiring is a one-line YAML edit deferred to a future iter
- iter-5f+ adds more probes (metrics_baseline_probe, flcc_node_zap_probe, ai_summary_probe) using the same shape

## 2. Scope decisions

| Decision | Value | Why |
|---|---|---|
| Probe model | **Single-shot `Run` that blocks until all CTs complete or step ctx times out** | Existing `Probe.Run` returns one `Output` per call. iter-5e fits inside this — internal polling loop with ticker + `select` on ctx. The engine treats the whole walk as one StepStarted → StepCompleted. |
| Per-CT evidence | **Probe writes evidence directly via new `EvidenceWriter` port in `probe.Clients`** | `Probe.Run` returns ONE `Output`. iter-5e needs N evidence rows (one per CT). Extending Clients with a narrow EvidenceWriter interface keeps the probe abstraction simple and avoids returning `[]Output` (which would be a breaking change for every existing probe). |
| Aggregate evidence | **Engine still writes one rack-level evidence row from the probe's returned `Output`** | Sources/per-step summary lands in the existing flow. The per-CT rows are additional, written by the probe itself. Operator gets both views: `GET /api/runs/{id}/evidence/{eid}` returns the aggregate; iter-5f's RunDetail rewrite will list per-CT rows separately. |
| Workflow snapshot path | **Engine reads `RLCCWorkflow` from `RunState` (projected from `RunCreated`), passes it to probes via new `StepContext.RLCCWorkflow` field** | Probe doesn't query the store — engine already has the snapshot in state. Same pattern as Inputs (already in StepContext). |
| Per-step config | **`StepContext.Config` populated from the template step's `action.args` block** | Today Config is always nil. iter-5e teaches the runner to extract per-step config from the parsed template. Each probe defines its own config keys. |
| `rlcc_action_probe` config keys | **`match_handler`, `target_state`, `poll_interval`** (string, string, duration) | Minimum surface for matching the action + driving the polling loop. Defaults: poll_interval=60s. |
| Action selection | **First action whose `Handler` matches `config.match_handler`** | Multiple matches: take the first; log a warning. Workflows rarely have two identical handlers, but the contract is deterministic. |
| Per-tick PromQL backend | **New `lifecycleclient.QueryActionState(ctx, deviceslots []string, targetState string) (map[string]bool, error)`** | Returns per-CT completion. MapClient backend simulates a 3-tick walk for deterministic tests; PromClient calls Prometheus with `rlcc_state_completed{rack="...",node="...",state="..."} == 1`. |
| Per-tick RLCC re-detect | **Reuse iter-5b's `lifecycleclient.QueryRLCCIgnored` at every tick** | Newly-ignored CTs emit `BMNIgnored(source=rlcc)` via the EvidenceWriter (or a separate event-emitter port — see §3). Newly-ignored CTs drop out of the polling pool. |
| Verdict semantics | **Probe leaves `Output.Verdict` empty** (gatherer-style for iter-5e) | iter-5e doesn't ship assertion logic. The per-CT evidence rows carry per-CT outcomes; a future iter computes a rack verdict from them. Matches iter-4b's gatherer-only posture. |
| Failure modes | **Return error from `Run` ⇒ `StepFailed` event** for: workflow snapshot missing, no matching action, lifecycleclient unrecoverable error. Per-CT failures are recorded as evidence; don't fail the step. | Step-level failures are infrastructural. Per-CT failures are data the operator needs to see, not engine errors. |
| Probe→engine emission | **Add `EventEmitter` port to `probe.Clients`** — narrow interface with `EmitBMNIgnored(ctx, runID, payload)` | The probe needs to write to the event log when it detects mid-run RLCC ignores. Going through `store.AppendEvent` directly would couple the probe to store internals; a narrow port keeps the dependency surface explicit. |
| MapClient fallback | **Same posture as iter-5b**: `MapClient` when `AWXCTL_VMAUTH_USERNAME/PASSWORD` unset, real `PromClient` when set | `cmd/qac/serve.go` selects backend at startup. Tests always use MapClient. |
| Canary template changes | **None** | iter-5e ships the probe library. Wiring is template config, deferred. |
| Probe instantiation in serve.go + tests | **`reg.Register(rlccactionprobe.New())`** added next to alertprobe/factprobe registration | Probe is available for any future step that declares `probe_type: rlcc_action_probe`. |
| Probe.Clients grow | **Adds 3 ports: `LifecycleClient`, `EvidenceWriter`, `EventEmitter`** | All three pre-existed in other forms (iter-5b has lifecycleclient; iter-5b's store has PutEvidence/AppendEvent). iter-5e just adds the narrow probe-facing interfaces. |

## 3. Architecture

The probe is a self-contained library in `internal/probe/rlccactionprobe/`. The engine wires it via the existing probe Registry; the runner passes a populated StepContext including the workflow snapshot and per-step config.

```
                                         ┌──────────────────────────────────┐
                                         │ internal/probe/rlccactionprobe/  │
                                         │                                  │
RunCreated.payload.rlcc_workflow         │  Probe.Run(ctx, clients, sc):    │
       │                                 │    1. Pick matching action       │
       ▼                                 │    2. List CTs (BMNDiscovered    │
engine.Project ─→ RunState.RLCCWorkflow  │       minus BMNIgnored)          │
       │                                 │    3. ticker := time.NewTicker   │
       │ (StepContext.RLCCWorkflow)      │       (poll_interval)            │
       ▼                                 │    4. for each tick:             │
engine.runner ──→ probe.Run ─────────────┤      a) QueryActionState         │
       ▲                                 │      b) QueryRLCCIgnored         │
       │                                 │      c) EmitBMNIgnored(new)      │
       │                                 │      d) WriteEvidence(per-CT)    │
       │                                 │    5. terminate when all CTs     │
       │                                 │       complete OR ctx.Done       │
       │                                 │    6. return Output (aggregate)  │
       │                                 │                                  │
       └─── Output (aggregate evidence) ─┴──────────────────────────────────┘
```

### Layer-by-layer change surface

| Layer | iter-5e change | Magnitude |
|---|---|---|
| `internal/probe/rlccactionprobe/probe.go` | **CREATE** — new package. Implements `probe.Probe` interface: `Type() string`, `Category() probe.Category`, `Run(ctx, clients, sc)`. | ~250 lines |
| `internal/probe/rlccactionprobe/probe_test.go` | **CREATE** — unit + integration tests with mock LifecycleClient + mock EvidenceWriter + mock EventEmitter. | ~300 lines |
| `internal/probe/probe.go` | Grow `Clients` with three new ports; grow `StepContext` with `RLCCWorkflow *rlccclient.WorkflowSnapshot` field. | ~30 lines |
| `internal/lifecycleclient/client.go` | Grow `Client` interface with `QueryActionState(ctx, deviceslots, targetState) (map[string]bool, error)`. | ~5 lines |
| `internal/lifecycleclient/mapclient.go` | Implement `QueryActionState` — seeded per-CT state map; supports per-test "advance the simulated walk" hooks. | ~50 lines |
| `internal/lifecycleclient/promclient.go` | Implement `QueryActionState` — per-CT PromQL fanout. Reuses existing auth + transport. | ~80 lines |
| `internal/lifecycleclient/queries/rlcc_action_state.promql` | **CREATE** — embedded template `rlcc_state_completed{rack="{rack}",node="{node}",state="{state}"} == 1`. | ~5 lines |
| `internal/engine/project.go` | Project `RLCCWorkflow` from `RunCreated` payload into `RunState`. | ~3 lines |
| `internal/engine/state.go` | Add `RLCCWorkflow *rlccclient.WorkflowSnapshot` field to `RunState`. | ~3 lines |
| `internal/engine/runner.go` | Populate `StepContext.RLCCWorkflow` from state; extract `StepContext.Config` from template step's `action.args` block. | ~10 lines |
| `internal/engine/runner.go` | Construct `probe.Clients` with all three new ports wired to the engine's store + lifecycleclient. | ~5 lines |
| `cmd/qac/serve.go` | Register `rlccactionprobe.New()` in the engine's probe Registry. | ~2 lines |
| `internal/api/api_test.go` | `newTestServer` registers `rlccactionprobe.New()` alongside alertprobe/factprobe. | ~2 lines |
| `internal/engine/state_test.go` (or project_test.go) | Add test: replaying a RunCreated event with `rlcc_workflow` populates `RunState.RLCCWorkflow`. | ~30 lines |

**No canary template changes.** No `firmware-release-canary.yaml` edits. No version bump.

## 4. Probe public API

### Type registration

```go
package rlccactionprobe

func New() probe.Probe { return &probeImpl{} }

func (p *probeImpl) Type() string         { return "rlcc_action_probe" }
func (p *probeImpl) Category() probe.Category { return probe.CategoryGatherer }
```

### Config schema (declared in the probe's doc comments)

The probe reads `sc.Config` which is the parsed `action.args` map from the template step. Recognized keys:

```yaml
# template step block — example for a future l11_fielddiag step:
- id: l11_fielddiag
  kind: probe
  probe_type: rlcc_action_probe
  action:
    args:
      match_handler: l11-fielddiag    # required: matches against rlcc_workflow.actions[].handler
      target_state: l11-fielddiag     # required: PromQL state= label
      poll_interval: 60s              # optional, default 60s
    timeout: 4h                       # step-level timeout, enforced by engine ctx
```

- `match_handler` (required, string): the `handler` kind to match in `RLCCWorkflow.Actions[]`
- `target_state` (required, string): the PromQL `state=` label for `rlcc_state_completed`
- `poll_interval` (optional, duration string parseable by `time.ParseDuration`): default 60s

Missing required keys → `Run` returns error → `StepFailed`.

### Run behavior

```go
func (p *probeImpl) Run(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error)
```

1. **Validate prerequisites:**
   - `sc.RLCCWorkflow != nil` (otherwise: error "no workflow snapshot on run")
   - `sc.Config["match_handler"]` and `sc.Config["target_state"]` present and non-empty
   - `c.LifecycleClient != nil`, `c.EvidenceWriter != nil`, `c.EventEmitter != nil`

2. **Find action:**
   - Iterate `sc.RLCCWorkflow.Actions` for first match of `Handler == match_handler`
   - No match → error "no matching action in workflow snapshot for handler=%q"

3. **Build CT pool:**
   - For each rack in `sc.Racks`: query `c.InventoryResolver.ResolveBMNs(ctx, rack)` → []BMN
   - Subtract operator-ignored deviceslots (looked up from the run's event log via a Clients port — see §5)
   - Subtract initially-ignored RLCC deviceslots (initial query of `c.LifecycleClient.QueryRLCCIgnored`)
   - Remaining BMNs form the active pool

4. **Poll loop:**
   ```go
   ticker := time.NewTicker(pollInterval)
   defer ticker.Stop()
   active := initialActivePool

   for len(active) > 0 {
       select {
       case <-ctx.Done():
           return p.summarize(activeAtStart, active, completed, ignoredMidRun, ctx.Err())
       case <-ticker.C:
           // a) Query action state for active CTs
           states, _ := c.LifecycleClient.QueryActionState(ctx, deviceslotsOf(active), target_state)
           // b) Move completed CTs from active → completed; write per-CT evidence
           // c) Query RLCC ignores for active rack(s)
           rlccIgnoredNow, _ := c.LifecycleClient.QueryRLCCIgnored(ctx, rackKey)
           // d) For each newly-ignored CT: EmitBMNIgnored + drop from active + record in ignoredMidRun
       }
   }
   return p.summarize(activeAtStart, completed, ignoredMidRun, nil), nil
   ```

5. **Return:** one `probe.Output` with aggregate `StructuredData` (counts of completed/ignored/timed-out CTs, sources, target_state, action_name, duration_ticks). `Sources` includes "lifecycleclient" or "lifecycleclient-fixture" depending on backend.

## 5. New ports in `probe.Clients`

```go
// LifecycleClient is the read-only interface the probe uses to poll
// per-CT state and re-query RLCC ignores at each tick.
type LifecycleClient interface {
    QueryActionState(ctx context.Context, deviceslots []string, targetState string) (map[string]bool, error)
    QueryRLCCIgnored(ctx context.Context, rk lifecycleclient.RackKey) ([]lifecycleclient.IgnoredBMN, error)
}

// EvidenceWriter is a narrow port for writing per-CT evidence rows
// during a probe run. The engine still writes the aggregate evidence
// from the probe's returned Output; this is for the per-CT rows that
// use iter-5b's evidence.deviceslot column.
type EvidenceWriter interface {
    PutEvidence(ctx context.Context, ev store.Evidence) error
}

// EventEmitter is a narrow port for emitting events from inside a probe
// (e.g., BMNIgnored when the probe detects a CT becoming RLCC-ignored
// mid-run). Bigger than just AppendEvent so we don't expose the full
// store API to probes.
type EventEmitter interface {
    EmitBMNIgnored(ctx context.Context, runID string, payload BMNIgnoredPayload) error
}

// BMNIgnoredPayload mirrors engine.BMNIgnoredPayload for cross-package
// access; same JSON tags.
type BMNIgnoredPayload struct {
    Rack       string `json:"rack"`
    Deviceslot string `json:"deviceslot"`
    Source     string `json:"source"`        // "rlcc" when emitted by this probe
    Reason     string `json:"reason,omitempty"`
    MarkedAt   int64  `json:"marked_at"`
}
```

These interfaces live in `internal/probe/probe.go`. The engine's runner constructs concrete implementations from `*store.Store` and `lifecycleclient.Client` and stuffs them into `probe.Clients`:

```go
// in engine.New() or wherever Clients is built
clients := probe.Clients{
    AlertClient:       alertC,
    InventoryResolver: invR,
    LifecycleClient:   lifeC,                       // satisfies probe.LifecycleClient
    EvidenceWriter:    s,                            // *store.Store satisfies it
    EventEmitter:      probe.NewStoreEmitter(s),    // small adapter
}
```

The `StoreEmitter` adapter (in `probe/probe.go`):

```go
type StoreEmitter struct { s *store.Store }
func NewStoreEmitter(s *store.Store) *StoreEmitter { return &StoreEmitter{s: s} }
func (e *StoreEmitter) EmitBMNIgnored(ctx context.Context, runID string, p BMNIgnoredPayload) error {
    payload, _ := json.Marshal(p)
    return e.s.AppendEvent(ctx, runID, "BMNIgnored", payload)
}
```

## 6. Engine extensions

### `RunState` and `Project`

```go
// internal/engine/state.go (or wherever RunState lives — it's in project.go today)
type RunState struct {
    ID         string
    TemplateID string
    Rack       string
    CreatedBy  string
    Status     string
    CreatedAt  time.Time
    UpdatedAt  time.Time
    StepStates map[string]StepState
    // iter-5e: workflow snapshot projected from RunCreated.payload.rlcc_workflow.
    // Nil for pre-iter-5d runs; populated for iter-5d+ runs. iter-5e probes
    // read this via StepContext.RLCCWorkflow.
    RLCCWorkflow *rlccclient.WorkflowSnapshot
}
```

`Project` (in `internal/engine/project.go`) populates the field from `RunCreated.payload`:

```go
case KindRunCreated:
    var p RunCreatedPayload
    _ = json.Unmarshal(e.Payload, &p)
    st.TemplateID = p.TemplateID
    st.Rack = p.Rack
    st.CreatedBy = p.CreatedBy
    st.CreatedAt = e.OccurredAt
    st.Status = StatusPending
    st.RLCCWorkflow = p.RLCCWorkflow   // ← iter-5e
```

### `StepContext` and runner

```go
// internal/probe/probe.go
type StepContext struct {
    RunID        string
    StepID       string
    Racks        []string
    Inputs       map[string]any
    Config       map[string]any
    // iter-5e:
    RLCCWorkflow *rlccclient.WorkflowSnapshot
}
```

`internal/engine/runner.go` constructs SC from the state + the step's parsed `action.args`:

```go
// Pseudocode — actual code reads template step shape from store
stepConfig := step.Action.Args  // map[string]any from the template YAML
sc := probe.StepContext{
    RunID:        runID,
    StepID:       step.ID,
    Racks:        racks,
    Inputs:       inputs,
    Config:       stepConfig,         // ← iter-5e: was nil
    RLCCWorkflow: state.RLCCWorkflow, // ← iter-5e
}
```

## 7. Lifecycle client extension

### Interface

```go
// internal/lifecycleclient/client.go — add to Client interface
type Client interface {
    QueryRLCCIgnored(ctx context.Context, rk RackKey) ([]IgnoredBMN, error)
    QueryActionState(ctx context.Context, deviceslots []string, targetState string) (map[string]bool, error) // iter-5e
}
```

### MapClient implementation

```go
// internal/lifecycleclient/mapclient.go
type MapClient struct {
    rlccIgnored      map[RackKey][]IgnoredBMN
    // iter-5e:
    actionState      map[string]map[string]bool          // {targetState: {deviceslot: completed}}
    actionTicks      map[string]int                       // {targetState: ticks before completion}
    mu               sync.Mutex
    tickCount        map[string]int                       // {targetState: current tick count}
}

func (c *MapClient) QueryActionState(_ context.Context, deviceslots []string, targetState string) (map[string]bool, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.tickCount[targetState]++
    out := make(map[string]bool, len(deviceslots))
    completeAfter := c.actionTicks[targetState]
    if completeAfter == 0 {
        completeAfter = 3 // default: 3 ticks to completion in fixtures
    }
    for _, ds := range deviceslots {
        // Default: completed once we've ticked >= completeAfter times.
        out[ds] = c.tickCount[targetState] >= completeAfter
    }
    return out, nil
}
```

This gives deterministic test behavior: a probe walks for 3 ticks then sees all CTs complete. Per-deviceslot overrides via `actionState` allow tests to simulate partial completion.

### PromClient implementation

```go
//go:embed queries/rlcc_action_state.promql
var rlccActionStateTemplate string

func (c *PromClient) QueryActionState(ctx context.Context, deviceslots []string, targetState string) (map[string]bool, error) {
    out := make(map[string]bool, len(deviceslots))
    for _, ds := range deviceslots {
        // Parse deviceslot into rack + node parts. Format:
        // dh3-r012-node-07-us-east-01a → rack=dh3-r012-us-east-01a, node=node-07
        rack, node := parseDeviceslot(ds)
        q := substitute(rlccActionStateTemplate, map[string]string{
            "rack":  rack,
            "node":  node,
            "state": targetState,
        })
        v, err := c.queryInstant(ctx, q)
        if err != nil { return nil, err }
        out[ds] = len(v) > 0 && v[0] == 1.0  // metric present and =1
    }
    return out, nil
}
```

The embedded query:

```
# internal/lifecycleclient/queries/rlcc_action_state.promql
rlcc_state_completed{rack="{rack}",node="{node}",state="{state}"} == 1
```

(Real RLCC metric name may differ — this is the contract iter-5e ships against. Adjust later when wiring against production Prometheus is needed.)

## 8. Error handling

| Scenario | Behavior |
|---|---|
| `sc.RLCCWorkflow == nil` | Return error → engine emits `StepFailed` with reason "no workflow snapshot on run (legacy or non-iter-5d run)" |
| No action in workflow matches `config.match_handler` | Return error → `StepFailed` with reason `"no matching action: handler=%q"` |
| `config.match_handler` or `config.target_state` missing/empty | Return error → `StepFailed` with reason `"missing required config: match_handler / target_state"` |
| `config.poll_interval` is unparseable | Return error → `StepFailed` |
| `LifecycleClient.QueryActionState` returns error | Log + retry on next tick. Three consecutive failures → return error → `StepFailed` |
| `LifecycleClient.QueryRLCCIgnored` returns error | Log + retry on next tick (non-fatal — ignore re-detection is best-effort) |
| `EvidenceWriter.PutEvidence` returns error | Log + continue. Per-CT evidence is best-effort; aggregate evidence (returned via Output) is canonical |
| `EventEmitter.EmitBMNIgnored` returns error | Log + retry next tick. The CT stays in the active pool until the emit succeeds |
| Step ctx times out (`ctx.Done()` fires) | Return Output with `StructuredData.timed_out: true` and remaining CTs listed. Engine emits `StepCompleted` (no Verdict — caller decides). |
| Initial CT pool is empty (all racks have no BMNs or all are pre-ignored) | Return Output with `count_active: 0` and zero ticks. `StepCompleted`. |

## 9. Backward compatibility

- **Pre-iter-5d runs** (no `rlcc_workflow` snapshot in `RunCreated.payload`): `Project` leaves `RunState.RLCCWorkflow == nil`. Any future step that references `probe_type: rlcc_action_probe` will fail at Run time with "no workflow snapshot on run." Since iter-5e doesn't wire any step to the probe, no pre-iter-5d run is affected.
- **Pre-iter-5e runs created with iter-5d's `Probe.Clients`** (alertprobe, factprobe): no change. The new Clients fields are additive; existing probes ignore them.
- **`StepContext.Config` was nil**: existing probes (alertprobe, factprobe) don't read Config. Even after iter-5e populates it from `action.args`, those probes ignore it.

## 10. Testing

| Test | Lives in | What it asserts |
|---|---|---|
| Action selection — happy path | `rlccactionprobe/probe_test.go` | Probe finds the first action with matching `Handler`. Output's StructuredData includes `action_name`. |
| Action selection — no match | `rlccactionprobe/probe_test.go` | Probe returns error `"no matching action: handler=…"`. |
| Action selection — missing snapshot | `rlccactionprobe/probe_test.go` | `sc.RLCCWorkflow == nil` ⇒ error `"no workflow snapshot on run"`. |
| Config validation — missing match_handler | `rlccactionprobe/probe_test.go` | Returns error. |
| Config validation — missing target_state | `rlccactionprobe/probe_test.go` | Returns error. |
| Config validation — unparseable poll_interval | `rlccactionprobe/probe_test.go` | Returns error. |
| Walk — all CTs complete after 3 ticks | `rlccactionprobe/probe_test.go` | MapClient configured with `actionTicks[target_state] = 3`; probe runs for 3 ticks; 18 per-CT evidence rows written; Output has `count_completed = 18`. |
| Walk — partial completion at ctx timeout | `rlccactionprobe/probe_test.go` | ctx with 100ms deadline + 50ms ticker + MapClient with `actionTicks = 10`; probe times out at tick 2; Output has `timed_out = true` and `count_active > 0`. |
| Walk — RLCC ignore mid-run | `rlccactionprobe/probe_test.go` | MapClient's `QueryRLCCIgnored` returns 1 new ignored BMN on tick 2; probe emits `BMNIgnored(source=rlcc)`, drops the CT from active pool, completes other CTs normally. |
| Walk — operator-ignored CTs skipped at start | `rlccactionprobe/probe_test.go` | Pre-seeded BMNIgnored events for 4 deviceslots; active pool is 14, not 18. |
| Per-CT evidence shape | `rlccactionprobe/probe_test.go` | Each evidence row has `deviceslot` set, `structured_data` includes `target_state`, `completed_at_tick`, `ct_position`. |
| Engine projects RLCCWorkflow into RunState | `internal/engine/project_test.go` | After projecting a RunCreated event with payload `rlcc_workflow`, `RunState.RLCCWorkflow != nil` with matching Name/SourceSHA/Actions. |
| Runner populates StepContext.RLCCWorkflow | `internal/engine/runner_test.go` (extend if exists) | After a step runs against a registered no-op probe, the probe receives a non-nil `sc.RLCCWorkflow`. |
| LifecycleClient.MapClient.QueryActionState | `internal/lifecycleclient/mapclient_test.go` | Returns expected map; tick counter increments per call. |
| LifecycleClient.PromClient.QueryActionState | `internal/lifecycleclient/promclient_test.go` | httptest-backed; verify query is constructed correctly per CT; auth header sent. |
| Probe registered in serve.go | `cmd/qac/serve_test.go` (or smoke build) | Build succeeds. |
| Probe registered in newTestServer | `internal/api/api_test.go` | Existing tests still pass after registration; no name collision. |
| Backward compat — pre-iter-5d run still loads | `internal/api/runs_test.go` (extend) | A RunCreated event without `rlcc_workflow` projects to RunState with `RLCCWorkflow == nil`; engine doesn't crash. |

Run via `go test ./...` and `cd web && yarn test --run` (FE should be entirely unaffected; smoke check only).

## 11. Acceptance criteria

iter-5e ships when all of the following are true:

1. `rlccactionprobe.New()` returns a `probe.Probe` with `Type() == "rlcc_action_probe"`.
2. The probe walks a 3-tick MapClient fixture and writes 18 per-CT evidence rows + 1 aggregate Output.
3. The probe correctly drops mid-run RLCC-ignored CTs from the active pool and emits `BMNIgnored(source=rlcc)` events.
4. The probe correctly excludes operator-ignored CTs (from `BMNIgnored(source=operator)` events) from the initial pool.
5. Missing config keys (`match_handler`, `target_state`) cause `StepFailed` with a clear reason.
6. Missing workflow snapshot (`sc.RLCCWorkflow == nil`) causes `StepFailed`.
7. ctx timeout returns a partial Output with `timed_out = true` and remaining-CT count.
8. `cmd/qac/serve.go` registers the probe in the engine's Registry alongside alertprobe + factprobe.
9. `internal/api/api_test.go`'s `newTestServer` registers the probe.
10. `firmware-release-canary.yaml` is unchanged (iter-5e ships the library, not the wiring).
11. `go test ./...` is green. `cd web && yarn test --run` is green (FE unaffected; sanity check only).
12. All 18 (or however many) test cases listed in §10 pass.

## 12. Out of scope (deferred to later iters)

- **Wiring `rlcc_action_probe` into any canary template step.** That's the next iter (likely iter-5f): edit `firmware-release-canary.yaml` to flip `l11_fielddiag` (or other) from no-probe to `probe_type: rlcc_action_probe`.
- **Other probe types** (`metrics_baseline_probe` for baseline_pre/baseline_post, `flcc_node_zap_probe` for zap_canary, `ai_summary_probe`). Each is its own iter, same shape as iter-5e.
- **Per-step verdict aggregation.** When does a per-step verdict become `failed` vs `warning` based on per-CT outcomes? Threshold-based? All-must-pass? Deferred until at least two probes ship.
- **Frontend per-CT view** on RunDetail. iter-5f or later.
- **Production Prometheus deployment**. PromClient ships in iter-5e but isn't exercised against a live cluster.
