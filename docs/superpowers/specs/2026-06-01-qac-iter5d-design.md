# qac iter-5d — RLCC workflow materialization

**Status:** draft
**Branch:** `wp/add-qac` (continues iter-5c)
**Predecessor:** [iter-5c spec](2026-05-31-qac-iter5c-design.md), iter-5c HEAD at `034503c`
**PR posture:** still local. iter-5d is the first iter that flips `rlcc_workflow` to required and starts using the picked workflow to shape the run.

## 1. Purpose

The iter-5c wizard already submits `inputs.rlcc_workflow` — the operator's pick from `/api/rlcc/workflows`. Today the value lands in the run's `inputs` map, gets promoted to a fact, and goes nowhere else. The engine still walks the static canary template's L10/L11/L12 steps with their hand-written FLCC/RLCC arguments.

iter-5d makes the picked workflow **shape the run**:

- At run-create, the handler fetches the workflow's full graph (`{name, source_sha, actions[]}`) from `rlccclient`.
- The graph is **inlined** into `RunCreated.payload.rlcc_workflow` as a snapshot — pinned at run-create-time so the run is replay-deterministic even if the chart changes upstream.
- The template's `rlcc_workflow` input flips from `required:false` to `required:true`. Every firmware-release-canary run now has a pinned workflow.

What iter-5d does **not** do:

- It does not change the canary template's step list. The 14-step spine stays.
- It does not introduce a new probe that walks the workflow's actions. That's iter-5e.
- It does not change the engine's projection or step-walking. The projector reads the new `rlcc_workflow` field but ignores it (iter-5e reads it).

After iter-5d ships: the snapshot is in the event log, addressable from the event payload, ready for iter-5e's `rlcc_action_probe` to consume.

## 2. Scope decisions

| Decision | Value | Why |
|---|---|---|
| Materialization shape | **Option B — inlined snapshot in `RunCreated.payload.rlcc_workflow`** | The canary template's spine (gather → assert → modify → verify → report) is the interpretation framework; the workflow informs *what* the L-steps do, not *what shape* the run has. Workflows with fewer actions just leave some L-steps as no-ops in iter-5e. Inlining (vs storing a pinned reference) makes the run truly self-contained — Sourcegraph can go down, the chart can rewrite, and replay still works. |
| Pin semantics | **Freeze at run-create** | Firmware-canary work is auditable verification. Once a run starts, its recipe must not change — that's the whole point of pinning a version. In-flight runs do not pick up upstream chart edits. New runs reflect whatever the chart says at the moment of POST. |
| SHA origin | **Handler looks up at POST time** via `rlccclient.GetWorkflow(name)` | Smallest wire surface. The wizard's job stays simple: pick by name; the server figures out the rest. The race window (chart edits between wizard load and POST) is tiny in practice. If it ever bites, we can layer "wizard sends optional SHA" later without breaking the contract. |
| `rlcc_workflow.required` | **Flip `false → true` in this iter** | Keeping the fallback path alive carries two engine paths and a non-canonical seed-demo into iter-5e+. The cost of the flip is one line in the template, ~3 lines in seed, ~1 line in canonical integ test. |
| Template version bump | **6 → 7** | Any change to `inputs[].required` is a per-template revision bump (same posture as iter-4c's `fact_baseline` addition and iter-5c's `rlcc_workflow` addition). Schema id stays `qac.template/v1`. |
| Storage | **Same JSON `payload` column on `events` table** | The new field is one more key in the existing JSON blob. No migration, no new column, no new event kind. `omitempty` pointer-to-struct round-trips cleanly for legacy pre-iter-5d rows. |
| Where materialization happens | **`createRunHandler`, synchronously, between template resolution and `RunCreated` emit** | All-or-nothing: if workflow lookup fails, no `RunCreated` event lands. No half-baked state. Mirrors iter-5b's pattern (BMN resolution before BMN events emit). |
| Workflow-not-found behavior | **400 `rlcc_workflow_unknown`** before any event emit | Fail loud; the wizard already filtered to known names, so this only fires for curl/CLI typos. |
| `action_count == 0` (state-mover) | **400 `rlcc_workflow_unknown`** with message "no actions to walk" | A workflow with no actions is meaningless to a canary run. Wizard greys these out; the API enforces. |
| Frontend code changes | **None** | The wizard already submits `inputs.rlcc_workflow`. No new field. No new error path the user sees differently. Only MSW fixture data needs the template version + required-flag bump to mirror the backend. |
| Pre-iter-5d runs already in DB | **Continue to function** | Engine projector ignores `rlcc_workflow`. iter-5e's `rlcc_action_probe` will need to handle the absence; deferred to iter-5e's design. Likely path: skip the probe entirely for pre-iter-5d runs (they're auditable historical records). |

## 3. Architecture

iter-5d adds **one new step** to `createRunHandler`. Everything else is downstream and changes minimally.

```
POST /api/runs
     │
     ▼
createRunHandler:
  1. Decode body, ValidateInputs (existing — now rejects missing rlcc_workflow)
  2. Resolve template from store (existing)
  3. ── NEW: Resolve workflow snapshot ──
       snapshot, err := rlccClient.GetWorkflow(ctx, inputs["rlcc_workflow"])
       → returns {Name, SourceSHA, Actions[]}
       → 400 rlcc_workflow_unknown if not found or len(Actions) == 0
  4. Generate run id (existing)
  5. Emit RunCreated event with snapshot inlined in payload (CHANGED shape, same event kind)
  6. Emit FactsDiscovered for operator inputs (existing)
  7. Resolve racks → BMNs (iter-5b — existing)
  8. Emit BMNDiscovered / BMNIgnored events (iter-5b — existing)
  9. Engine.Wake(runID) (existing)
```

The engine does not grow a new event kind. The store does not grow a new column or migration. The frontend does not change. The only library-level change is `rlccclient.Client` growing a `GetWorkflow(ctx, name)` method.

### Layer-by-layer change surface

| Layer | iter-5d change | Magnitude |
|---|---|---|
| `internal/rlccclient/client.go` | **No change.** `Client.GetWorkflow(ctx, name) (WorkflowGraph, SourceRef, error)` and `ErrWorkflowNotFound` already exist from iter-5a. | 0 lines |
| `internal/rlccclient/mapclient.go` | **No change.** Map-backed `GetWorkflow` already returns the seeded workflow or `ErrWorkflowNotFound`. | 0 lines |
| `internal/rlccclient/sourcegraph_client.go` | **No change.** Sourcegraph-backed `GetWorkflow` already looks up by name in the cached `map[string]WorkflowGraph`. | 0 lines |
| `internal/rlccclient/types.go` | Add `WorkflowSnapshot` value type (slimmed `WorkflowGraph` for storage — name + SHA + actions, no chart tag). | ~10 lines |
| `internal/api/errors.go` | Add `codeRLCCWorkflowUnknown = "rlcc_workflow_unknown"`. | 1 line |
| `internal/api/runs.go` | Add step 3 to `createRunHandler`. Inline snapshot into `RunCreated` payload. | ~30 lines |
| `internal/api/router.go` / `server.New` | Router signature grows by 0 args — `rlccclient.Client` is already wired through (iter-5a). | 0 lines |
| `internal/store/events.go` | Grow `runCreatedPayload` with `RLCCWorkflow *WorkflowSnapshot \`json:"rlcc_workflow,omitempty"\``. Projector unchanged (it doesn't consume the field). | ~5 lines |
| `internal/engine/event.go` | Mirror the grow on `RunCreatedPayload`. | ~5 lines |
| `templates/firmware-release-canary.yaml` | Flip `rlcc_workflow.required: false → true`. Bump `version: 6 → 7`. | 2 lines |
| `internal/template/embedded_test.go` | Bump version assertion `6 → 7`. | 1 line |
| `internal/seed/seed.go` | Add `"rlcc_workflow": "gb200-rack-bringup-v4"` to `SeedDemoRun()` inputs. | 1 line |
| `internal/api/canonical_integ_test.go` | Extend request body with `rlcc_workflow`; assert the inlined snapshot on the resulting run's `RunCreated.payload`. | ~5 lines |
| `web/src/mocks/handlers.ts` | Bump template version `6 → 7`; flip `rlcc_workflow.required: false → true` in the MSW template fixture. | 2 lines |
| Frontend | No code change. | 0 lines |

## 4. Data shapes

### 4.1 `WorkflowSnapshot` (new type)

In `internal/rlccclient/types.go`:

```go
// WorkflowSnapshot is the run-create-time pin: enough to walk the workflow
// without consulting Sourcegraph again. JSON-serialized into RunCreated.
// Name + SourceSHA together uniquely identify a workflow revision; Actions
// is the linear sequence iter-5e's rlcc_action_probe walks.
type WorkflowSnapshot struct {
    Name      string   `json:"name"`
    SourceSHA string   `json:"source_sha"`
    Actions   []Action `json:"actions"`
}
```

`Action` is unchanged from iter-5a — same struct, same field tags.

### 4.2 `RunCreated.payload` — after iter-5d

```json
{
  "template_id": "firmware-release-canary",
  "template_version": 7,
  "inputs": {
    "bundle_tag": "dell-xe9712-default-2.0.0-rc",
    "canary_racks": ["dh3-r012-us-east-01a"],
    "requester": "wallace",
    "rlcc_workflow": "gb200-rack-bringup-v4"
  },
  "rlcc_workflow": {
    "name": "gb200-rack-bringup-v4",
    "source_sha": "abc123def456…",
    "actions": [
      { "name": "checked-in → onboarded",
        "handler": "hlcc-state",
        "arguments": { "components": { "rack": { "state": "onboarded" }}}},
      { "name": "l11 fielddiag",
        "handler": "l11-fielddiag",
        "arguments": { "components": { "computetray": {
          "workflow": "fielddiag",
          "checks": ["smoke-tests", "transient-link"]
        }}}}
      // … remaining actions
    ]
  },
  "created_by": "wpena"
}
```

Two `rlcc_workflow` references:

- `inputs.rlcc_workflow` (string) — what the operator *picked*. Preserved as their intent.
- `rlcc_workflow` (object, top-level) — what the server *resolved*. The pinned snapshot.

The duplication is intentional. The string captures the operator's choice and remains stable across iters. The object is the resolved truth that iter-5e walks.

### 4.3 What iter-5e probes will read

```go
// Reference — iter-5e's rlcc_action_probe, expected shape (not built in 5d):
runCreated := getFirstEvent(runID, "RunCreated")
var p engine.RunCreatedPayload
if err := json.Unmarshal(runCreated.Payload, &p); err != nil { ... }
if p.RLCCWorkflow == nil {
    // pre-iter-5d run; skip workflow-driven probe entirely
    return nil
}
for _, action := range p.RLCCWorkflow.Actions {
    // walk one action; emit StepStarted/StepCompleted with action.Name
}
```

iter-5d's success criterion: a probe written like this can read `p.RLCCWorkflow.Actions[0].Name` and get a non-empty string for any wizard-created run.

## 5. Error handling

```go
// in createRunHandler, between template resolution and runID generation:
wfName, _ := inputs["rlcc_workflow"].(string)  // already validated by ValidateInputs (required:true)
graph, src, err := rlccClient.GetWorkflow(r.Context(), wfName)
switch {
case errors.Is(err, rlccclient.ErrWorkflowNotFound):
    writeError(w, http.StatusBadRequest, codeRLCCWorkflowUnknown,
        "RLCC workflow not found: "+wfName)
    return
case err != nil:
    writeError(w, http.StatusInternalServerError, codeInternal,
        "Failed to resolve RLCC workflow")
    return
case len(graph.Actions) == 0:
    writeError(w, http.StatusBadRequest, codeRLCCWorkflowUnknown,
        "RLCC workflow has no actions to walk: "+wfName)
    return
}
snapshot := rlccclient.WorkflowSnapshot{
    Name:      graph.Name,
    SourceSHA: src.SHA,
    Actions:   graph.Actions,
}
// snapshot is valid; proceed to emit RunCreated with rlcc_workflow inlined
```

Error matrix:

| Scenario | HTTP | Code | Body message |
|---|---|---|---|
| `rlcc_workflow` missing on POST (after required:true flip) | 400 | `invalid_request` | "rlcc_workflow is required" *(existing ValidateInputs)* |
| Workflow name not in chart | 400 | `rlcc_workflow_unknown` | "RLCC workflow not found: \<name\>" |
| Workflow exists but `len(Actions) == 0` | 400 | `rlcc_workflow_unknown` | "RLCC workflow has no actions to walk: \<name\>" |
| Sourcegraph degraded (auth missing) | 200 | — | Lookup falls through to embedded fixture (same as iter-5a) |
| Sourcegraph reachable, YAML malformed | 500 | `internal` | "Failed to resolve RLCC workflow" |

All error paths return **before** any event is emitted. No half-baked runs land in the event log.

## 6. Backward compat

Pre-iter-5d runs already in the SQLite DB have no `rlcc_workflow` field in `RunCreated.payload`. Three properties hold:

1. **The engine projector continues to load them.** The projector doesn't read `rlcc_workflow` in iter-5d. JSON-unmarshaling a payload without the field into the new `RunCreatedPayload` struct leaves `RLCCWorkflow` as `nil` (pointer-to-struct, `omitempty`).
2. **`GET /api/runs/{id}` continues to return them.** The events array round-trips verbatim; `racks[]` rollup is unchanged.
3. **Iter-5e probes treat them as legacy.** When iter-5e's `rlcc_action_probe` finds `p.RLCCWorkflow == nil`, it skips entirely (`return nil`). The static template's L-level steps walk as no-ops or run their hand-written args (decided in iter-5e's design).

Three callers need updating in lockstep with the `required: false → true` flip — same commit:

| Caller | Update |
|---|---|
| `internal/seed/seed.go` — `SeedDemoRun()` / `SeedDemoMultiRack()` | Add `"rlcc_workflow": "gb200-rack-bringup-v4"` to seed inputs. |
| `internal/api/canonical_integ_test.go` | Test request body adds `rlcc_workflow`. Test fixture (`MapClient`) seeds `gb200-rack-bringup-v4`. |
| `web/src/mocks/handlers.ts` | Bump template version `6 → 7`; flip `rlcc_workflow.required` in MSW template fixture. |

The wizard itself already submits `inputs.rlcc_workflow` on every run — no FE code change.

## 7. Testing

| Test | Lives in | Asserts |
|---|---|---|
| `MapClient.GetWorkflow` happy + unhappy | `internal/rlccclient/parse_test.go` *(already passes)* | Existing iter-5a coverage. No change in iter-5d. |
| Sourcegraph-backed `GetWorkflow` happy + unhappy | `internal/rlccclient/sourcegraph_client_test.go` *(already passes)* | Existing iter-5a coverage. No change in iter-5d. |
| `createRunHandler` inlines the snapshot | `internal/api/runs_test.go` *(new subtests)* | POST with `rlcc_workflow: gb200-rack-bringup-v4` → 201; `GET /runs/{id}` returns `RunCreated.payload.rlcc_workflow` with `name`, `source_sha` (non-empty), `len(actions) > 0`. |
| `createRunHandler` rejects unknown workflow | `internal/api/runs_test.go` | POST with `rlcc_workflow: nope` → 400 `rlcc_workflow_unknown`. No `RunCreated` event in store. |
| `createRunHandler` rejects empty-actions workflow | `internal/api/runs_test.go` | POST with `rlcc_workflow: checked-in` (action_count=0) → 400 `rlcc_workflow_unknown`. |
| Template required-flip enforcement | `internal/api/runs_test.go` | POST without `rlcc_workflow` → 400 `invalid_request`. |
| `runCreatedPayload` JSON round-trip | `internal/store/events_test.go` *(extend)* | A payload without `rlcc_workflow` (legacy) decodes to nil pointer; a payload with one decodes to a populated `WorkflowSnapshot`. |
| Engine projection ignores `rlcc_workflow` | `internal/engine/project_test.go` *(extend)* | Replay a run with workflow → engine state is identical to a run without workflow (modulo the `RLCCWorkflow` field, which is not used by the projector). |
| Canonical integ extension | `internal/api/canonical_integ_test.go` | After walking the demo seed run end-to-end, assert `RunCreated.payload.rlcc_workflow.name == "gb200-rack-bringup-v4"`, `.source_sha != ""`, `len(.actions) > 0`. |
| Frontend wizard test | `web/src/features/runs/wizard/NewRunWizard.test.tsx` *(extend)* | After template version 6 → 7 in MSW, the wizard still validates and submits `rlcc_workflow` correctly. (Existing test should already pass; this is a sanity assertion.) |

Run via `go test ./...` and `yarn test --run`.

## 8. Acceptance criteria

iter-5d ships when all of the following are true:

1. `POST /api/runs` with `rlcc_workflow: gb200-rack-bringup-v4` lands a `RunCreated` event whose `payload.rlcc_workflow` is the full `WorkflowSnapshot`.
2. `POST /api/runs` without `rlcc_workflow` returns 400 `invalid_request` ("rlcc_workflow is required").
3. `POST /api/runs` with an unknown workflow name returns 400 `rlcc_workflow_unknown` and emits no events.
4. `POST /api/runs` with a state-mover workflow (`action_count == 0`) returns 400 `rlcc_workflow_unknown`.
5. The pinned `source_sha` is non-empty for any successful run.
6. `MapClient.GetWorkflow` covers happy and unhappy paths in tests.
7. Real Sourcegraph-backed client's `GetWorkflow` covers the same paths against the `genrlccfixture`.
8. The canonical integ test passes end-to-end with the new request shape.
9. Pre-iter-5d runs in the DB are still queryable via `GET /api/runs/{id}` without error.
10. `go test ./...` is green. `cd web && yarn test --run && yarn tsc --noEmit && yarn lint` is green.

## 9. Out of scope (deferred to iter-5e)

- `rlcc_action_probe` — the new probe that consumes `rlcc_workflow.actions[]` and walks them.
- Per-CT action attribution — each action becomes one step *per CT*, parallelized.
- Per-tick PromQL fanout — using `await-checkpoint` actions' arguments to construct per-CT Prometheus queries.
- Per-CT evidence rows — using iter-5b's `evidence.deviceslot` column to attribute results.
- Per-tick RLCC ignore re-detection — checking `baremetal_node_rlcc_provisioning_ignored` at each tick to handle CTs that get RLCC-ignored mid-run.
- Frontend per-CT view on RunDetail (iter-5f).
- `workflowOverrides` from iter-6+ — operator-supplied parameter overrides on a per-action basis.
- Dispatch capability — actually *triggering* RLCC state transitions (today we observe; we don't dispatch).
