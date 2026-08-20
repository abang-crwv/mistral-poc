# qac iter-5k — operator actions endpoint (advance + sign-off) (design)

**Date:** 2026-06-03
**Status:** approved (standing authorization; two forks confirmed with user)
**Predecessor:** the engine walk halts at operator-gated steps (`review`/`ai_assess`/`signoff`) and emits nothing — there's no way to drive a run past them. iter-5i already projects `OperatorAction{signoff}` → `signed_off` in both projections.

## 1. Problem

A canary run halts at `evidence_review`, can't proceed, and never reaches sign-off — the operator has no API to advance gated steps or sign off. This is the "iter-4d debt" the runner comment names. Without it, no run completes the operator workflow.

## 2. Decisions (confirmed with user)
- **One generic endpoint:** `POST /api/runs/{id}/actions` with `{step_id, action, ...}` (extensible to future actions).
- **Sign-off → `signed_off` terminal**, with the operator's final verdict + signer + notes recorded on the `OperatorAction` event (the automated verdict stays visible in step history). Matches the existing projection.

## 3. Design

### 3.1 `OperatorActionPayload` grows (`internal/engine/event.go`)
```go
type OperatorActionPayload struct {
	StepID     string `json:"step_id"`
	Action     string `json:"action"`              // "advance" | "signoff"
	Verdict    string `json:"verdict,omitempty"`   // signoff: operator's final pass|fail
	SignerName string `json:"signer_name,omitempty"`
	SignerRole string `json:"signer_role,omitempty"`
	Notes      string `json:"notes,omitempty"`
}
```
The store's local `operatorActionPayload` mirror (iter-5i) gains `StepID` too (the projection still only branches on `Action`).

### 3.2 Resume mechanism (`internal/engine/runner.go`)
Teach `stepIDsFromEvents` to also mark a step `seen` when an `OperatorAction` event carries that `StepID`. Then a re-`Kick` lets `findNextRunnableStep` skip the actioned gate and proceed to the next gate (or complete). No other walk change — the walk still halts at the next un-actioned gated step.

### 3.3 Endpoint `POST /api/runs/{id}/actions` (`internal/api`)
Body: `{step_id, action, verdict?, signer_name?, signer_role?, notes?}`. Handler `operatorActionHandler(s, eng)`:
1. Load the run's events; load + parse its template (reuse the existing path: RunCreated→`template_id`→`store.GetTemplate`→`template.Parse`).
2. **Validate** (stable error codes, mirroring `runs.go`):
   - run/template not found → 404 `not_found`.
   - `step_id` not a gated step (`review`/`ai_assess`/`signoff`) in the template → 422 `invalid_request`.
   - `step_id` is not the **first un-actioned gated step** (compute gated steps in template order; an `OperatorAction` with a StepID marks that gate actioned) → 422 `invalid_request` ("not the current step"). This enforces gated-step order without the runtime walk internals.
   - `action` mismatched to kind (`advance` requires `review`/`ai_assess`; `signoff` requires `signoff`) → 422.
   - `signoff` missing `signer_name`/`signer_role`, or `verdict` not in the step's `final_verdict_options` → 422.
3. Append the `OperatorAction` event (store projection sets `signed_off` for signoff; bumps `updated_at` for advance).
4. `eng.Kick(r.Context(), runID)` to resume the walk.
5. Respond 200 with the updated run (`{"run": …}`) or 202; **201/200 even if the projection read is degraded** (never 500 after a successful append — the project's standing rule).
Register `POST /api/runs/{id}/actions` in `api.go`; thread `eng` (the route signature already has it via `Router`).

### 3.4 Projection parity
`signoff`→`signed_off` and `advance`→no-status-change already hold in both projections (iter-5i). Add `advance` + a full operator-sequence script to the parity test. `stepIDsFromEvents` is a runner-only change (no projection impact).

## 4. Out of scope
- Actual `ai_assess` AI execution (advanced past for now; a later iter runs the LLM summary).
- `ignore_node`/`retry_step` and other richer operator interventions.
- Real authn/identity (qac has none yet — signer fields are free-text the operator supplies).
- Slack reply on sign-off (`slack_reply: true` in the template — future).
- Frontend (the boundary).

## 5. Testing
- `internal/engine` — `stepIDsFromEvents` marks an `OperatorAction` StepID as seen → `findNextRunnableStep` advances; the existing walk tests stay green.
- `internal/api` — `operatorActionHandler`: advance a review step → 200 + walk resumes (next gate reached); signoff → 200 + run `signed_off`; each validation error (404, not-gated, not-current, wrong-action-for-kind, missing signer/verdict).
- parity test — `advance` script (no status change) + a full `…→OperatorAction{advance}→…→OperatorAction{signoff}` script (ends `signed_off`), store == engine.
- e2e — extend the canonical run: after it halts at `evidence_review`, POST advance → halts at `ai_summary` → advance → halts at `operator_signoff` → POST signoff{verdict,signer} → assert run status `signed_off` and the walk is complete.
- Full `go test ./...` + `cd web && yarn test --run` green.

## 6. Acceptance criteria
1. `OperatorActionPayload` has `StepID` + signoff fields; store mirror updated.
2. `stepIDsFromEvents` marks `OperatorAction` steps seen; the walk advances past an actioned gate on re-Kick.
3. `POST /api/runs/{id}/actions` validates (gated, current, kind-match, signoff fields), appends the event, and resumes the walk; sign-off yields `signed_off`.
4. The e2e drives the full evidence_review→ai_summary→signoff operator sequence to `signed_off`.
5. parity holds (advance + full-sequence scripts); `go test ./...` + `yarn test --run` green.

## 7. Parallelism
Sequential: payload+resume (engine) → endpoint (api) → parity/e2e. Subagent-driven.
