# qac rebuild — design spec

**Date:** 2026-05-27
**Status:** Draft, pending user sign-off
**Location:** `fleet-ops-sandbox/team/wpena/qac/`
**Predecessor:** `~/Documents/argo/jason/fwqual/` (v0.0.1, Go + chi + templ + HTMX + SQLite)

---

## 1. Purpose

Rebuild qac from scratch at `team/wpena/qac/` as a self-contained Go binary that serves an embedded React SPA. The product goal is unchanged from the predecessor: replace the Slack "ping Wallace" firmware-canary handoff with a single tool that drives a canary release through capacity check → AWX zap → L10/L11/L12 perftests → alert sweep → per-rack evidence review → operator sign-off, and emits an auditable artifact.

This spec covers a **walking skeleton** only — the minimum end-to-end slice that exercises every architectural layer with one truthful feature ("list runs"). Subsequent iterations layer real probes, templates, the run-detail view, and the eventual five-checklist RLCC/FLCC model on top of this spine without disturbing it.

## 2. Constraints

1. **Self-contained.** No `replace` directives. No sibling-repo imports. Module path is bare `qac`. Integrations that exist elsewhere (`~/Documents/argo/cwf`, `~/Documents/argo/tiphys/inventory`, `~/Documents/argo/awxctl`, the original `~/Documents/argo/jason/fwqual`) are *ported from*, not *depended on*. If a port becomes a heavy duplication, surface the trade-off — don't silently break the constraint.
2. **Single binary.** `make build` produces one executable at `team/wpena/bin/qac` (on `$PATH` per sandbox convention). The React build artifact is embedded into the Go binary via `//go:embed`.
3. **No external services.** SQLite file at `$XDG_DATA_HOME/qac/qac.db` (override with `--db`). No auth, no Prometheus, no Glean, no AWX in the skeleton. Stubs only.
4. **Aligned with Cloud Console frontend conventions.** Stack and copy guide follow `coreweave/cloud-console/docs/frontend-style-guide.md` and the in-product copy guide (sentence case; `and` not `&`; no exclamation points; preserve technical identifiers verbatim).

## 3. Stack

| Layer | Choice | Pinned version (matches cloud-console where applicable) |
|---|---|---|
| Frontend framework | React + TypeScript + Vite | React 18.2, TS 5.3, Vite 5.4 |
| Styling | Tailwind 3.4 + `tailwind-styled-components` + `tailwindcss-animate` | match cloud-console |
| UI primitives | Radix UI (à la carte: `dialog`, `dropdown-menu`, `popover`, `select`, `tabs`, `tooltip`) | match cloud-console |
| Variants | `class-variance-authority` (`cva`) | 1.0.0-beta.1 |
| Tables | `@tanstack/react-table` (+ `react-virtual` when row counts demand) | 8.17.3 |
| Data fetching | `@tanstack/react-query` | latest stable |
| Routing | `react-router-dom` 6 | match cloud-console |
| Client state | `zustand` | 5.0.11 |
| Forms | `react-hook-form` + `zod` + `@hookform/resolvers` | match cloud-console |
| Toasts | `sonner` | match cloud-console |
| Icons | `lucide-react` | match cloud-console |
| Dates | `date-fns` | match cloud-console |
| Tests | `vitest` + `@testing-library/react` + `jsdom` + `msw` | match cloud-console |
| Lint | ESLint (`airbnb-typescript`) + Prettier + sort-imports plugin | match cloud-console |
| Tooling | Node ≥20.10, Yarn 4, `mise` for version management | match cloud-console |
| Backend | Go ≥1.22, `net/http` + `ServeMux`, `modernc.org/sqlite` (pure-Go, no CGO), `spf13/cobra` | (diverges from cloud-console's Python; justified in §4.1) |
| Single-binary glue | `//go:embed all:web/dist` | stdlib |

### Stack divergence: Go backend (not Python/FastAPI/GraphQL)

Cloud Console's backend is Python (FastAPI + Django + GraphQL). qac diverges deliberately:

- **Single-binary deploy.** `team/wpena/bin/qac` is on `$PATH` already; no venv, no `uv run`, no Python version drift.
- **Typed state machine.** Go's exhaustive `switch` on an event-kind enum gives compile-time coverage of the run-state fold (see §6); Python's `match` is close but the toolchain around it is weaker.
- **Concurrency model.** When real probes arrive, parallel-rack execution maps cleanly to goroutines + channels.
- **Reference code exists.** ~80% of the predecessor is Go, so the rebuild benefits from existing patterns even when "starting fresh."

### Stack subset rationale

Adopted from cloud-console: React+TS+Vite, Tailwind+Radix+CVA, TanStack Table, React Router, Zustand, react-hook-form+zod, sonner, lucide-react, date-fns, vitest+testing-library+msw, ESLint+Prettier.

Skipped from cloud-console (not justified for a sandbox tool): Apollo Client + GraphQL, Sentry, Mixpanel, Storybook (defer), OpenFeature/Flagsmith, AWS SDKs, `@monaco-editor`, `@rjsf`, `react-google-recaptcha`, `framer-motion` (defer), `recharts` (defer).

Added (not in cloud-console because they use Apollo for fetching): `@tanstack/react-query` as the REST-cache equivalent.

## 4. Architecture

Single Go process. The Go side owns the HTTP server, SQLite store, event-fold projection, and the JSON API. The React side is a Vite-built SPA, compiled to `web/dist/`, embedded into the Go binary at compile time via `//go:embed all:web/dist`, and served from `/`. Non-asset routes match `/api/*` and return JSON.

In **dev**, Vite runs on `:5173` and proxies `/api/*` → `:8080` (Go); hot-reload works because Vite owns the SPA serving. In **prod**, the Go binary serves both the SPA (from embedded FS) and the API.

```
┌──────────────────────────┐
│   browser ── React SPA   │   served from / (embedded in prod, vite in dev)
└─────────┬────────────────┘
          │ /api/*    JSON, error envelope
┌─────────▼────────────────┐
│  internal/api  handlers  │   the only crossing into engine from HTTP
└─────────┬────────────────┘
┌─────────▼────────────────┐
│  internal/engine         │   Project(events) → RunState (pure function)
└─────────┬────────────────┘
┌─────────▼────────────────┐
│  internal/store          │   AppendEvent / ListRuns / LoadEvents (SQLite, WAL)
└──────────────────────────┘
```

**Boundaries (enforced by directory + code review, not yet by tooling):**

- `internal/api` is the only thing that calls `internal/engine` from HTTP.
- `internal/engine` is the only thing that calls `internal/store`.
- Inside `web/`, the `features/` layer talks to `lib/api` only — components stay dumb.
- Each feature directory owns its types, hooks, and tests; nothing in `features/runs/` is imported from outside `features/runs/` except its route registration.

### Directory layout

```
team/wpena/qac/
├── cmd/qac/
│   └── main.go              # cobra root: serve | seed-demo | (later: template load)
├── internal/
│   ├── store/               # SQLite, migrations, runs+events tables
│   ├── engine/              # event fold → RunState projection
│   ├── api/                 # /api/* handlers, JSON encoding, error envelope
│   ├── server/              # http.Server wiring, embed.FS for SPA, middleware
│   └── seed/                # seed-demo data
├── web/
│   ├── index.html
│   ├── package.json         # yarn 4
│   ├── vite.config.ts       # dev proxy /api → :8080
│   ├── tailwind.config.ts
│   ├── tsconfig.json
│   ├── eslint.config.js     # airbnb-typescript + prettier
│   ├── src/
│   │   ├── main.tsx
│   │   ├── App.tsx          # Router + QueryClient + Toaster
│   │   ├── routes/          # route configs only
│   │   ├── features/
│   │   │   └── runs/        # RunsList.tsx, useRuns.ts, runs.types.ts, *.test.tsx
│   │   ├── components/      # shared UI primitives (Badge wrapping Radix/CVA)
│   │   ├── lib/             # api client (fetch wrapper + types), utils
│   │   ├── styles/          # globals.css
│   │   └── mocks/           # MSW handlers for dev/test
│   └── dist/                # vite build output, embedded by Go (gitignored)
├── docs/
│   └── superpowers/
│       ├── specs/           # this file + later
│       └── plans/           # writing-plans output
├── Makefile                 # ui | backend | build | dev | test | lint | fmt | clean
├── go.mod                   # module qac
├── go.sum
├── .gitignore               # web/dist, web/node_modules, qac.db*, ../bin/qac
└── README.md
```

## 5. Data model

Two SQLite tables, event-sourced. The `events` table is the source of truth; `runs` is a denormalized projection kept current by the store on every `AppendEvent`.

```sql
CREATE TABLE runs (
  id            TEXT PRIMARY KEY,         -- ULID (sortable by creation time)
  template_id   TEXT NOT NULL,
  created_at    INTEGER NOT NULL,         -- unix epoch seconds
  updated_at    INTEGER NOT NULL,
  status        TEXT NOT NULL,            -- pending | running | passed | warning | failed | signed_off
  rack          TEXT NOT NULL,            -- comma-joined for multi-rack canaries
  created_by    TEXT NOT NULL DEFAULT 'wpena'
);

CREATE TABLE events (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id        TEXT NOT NULL REFERENCES runs(id),
  seq           INTEGER NOT NULL,         -- monotonic per run, gap-free
  kind          TEXT NOT NULL,            -- RunCreated | StepStarted | StepCompleted | OperatorAction | ...
  payload       TEXT NOT NULL,            -- JSON blob, schema by kind
  occurred_at   INTEGER NOT NULL,
  UNIQUE(run_id, seq)
);

CREATE INDEX idx_events_run ON events(run_id, seq);
CREATE INDEX idx_runs_updated ON runs(updated_at DESC);
```

**Asymmetry by design.** `GET /api/runs` (list view) reads `runs` directly — a single indexed scan. `GET /api/runs/:id` (detail view) loads events from the log and folds them. List = scan; detail = fold.

## 6. Engine + projection

**Single writer.** `internal/store` exposes one mutation method:

```go
func (s *Store) AppendEvent(ctx context.Context, runID string, kind string, payload any) error
```

In one SQL transaction it (a) computes the next `seq`, (b) inserts into `events`, (c) updates the matching `runs` row based on the kind. There is no other path to mutate state. This is the linchpin that keeps the `runs` projection in sync with the event log without a background reconciler.

**Event types.** One Go struct per kind, marshalled as the `payload` JSON. Walking-skeleton scope only emits `RunCreated`; the rest are declared so the projection's switch is exhaustive on day one.

```go
type Event struct {
    Seq        int64
    Kind       string
    OccurredAt time.Time
    Payload    any
}

type RunCreated     struct{ TemplateID, Rack, CreatedBy string }
type StepStarted    struct{ StepID string }
type StepCompleted  struct{ StepID string; Verdict string; Signals []Signal }
type OperatorAction struct{ Action string; Reason string }
```

Kind constants live in `internal/engine/kinds.go` (e.g. `const KindRunCreated = "RunCreated"`).

**Projection.** A pure function:

```go
type RunState struct {
    ID, TemplateID, Rack, CreatedBy string
    Status     string
    CreatedAt  time.Time
    UpdatedAt  time.Time
    StepStates map[string]StepState  // empty in skeleton; populated by probes later
}

func Project(events []Event) RunState
```

No I/O, no DB. Called by `internal/api` when serving `GET /api/runs/:id`. The list endpoint doesn't fold — it reads `runs` directly.

**Status transitions** (exhaustive `switch` on `Event.Kind`):

```
RunCreated                    → status = pending
StepStarted                   → status = running
StepCompleted                 → status = worst({prior, verdict})       (passed < warning < failed)
OperatorAction{Action=signoff}→ status = signed_off
```

The exhaustive `switch` is enforced by code review for now; a small linter check can be added later if it drifts.

**Why pure-function projection matters for testing.** `Project` takes a `[]Event` and returns a `RunState`. Tests don't need a database, a server, or a clock — they construct event slices in code and assert on the output. Fast (sub-millisecond), deterministic, no flakes.

## 7. API surface (walking skeleton)

Three endpoints, all under `/api`:

| Method | Path | Body | Returns |
|---|---|---|---|
| `GET` | `/api/runs` | — | `{ "runs": Run[] }` sorted by `updated_at` desc |
| `GET` | `/api/runs/:id` | — | `{ "run": Run, "events": Event[] }` (consumed in next iteration; stub OK) |
| `GET` | `/api/health` | — | `{ "ok": true, "version": "...", "db_path": "..." }` (version from `runtime/debug.ReadBuildInfo()` — module version + vcs.revision baked in at build time) |

`Run` JSON shape:

```json
{
  "id": "01HZX...",
  "template_id": "firmware-release-canary",
  "created_at": "2026-05-27T15:04:05Z",
  "updated_at": "2026-05-27T15:04:05Z",
  "status": "pending",
  "rack": "as1-r07",
  "created_by": "wpena"
}
```

All timestamps are ISO-8601 strings at the API boundary, unix-int seconds in storage. Run IDs are ULIDs.

**Error envelope** (every non-2xx response):

```json
{ "error": { "code": "not_found", "message": "Run not found" } }
```

Codes are stable, short `snake_case` strings: `not_found`, `invalid_request`, `internal`. The frontend's `lib/api` reads `error.message` for Sonner toasts and `error.code` for branching.

**Not in the skeleton:** mutations (POST/PUT/DELETE), SSE/long-poll for live events, template-load command, probe execution, authentication. Each is its own follow-up iteration.

## 8. Frontend feature shape

### `web/src/lib/api.ts`

Single typed fetch wrapper. Used by every feature. `apiPost`/`apiPut` follow when mutations land.

### `web/src/features/runs/`

- **`runs.types.ts`** — TypeScript mirror of the Go `Run` shape. Hand-maintained; swap to `openapi-typescript` codegen when hand-sync becomes painful.
- **`useRuns.ts`** — TanStack Query hook with a 5-second `staleTime` (long enough to avoid refetch storms on tab switches, short enough for new runs to surface within a heartbeat).
- **`RunsList.tsx`** — the page. TanStack Table columns: `id` (monospaced ULID), `rack`, `status` (Badge with CVA tone variants), `template_id`, `created_at` (relative via `date-fns`), `created_by`. Sortable on all columns, rack-filter input above the table. Rows clickable → navigate to `/runs/:id` (stub page in the skeleton; real detail view ships in the next iteration). Empty state copy: *"No runs yet. Create one to get started"*. Error state: inline error banner with retry button + Sonner toast for the technical message.
- **`statusToTone.ts`** — maps `RunStatus` → Badge tone (`pending`→`neutral`, `running`→`info`, `passed`→`success`, `warning`→`warn`, `failed`→`danger`, `signed_off`→`neutral`). Status semantics are domain-local, so this stays in `features/runs/`.

### `web/src/components/Badge.tsx`

CVA-driven variant component. Tones: `neutral | info | warn | danger | success`. Pure presentation; carries no domain knowledge.

### `web/src/App.tsx`

Router + `QueryClientProvider` + `<Toaster />` from sonner. Routes: `/` → `RunsList`; `/runs/:id` → stub element for the next iteration.

### MSW (`web/src/mocks/handlers.ts`)

Mirrors the Go API. Returns three fixture runs in different statuses so the table has visual variety in `dev:mocks` mode (no backend running). Enabled only when `VITE_MOCKS=true` — same pattern as cloud-console.

### Copy guide applied throughout

- Sentence case on column headers (`Status`, `Created`, not `STATUS`).
- "Create run", not "Create New Run".
- No exclamation points anywhere.
- Empty-state and error messages reviewed against the in-product copy guide (clarity > marketing tone).
- Technical identifiers (ULIDs, rack names like `as1-r07`) preserved verbatim, not sentence-cased.

## 9. Testing

### Go side — three layers

- **Unit (pure).** `engine.Project` — feed event slices, assert on the projected `RunState`. No DB, no HTTP. Sub-millisecond per case.
- **Store (integration).** Each test gets a fresh SQLite file in `t.TempDir()` (auto-cleaned). Exercises migrations + `AppendEvent` + `ListRuns` + `LoadEvents`. Helper: `newTestStore(t)`.
- **API (handler).** `httptest.NewServer` + real store with a temp DB; tests issue HTTP requests against `/api/runs` and assert on the JSON response. This is the only cross-package layer — it serves as the contract test for the frontend.

No mocking framework. When the engine eventually needs a clock, it gets a `Clock` interface (Go idiom for testability: pass dependencies as interfaces, default to a real impl, swap a fake in tests).

### React side — Vitest + Testing Library + MSW

- `RunsList.test.tsx` — MSW returns three fixture runs; assert rows, rack-filter narrowing, empty-state copy, error-state copy.
- `api.test.ts` — `apiGet` against MSW handlers returning success, 404 with envelope, 500 with envelope; assert `ApiException.code` matches.
- No tests for `Badge.tsx` — pure presentation, covered transitively.

`make test` runs both: `go test ./...` and `yarn --cwd web test`.

## 10. Build + dev pipeline

Top-level `Makefile` orchestrates both sides:

```make
.PHONY: ui backend build dev test lint fmt clean

ui:           # vite build → web/dist (consumed by embed.FS)
	cd web && yarn install --immutable && yarn build

backend:      # depends on ui having run, so dist/ exists for embed
	go build -o ../bin/qac ./cmd/qac

build: ui backend

dev:          # two procs; vite proxies /api → :8080
	@echo "Go on :8080, Vite on :5173 (proxies /api → :8080)"
	@bash -c 'trap "kill 0" EXIT; \
	  go run ./cmd/qac serve --addr 127.0.0.1:8080 & \
	  cd web && yarn dev & \
	  wait'

test:
	go test ./...
	cd web && yarn test

lint:
	go vet ./...
	cd web && yarn lint

fmt:
	go fmt ./...
	cd web && yarn format

clean:
	rm -rf web/dist web/node_modules ../bin/qac
```

Output binary at `team/wpena/bin/qac` (sandbox `$PATH`).

**`//go:embed` for the Go beginner.** `internal/server/static.go` will hold:

```go
//go:embed all:web/dist
var distFS embed.FS
```

The `//go:embed` directive is a *compile-time* instruction — the Go compiler reads `web/dist` from disk at build time and bakes the files into the binary. Nothing reads `web/dist` at runtime. The `all:` prefix means "include files beginning with `.` or `_`" (Vite emits some — for example, `.vite/manifest.json`). At runtime we hand `distFS` to `http.FileServer` after `fs.Sub(distFS, "web/dist")`.

## 11. Acceptance criteria

The walking skeleton is **done** when all of the following are true:

1. `make build` succeeds from a fresh clone. Produces a single binary at `team/wpena/bin/qac`.
2. `./qac seed-demo` creates one demo run in the SQLite file.
3. `./qac serve` starts on `:8080`, serves the React SPA at `/`, returns the seeded run at `GET /api/runs`.
4. Visiting `http://localhost:8080` shows the runs list with the seeded row; rack filter narrows results; status badge is styled per the CVA tone variants; relative time renders via `date-fns`; copy passes the copy-guide checklist (sentence case; no `!`; `and` not `&`; technical identifiers preserved).
5. `make dev` runs Vite on `:5173` against the live Go backend; hot-reload works on `.tsx` and `.ts` edits.
6. `make test` passes — Go unit + store + API layers green; React component + api-client tests green.
7. `make lint` passes — `go vet` clean, ESLint clean.
8. `README.md` has a quickstart that walks a fresh developer through `make build` → `seed-demo` → `serve` in under five commands.
9. No `replace` directives in `go.mod`; no sibling-repo imports; no `tiphys/*` references anywhere.

## 12. Out of scope (future iterations, after acceptance)

Each is its own design+plan cycle. Ordered roughly by likely sequence, not commitment:

1. Run detail page with event timeline (consumes `GET /api/runs/:id`).
2. New-run form (template selection, rack input, `react-hook-form` + `zod` validation).
3. Template loader (`qac template load <yaml>`).
4. First real probe — likely `baseline_pre` against a stubbed `internal/inventoryclient` (ported from `~/Documents/argo/tiphys/inventory`).
5. SSE for live event streaming on the detail page.
6. The five RLCC/FLCC checklists from the 2026-05-25 refit (`gb200-rack-provision-v4`, `gb200-rack-power-cycle`, `gb200-l11-fielddiag`, `gb200-rack-hpc-verification-v4`, `move-bmns-to-ready`).
7. Per-node evidence table (the eventual *product* — sortable/filterable across racks, with active alerts, HPC verification failures, transient counts, Grafana explore links).
8. AI summary + operator sign-off → Slack draft text.

## 13. Integration sources (forward note)

When the iterations above are designed and built, look at and **port from** (don't import) the matching projects under `~/Documents/argo/`:

- `~/Documents/argo/jason/fwqual/` — the predecessor; first reference for engine/store/state-machine patterns.
- `~/Documents/argo/tiphys/inventory` — node/rack inventory; informs `internal/inventoryclient` and any rack-aware feature.
- `~/Documents/argo/cwf` — fleet CLI (`cwf where`, capacity checks); informs `capacity_check` and rack-location lookups.
- `~/Documents/argo/awxctl` — AWX dispatcher; informs how `zap_canary` / FLCC workflow probes would call AWX.
- Also relevant in the same tree: `nodeprofile`, `kgraph`, `warpkey`, `projectdj`, `gb200/Teleport/Infiniband` helpers.

**Pattern:** scan the matching project for prior art *before* designing fresh; port the relevant types/queries/logic into qac's `internal/`; do not add upstream paths to `go.mod`. If a port turns into heavy duplication, flag the trade-off — don't silently break the self-contained constraint.

## 14. Decisions log

- **Fresh start vs port-as-is.** Fresh start. The predecessor is reference material; the new tree is not constrained by old design choices.
- **Self-contained module.** No `replace` directives. Module path is bare `qac`.
- **Walking skeleton vs full v1.** Skeleton, with the runs list as the spine slice. Subsequent iterations add probes, templates, the detail page, and the five-checklist model.
- **Backend language.** Go, not Python — single-binary deploy, typed state machine, concurrency model for future parallel probes, existing reference code.
- **Frontend stack.** React + TS + Vite + Tailwind + Radix + CVA + TanStack Table + TanStack Query + React Router + Zustand + react-hook-form + zod + sonner + lucide-react + date-fns. Subset of cloud-console's stack, omitting GraphQL/Apollo/Sentry/Mixpanel/Storybook/etc.
- **Data fetching.** TanStack Query (cloud-console uses Apollo because they're GraphQL; we're REST, so this is the natural pair).
- **Module path.** Bare `qac` — not `github.com/...`. The project isn't imported externally.
- **Location.** `fleet-ops-sandbox/team/wpena/qac/` (not under `team/wpena/projects/`). Sandbox CI still applies because it scopes to `team/**`.
- **Storage.** `modernc.org/sqlite` (pure-Go, no CGO). Event-sourced; two tables; `runs` is a denormalized projection of `events`.
- **Copy guide.** Cloud Console in-product copy guide applies to all UI text.
