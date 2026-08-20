# qac frontend integration — Phase 2: wire Canary FW Release to the live backend (design)

**Date:** 2026-06-04
**Status:** approved (design confirmed with user; three forks + two controller decisions settled)
**Predecessor:** Phase 1 stood the whole design up in Vite on seed data (`docs/superpowers/specs/2026-06-04-qac-frontend-phase1-design.md`). The Canary pages (`features/canary/{RunsList,RunDetail,NewRunModal}.jsx`) currently read `@/design/seed` globals. The retained data layer — `lib/api.ts`, the `features/runs/use*.ts` hooks, MSW `mocks/` — is in place and unused.

## 1. Goal

Make **Canary FW Release** real: RunsList, RunDetail, New-run, and Sign-off/advance read and write the live backend. A typed `.ts` **adapter layer** maps real API responses to the shapes the design pages expect; TanStack Query hooks in the route wrappers fetch + adapt and pass data to the pages as **props**. Every other nav item stays on seed data (Phase 1).

## 2. Settled decisions
1. **Evidence:** faithful now — map the `alert_baseline` (alert_probe) evidence to the design's existing evidence panel. The per-CT FLCC + `fail_cause` data (iters 5g–5l) is **out of scope** for Phase 2 (needs new UI; a noted follow-up).
2. **New-run workflow:** add an RLCC workflow picker to the modal, populated from `GET /api/rlcc/workflows`.
3. **Sequencing:** Stage 1 = reads (RunsList + RunDetail). Stage 2 = writes (New run → POST, Sign-off/advance → actions).
4. **Feed mechanism (controller decision):** route wrappers (`CanaryRoute`/`RunDetailRoute`) call hooks and pass adapted data as props; the 3 design pages are edited to accept props instead of reading `@/design/seed`.
5. **Missing fields (controller decision):** `currentStep`, `verdict`, `requester` are derived client-side in the adapter (no backend change).

## 3. Real API ↔ design shape mapping

| Design shape (page reads) | Real source | Adapter mapping |
|---|---|---|
| `Run` `{id,bundle,template_id,status,racks[],requester,created_by,created_at,updated_at,currentStep,verdict?}` | `GET /api/runs` (list) and `GET /api/runs/{id}` (`{run,events,racks}`) | direct: id/bundle/template_id/status/created_by/created_at/updated_at. `racks` ← detail `racks` or `inputs.canary_racks` (list). `requester` ← `inputs.requester`. `currentStep` ← derived from events (last `StepStarted` with no later `StepCompleted`/`StepFailed`; null in list view where events aren't loaded). `verdict` ← from status (`failed`→`fail`; `passed`/`signed_off`→`pass`; else undefined). |
| `EVENTS[runId]` `[{seq,kind,occurred_at,detail}]` | detail `events[]` `{seq,kind,payload,occurred_at}` | map fields; **`detail`** ← `detailFor(kind, payload)` — a pure formatter producing the design's human string per kind (RunCreated/FactsDiscovered/StepStarted/StepCompleted/StepFailed/OperatorAction/BMNIgnored/…). Unknown kinds → generic `kind` label. |
| `FACTS[runId]` `{instance_type,sku,variant,region,nodes,source}` | `GET /api/runs/{id}/facts` → `{facts:{…}}` | map `instance_type`/`sku`/`variant`/`region` directly; `source` ← a label (`"inventory · cwf where"`); `nodes` ← best-effort (`racks.length`×CT-count if unavailable as a fact, else omit). Facts may be empty for older runs → render the panel's empty state. |
| `ALERT_EVIDENCE[runId]` `{probed_at,source,alerts_clear,max_severity,per_rack}` | `GET /api/runs/{id}/evidence?step=alert_baseline` → the alert_probe aggregate row's `payload` | the alert_probe Output **already has this exact shape** (`per_rack`, `max_severity`, `alerts_clear`, `probed_at`, `source`) — pass the aggregate payload through. No alert_baseline evidence yet → panel empty state. |
| `TEMPLATE` + `FLAT_STEPS` + `STEP_INDEX` | `GET /api/templates/{id}` (qac.template/v1, `steps[]`) | build `FLAT_STEPS` from `template.steps` (`{id,title,kind,tool?,live,timeout,desc}`); `STEP_INDEX` from order. Render the rail **ungrouped** (pass `showRailGroups={false}`) to avoid a fragile group-mapping — grouping is cosmetic and the prop already exists. `stepStatesFor` stays the design's pure helper, fed the adapted run + flat steps. |

`RUN_STATUS` (in `design/components.jsx`) must cover the real status vocabulary (`pending`/`running`/`passed`/`warning`/`failed`/`signed_off`) — add any missing pill style.

## 4. Architecture

```
features/canary/
  adapters.ts          # pure real→design mappers + detailFor(); fully unit-tested (.ts)
  types.ts             # design-shape TS types (Run, RunEvent, Facts, AlertEvidence, Step)
  useCanaryRuns.ts     # TanStack: GET /api/runs → adapted Run[]            (Stage 1)
  useCanaryRun.ts      # TanStack: GET /api/runs/{id} → {run,events,racks}  (Stage 1)
  useRunFacts.ts       # TanStack: GET /api/runs/{id}/facts → Facts          (Stage 1)
  useRunAlertEvidence.ts # TanStack: GET …/evidence?step=alert_baseline      (Stage 1)
  useCanaryTemplate.ts # TanStack: GET /api/templates/{id} → FLAT_STEPS      (Stage 1)
  useCreateCanaryRun.ts# TanStack mutation: POST /api/runs                   (Stage 2)
  useRunAction.ts      # TanStack mutation: POST /api/runs/{id}/actions      (Stage 2)
  routes.tsx           # CanaryRoute / RunDetailRoute: fetch+adapt, handle loading/error, pass props
```
Reuse `lib/api.ts` (`apiGet`/`apiPost`) and, where shapes match, the existing `features/runs/use*.ts` (e.g. `useRlccWorkflows` for the picker). The 3 design pages change their signatures to receive data via props (e.g. `RunsList({ runs, loading, error, onOpenRun, onNewRun })`, `RunDetail({ run, events, facts, evidence, steps, onBack, onSignoff, onAdvance })`, `NewRunModal({ open, onClose, onCreate, workflows })`) and drop their `@/design/seed` imports.

**Loading / error:** route wrappers render the design's existing spinner/empty affordances (or a minimal design-styled fallback) while `isLoading`, and a design-styled error panel on `isError` (using `ApiException.message`). Pages assume data present once mounted with props.

### Stage 1 — reads
Adapters + the five read hooks + wrapper wiring + page prop refactors for RunsList & RunDetail. RunsList shows live runs (poll/refetch); clicking a row routes to `/runs/:id`; RunDetail shows live run/events/facts/alert-evidence/steps. `make build`/`test`/`lint` green; the page renders real data end-to-end against a running backend.

### Stage 2 — writes
`useCreateCanaryRun` (POST `/api/runs` with `{template_id:'firmware-release-canary', inputs:{bundle_tag, canary_racks, requester, rlcc_workflow}}`) wired to NewRunModal's `onCreate`; the modal gains the workflow picker (`useRlccWorkflows`). `useRunAction` (POST `/api/runs/{id}/actions`) wired to RunDetail's Sign-off button (→ `{action:'signoff', step_id, verdict, signer_name, signer_role}`) and advance affordances for `review`/`ai_assess` gates (→ `{action:'advance', step_id}`). On success, invalidate the run query so the UI reflects the new state. Errors surface via `sonner` toast (already mounted) using `ApiException.message`.

## 5. Out of scope (follow-ups)
- Per-CT FLCC + `fail_cause` UI in RunDetail (the richer evidence) — separate design.
- The non-Canary nav pages (still seed) and the 8 stubs.
- Real auth, Knowledge RAG, Web Push, Jira/PagerDuty/Grafana live integrations.
- Backend changes (everything maps client-side; `currentStep`/`verdict` derived).
- Grouped step rail (render ungrouped).

## 6. Testing
- **`adapters.test.ts`** (the priority — pure functions): run mapping (racks/requester/currentStep/verdict derivation, including list-without-events), `detailFor` per event kind, facts mapping + empty, alert-evidence passthrough + empty, template→FLAT_STEPS/STEP_INDEX.
- **MSW** (`mocks/handlers.ts`): return **real-shaped** responses for `/api/runs`, `/api/runs/:id`, `/api/runs/:id/facts`, `/api/runs/:id/evidence?step=`, `/api/templates/:id`, `/api/rlcc/workflows`, `POST /api/runs`, `POST /api/runs/:id/actions`.
- **Component/route tests:** CanaryRoute renders rows from a mocked list; loading + error states; RunDetailRoute renders run/steps/facts/evidence; NewRunModal create → POST fires with the right body; Sign-off → action POST fires.
- Phase 1 smoke tests stay green. Full `go test ./...` (unchanged) + `cd web && yarn test --run` + `yarn lint` + `make build`.

## 7. Acceptance criteria
1. Against a running backend, `/canary` lists real runs and `/runs/:id` shows real run/events/facts/alert-evidence/steps (Stage 1).
2. New run (with workflow picker) creates via `POST /api/runs` and the run appears; Sign-off/advance drive the run via `POST /api/runs/{id}/actions` and the UI updates (Stage 2).
3. Adapters are pure + unit-tested; pages receive data via props; no `@/design/seed` import remains in the 3 Canary pages.
4. Loading and error states render (no white screen / crash on a failed fetch).
5. `make build`, `cd web && yarn test --run`, `yarn lint` green; other nav items still render on seed data.

## 8. Parallelism
Stage 1 then Stage 2. Within Stage 1: `adapters.ts` + `types.ts` first (everything depends on them), then the hooks (independent, parallel), then the wrapper+page wiring. Subagent-driven.
