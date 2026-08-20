# qac iter-5g — FLCC-sourced per-BMN signal + evaluation-unit foundation (design)

**Date:** 2026-06-02
**Status:** approved (design)
**Predecessor:** iter-5f wired `rlcc_action_probe` into the canary's `l11_fielddiag` step, walking per-CT and polling `rlcc_state_completed`.

## 1. Problem

iter-5f got `rlcc_action_probe` running end-to-end, but a long brainstorming pass exposed that its core model is only correct for L10:

1. **Wrong signal layer.** The probe polls `rlcc_state_completed` (RLCC, rack-orchestration). The per-BMN truth — what each tray is actually doing — is **FLCC**, exposed as `flcc_node_state{node,state,step,prev_state,prev_step,workflow,...}`. RLCC tells you about the rack; FLCC tells you about the BMNs.
2. **No fail/ignore handling → unbounded wait.** The probe only recognizes "completed"; it polls forever for a tray that fails or gets pulled. A failed tray actually lands in an observable FLCC state (`state="fail"`), and pulled/abnormal trays land in ignorable FLCC states — both are terminal signals the probe ignores.
3. **Per-CT is the wrong unit for L11/L12.** L10-FD is per-tray; **L11-FD is a whole-rack NVLink-domain diagnostic** (unit = the NVLink domain); L12 is backend fabric. The probe hardcodes per-CT.

iter-5g fixes the foundation: move the per-BMN signal to FLCC, classify each tray (success / failed / ignorable / in-progress) so terminal trays exit the pool, add a step-timeout backstop so nothing hangs, and introduce an explicit evaluation-unit concept. It stays a **gatherer** — no verdict yet.

## 2. Scope

**In:** FLCC `flcc_node_state` reads in `lifecycleclient`; per-BMN classification; terminal-exit; `step.Timeout` backstop; an `evaluation_unit` field on the probe step (recorded, with only `tray` semantics exercised this iter); per-BMN FLCC evidence + aggregate counts.

**Deferred to iter-5h (verdict):** the verdict itself; the NVLink-domain *rollup* (joining `baremetal_node_nvlink_domain_info` and collapsing a domain's trays into one outcome); precise success-vs-advanced semantics; mid-run swap handling. iter-5g records the domain-tagged per-BMN signal that 5h rolls up.

## 3. Design decisions (resolved during brainstorming, validated against source)

### 3.1 Signal is FLCC `flcc_node_state`, not RLCC
The probe's per-tick poll moves from `QueryActionState` (`rlcc_state_completed`) to a new `QueryFLCCState` reading `flcc_node_state`. Confirmed against the exporter source (`fleet-lifecycle-controller/internal/metrics/sox/flcc_node_state.go`): labels include `node, serial, device_slot, state, step, prev_state, prev_step, workflow`. (A second, coarser stateset `baremetal_node_status_flcc_state` exists from baremetal-reporter; we use the richer `flcc_node_state`.)

### 3.2 Fail = `state="fail"`, attributed by `step`/`prev_step`
There is **no per-level fail string**. Confirmed against real exporter output: `flcc_node_state{state="fail", step="test", prev_state="test", workflow="provision"}`. So a tray failed when its observed `state == "fail"`; `step`/`prev_step` records which phase. The FLCC chart (already parsed by `flccclient.FLCCPhase{State, FailState, Timeout}`) supplies the *semantics* (which phase, its failState, its timeout); the metric supplies the *observable*. Chart and metric are different surfaces and must not be conflated.

### 3.3 Ignore set = FLCC states `rma`/`broken`/`debug`/`dev` + workflow `broken-collect`
A tray in any of these is out of the normal production lifecycle (pulled / RMA / under debug) → drop from pool, don't count. Distinct from `fail`.

### 3.4 Classification is coarse + the raw observation is recorded
Because iter-5g is a gatherer (no verdict), it does **not** need a perfect success predicate. Per tray it records the **raw FLCC observation** (`state`, `step`, `prev_state`, `prev_step`, `workflow`) plus a coarse bucket:
- **failed** — `state == "fail"` (certain)
- **ignorable** — `state ∈ {rma,broken,debug,dev}` OR `workflow == "broken-collect"` (certain)
- **success** — `state == cfg.success_state`, where `success_state` is an **optional** config value (the FLCC state that means "passed"). When unset, no observation is bucketed success this iter.
- **in-progress** — otherwise

`failed`, `ignorable`, and (when `success_state` is configured) `success` are terminal → the tray leaves the active pool. `in-progress` keeps polling, bounded by §3.6. The `failed`/`ignorable` buckets are the load-bearing ones for iter-5g and are fully determined; the **precise success criterion** (the exact L11 FLCC state, or the chart-phase-order "advanced past the diagnostic phase" rule) is deferred to iter-5h, which computes the verdict on top of the raw observations recorded here. iter-5g leaves `success_state` unset on the canonical template rather than guess it — happy-path trays ride the §3.6 backstop until 5h sets the criterion.

### 3.5 Explicit `evaluation_unit`, NVLink-domain rollup deferred
The probe step gains `evaluation_unit: tray | nvlink-domain | backend-fabric` (default `tray`). iter-5g records it on evidence/output so 5h knows the intended rollup. Only `tray` changes behavior this iter; the domain *join* (`baremetal_node_nvlink_domain_info{nvlink_domain,node,deviceslot}`) and rollup are 5h. Assumption: one NVLink domain per rack for NVL72 (gb200-4x/gb300-4x); kept as an explicit field so NVL36/multi-domain can be added without rework.

### 3.6 `step.Timeout` backstop so nothing hangs
Terminal classification (3.4) handles the common cases (fail/pull). The residual hang — a tray stuck `in-progress` forever (metric/pipeline gap, FLCC down) — is bounded by enforcing the step's `timeout`: the runner runs the probe under `context.WithTimeout(context.Background(), step.Timeout)`. On expiry the probe returns its partial Output with `timed_out=true` (the path that already exists but was unreachable). This is a **server-side** bound, independent of the request context (the engine deliberately does not thread the HTTP ctx into the runner — that stays true).

## 4. Changes

| File | Change |
|---|---|
| `internal/lifecycleclient/queries/flcc_node_state.promql` | Create: `flcc_node_state{node="{node}"}` (one series/node; read `state`/`step`/`prev_state`/`prev_step`/`workflow` labels). |
| `internal/lifecycleclient/client.go` | Add `QueryFLCCState(ctx, nodes []string) (map[string]FLCCObservation, error)` to the `Client` interface; add the `FLCCObservation{State, Step, PrevState, PrevStep, Workflow string}` type. |
| `internal/lifecycleclient/mapclient.go` | Implement `QueryFLCCState` deterministically + setters so tests can script per-node FLCC observations over ticks. |
| `internal/lifecycleclient/promclient.go` | Implement `QueryFLCCState` against `/api/v1/query`, parsing the label set. |
| `internal/lifecycleclient/parse.go` | Add `renderFLCCStateQuery` + label parsing. |
| `internal/probe/rlccactionprobe/rlccactionprobe.go` | Replace the `QueryActionState` poll with `QueryFLCCState` + the §3.4 classifier; terminal buckets drop from the pool; per-BMN evidence records the raw observation + bucket + `evaluation_unit`; aggregate Output reports counts per bucket + `evaluation_unit` + `timed_out`. Config gains `evaluation_unit` (default `tray`) and optional `success_state`. |
| `internal/engine/runner.go` | Wrap the probe call in `context.WithTimeout(context.Background(), step.Timeout)` (parse `step.Timeout`; if unset/zero, no deadline). |
| `templates/firmware-release-canary.yaml` | `l11_fielddiag` gains `evaluation_unit: nvlink-domain` (a known value). No `success_state` this iter (§3.4) — 5h sets the success criterion. Bump template version. |
| `cmd/qac/serve.go`, `internal/api/api_test.go` | No new ports (lifecycleclient already wired); `QueryFLCCState` rides the existing `LifecycleClient`. |

The `FLCCObservation` raw fields land in per-BMN evidence so iter-5h can compute verdicts without re-querying.

## 5. Error handling / semantics

- **Tray reaches `fail` / ignorable / `success_state`** → terminal, recorded, dropped from pool.
- **Tray stuck in-progress** → bounded by `step.Timeout` (§3.6) → partial Output `timed_out=true`; the still-in-progress trays are recorded as such.
- **`QueryFLCCState` transient error** → existing consecutive-failure tolerance (3 strikes) before the probe errors → `StepFailed` (walk continues).
- **Missing workflow snapshot / config** → error → `StepFailed` (unchanged from iter-5e).
- The probe remains a **gatherer**: `Output.Verdict` stays empty; no pass/fail is computed.

## 6. Testing

- `lifecycleclient`: MapClient `QueryFLCCState` (scripted observations across ticks); PromClient `QueryFLCCState` via httptest with a real `flcc_node_state` label payload.
- `rlccactionprobe`: classifier unit tests for each bucket (fail / each ignorable state / broken-collect workflow / success / in-progress); terminal-exit (failed+ignorable+success trays leave the pool, in-progress persists); `timed_out=true` when a tray never reaches terminal within the (fake-clock) deadline; per-BMN evidence carries the raw observation + bucket + `evaluation_unit`.
- `engine`: a step with `timeout` bounds a never-terminal probe (deterministic via MapClient that keeps a tray in-progress).
- Update the iter-5f canonical e2e test (`internal/api/canonical_integ_test.go`) to seed FLCC observations (a mix of success + one fail + one ignorable) and assert per-bucket evidence/counts instead of the old completed-count assertions.
- Full suite green: `go test ./...` and `cd web && yarn test --run`.

## 7. Out of scope (iter-5h and later)

- The **verdict** (per-unit pass/fail) and how per-BMN buckets roll up.
- **NVLink-domain rollup** — joining `baremetal_node_nvlink_domain_info` and collapsing a domain's trays into one L11 outcome.
- Precise **success vs. advanced-past-phase** semantics using the chart's phase order.
- **Mid-run swap** handling (`device_slot_asset_name_transition_total` / BMN-change-at-deviceslot) and its verdict effect.
- **Operator-advance / signoff** endpoints and the **per-CT evidence list** endpoint (the later items in the frontend-readiness sequence).

## 8. Acceptance criteria

1. `lifecycleclient.QueryFLCCState` reads `flcc_node_state` and returns per-node `{State, Step, PrevState, PrevStep, Workflow}` (MapClient + PromClient).
2. The probe classifies each BMN into failed (`state=="fail"`) / ignorable (`rma|broken|debug|dev` or `broken-collect` workflow) / success (`==success_state` when configured) / in-progress, records the raw observation + bucket + `evaluation_unit` as per-BMN evidence, and drops terminal trays from the pool.
3. A never-terminal tray no longer hangs the step: `step.Timeout` bounds the probe and yields a partial Output `timed_out=true`.
4. `evaluation_unit` is parsed and recorded; `tray` semantics exercised; `nvlink-domain` accepted and tagged (rollup deferred).
5. The probe remains gatherer-only (`Output.Verdict` empty).
6. `go test ./...` and `cd web && yarn test --run` green.

## 9. Parallelism

Minimal — a mostly sequential chain (lifecycleclient FLCC read → probe classifier + terminal-exit → runner timeout → template + e2e). Built inline, no workflow.
