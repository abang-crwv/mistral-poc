# qac iter-5j — per-CT evidence-list endpoint (design)

**Date:** 2026-06-03
**Status:** approved (standing authorization through the backend sequence)
**Predecessor:** iter-5g/5h made `rlcc_action_probe` write one evidence row per CT (with `deviceslot` + `outcome` + raw FLCC observation) plus an aggregate; iter-5i made the verdict reach run status. But evidence is only fetchable one-at-a-time by id (`GET /api/runs/{id}/evidence/{eid}`) — the per-tray rows aren't enumerable.

## 1. Problem

The per-CT signal qac now produces is invisible to any client: there's no way to *list* a run's (or a step's) evidence. A future frontend per-CT view, and operators inspecting per-tray outcomes, need a list endpoint. This is a prerequisite for surfacing the per-CT data.

## 2. Design

### 2.1 Route
`GET /api/runs/{id}/evidence` — list evidence for a run. Optional query `?step=<stepID>` filters to one step. Distinct from the existing single-fetch `GET /api/runs/{id}/evidence/{eid}` (different ServeMux 1.22 pattern; no conflict).

### 2.2 Store accessor
`ListEvidence(ctx context.Context, runID, stepID string) ([]Evidence, error)` in `internal/store/evidence.go`:
- `stepID == ""` → all evidence rows for the run.
- else → rows for that run+step.
- `ORDER BY created_at, deviceslot` so per-CT rows sort stably under their step (NULL deviceslot — the aggregate — sorts first via `deviceslot IS NULL DESC, deviceslot` or simply `created_at, deviceslot`).
- Returns `[]Evidence` (empty slice, not error, when none).

### 2.3 Handler
`evidenceListHandler(s *store.Store)` for the route. Because `store.Evidence.Payload` is `json:"-"` (served raw by the single-fetch handler), the list uses a response DTO that inlines the payload as `json.RawMessage`:
```go
type evidenceListItem struct {
	ID          string          `json:"id"`
	StepID      string          `json:"step_id"`
	Deviceslot  *string         `json:"deviceslot,omitempty"`
	ContentType string          `json:"content_type"`
	CreatedAt   int64           `json:"created_at"`
	Payload     json.RawMessage `json:"payload"`
}
```
Response envelope: `{"evidence": [ …items… ]}` (consistent with `getRunHandler`'s `{"run":…,"events":…}` style). Read `?step` via `r.URL.Query().Get("step")`. Empty array for a run with no evidence (no 404 — REST list semantics). All current evidence is `application/json`, so inlining `Payload` as `json.RawMessage` is valid; if a non-JSON payload ever exists it would produce invalid JSON — acceptable for now since every producer writes JSON (note for a future content-type guard).

### 2.4 Registration
Add `mux.HandleFunc("GET /api/runs/{id}/evidence", evidenceListHandler(s))` in `internal/api/api.go`, next to the existing evidence route.

## 3. Out of scope (YAGNI)
- Pagination (a run has tens of rows — 18 per-CT + a handful of aggregates).
- `?deviceslot` filter (the FE can group client-side; `?step` covers the per-CT-of-a-step view).
- Frontend consumption (the frontend boundary).

## 4. Testing
- `internal/store` — `ListEvidence`: all rows for a run; `?step` filter; ordering (aggregate vs per-CT); empty run.
- `internal/api` — `evidenceListHandler`: lists a run's evidence; `?step` filters; empty array for no evidence; payloads inlined as JSON.
- Extend `TestCanonicalCanary_RLCCActionProbe_EndToEnd` (or a focused test): after the run, `GET /api/runs/{id}/evidence?step=l11_fielddiag` returns the 18 per-CT rows (deviceslot set) + 1 aggregate (deviceslot null).
- Full `go test ./...` + `cd web && yarn test --run` green.

## 5. Acceptance criteria
1. `store.ListEvidence(runID, stepID)` returns ordered evidence; `stepID==""` = all, else filtered; empty slice when none.
2. `GET /api/runs/{id}/evidence` (+ optional `?step=`) returns `{"evidence":[…]}` with each row's `payload` inlined as JSON.
3. The e2e proves `l11_fielddiag` exposes its 18 per-CT rows + aggregate via the endpoint.
4. `go test ./...` and `cd web && yarn test --run` green.

## 6. Parallelism
None — store accessor → handler → registration → tests, sequential. Subagent-driven.
