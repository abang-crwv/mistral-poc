# qac iter-5h — L11 verdict (fail-detection): rlcc_action_probe graduates to assertion (design)

**Date:** 2026-06-03
**Status:** approved (design)
**Predecessor:** iter-5g made `rlcc_action_probe` read FLCC `flcc_node_state`, classify each tray, terminal-exit, and recorded an `evaluation_unit` — but stayed gatherer-only (no verdict).

## 1. Problem

iter-5g produces the per-tray FLCC signal and bucket counts but computes **no verdict** — the canary never reaches pass/fail. The consuming machinery already exists and is unused: `StepCompletedPayload.Verdict` (`passed|warning|failed`, `internal/engine/event.go`) with the comment "Assertion probes will set Verdict," and `internal/engine/project.go` already rolls each step's verdict into run status (`worstVerdict` → `verdictToStatus`). `project.go:94` reserves the rack canary verdict explicitly for this probe. iter-5h makes the probe **set** the verdict; the projection consumes it for free.

## 2. Verdict model (approved, then source-grounded via Glean)

The canary is **fail-detection**, not success-certification — gather on both pass and fail, weight fails. The "why did it fail" enrichment is a **separate future probe**, explicitly not iter-5h.

- **fail** = tray is in a fail state.
- **pass** = tray switched to a non-fail terminal state (left the in-progress diagnostic state without failing).
- **all-must-pass** rule: any non-ignored fail → step `failed`; all non-ignored pass → `passed`; any tray still in-progress at step end/timeout → `warning` (the operator's "needs_review").

**Source-grounded FLCC signal for L11** (Glean → `fleet-lifecycle-controller`): an RLCC `l11-fielddiag` action is a noop FLCC placeholder — the node sits at `state="l11-fielddiag"` while AWX runs L11; RLCC moves it onward when done.
- in-progress L11 = `state == "l11-fielddiag"` — this is the **existing `target_state` config** on the step; no new config.
- fail = `state == "fail"`.
- pass = left `l11-fielddiag` without failing — detect via `prev_state == "l11-fielddiag" && state != "fail"` (using `prev_state` avoids calling a tray that hasn't *reached* L11 yet a "pass").

## 3. Design

Small, mostly in the probe. Replaces iter-5g's unused `success_state` config with `target_state`-anchored terminal classification, then computes + sets the verdict.

- **`internal/probe/probe.go`** — add `Verdict string` to `Output` (values `passed|warning|failed`; empty = no verdict, preserving gatherer probes).
- **`internal/probe/rlccactionprobe/classify.go`** — `classifyFLCC` takes the diagnostic in-progress state (the step's `target_state`) instead of `successState`. Order: `state=="fail"`→failed; ignorable→ignorable; `state==diagnosticState`→in-progress; `prevState==diagnosticState`→success (passed); else→in-progress (pre-L11). `outcomeSuccess` keeps its name (means "passed").
- **`internal/probe/rlccactionprobe/rlccactionprobe.go`** — `parseConfig` drops `successState` (reuses `targetState`); poll loop passes `cfg.targetState` to `classifyFLCC`; `summarize` computes the verdict (all-must-pass) and sets `Output.Verdict`. Verdict strings are local consts matching engine's values (probe must not import engine).
- **`internal/engine/runner.go`** — `emitStepCompleted` gains a `verdict` param → `StepCompletedPayload.Verdict`; call site passes `output.Verdict`. **No `project.go` change** — projection already maps verdict → status.

**Verdict rule (in `summarize`):** `failed>0` → `failed`; else `inProgress>0` (= `len(active)` at exit) or `timedOut` → `warning`; else → `passed`. Known edge: zero non-ignored population → vacuously `passed` (plain all-must-pass; min-population guard was declined).

## 4. Error handling / semantics
- Unchanged from iter-5g for missing snapshot/config, query errors, timeout backstop. The only new behavior is a non-empty `Output.Verdict` flowing to `StepCompleted` and thence run status.

## 5. Out of scope (later iters)
- "Why-did-it-fail" enrichment probe (alerts/cause correlation).
- NVLink-domain multi-domain join — one domain per rack (NVL72) holds; the step verdict over the rack's trays *is* the domain verdict.
- Mid-run swap handling; operator-advance/signoff + per-CT evidence-list endpoints.

## 6. Testing
- `classify_test.go` — prev_state/diagnosticState cases (in-progress / passed / pre-L11), fail/ignorable unchanged.
- `rlccactionprobe_test.go` — assert `Output.Verdict`: all-pass→passed, one fail→failed, in-progress-at-timeout→warning.
- `engine_test.go` — a probe returning `Verdict:"failed"` → `StepCompleted.Verdict=="failed"` → projection `StatusFailed`.
- `canonical_integ_test.go` — reseed the e2e for the prev_state model (16 `{PrevState:"l11-fielddiag",State:"ready"}` / 1 `{State:"fail"}` / 1 `{State:"rma"}`), drop the `success_state` injection + `SetFLCCCompleteAfter`, assert `l11_fielddiag` verdict `failed` (1 fail under all-must-pass) end-to-end.
- Full `go test ./...` + `cd web && yarn test --run` green. No template/version change this iter.

## 7. Acceptance criteria
1. `probe.Output` has a `Verdict` field; gatherer probes leave it empty.
2. `rlcc_action_probe` classifies pass via `prev_state==target_state && state!=fail`, fail via `state=="fail"`, ignorable as before, in-progress otherwise; computes an all-must-pass verdict and sets `Output.Verdict`.
3. The runner copies `output.Verdict` into `StepCompletedPayload.Verdict`; the existing projection yields the matching run status.
4. The e2e proves a seeded fail produces a `failed` verdict end-to-end (first real canary pass/fail).
5. `go test ./...` and `cd web && yarn test --run` green.

## 8. Parallelism
None — a short sequential chain (Output.Verdict → classifier → probe verdict compute → runner carries it → tests). Built inline / subagent-driven, no workflow.
