# qac frontend integration — Phase 1: design standup on seed data (design)

**Date:** 2026-06-04
**Status:** approved (design confirmed with user; four architecture forks + two cleanup calls settled)
**Predecessor:** the backend is feature-complete through iter-5l (canary drives probe→verdict→evidence→operator-signoff). The current `web/` frontend is a bare placeholder (a centered runs list + run-detail, Tailwind, no app shell). A high-fidelity design app was delivered (`HANDOFF.md` + a Babel-in-browser React prototype) to become the real frontend.

## 1. Goal

Stand up the **entire delivered design** inside the existing Vite app, on **seed data**, with real routing — every sidebar nav item visible and navigable. Nothing is wired to the backend in Phase 1; in-page interactions may be inert (the user explicitly accepts dead clicks). **Phase 2** (separate spec) wires Canary FW Release to the live API.

This is the first of two sub-projects:
- **Phase 1 (this spec):** design → Vite, app shell + sidebar nav, all pages render on seed data, stubs visible. Build + lint + smoke tests green.
- **Phase 2 (later):** Canary FW Release (RunsList/RunDetail/New run/sign-off) wired to the real backend with typed `.ts` adapters; everything else stays seed.

## 2. Settled decisions
1. **Sequencing:** whole design first (Phase 1), then wire Canary (Phase 2).
2. **Architecture:** keep **react-router** (real URLs; `/runs/:id` already shareable) + **TanStack Query** + `lib/api.ts` + existing hooks (retained for Phase 2; unused in Phase 1).
3. **Language:** design pages stay **`.jsx`** (copied as-is per the handoff "keep components as-is"); the Phase 2 Canary data layer will be strict `.ts`.
4. **Canary scope (Phase 2):** full — list + detail + new run + sign-off.
5. **Auth gate:** **hidden, not dropped** — keep `Login.jsx`/`LoginCodec.jsx` and the codec art; the app boots straight to the shell (no startup gate); the header lock button lets you *view* the codec login on demand. Real SSO replaces it later.
6. **Tweaks panel:** dropped (prototype-only design tool).

## 3. Architecture

### 3.1 App shell + routing
A react-router layout route renders the design's `Header` + `Sidebar` with an `<Outlet/>` for the active page. The Sidebar's `useState`-string nav (`current`/`onNav`) is rewired to router navigation (`useNavigate`) + active-detection (`useLocation`). Nav → route table (all 17 items remain visible):

| Route | Page (Phase 1) | Source |
|---|---|---|
| `/` → redirect `/canary` | — | — |
| `/canary` | `RunsList` | seed (`RUNS`) — real in Phase 2 |
| `/runs/:id` | `RunDetail` | seed — real in Phase 2 |
| `/node-tracker` | `NodeTracker` | seed (`NODE_FIXES`) |
| `/rack-tracker` | `RackTracker` | seed (`RACK_FIXES`) |
| `/jira` | `Jira` | seed (`JIRA`) |
| `/knowledge` | `Knowledge` | seed (in-file consts; Ask box degraded) |
| `/grafana` | `GrafanaLinks` | static config |
| `/silences` | `Silencer` | seed (`PAGERDUTY`) |
| `/notifications` | `WebPush` | seed (`WEB_PUSH`) |
| `/l11-fielddiag`, `/templates`, `/probes`, `/agents`, `/history`, `/reports`, `/integrations`, `/settings` | `<StubPage>` | none ("not built yet") |

`/runs/:id` is preserved (it already exists). Index `/` redirects to `/canary`. A catch-all `*` renders `<StubPage>` (or a 404 styled in-design).

### 3.2 File structure (`web/src/`)
```
main.tsx                     # imports tokens CSS + @fontsource; mounts <App/>
App.tsx                      # QueryClientProvider + RouterProvider + Toaster
app/AppShell.tsx             # layout route: <Header/> + <Sidebar/> + <Outlet/>
app/routes.tsx               # the §3.1 route table
design/components.jsx        # UI primitives + icons map + helpers (exported)
design/Shell.jsx             # Header + Sidebar (rewired to router)
design/seed.js               # ex-data.jsx constants, exported (no window globals)
design/StubPage.jsx          # generic "not built yet" page
features/canary/RunsList.jsx # design RunsList (seed in P1)
features/canary/RunDetail.jsx
features/canary/NewRunModal.jsx
features/tracking/Trackers.jsx        # NodeTracker + RackTracker
features/{jira,knowledge,grafana,silences,webpush}/<Page>.jsx
auth/{Login,LoginCodec}.jsx           # viewable via lock button; not a gate
styles/colors_and_type.css            # design tokens (font-face block replaced — §3.4)
```
Retained for Phase 2 (untouched, unused in P1): `lib/api.ts`, `features/runs/use*.ts` hooks, MSW `mocks/`.
Removed (superseded by the design): the bare `App.tsx` body, old `features/runs/RunsList.tsx`/`RunDetail.tsx`/`wizard/*` **and their tests**, `components/Badge.tsx`/`Dialog.tsx` (design has its own). The retained hooks live on for Phase 2; if a removed test imported a retained hook, the hook stays.

### 3.3 The two handoff seams (the mechanical bulk)
1. **Babel-in-browser → Vite.** Drop `fwqual.html`'s `unpkg` + `<script type="text/babel">` tags; Vite bundles React. The inline `<App/>` becomes `App.tsx` (router + providers). `index.html` stays the Vite entry.
2. **`window` globals → ES modules.** Every design file ends with `Object.assign(window, {…})` and reads deps globally (`Icon`, `Card`, `icons`, `RUNS`, …) — **including `React` itself** (they assume the unpkg global, e.g. `const { useState } = React`). For each file: add `import React, { useState, … } from 'react'` (and `import ReactDOM` only where needed), add `export` to its components/helpers, add `import { … } from '@/design/components'` (and `from '@/design/seed'` for data) at the top, and delete the `Object.assign(window, …)` block. Seed constants move from `data.jsx` to `design/seed.js` as named exports. `stepStatesFor`, `FIX_STATUSES`, `BUNDLE_STAGES`, `GRAFANA_LINKS` stay as presentation helpers/consts (not server data).

**TS interop:** `.jsx` files are imported by `.tsx` (e.g. `App.tsx` → `design/Shell.jsx`). `tsconfig` needs `allowJs: true` (and `checkJs: false`) so Vite/tsc accept the untyped design files without type-checking them. Vite transpiles `.jsx` natively.

### 3.4 Design system / styling
The design is **pure inline styles + CSS custom-property tokens** (`var(--cw-*)`, `--font-*`, `--status-*`) — no Tailwind. Import `styles/colors_and_type.css` in `main.tsx`; keep components verbatim. Tailwind stays installed but unused by design code (harmless; may be removed in a later cleanup). The header **theme toggle** (light/dark by re-pointing CSS vars at runtime) is kept — portable and self-contained; lift its handful of effects from the old `<App/>` into `App.tsx`/`AppShell.tsx`.

**Fonts.** The token CSS only `@font-face`s **Source Sans Pro** (bundled OTFs, relative URLs); `--font-display` (Plus Jakarta Sans), `--font-ui` (Inter), `--font-mono` (JetBrains Mono) have no face and fall back to system fonts. For a faithful, self-hosted build: replace the OTF `@font-face` block in `colors_and_type.css` with **`@fontsource`** imports (woff2) for all four families — `@fontsource/source-sans-3`, `@fontsource/plus-jakarta-sans`, `@fontsource/inter`, `@fontsource/jetbrains-mono` — imported in `main.tsx`. Point `--font-sans` at `"Source Sans 3"`. Retire the bundled OTFs (woff2 is far smaller — keeps the embedded binary lean).

**Assets.** `logo-mark-blue.svg`, `fro-hq-avatar.png`, and the codec art (`porsche-ascii.png`, `mustache-cut.png` — used only by `LoginCodec`) go to `public/assets/`; fix the relative `src="assets/…"` URLs to absolute `/assets/…`.

### 3.5 Degraded-for-Phase-1 seams
- **Knowledge Ask box** calls `window.claude.complete` (a design-host-only helper that won't exist). Guard it: the page renders (glossary + source index from its in-file consts), and the Ask box shows a graceful disabled/"model not connected" state instead of throwing. The offline `rankSources`/`renderMd` helpers stay.
- **Dead interactions are acceptable** (per user): seed-page buttons (filters, row menus, New run submit, sign-off, etc.) may be inert in Phase 1. They must not crash the page — guard any handler that would call a missing global/endpoint.
- **Auth lock** (§2.5): the header lock button toggles a viewable codec-login overlay; it does not gate or persist.

## 4. Build / embed
`make build` → `vite build` → `web/dist`; the Go binary embeds `all:web/dist` (unchanged). Only `web/dist/index.html` stays committed (gitignore unchanged); JS/CSS/fonts/assets regenerate at build. Watch embedded size: woff2 fonts + the ~1.3MB `porsche-ascii.png` (kept, per §2.5). Vite `server.proxy` for `/api` stays (Phase 2 uses it). `make dev` (go run :8080 + vite :5173) still works.

## 5. Out of scope (Phase 2 or later)
- Any backend wiring (Canary or otherwise) — Phase 2.
- Typed `.ts` conversion of design pages — only the Phase 2 Canary data layer is `.ts`.
- Real auth/SSO, real RAG for Knowledge, Web Push service-worker, real Jira/PagerDuty/Grafana integrations.
- Building the stub pages (`L11 FieldDiag`, Templates, Probes, Agents, History, Reports, Integrations, Settings).
- Removing Tailwind / dependency cleanup.

## 6. Testing
- **Smoke tests** (Vitest + Testing Library): `AppShell` renders header + sidebar with all nav sections; each route mounts its page component without throwing (render-only, seed data); `<StubPage>` renders for a stub route; the catch-all renders. No data-fetch assertions (nothing is wired).
- Remove obsolete tests tied to deleted components (old RunsList/wizard tests).
- `make build` succeeds and the binary serves the SPA at `/` (manual/`validator-tester` smoke).
- `make lint` (eslint over `.jsx`/`.tsx`) green; ESLint config must accept `.jsx` (and not require types in design files).

## 7. Acceptance criteria
1. `make build` produces `../bin/qac`; running it serves the full design at `/`.
2. Every sidebar item navigates to its route and renders (seed pages look real; stubs clearly read "not built yet"); theme toggle works; codec login is viewable via the lock button but does not gate startup.
3. No `window`-global data reads remain in design code (all via ES-module imports from `design/seed`); no `unpkg`/Babel `<script>` tags; tweaks panel absent.
4. All four font families self-hosted via `@fontsource` (woff2); no references to the retired OTFs.
5. `make test` (Go unchanged + web smoke tests) and `make lint` green. Only `web/dist/index.html` committed under `web/dist/`.

## 8. Parallelism
Sequential spine (scaffold → tokens/fonts → components/Shell → globals→modules per page → routing → smoke tests), but the page conversions (§3.3 step 2) are independent per file and can be batched. Subagent-driven.
