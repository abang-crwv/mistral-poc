# qac iter-5i — reconcile the projections: verdict reaches run status (design)

**Date:** 2026-06-03
**Status:** approved (design)
**Predecessor:** iter-5h made `rlcc_action_probe` set `StepCompleted.Verdict`, but the **store** projection (which the API serves) ignores it — so the verdict never reaches the operator-visible `run.status`.

## 1. Problem

qac has two event projections that have diverged:
- **Store projection** (`internal/store/events.go`, inside `AppendEvent`) writes the denormalized `runs` table the API serves (`getRunHandler`/`listRunsHandler` → `store.ListRuns`). It can only set `pending`/`running`/`failed`; it **explicitly discards `StepCompleted.Verdict`** (`_ = p // … uses none of the fields`) and never sets `passed`/`warning`/`signed_off`.
- **`engine.Project`** (`internal/engine/project.go`) is a pure in-memory fold with the *complete* status logic (`worse`/`verdictToStatus`, all six states, per-step `StepStates`) — but has **zero non-test callers**; it was built for a future `/state` endpoint that never landed.

Result: iter-5h's verdict is stranded on the event; a failed canary still shows `running`. CLAUDE.md says to "update both projections together," but they've drifted.

## 2. Decisions (approved)

- **Store projection stays canonical** (it serves the API, per the denormalized design). Keep `engine.Project` as the exhaustively-testable pure fold (the "spec"). Make them share one logic source so they can't diverge.
- **Run status reflects the automated verdict immediately**: a run is `passed`/`warning`/`failed` as soon as its automated steps yield verdicts (at the `evidence_review` halt). `passed` = "automated checks passed, awaiting sign-off." `signed_off` is the separate terminal set at sign-off. (Matches `engine.Project` today.)

## 3. Design

### 3.1 New `internal/verdict` package — single source of the status vocabulary + rollup
One responsibility, depends on nothing (avoids the import cycle: `engine` imports `store`, so shared logic can't live in `engine`):
- verdict consts `Passed`/`Warning`/`Failed`; status consts `StatusPending`/`StatusRunning`/`StatusPassed`/`StatusWarning`/`StatusFailed`/`StatusSignedOff`
- `Worse(a, b string) string` (rank `"":0, passed:1, warning:2, failed:3`; returns the higher-ranked)
- `ToStatus(verdict string) string` (`passed→passed`, `warning→warning`, `failed→failed`, else `running`)

### 3.2 `engine` re-points to `verdict`
`engine/event.go` + `project.go` use `verdict.*`. To avoid churning all callers/tests, alias the existing exported consts (`const StatusFailed = verdict.StatusFailed`, …) and make `worse`/`verdictToStatus` delegate to `verdict.Worse`/`verdict.ToStatus`. `engine.Project` behavior unchanged — it's now built on the shared helper.

### 3.3 `rlcc_action_probe` uses `verdict` consts
Replace the iter-5h local `verdictPassed/Warning/Failed` consts with `verdict.Passed/Warning/Failed` (the probe may import `verdict` — it depends on nothing). Removes the "must match engine's literals" fragility.

### 3.4 Store projection mirrors `engine.Project` (the core fix)
- **Migration:** add `worst_verdict TEXT NOT NULL DEFAULT ''` to `runs` via the idempotent `addColumnIfMissing` pattern (introspect `pragma_table_info`).
- **StepCompleted:** decode `p.Verdict` (currently discarded); `SELECT worst_verdict FROM runs WHERE id=?`; `nw = verdict.Worse(cur, p.Verdict)`; `UPDATE runs SET worst_verdict=?, status=?, updated_at=? WHERE id=? AND status != 'signed_off'` with `status = verdict.ToStatus(nw)`. (Empty verdict from gatherer steps → `Worse(cur,"")==cur` → status unchanged from the running baseline — matches `engine.Project`.)
- **StepFailed:** also set `worst_verdict='failed'` (status `failed` as today, signed_off-guarded).
- **OperatorAction:** decode the payload; if `action == "signoff"` → `UPDATE runs SET status='signed_off', updated_at=? WHERE id=?` (closes the second parity gap; the *emitting* endpoint is a later iter — only the projection mapping lands here).

### 3.5 Parity test — the anti-divergence guarantee
A test that, for several representative event scripts (gatherer-only, one fail, all-pass, mixed-then-fail, signoff), appends them through a real `store` AND folds them through `engine.Project`, and asserts `store run.status == engine.Project(evs).Status`. This is what makes "can't diverge" real and enforces CLAUDE.md's "update both together."

### 3.6 Re-add the iter-5h e2e run-status assertion
In `TestCanonicalCanary_RLCCActionProbe_EndToEnd`, re-add the `GET /api/runs/{id}` → `run.status == "failed"` assertion (decoding the nested `run` object, not top-level). Now it passes — closing "first real canary pass/fail end-to-end."

## 4. Out of scope
- Surfacing per-step `StepStates`/verdicts in the API or frontend.
- The operator-advance / sign-off **endpoint** (only its `OperatorAction→signed_off` projection mapping is here).
- FE status-tone polish for the newly-reachable `passed`/`warning` (follow-up FE iter).

## 5. Testing
- `internal/verdict` — unit tests for `Worse` (ranking, empty handling) and `ToStatus`.
- `internal/store` — projection tests: StepCompleted with passed/warning/failed verdict sets `runs.status` accordingly; worst-wins across steps; signed_off guard; OperatorAction{signoff}→signed_off; gatherer empty-verdict leaves running.
- `internal/engine` — existing `project_test.go` stays green (now via the shared helper).
- Parity test (§3.5).
- `internal/api` — the re-added e2e run-status assertion.
- Full `go test ./...` + `cd web && yarn test --run` green.

## 6. Acceptance criteria
1. `internal/verdict` exists with `Worse`/`ToStatus` + consts; `engine` and `rlcc_action_probe` use it; no import cycle.
2. The store projection sets `runs.status` from `StepCompleted.Verdict` (worst-wins, signed_off-guarded), sets `worst_verdict`, and maps `OperatorAction{signoff}→signed_off`.
3. A parity test asserts store `run.status == engine.Project(evs).Status` across representative event scripts.
4. The e2e proves a seeded fail → `run.status == "failed"` via the live API.
5. `go test ./...` and `cd web && yarn test --run` green.

## 7. Parallelism
Mostly sequential (verdict package → engine re-point → store projection → tests), though the `verdict` package + engine re-point and the store-projection change could be done by separate subagents. Subagent-driven, no workflow.
