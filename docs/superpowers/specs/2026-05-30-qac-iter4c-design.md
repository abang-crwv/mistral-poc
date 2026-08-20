# qac iter-4c — multi-step walk + fact_probe (second gatherer) end-to-end

**Status:** draft
**Branch:** `wp/add-qac` (continues iter-4b)
**Predecessor:** [iter-4b spec](2026-05-29-qac-iter4b-design.md), iter-4b HEAD at `3d925f0`
**PR posture:** still local. iter-4b was the documented PR-opener; user opted to keep layering before pushing. iter-4c continues backend-only.

## 1. Purpose

Land the engine's multi-step walk and a second concrete probe. After iter-4b the engine ran exactly one probe per Kick and stopped. iter-4c teaches it to keep walking the template — running every runnable `probe_type` step in template order, continuing past individual step failures, halting only when there are no more runnable steps or when it hits an operator-gated step kind. The second probe (`fact_probe`) gives us a non-trivial multi-step canary on the canonical template: a real run after iter-4c records both an alert snapshot and an inventory snapshot end-to-end.

## 2. Scope decisions (from brainstorming)

| Decision | Value | Why |
|---|---|---|
| Runner architecture | **Loop inside `runOnce`** | Single goroutine per run drives every step sequentially. Matches strict-sequential canary semantics; keeps `Wait(runID)` clean (one channel per run, closes on whole-run end); existing iter-4b lifecycle tests stay valid. |
| Walk halt conditions | **(a) no more runnable probe_type steps, (b) review/ai_assess/signoff step encountered, (c) DB read/write breaks mid-walk** | Each halt is "we have nothing more to observe right now," not "we observed a failure." Aligns with qac's observability framing — qac snapshots; FLCC/RLCC do the actual work. |
| StepFailed walk semantics | **Continue walking** | A failed probe is one data point. Other observations still have value; qac gathers what it can. iter-4b's "halt on StepFailed" is explicitly relaxed. |
| Unknown probe type walk semantics | **Emit StepFailed + continue** | Same reasoning. iter-4b halted on unknown; iter-4c relaxes. |
| `runs.status='failed'` meaning | **qac self-diagnostic** | "qac couldn't complete the full observation set for this rack" — *not* a verdict on the rack. The rack does whatever FLCC/RLCC are doing regardless. Status remains store-level projection driven by `StepFailed`. |
| Event re-loading between iterations | **Re-call `LoadEvents` at top of each iteration** | Avoids in-memory drift; persistence is the source of truth. Cheap at iter-4c scale (few events per run). Pays off when iter-4d adds SSE / restart-after-crash. |
| Already-started filter | **Skip steps whose ID appears in a prior `StepStarted`** | Without this, a loop infinite-loops on the same first step. Iter-4b's single-step engine never needed the check; now it does. |
| Second probe | **`fact_probe` (gatherer)** | Re-uses iter-4a's `inventoryclient.Resolver` port — no new client surface. Snapshots "what does inventory say about each rack at probe-run time" so a long canary can compare against the run-create-time fact set. |
| Probe ordering on canonical template | **`alert_baseline` then `fact_baseline`** | Order is arbitrary for two independent gatherer snapshots — chosen for narrative readability. Engine walks template order strictly. See §14 open question. |
| Frontend | **No changes** | RunDetail still shows the flat events list. Surfacing the multi-step walk visually is iter-4d. |
| Operator-advance endpoint | **Deferred** | Engine halts cleanly at review/ai_assess/signoff steps; the operator-advance HTTP endpoint that unblocks them lands in iter-4d. |
| Real Alertmanager / Netbox integration | **Still stubbed** | iter-4c stays focused on engine + probe additions. Real-backend integration is iter-4d+. |

## 3. Walk semantics (codified)

**The walk is a fold over `template.Steps[]`.** Pseudo-procedure:

```
runOnce(runID):
  defer panic-recover → StepFailed for current step + exit goroutine
  loadTemplate(once)
  loadInputs(once)
  loop:
    evs = LoadEvents(runID)
    seenSteps = set of step IDs that appear in any StepStarted/StepCompleted/StepFailed event
    step = first template.Step whose ID not in seenSteps
    
    if step is nil:
      log("walk complete")
      return
    
    if step.Kind in {review, ai_assess, signoff}:
      log("halted at operator-gated step", step.ID)
      return
    
    if step.ProbeType == "" (i.e., action.tool-shaped):
      // Engine doesn't dispatch tool-based steps yet. Skip without
      // emitting StepStarted, but mark as "considered" by adding a
      // pseudo-skip event? NO — simpler: just continue the loop with
      // step still un-seen, which means the loop will re-pick it next
      // iteration and re-skip forever. Need an in-memory "skipped"
      // set to track these without persisting events.
      add step.ID to skippedThisRun (in-memory)
      continue
    
    probe = registry.Get(step.ProbeType)
    if probe == nil:
      emitStepFailed(step.ID, "probe type X not registered")
      continue
    
    emitStepStarted(step.ID, step.ProbeType, racks, now)
    output, err = probe.Run(ctx, clients, sc)
    if err != nil:
      emitStepFailed(step.ID, err.Error())
      continue
    
    evidenceID = ulid
    err = store.PutEvidence(...)
    if err != nil:
      emitStepFailed(step.ID, "persist evidence: " + err)
      continue
    
    err = emitStepCompleted(step.ID, evidenceID, ...)
    if err != nil:
      log("emit StepCompleted failed, evidence already persisted", err)
      // continue anyway — the StepCompleted event is lost but evidence
      // row landed; operator can reconstruct.
```

**Implementation note: the `skippedThisRun` in-memory set.** Action.tool-shaped steps don't emit any event when skipped (no `StepSkipped` event kind in iter-4c — would add noise; the steps stay un-runnable until iter-4d migrates them). So the loop tracks "already skipped this iteration of the goroutine" via a local map. After the goroutine exits, that state is lost; a re-Kick (iter-4d's restart story) would re-evaluate from scratch. Acceptable because re-evaluating is idempotent — action.tool steps are still un-runnable.

**Read-then-write race window between `LoadEvents` and `emitStepStarted`:** vanishingly small in iter-4c because only the engine writes step events for a given runID, and Kick is idempotent per-runID (no two goroutines for the same run). If we ever had multiple writers, the seenSteps filter could lag. Not a concern at iter-4c.

## 4. New + extended packages

```
internal/probe/factprobe/      # NEW
  factprobe.go                   FactProbe (Type, Category, Run)
  factprobe_test.go              5 subtests: type+category, multi-rack happy,
                                 partial-resolve, AmbiguousError, transport error

internal/engine/               # EXTEND
  runner.go                      runOnce body becomes a loop; findNextRunnableStep
                                 helper grows event-aware skip; skippedThisRun set
  engine_test.go                 4 new subtests for the walk loop

internal/api/                  # EXTEND
  canonical_integ_test.go        Assert two StepCompleted events land, both
                                 evidence rows readable, all_resolved field present
  api_test.go                    No source change required — newTestServer already
                                 registers alertprobe; will need to also register
                                 factprobe (otherwise canonical test fails because
                                 fact_baseline step references unregistered probe)

internal/template/             # EXTEND
  embedded_test.go               Version 3 → 4; step count 13 → 14

templates/firmware-release-canary.yaml  # MODIFY
                                  Bump version 3 → 4; insert fact_baseline as
                                  the second step (after alert_baseline)

cmd/qac/serve.go            # EXTEND
                                  Register factprobe.New() alongside alertprobe.New()
```

No new HTTP routes, no new event kinds, no new store tables, no new client surface.

## 5. Core types

### 5.1 factprobe.FactProbe

```go
package factprobe

type FactProbe struct{}

func New() *FactProbe { return &FactProbe{} }

var _ probe.Probe = (*FactProbe)(nil)

func (*FactProbe) Type() string             { return "fact_probe" }
func (*FactProbe) Category() probe.Category { return probe.CategoryGatherer }

func (*FactProbe) Run(ctx context.Context, clients probe.Clients, sc probe.StepContext) (probe.Output, error) {
    // For each rack in sc.Racks, call clients.InventoryResolver via
    // inventoryclient.ResolveRack. Per-rack: success → resolved=true +
    // facts populated; ErrNotFound or AmbiguousError → resolved=false +
    // error populated. Only an unexpected transport error (not
    // ErrNotFound / not AmbiguousError) bubbles up as a StepFailed.
}
```

### 5.2 factprobe output schema

```json
{
  "probed_at": 1748000000,
  "source": "inventory-stub",
  "per_rack": {
    "dh3-r012-us-east-01a": {
      "resolved": true,
      "facts": {
        "instance_type": "gb200-4x",
        "sku": "nvl72",
        "variant": "gb200",
        "gb_generation": "blackwell",
        "region": "us-east",
        "cluster": "use-prod-01"
      }
    },
    "dh99-r999-us-east-99z": {
      "resolved": false,
      "error": "inventoryclient: not found"
    }
  },
  "all_resolved": false
}
```

`all_resolved` is `true` iff every per_rack entry has `resolved=true`. The probe never returns a Go error for "rack not found" or "rack ambiguous" — those are data, captured in `per_rack[rack].resolved=false`. Only an unexpected error (none defined today; defensive for iter-4d real backend) becomes a probe-level error → StepFailed.

### 5.3 Engine runner additions

No struct-shape change to `Engine`. The two additions are in `runner.go`:

```go
// runOnce becomes a loop (see §3 pseudo-procedure).

// findNextRunnableStep is the existing findNextProbeStep, extended to
// take a seenSteps + skippedThisRun set and skip any step already
// observed. Returns (Step{}, false) when there are no more runnable
// steps.
func findNextRunnableStep(tpl template.Template, seen map[string]bool, skipped map[string]bool) (template.Step, bool)
```

iter-4b's `findNextProbeStep` is renamed/replaced by `findNextRunnableStep`. Tests in engine_test.go that referenced the old helper update accordingly.

## 6. Event kinds (no additions)

iter-4b's `StepStarted`, `StepCompleted`, `StepFailed` cover iter-4c. The engine emits the same shapes; the only behavior change is that multiple instances may now appear per run.

## 7. HTTP surface (no additions)

`POST /api/runs`, `GET /api/runs/{id}`, `GET /api/runs/{id}/facts`, `GET /api/runs/{id}/evidence/{eid}` all unchanged. The `GET /api/runs/{id}` events list will now include more events per run (multiple StepStarted/StepCompleted pairs).

## 8. Data flow (happy path)

```
1. POST /api/runs (iter-4a + iter-4b path)
2. iter-4a discovery emits RunCreated + FactsDiscovered events
3. createRunHandler tail-calls engine.Kick → returns 201
4. ─── async, in goroutine ───
5. Engine loads template + inputs once
6. Loop iteration 1:
   a. LoadEvents → seenSteps = {} (no step events yet)
   b. findNextRunnableStep → alert_baseline
   c. emit StepStarted(alert_baseline, alert_probe, racks)
   d. alertprobe.Run → Output
   e. PutEvidence → evidence_id_1
   f. emit StepCompleted(alert_baseline, evidence_id_1)
7. Loop iteration 2:
   a. LoadEvents → seenSteps = {alert_baseline}
   b. findNextRunnableStep → fact_baseline
   c. emit StepStarted(fact_baseline, fact_probe, racks)
   d. factprobe.Run → Output
   e. PutEvidence → evidence_id_2
   f. emit StepCompleted(fact_baseline, evidence_id_2)
8. Loop iteration 3:
   a. LoadEvents → seenSteps = {alert_baseline, fact_baseline}
   b. findNextRunnableStep → baseline_pre (action.tool-shaped)
   c. ProbeType == "" → add to skippedThisRun, continue
9. Loop iteration 4..12:
   a. Successive action.tool-shaped steps (baseline_pre through baseline_post)
      added to skippedThisRun, continue
10. Loop iteration 13:
   a. findNextRunnableStep → evidence_review (kind=review)
   b. Step kind is operator-gated → log "halted at operator-gated step", return
11. Goroutine done channel closes; Wait callers unblock
12. Operator polls GET /api/runs/{id} → sees StepStarted + StepCompleted × 2
13. Operator follows evidence_id_1 + evidence_id_2 → reads both snapshots
```

## 9. Error paths

| When | Behavior | Walk continues? |
|---|---|---|
| `LoadEvents` fails (DB read) | emitStepFailed("" stepID), exit goroutine | No — can't safely continue without event log |
| `findNextRunnableStep` returns no match | Log "walk complete", return cleanly | N/A |
| Next step is review/ai_assess/signoff | Log "operator-gated", return cleanly | N/A |
| Next step is action.tool-shaped (ProbeType=="") | Add to skippedThisRun, continue | Yes |
| `probes.Get(step.ProbeType)` returns false | emitStepFailed("probe type X not registered") | **Yes** (relaxed from iter-4b) |
| `emitStepStarted` AppendEvent fails | emitStepFailed (best-effort), exit goroutine | No — persistence broken |
| `probe.Run` returns error | emitStepFailed(err.Error()) | **Yes** (relaxed from iter-4b) |
| `probe.Run` panics | defer recover → emitStepFailed("panic: ..."), exit goroutine | No — undefined state after panic |
| `PutEvidence` fails | emitStepFailed("persist evidence: ...") | **Yes** |
| `emitStepCompleted` AppendEvent fails | Log + continue | **Yes** (evidence already landed) |

**Rationale for "DB write to fail → exit goroutine, probe to fail → continue":** if persistence is broken, every subsequent step also fails (we can't write its events either). If one observation tool errored, the others can still observe productively. We cut losses on systemic failures, plow through tool-specific failures.

## 10. Template change

`templates/firmware-release-canary.yaml` bumps `version: 3 → 4`. One new step inserted after `alert_baseline`:

```yaml
steps:
  - id: alert_baseline
    kind: probe
    probe_type: alert_probe
    title: "Pre-zap alert baseline"
    racks: "{{ inputs.canary_racks }}"
    timeout: 30s

  - id: fact_baseline                       # NEW
    kind: probe                             # NEW
    probe_type: fact_probe                  # NEW
    title: "Pre-zap inventory facts snapshot"  # NEW
    racks: "{{ inputs.canary_racks }}"      # NEW
    timeout: 30s                            # NEW

  - id: baseline_pre
    # ... existing action.tool-shaped step, unchanged ...
```

The 9 existing action.tool-shaped probe steps (baseline_pre, zap_canary, l10_fielddiag, l10_check, l11_fielddiag, l11_check, l12_seatrial, l12_check, baseline_post) and the 3 operator-gated steps (evidence_review of kind `review`, ai_summary of kind `ai_assess`, operator_signoff of kind `signoff`) remain exactly as-is. The engine skips action.tool-shaped steps as it walks and halts cleanly the first time it sees an operator-gated step — which in the canonical template will be evidence_review.

`internal/template/embedded_test.go` updated: `Version = 3` → `Version = 4`, and the step-count assertion `Steps len = 13` → `Steps len = 14`. The accompanying comment block in the test that enumerates step IDs also gains `fact_baseline` (insert immediately after `alert_baseline`).

## 11. Wiring (cmd/qac/serve.go)

```go
reg := probe.NewRegistry()
reg.Register(alertprobe.New())
reg.Register(factprobe.New())    // NEW
eng := engine.New(s, reg, probe.Clients{
    AlertClient:       alertC,
    InventoryResolver: resolver,
})
```

`probe.Clients.InventoryResolver` is already wired (iter-4a). `internal/api/api_test.go`'s `newTestServer` also gets `factprobe.New()` registered.

## 12. Testing strategy

### 12.1 Unit tests

**`internal/probe/factprobe/factprobe_test.go`** — 5 subtests using a test-local `fakeInventoryResolver` (not the iter-4a `MapResolver` — keeps the unit test focused):

- `TestFactProbe_TypeAndCategory` — sanity
- `TestFactProbe_Run_MultiRack_AllResolved` — happy: 2 racks both resolve; assert `all_resolved=true`
- `TestFactProbe_Run_PartialResolve_NotFound` — one rack returns ErrNotFound; assert that rack's `resolved=false`, others succeed, probe returns nil error, `all_resolved=false`
- `TestFactProbe_Run_AmbiguousError_RecordedAsData` — one rack returns AmbiguousError; assert recorded as `resolved=false` + error; probe returns nil
- `TestFactProbe_Run_UnexpectedTransportError_Bubbles` — test-injected non-sentinel error; assert probe returns the error

**`internal/engine/engine_test.go`** — 4 new subtests for the walk loop, using the existing `mockProbe` registered under multiple type names:

- `TestEngine_Walk_MultipleStepsRunSequentially` — synthetic 3-probe_type-step template; assert events end with StepStarted×3, StepCompleted×3 in the correct interleaving
- `TestEngine_Walk_ContinuesPastStepFailed` — middle step's mockProbe returns error; assert all 3 StepStarted fire, middle ends in StepFailed, last produces evidence
- `TestEngine_Walk_HaltsAtNonProbeStep` — synthetic template: 2 probe_type steps + 1 review step; assert walk stops after step 2, no event for review step
- `TestEngine_Walk_UnknownProbeType_RestContinue` — middle step references unregistered probe_type; assert StepFailed lands for it AND the third probe_type step still runs

### 12.2 Integration tests

**`internal/api/canonical_integ_test.go`** — extend the existing `TestCreateRun_CanonicalCanaryTemplate_LegacyShape`:

- After `eng.Wait(out.Run.ID)` closes, assert events list contains **two** StepCompleted events (one each for alert_baseline + fact_baseline)
- Each StepCompleted has a non-empty evidence_id
- Both evidence rows are readable; the second one's payload contains the `all_resolved` field

### 12.3 No new test infrastructure

No new fixtures, no new MapClient subclasses, no new test helpers beyond the inline synthetic templates already in engine_test.go.

## 13. Acceptance criteria

| # | Check |
|---|---|
| A1 | `make build` clean |
| A2 | `make lint` clean (go vet + yarn lint) |
| A3 | `make test` green; no regressions in iter-4b's tests |
| A4 | Engine walks at least 2 probe_type steps end-to-end on the canonical template (event log shows StepStarted×2, StepCompleted×2 minimum) |
| A5 | Engine halts cleanly at the first non-probe_type-non-skipped step (review/ai_assess/signoff) without emitting StepFailed for it |
| A6 | Walk continues past a StepFailed; the subsequent step still produces evidence (verified by `TestEngine_Walk_ContinuesPastStepFailed`) |
| A7 | Walk continues past an unknown probe_type; the subsequent step still runs |
| A8 | fact_probe records ErrNotFound / AmbiguousError as per-rack `resolved=false` data (no StepFailed) |
| A9 | fact_probe returns probe-level error only on unexpected transport error |
| A10 | Canonical-integ asserts both alert_baseline + fact_baseline evidence rows landed and are GET-able |
| A11 | Template `version: 4` reported by `qac template load` |
| A12 | iter-4a + iter-4b tests still pass (no regressions in facts / discovery / inventory / engine lifecycle / alert_probe) |
| A13 | `qac seed-demo` still works against the new template version |
| A14 | A run whose first step probe fails still ends with both StepStarted+StepFailed AND a subsequent StepStarted+StepCompleted in the event log (no early termination) |

## 14. Risks + open questions

- **Template step ordering between independent gatherers (open question).** The current YAML places `alert_baseline` before `fact_baseline`, but the two are independent observations with no causal dependency. The order is arbitrary; the engine walks template order strictly. This was flagged during brainstorming as something to revisit — possible follow-ups include: a parallel-group primitive in the template schema, a composite "pre-zap snapshot" step that internally runs both probes, or just clearer documentation in the YAML. Not in iter-4c scope; tracked for iter-4d.
- **`skippedThisRun` is per-goroutine state.** If a goroutine crashes after skipping action.tool steps and a re-Kick fires (iter-4d's crash recovery story), the new goroutine re-skips. Idempotent but wasteful at iter-4d scale; revisit when restart-after-crash lands.
- **Walk doesn't currently rate-limit between probes.** A run with 20 probe_type steps would fire all 20 back-to-back. Fine for stubs; iter-4d real backends may need per-probe rate limiting. Not in iter-4c scope.
- **`StepCompleted` AppendEvent failure leaves evidence orphaned.** The evidence row landed, the StepCompleted didn't. Operator can find the evidence via direct DB query but the events list won't reference it. Logged + continue today. Iter-4d could add a reconciliation pass at boot.

## 15. Forward compatibility hooks

- **Parallel walk.** The loop is sequential by design; a future "parallel block" primitive in the template schema can introduce concurrency inside a run without changing the runner's outer goroutine lifecycle.
- **Operator advance.** When iter-4d adds `POST /api/runs/{id}/advance`, it Kicks the engine again — the seenSteps filter naturally skips already-completed steps, so the next iteration picks up where the previous walk halted.
- **Real backends.** fact_probe references `inventoryclient.Resolver` (interface). Swapping `MapResolver` for a real Netbox-backed resolver in serve.go remains a one-line change.
- **Assertion probes.** Category metadata lets iter-4d's first assertion probe branch differently — its StepCompleted carries a Verdict; the projection treats it as a verdict-producing step.

## 16. Non-goals

- Operator-advance endpoint (iter-4d)
- Action.tool-shaped step migration
- Probe timeout enforcement (still read-but-not-enforced)
- Frontend changes
- SSE / live event streaming
- Run-resume after crash
- Step retry endpoint
- Real Alertmanager / Netbox integration
- Checkpoint expression evaluator
- First assertion probe
- Per-node probes
- Parallel-block template primitive
- Composite probe primitive (one step → multiple probe runs)
