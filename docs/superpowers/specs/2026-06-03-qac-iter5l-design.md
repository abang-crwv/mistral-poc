# qac iter-5l — fail-cause gatherer (design)

**Date:** 2026-06-03
**Status:** approved (design confirmed with user; two forks resolved)
**Predecessor:** `rlcc_action_probe` (iter-5h/5i) detects per-tray FLCC fails and forms the run verdict; iter-5j's `ListEvidence` made a run's per-CT evidence enumerable; `alert_probe` (iter-4b) gathers rack-level alerts but has no node/deviceslot dimension.

## 1. Problem

When `l11_fielddiag` fails a tray, qac records *that* it failed (per-CT FLCC outcome) but nothing about *why* — the firing alerts on that node. The operator (and a future AI agent) need the cause correlated to the exact failed tray. Alerts today are **rack-scoped only** (`alertclient.Alert.Scope = rack id`), so a failed tray can't be tied to its node-level alerts.

## 2. Data-flow model (confirmed with user)

Probes **gather**; an AI agent **interprets**. This iter builds only the gatherer.

```
l11_fielddiag  →  per-CT evidence rows (deviceslot + outcome)        [exists, iter-5h]
       ↓ (read the failed deviceslots via ListEvidence)
fail_cause gatherer step  →  per-failed-node alert correlation        [iter-5l, THIS]
       ↓ (aggregate evidence)
ai_assess step (ai_summary)  →  "why did it fail" narrative           [iter-5m, FUTURE — stub]
```

The `ai_assess` step stays a stub (the engine walk advances past it via an operator `advance`, per iter-5k). Its agent — likely an **MCP server** — is iter-5m. Do not bake an inline-LLM assumption anywhere in this iter.

## 3. Decisions (confirmed with user)
- **Granularity:** extend the alert port to node/deviceslot (not rack-granularity). Cause is pinned to the exact failed tray.
- **Placement:** a **separate gatherer probe** (not folded into `rlcc_action_probe`). Probes gather for the agent to interpret; this keeps `rlcc_action_probe` focused on the verdict.
- **Unconditional:** the gatherer runs every time (gather regardless; pay more attention to fails). On an all-pass run it finds zero failed deviceslots and emits a clean/empty cause set.

## 4. Design — four pieces

### 4.1 Extend the alert port (`internal/alertclient`)
- Add `Deviceslot string \`json:"deviceslot,omitempty"\`` to `Alert` (node-scoped alerts carry it; rack-only alerts leave it empty).
- Add to the `Client` interface:
  ```go
  ListByDeviceslot(ctx context.Context, deviceslot string) ([]Alert, error)
  ```
- `MapAlertClient` gains a deviceslot lookup table + `ListByDeviceslot`. `ListByRack` is unchanged. A `failing-source-deviceslot` sentinel mirrors the existing `failing-source-rack` so error-path tests have a deviceslot analogue.
- `SeedDemoAlerts` (or a sibling `SeedDemoDeviceslotAlerts`) seeds a few deviceslot-keyed alerts whose deviceslots line up with the canonical canary's per-CT fixtures, so the e2e correlates a real failed tray to a real alert.
- **Backward compatible:** `alert_probe` (iter-4b) keeps calling `ListByRack`; it ignores the new field. The compile-time `var _ Client = (*MapAlertClient)(nil)` still holds after the interface grows because `MapAlertClient` implements the new method.

> **Interface-widening fallout (verified):** adding a method to an interface breaks every *other* implementation until it too implements the method. Implementers found: `*MapAlertClient` (production) — gets the real method — and **`fakeAlertClient` in `internal/probe/alertprobe/alertprobe_test.go:20`** (a test mock assigned to `probe.Clients.AlertClient`). That fake MUST gain a `ListByDeviceslot` stub or `internal/probe/alertprobe` stops compiling. No `AlertmanagerClient` exists yet (only doc-comment references), so nothing else breaks.

### 4.2 New `EvidenceReader` port (`internal/probe`)
Add to `probe.Clients`:
```go
EvidenceReader EvidenceReader // iter-5l
```
```go
// EvidenceReader is a read-only port for a gatherer that consumes a prior
// step's per-CT evidence. Satisfied by *store.Store (ListEvidence, iter-5j).
type EvidenceReader interface {
    ListEvidence(ctx context.Context, runID, stepID string) ([]store.Evidence, error)
}
```
`var _ EvidenceReader = (*store.Store)(nil)`. This is how the gatherer learns which deviceslots failed without re-querying FLCC.

### 4.3 New `fail_cause_probe` (`internal/probe/failcauseprobe`)
Gatherer (`Category() == CategoryGatherer`, `Type() == "fail_cause_probe"`, no verdict). `Run`:
1. Require `clients.EvidenceReader` and `clients.AlertClient` non-nil (else error → engine maps to StepFailed, consistent with the other probes).
2. Read `sc.Config["fail_step"]` (string; the step whose fails to explain, e.g. `l11_fielddiag`). Missing/empty → error.
3. `ListEvidence(ctx, sc.RunID, failStep)`; for each row, decode the payload's `outcome` and read the `Deviceslot` column; collect deviceslots where `outcome == "failed"`. (Skip the aggregate row — `Deviceslot == nil`.)
4. For each failed deviceslot, `ListByDeviceslot` → its firing alerts; write one **per-failed-node cause evidence row** (`store.Evidence` with `Deviceslot` set, payload `{deviceslot, alerts, max_severity, count_by_severity}`) via `clients.EvidenceWriter`.
5. Return an **aggregate** `Output` (gatherer, `Verdict` empty): `{probed_at, fail_step, failed_count, with_alerts_count, max_severity, source}`. Reuse `alert_probe`'s `severityRank` rollup logic (port the small helper; don't import across probe packages — copy the few lines, matching the project's port-don't-import norm).

The gatherer reuses the iter-5e `EvidenceWriter` port for per-CT rows (same pattern as `rlcc_action_probe`).

### 4.4 Template + wiring
- `templates/firmware-release-canary.yaml`: insert a `kind: probe` gatherer step **after `l11_fielddiag`, before `evidence_review`**, e.g. `id: fail_cause`, `probe_type: fail_cause_probe`, `action.args: {fail_step: l11_fielddiag}`. Bump template `version`. Update `internal/template/embedded_test.go`'s hardcoded version want (the iter-3a/5f regression lesson).
- `cmd/qac/serve.go`: `reg.Register(failcauseprobe.New())` next to the other probe registrations; ensure the `EvidenceReader` is wired into `probe.Clients` where `Clients` is assembled (it's `*store.Store`, already in scope).

## 5. Out of scope (YAGNI)
- The AI agent / `ai_assess` execution (iter-5m; MCP-server candidate) — stays stubbed.
- Real Alertmanager (still `MapAlertClient`; `AlertmanagerClient` remains the iter-4c placeholder).
- Frontend surfacing of the cause (the frontend boundary).
- BMN/serial enrichment of alerts beyond deviceslot (deviceslot is the stable correlation key; BMN can come later if a probe needs node identity).

## 6. Testing
- `internal/alertclient` — `ListByDeviceslot`: hit, miss (empty slice), `failing-source-deviceslot` → `ErrSourceUnavailable`; `Alert.Deviceslot` round-trips JSON. Fixture-coverage test extended for deviceslot keys.
- `internal/probe/failcauseprobe` — table tests with a map evidence-reader + map alert client: (a) one failed deviceslot with a critical alert → one per-CT cause row + aggregate `max_severity=critical`, `with_alerts_count=1`; (b) failed deviceslot with no alerts → cause row with empty alerts; (c) all-pass run (no failed rows) → zero cause rows, aggregate `failed_count=0`; (d) missing `fail_step` config → error; (e) nil AlertClient/EvidenceReader → error.
- `internal/api` (canonical e2e) — extend `TestCanonicalCanary_RLCCActionProbe_EndToEnd`: after the run halts at `evidence_review`, the `fail_cause` step has run; `GET /api/runs/{id}/evidence?step=fail_cause` returns ≥1 per-failed-node cause row (the failed tray's deviceslot) + aggregate, and the failed deviceslot's row carries its seeded alert. (Seed a deviceslot alert in the test's alert client matching the fail tray's deviceslot.)
- Full `go test ./...` + `cd web && yarn test --run` green.

## 7. Acceptance criteria
1. `alertclient.Alert` has `Deviceslot`; `Client` has `ListByDeviceslot`; `MapAlertClient` implements it with a sentinel failure path; fixtures seed deviceslot alerts.
2. `probe.Clients` has an `EvidenceReader` port satisfied by `*store.Store`.
3. `fail_cause_probe` (gatherer, no verdict) reads the fail step's failed deviceslots and emits per-failed-node cause evidence + an aggregate; runs unconditionally (clean set on all-pass).
4. The canonical template runs `fail_cause` after `l11_fielddiag`; embedded version test updated; probe registered in `serve.go`.
5. The e2e proves a real failed tray gets its alert cause via `GET …/evidence?step=fail_cause`.
6. `ai_assess` remains a stub. `go test ./...` + `yarn test --run` green.

## 8. Parallelism
Mostly sequential (port → probe → template/wiring → e2e), but 4.1 (alertclient) and 4.2 (EvidenceReader port) are independent and can land in either order before the probe (4.3) depends on both. Subagent-driven.
