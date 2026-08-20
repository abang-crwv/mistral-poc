# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

**QAgenticCow** (CLI: `qac`, formerly `fwqual` through iter-5b) is a firmware-release canary verification tool for CoreWeave Fleet/FRO. It is a single self-contained Go binary that serves a React SPA and embeds canary templates. Operators create a run against a template (e.g. `fw-release-canary`), the engine drives the steps, and per-rack evidence accumulates on the run.

The repo is a standalone project at `/Users/wpena/coreweave/qac` (a peer of awxctl/cwctl/bmcow). It was formerly developed inside `fleet-ops-sandbox/team/wpena/qac/`, graduated to its own repo on 2026-07-05 with history preserved (`git subtree split`). The parent project at `~/Documents/argo/jason/fwqual/` is prior art (still named fwqual upstream).

## Build / dev / test

```bash
make build         # vite build + go build → bin/qac
make dev           # go run on :8080 + vite on :5173 (proxies /api → :8080)
make test          # go test ./... && cd web && yarn test --run
make lint          # go vet ./... && cd web && yarn lint
make fmt           # gofmt + prettier
```

Single-test invocations:
```bash
go test ./internal/api -run TestCreateRunHandler -v
cd web && yarn test --run src/features/runs/RunsList.test.tsx
```

Useful subcommands:
```bash
qac serve --addr 127.0.0.1:8080            # default port
qac seed-demo                               # populate demo runs
qac template load templates/firmware-release-canary.yaml
```

The default DB lives at `$XDG_DATA_HOME/qac/qac.db` (`~/.local/share/qac/qac.db` on macOS). `qac.db*` is gitignored. Operators with an existing `~/.local/share/fwqual/fwqual.db` from earlier iters can either copy the file across or pass `--db $XDG_DATA_HOME/fwqual/fwqual.db` to keep run history.

## Hard constraints

**Self-contained.** The Go module path is bare `qac` (not `github.com/...`). No `replace` directives. Do not import from sibling argo projects — **port, don't import** — when borrowing logic from `~/Documents/argo/{cwf,inventory,awxctl,jason/fwqual,tiphys/inventory}`.

**Cloud Console copy guide** applies to user-facing strings (error messages, CLI output, UI text): sentence case, "and" not "&", no exclamation points, preserve technical identifiers verbatim.

**PR discipline.** No PR is opened until the canary actually verifies something end-to-end (iter-4 territory). Branch may be pushed; PR is held.

## Backend architecture

```
cmd/qac/              main.go, serve.go, seed_demo.go, template.go (cobra subcommands)
embed_dist.go         //go:embed all:web/dist + templates/*.yaml at module root
internal/store/       SQLite layer (modernc.org/sqlite)
internal/api/         HTTP handlers (net/http ServeMux 1.22+ method-routing)
internal/server/      Server wiring, SPA fallback, request logging
internal/engine/      Event projection (runs table is a projection of events)
internal/template/    YAML loader + validator for fwqual.template/v1
internal/seed/        Embedded-template seeder (runs at every server boot)
templates/            Canonical canary templates (YAML, embedded)
```

**Event sourcing.** The `events` table is the source of truth; `runs` is a denormalized projection kept current in the same transaction as event appends. `RunCreated` is the only event type today — the projection derives `bundle` from `inputs.bundle_tag` and `rack` from `inputs.canary_racks` (with legacy fallback to flat fields for iter-2 events). When adding event kinds, update `internal/engine/project.go` and the projection writer in `internal/store/events.go` together.

**Migrations are idempotent.** Pattern: `CREATE TABLE IF NOT EXISTS …` in `schemaSQL`, then `addColumnIfMissing(ctx)` helpers that introspect via `pragma_table_info(...)`. Always introspect — never `ALTER TABLE` unconditionally.

**HTTP routes** (registered in `internal/api/api.go`):
- `GET /api/health`
- `GET|POST /api/runs`, `GET /api/runs/{id}`
- `GET /api/templates`, `GET /api/templates/{id}`

`POST /api/runs` accepts both `{bundle, rack}` (legacy iter-2) and `{template_id, inputs}` (iter-3a) — `normalize()` in `runs.go` converts legacy to canonical. Stable error codes: `not_found`, `invalid_request`, `internal`, `template_not_found` (422). Successful event append followed by projection-read failure returns **201 with a degraded body** (`warning: "projection_unavailable"`) — never 500 after a successful side-effect.

**Templates.** `fwqual.template/v1` schema lives in `internal/template/types.go`. `Parse` (YAML) → `Validate` (16 rules) → `UpsertTemplate` (stores both `body_yaml` and `parsed_json`). Probes carry explicit `action:` and `checkpoint:` sub-objects (RLCC-style). User-supplied regex on Input fields uses `regexp.Compile` (not `MustCompile`) so validation can surface a friendly error.

### Discovered facts (design constraint)

Template inputs like `instance_type`, `sku`, `variant` should be **discovered from inventory** (via the ported `cwf where`/inventory client), not collected from the operator. The operator supplies `bundle_tag`, `canary_racks`, `requester`, `nitwit_receipts`, `slack_thread`; the system looks up each rack and surfaces type/SKU/variant as evidence on the run.

`templates/firmware-release-canary.yaml` currently marks `instance_type`/`variant`/`requester`/`nitwit_receipts` as `required: false` as an iter-3a workaround so the iter-2 modal (which only collects bundle + rack) still works end-to-end. When iter-3b's dynamic form or an iter-4 fact-discovery pre-step lands, those move to a `discover:` section (or pre-step) and come out of `inputs[]` entirely. See memory `fwqual-discovered-facts`.

## Frontend

`web/` is a Yarn 4 (Berry, immutable installs) workspace. Stack: React 18 + Vite + TS + TanStack Query + react-router + react-hook-form + zod + Tailwind + Radix Dialog. Tests via Vitest + Testing Library + jsdom + MSW. Source under `web/src/{App.tsx, main.tsx, lib, components, features, mocks}`. The Vite dev server proxies `/api/*` to `127.0.0.1:8080`. `web/dist/` is committed (`.gitignore` whitelists `index.html` and `.gitkeep`) so the Go binary can embed it without a build step in CI.

## Working iteratively

Specs and plans land in `docs/superpowers/specs/YYYY-MM-DD-*.md` and `docs/superpowers/plans/YYYY-MM-DD-*.md`. Each iteration produces a working product (build + tests green) before moving on. Current state: **iter-3a complete** (template loader + template-aware run creation). Next candidates are iter-3b (dynamic form + step rail) and iter-4 (engine + first real probe + inventory client port).

When evolving the schema or HTTP surface, add an integration test that loads the on-disk canonical template (`templates/firmware-release-canary.yaml`) and exercises the full path — unit tests with stripped fixtures missed the iter-3a `required: true/false` regression once. See `internal/api/canonical_integ_test.go` for the pattern.
