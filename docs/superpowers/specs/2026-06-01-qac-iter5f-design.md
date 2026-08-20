# qac iter-5f — wire rlcc_action_probe into the canary (design)

**Date:** 2026-06-01
**Status:** approved (design)
**Predecessor:** iter-5e (`docs/superpowers/specs/2026-06-01-qac-iter5e-design.md`) shipped `rlcc_action_probe` as a standalone library, deliberately unwired.

## 1. Problem

iter-5e shipped `rlcc_action_probe` and registered it in `serve.go`, but **no template step invokes it**. Its `Run()` has only ever executed against hand-written port fakes; the engine integration test substitutes a `mockProbe`. So the real probe has never run through the real engine against real map-backed lifecycle/inventory clients, and there is no API path that triggers it. The runner↔probe seam has zero coverage.

iter-5f closes that gap: make the probe run for real, end-to-end, on the canonical template — the first genuine exercise of the probe outside fakes.

## 2. Scope

**Minimal wiring only.** Relax one validation rule, rewrite one template step, add one end-to-end test. No verdict aggregation, no frontend, no new probes, no `/api/probes` endpoint — those remain deferred (see §7).

## 3. Design decisions (resolved during brainstorming)

### 3.1 Config carrier + validation — Approach A (relax XOR, use `action.args` map)

The runner (`internal/engine/runner.go:127`) already populates `StepContext.Config` from `step.Action.Args` **when `Args` is a `map[string]any`**. The probe reads `match_handler`, `target_state`, `poll_interval` from `StepContext.Config`. So a `probe_type` step can carry its config in `action.args` (map form) with **zero runner change** — this is exactly what the iter-5e engine test (`TestEngine_Kick_PopulatesStepContext_iter5e`) already exercises, and what the iter-5e plan anticipated ("relaxing the XOR rule is an iter-5f wiring concern").

The only blocker is the validator: `internal/template/validate.go:96` rejects any probe step that has **both** `action` and `probe_type` (`if hasAction == hasProbeType`).

**Rejected alternative (Approach B):** add a dedicated `config:` field to the step and keep the XOR rule intact. Cleaner long-term (no vestigial `action.tool`), but needs a new schema field, a runner change, and a rewrite of the iter-5e test. Deferred until a third `probe_type` probe makes a real `config:` field worth it.

**Accepted wart:** with Approach A, a `probe_type` step's `action.tool` is vestigial — the probe ignores it. Documented inline in the template.

### 3.2 Step placement — replace `l11_fielddiag` in place

Replace the existing `l11_fielddiag` step rather than adding a new one, **keeping its `id` unchanged** so `evidence_review.sources` and `ai_summary.context_steps` (which reference `l11_fielddiag`) keep working — now pointing at richer per-CT evidence.

Rationale (operator's framing): the probe gathers L11 evidence regardless of pass/fail — "if it fails tell me why, if it passes grab the L11 link anyway and still read it." A separate pass/fail gate is redundant. And **nothing is blocking**: the engine does not halt on `StepFailed` — it continues the walk (verified by the unknown-probe-type test, where `step_c` completes after `step_b` fails). So removing the step's checkpoint costs no flow control. The "tell me why" emerges where it belongs: `ai_summary` reads the per-CT evidence (including any `timed_out`/incomplete CTs) and synthesizes it; `operator_signoff` is the final verdict gate.

The current `l11_fielddiag` and the probe verify the *same underlying thing* (l11-fielddiag completion) — the probe is the richer per-CT version. Replacing trades a rack-level pass/fail marker for per-CT evidence; that marker was never load-bearing.

## 4. Changes

### 4.1 `internal/template/validate.go` — relax the rule

Current logic (probe kind):
```go
hasAction := st.Action != nil
hasProbeType := strings.TrimSpace(st.ProbeType) != ""
if hasAction == hasProbeType {                 // both absent OR both present → error
    return fmt.Errorf("step %q: probe requires exactly one of action.tool or probe_type", st.ID)
}
if hasAction {
    // require Action.Tool, require Checkpoint, validate checkpoint targets
}
```

New logic:
- Reject only when **both** `action` and `probe_type` are absent.
- When `probe_type` is set: an accompanying `action` is a config carrier — **no** `tool` requirement, **no** `checkpoint` requirement.
- When `action` is set **without** `probe_type` (classic action probe): unchanged — require `Action.Tool` and `Checkpoint` and a valid checkpoint target.

Error message updated to reflect "requires action.tool or probe_type" (at least one).

### 4.2 `templates/firmware-release-canary.yaml` — rewrite `l11_fielddiag`, bump version

Bump `version: 7 → 8`. Replace the `l11_fielddiag` step body:

```yaml
  - id: l11_fielddiag            # id unchanged → evidence_review/ai_summary refs stay valid
    kind: probe
    probe_type: rlcc_action_probe
    title: "L11 fielddiag — whole-rack NVLink-domain diagnostic (per-CT walk)"
    racks: "{{ inputs.canary_racks }}"
    action:
      tool: rlcc                 # vestigial; the probe ignores tool (Approach A wart)
      args:                      # map form → runner maps into StepContext.Config
        match_handler: l11-fielddiag
        target_state: l11-fielddiag
        poll_interval: 30s
    timeout: 4h
```

No `checkpoint`. `evidence_review.sources` and `ai_summary.context_steps` are **unchanged** (they already list `l11_fielddiag`).

### 4.3 `internal/api/canonical_integ_test.go` — first true end-to-end test

Add a test that:
1. Loads the **on-disk** canonical template (`templates/firmware-release-canary.yaml`), exercising the relaxed validator on real content (the pattern that caught the iter-3a `required:` regression).
2. `POST /api/runs` with a real `rlcc_workflow` input (snapshot resolved into `RunCreated.payload.rlcc_workflow`).
3. Lets the live engine walk the run through the **real** `rlccactionprobe` and **real** map-backed lifecycle/inventory clients (the API test harness already registers the probe at `api_test.go:43` and wires the four ports).
4. Asserts `l11_fielddiag` produced **per-CT evidence rows** plus the **aggregate `Output`** (e.g. via `GET /api/runs/{id}` and `GET /api/runs/{id}/evidence/{eid}`).

This is the first execution of `rlcc_action_probe` end-to-end and the acceptance gate for the iter.

### 4.4 Touchpoints to verify (change only if a check fails)

- **Boot seeder** reseeds the embedded template — confirm it upserts at `version: 8` (the seeder runs every boot; `UpsertTemplate` is keyed by id+version).
- **Web MSW template fixture / FE tests** — confirm no frontend test hard-asserts template `version == 7` or the old `l11_fielddiag` shape; sync the fixture if so (iter-5e precedent: "web — sync MSW fixture to template v7").
- **`serve.go`** — already registers `rlccactionprobe.New()` and wires `InventoryResolver`/`LifecycleClient`/`EvidenceWriter`/`EventEmitter`; **no change expected**.

## 5. Error handling / semantics

- **CT never completes within `timeout`:** probe returns a partial `Output` with `timed_out=true` (iter-5e behavior); the step **completes** (no halt), evidence captures the incomplete CTs.
- **Missing `rlcc_workflow` snapshot on the run:** probe errors → `StepFailed`; the walk continues to `l11_check`, `l12_*`, etc. (engine never halts on failure).
- **Malformed step (neither `action` nor `probe_type`):** still rejected by the validator.
- **Step-level L11 pass/fail:** intentionally removed. `ai_summary` + `operator_signoff` are the verdict authorities. Per-step verdict aggregation stays deferred.

## 6. Testing

- `internal/template/validate_test.go` — add cases: `probe_type` + `action` (map args, no checkpoint) **passes**; neither present **fails**; classic action-only path (tool + checkpoint) still enforced.
- `internal/api/canonical_integ_test.go` — the §4.3 end-to-end test.
- Full suite (`go test ./...` + `cd web && yarn test --run`) stays green.

## 7. Out of scope (still deferred)

- Per-step verdict aggregation (rack verdict from per-CT outcomes).
- Frontend per-CT view on RunDetail.
- `GET /api/probes` probe-library / introspection endpoint.
- Additional probes (`metrics_baseline_probe`, `flcc_node_zap_probe`, `ai_summary_probe`).
- A dedicated `config:` step field (Approach B) — revisit when a third `probe_type` probe lands.

## 8. Acceptance criteria

1. The validator accepts a probe step carrying both `probe_type` and `action` (map args, no checkpoint); still rejects a probe step with neither.
2. `templates/firmware-release-canary.yaml` is `version: 8`; `l11_fielddiag` uses `probe_type: rlcc_action_probe` with map `action.args`; `evidence_review`/`ai_summary` references resolve.
3. An end-to-end test drives a real run through the real `rlccactionprobe` and asserts per-CT evidence rows + aggregate `Output` for `l11_fielddiag`.
4. `go test ./...` and `cd web && yarn test --run` are green.
5. No change required to `serve.go` (probe already registered + wired).

## 9. Parallelism

None. This is a 3-change sequential chain (relax validation → wire step → prove end-to-end), each gating the next. Built inline; no workflow.
