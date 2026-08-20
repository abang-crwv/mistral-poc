# qac frontend Phase 1 — design standup on seed data Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`).

**Goal:** Bring the delivered high-fidelity design into the existing Vite app on seed data — react-router app shell (Header + Sidebar), all 17 nav items navigable (seed pages + stubs), `window`-globals converted to ES modules — with `make build`/`make test`/`make lint` green. No backend wiring (Phase 2).

**Architecture:** Layout route renders the design `Header`+`Sidebar` with `<Outlet/>`; the Sidebar keeps its `current`/`onNav` prop interface and `AppShell` maps route↔nav-key. Design pages are copied as `.jsx` verbatim, with their trailing `Object.assign(window,{…})` blocks replaced by ES `import`/`export` (including `import React`). Seed data moves to `design/seed.js`. Tokens via `colors_and_type.css` + `@fontsource` (woff2).

**Tech Stack:** React 18 + Vite 5 (`@vitejs/plugin-react-swc`) + react-router 6 + TanStack Query 5 + Vitest. Yarn 4 (Berry, immutable). Spec: `docs/superpowers/specs/2026-06-04-qac-frontend-phase1-design.md`.

---

## Key facts (verified)
- Staged design source (gitignored, stable): `web/design-handoff/*.jsx`, `web/design-handoff/colors_and_type.css`, `web/design-handoff/assets/`, `web/design-handoff/fwqual.html`. Read verbatim page bodies from here.
- Existing Vite app: `web/src/{App.tsx,main.tsx}`, alias `@`→`src` (vite.config.ts + tsconfig paths). `main.tsx` imports `@/styles/globals.css`.
- eslint flat config (`web/eslint.config.js`) lints only `src/**/*.{ts,tsx}` — **`.jsx` files are not linted** (intended). `dist/**` ignored. No eslint change needed for `.jsx`.
- tsconfig is strict, `jsx: react-jsx`, no `allowJs`. Importing `.jsx` from `.tsx` requires `allowJs: true` + `checkJs: false`.
- Root `.gitignore`: `web/dist/*` with `!web/dist/.gitkeep`, `!web/dist/index.html`; `web/node_modules/`. Only `web/dist/index.html` is committed.
- Design files assume globals: `React` (e.g. `const { useState } = React`), `ReactDOM`, primitives/icons from `components.jsx`, seed consts from `data.jsx`. Each ends with `Object.assign(window,{…})`.
- Design `<App/>` (`fwqual.html:110-194`): `useTweaks` (drop), theme effect (`document.documentElement.dataset.theme`), accent effect (sets `--cw-blue`/`-700`/`-800`), auth gate (`Login`/`LoginCodec` via `sessionStorage`), nav-string switch, `NewRunModal`, `TweaksPanel` (drop).
- Design pages + props: `RunsList(onOpenRun,onNewRun,density)`, `RunDetail(runId,onBack,showRailGroups)`, `NewRunModal(open,onClose,onCreate)`, `NodeTracker()`/`RackTracker()` (Trackers.jsx), `Jira(onOpenRun)`, `Knowledge()`, `GrafanaLinks()`, `Silencer()`, `WebPush()`, `Login(onAcknowledge)`, `LoginCodec(onAcknowledge)`.
- Sidebar nav keys (`Shell.jsx`): `runs, nodetracker, racktracker, l11dance, jira, knowledge, templates, probes, agents, grafana, evidence(History), reports, silencer, webpush, integrations, settings`. Badge: `runCount` on `runs`; `liveSilences` on `silencer` (derived from `PAGERDUTY`).
- `make build` = `vite build` (outDir `web/dist`) + `go build`; Go embeds `all:web/dist`. `make test` = `go test ./...` + `cd web && yarn test --run`. `make lint` = `go vet` + `cd web && yarn lint`.

---

## Conversion recipe (used by Tasks 2, 3, 5 — every design `.jsx` file)
For each file copied from `web/design-handoff/`:
1. Add at top: `import React from 'react';` plus the hooks it uses (scan for `useState`/`useEffect`/`useMemo`/`useRef` — the files write `const { useState } = React;`; replace that line with a proper import, e.g. `import React, { useState, useEffect } from 'react';`).
2. Add `import { <names> } from '@/design/components';` for every primitive/helper/icon-map it references that is defined in `components.jsx` (`Icon, icons, Card, Input, Button, StatusPill, Eyebrow, Badge, LogoMark, fmtRel, fmtAbs, shortId`, …). Scan the file for which it uses.
3. Add `import { <CONSTS> } from '@/design/seed';` for every seed constant it reads (`RUNS, EVENTS, FACTS, ALERT_EVIDENCE, STEP_GROUPS, stepStatesFor, NODE_FIXES, RACK_FIXES, FIX_STATUSES, JIRA, PAGERDUTY, WEB_PUSH, GRAFANA_BASE, GRAFANA_LINKS, BUNDLES, BUNDLE_STAGES`, … — whichever it uses).
4. Delete the trailing `Object.assign(window, { … });` block.
5. Add `export default <ComponentName>;` for the file's primary component; for multi-component files (e.g. `Trackers.jsx` → `NodeTracker`, `RackTracker`) use named `export function …`.
6. Do **not** restyle or change markup. Inline styles + `var(--cw-*)` tokens stay verbatim.

---

## Task 1: scaffold — deps, tokens, fonts, tsconfig, blank shell builds

**Files:** Modify `web/package.json`, `web/tsconfig.json`, `web/src/main.tsx`; Create `web/src/styles/colors_and_type.css`, `web/public/assets/*`; Modify `web/src/App.tsx` (temporary blank)

- [ ] **Step 1: Add font deps** (Yarn 4). From `web/`:
```bash
yarn add @fontsource/source-sans-3 @fontsource/plus-jakarta-sans @fontsource/inter @fontsource/jetbrains-mono
```

- [ ] **Step 2: Copy tokens + assets**
```bash
cp web/design-handoff/colors_and_type.css web/src/styles/colors_and_type.css
mkdir -p web/public/assets && cp web/design-handoff/assets/* web/public/assets/
```

- [ ] **Step 3: Replace the OTF @font-face block with @fontsource** — in `web/src/styles/colors_and_type.css`, delete the 12 `@font-face { font-family: "Source Sans Pro"; src: url("fonts/…otf") … }` lines (≈ lines 8-19). Change the `--font-sans` token to lead with the @fontsource family:
```css
  --font-sans: "Source Sans 3", "Inter", system-ui, sans-serif;
```
(Leave `--font-display`/`--font-ui`/`--font-mono` as-is — they already name "Plus Jakarta Sans"/"Inter"/"JetBrains Mono", which @fontsource registers under those exact names.)

- [ ] **Step 4: Wire fonts + tokens in `main.tsx`** — replace `web/src/main.tsx` with:
```tsx
import React from 'react';
import ReactDOM from 'react-dom/client';
// Self-hosted fonts (the design only bundled Source Sans; the tokens also
// name Plus Jakarta Sans / Inter / JetBrains Mono — load all four as woff2).
import '@fontsource/source-sans-3/400.css';
import '@fontsource/source-sans-3/600.css';
import '@fontsource/source-sans-3/700.css';
import '@fontsource/plus-jakarta-sans/600.css';
import '@fontsource/plus-jakarta-sans/700.css';
import '@fontsource/plus-jakarta-sans/800.css';
import '@fontsource/inter/400.css';
import '@fontsource/inter/500.css';
import '@fontsource/inter/600.css';
import '@fontsource/jetbrains-mono/400.css';
import '@fontsource/jetbrains-mono/500.css';
import '@/styles/colors_and_type.css';
import App from '@/App';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
```
(Drops the old `@/styles/globals.css` import — the design tokens own `html/body`. Leave `globals.css` on disk for now; it's just unreferenced.)

- [ ] **Step 5: tsconfig allows `.jsx`** — in `web/tsconfig.json` `compilerOptions`, add:
```json
    "allowJs": true,
    "checkJs": false,
```

- [ ] **Step 6: Temporary blank App** — replace `web/src/App.tsx` with a minimal placeholder so the project builds before the shell exists:
```tsx
export default function App() {
  return <div style={{ fontFamily: 'var(--font-display)', padding: 24 }}>QAgenticCow — scaffolding…</div>;
}
```

- [ ] **Step 7: Verify build** — from `web/`: `yarn build`. Expected: succeeds, emits `web/dist`. Then `yarn dev` briefly (optional) — page shows the placeholder in Plus Jakarta Sans.

- [ ] **Step 8: Commit**
```bash
git add web/package.json web/yarn.lock web/tsconfig.json web/src/main.tsx web/src/styles/colors_and_type.css web/public/assets web/src/App.tsx
git commit -m "qac: web Phase 1 — scaffold tokens + @fontsource + tsconfig allowJs (iter frontend)"
```
(Do NOT `git add web/dist` — only `index.html` is tracked there and it's unchanged.)

---

## Task 2: design primitives + seed module

**Files:** Create `web/src/design/components.jsx`, `web/src/design/seed.js`

- [ ] **Step 1: Copy + convert `components.jsx`**
```bash
cp web/design-handoff/components.jsx web/src/design/components.jsx
```
Apply the **Conversion recipe**: replace `const { useState: _cwUseState } = React;` (and any `React.` usage) with `import React from 'react';` at the top; this file defines primitives, so it needs no `@/design/components` import; it has no seed reads. Delete the trailing `Object.assign(window, {…})` and instead `export` every symbol other files consume: `export const Icon = …`, `export const icons = …`, `export const Card = …`, `export const Input`, `export const Button`, `export const StatusPill`, `export const Eyebrow`, `export const Badge`, `export const LogoMark`, and the helpers `export const fmtRel`, `fmtAbs`, `shortId` (export whatever the file defines — read it and export each top-level component/helper). Keep all markup/styles verbatim.

- [ ] **Step 2: Copy + convert `data.jsx` → `seed.js`**
```bash
cp web/design-handoff/data.jsx web/src/design/seed.js
```
Delete the trailing `Object.assign(window, {…})`; add `export` to every top-level `const` (`RUNS`, `EVENTS`, `FACTS`, `ALERT_EVIDENCE`, `STEP_GROUPS`, `TEMPLATE`, `NODE_FIXES`, `RACK_FIXES`, `FIX_STATUSES`, `JIRA`, `PAGERDUTY`, `WEB_PUSH`, `GRAFANA_BASE`, `GRAFANA_LINKS`, `BUNDLES`, `BUNDLE_STAGES`, …) and to the `stepStatesFor` function. No React in this file (pure data) — if it references none, no import needed.

- [ ] **Step 3: Verify it compiles** — add a throwaway check (or rely on Task 3 import). From `web/`: `yarn build`. Expected: succeeds (the modules are valid even if unused). If `data.jsx` used JSX, ensure the file stays `.jsx` not `.js` — **check**: if `data.jsx` contains JSX, name it `seed.jsx` and adjust imports accordingly.

- [ ] **Step 4: Commit**
```bash
git add web/src/design/components.jsx web/src/design/seed.js
git commit -m "qac: web Phase 1 — design primitives + seed module (window→ES modules)"
```

---

## Task 3: Shell + AppShell (router-wired)

**Files:** Create `web/src/design/Shell.jsx`, `web/src/app/nav.ts`, `web/src/app/AppShell.tsx`

- [ ] **Step 1: Copy + convert `Shell.jsx`**
```bash
cp web/design-handoff/Shell.jsx web/src/design/Shell.jsx
```
Apply the recipe: `import React from 'react';`; `import { Icon, icons, Eyebrow, LogoMark } from '@/design/components';`; `import { PAGERDUTY } from '@/design/seed';` (the Sidebar derives `liveSilences` from it). Delete `Object.assign(window,{Header,Sidebar,NavItem})`; add `export { Header, Sidebar, NavItem };` (and `export const fwHeaderIconBtn` if referenced elsewhere — keep it local otherwise). **Keep** the Sidebar's `current`/`onNav`/`runCount` prop interface unchanged (AppShell drives them).

- [ ] **Step 2: Nav map** — create `web/src/app/nav.ts`:
```ts
// Maps the design Sidebar's nav keys to router paths and back.
export const navKeyToPath: Record<string, string> = {
  runs: '/canary',
  nodetracker: '/node-tracker',
  racktracker: '/rack-tracker',
  l11dance: '/l11-fielddiag',
  jira: '/jira',
  knowledge: '/knowledge',
  templates: '/templates',
  probes: '/probes',
  agents: '/agents',
  grafana: '/grafana',
  evidence: '/history',
  reports: '/reports',
  silencer: '/silences',
  webpush: '/notifications',
  integrations: '/integrations',
  settings: '/settings',
};

// Which nav key should appear active for a given pathname. /canary and
// /runs/:id both light up Canary FW Release ('runs').
export function pathToNavKey(pathname: string): string {
  if (pathname === '/' || pathname.startsWith('/canary') || pathname.startsWith('/runs/')) {
    return 'runs';
  }
  const hit = Object.entries(navKeyToPath).find(
    ([, p]) => p !== '/canary' && pathname.startsWith(p),
  );
  return hit ? hit[0] : 'runs';
}
```

- [ ] **Step 3: AppShell** — create `web/src/app/AppShell.tsx`:
```tsx
import { useEffect, useState } from 'react';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { Header, Sidebar } from '@/design/Shell';
import { RUNS } from '@/design/seed';
import { navKeyToPath, pathToNavKey } from '@/app/nav';
import LoginCodec from '@/auth/LoginCodec';

export default function AppShell() {
  const navigate = useNavigate();
  const location = useLocation();
  const [theme, setTheme] = useState<'light' | 'dark'>('dark');
  const [showCodec, setShowCodec] = useState(false);

  // Theme: token overrides hang off <html data-theme>.
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);
  // Accent: fixed at the design default (the Tweaks panel that varied it is dropped).
  useEffect(() => {
    const root = document.documentElement;
    for (const v of ['--cw-blue', '--cw-blue-700', '--cw-blue-800']) {
      root.style.setProperty(v, '#4264ff');
    }
  }, []);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh', background: 'var(--cw-gray-400)' }}>
      <Header
        health="ok · :8080"
        theme={theme}
        onToggleTheme={() => setTheme((t) => (t === 'dark' ? 'light' : 'dark'))}
        onLock={() => setShowCodec(true)}
      />
      <div style={{ display: 'flex', flex: 1, overflow: 'hidden', gap: 1, background: 'var(--cw-gray-400)' }}>
        <Sidebar
          current={pathToNavKey(location.pathname)}
          onNav={(key: string) => navigate(navKeyToPath[key] ?? '/canary')}
          runCount={RUNS.length}
        />
        <Outlet />
      </div>
      {showCodec && <LoginCodec onAcknowledge={() => setShowCodec(false)} />}
    </div>
  );
}
```
(`LoginCodec` is created in Task 5; this import resolves once that task lands. If running Task 3 standalone, temporarily stub `const LoginCodec = () => null;` and remove the import — but in subagent-driven order Task 5 precedes the final build, so prefer leaving the import and letting Task 5 satisfy it. To keep Task 3 independently buildable, add a placeholder `web/src/auth/LoginCodec.jsx` exporting `export default function LoginCodec(){return null;}` now, which Task 5 overwrites with the real one.)

- [ ] **Step 4: Placeholder LoginCodec (so Task 3 builds)** — create `web/src/auth/LoginCodec.jsx`:
```jsx
export default function LoginCodec() { return null; }
```

- [ ] **Step 5: Verify** — `yarn build` (App.tsx is still the blank placeholder; AppShell isn't routed yet, but must compile). Expected: succeeds.

- [ ] **Step 6: Commit**
```bash
git add web/src/design/Shell.jsx web/src/app/nav.ts web/src/app/AppShell.tsx web/src/auth/LoginCodec.jsx
git commit -m "qac: web Phase 1 — app shell (Header+Sidebar) wired to react-router"
```

---

## Task 4: routes + App.tsx + StubPage + canary route wrappers

**Files:** Create `web/src/design/StubPage.jsx`, `web/src/features/canary/routes.tsx`, `web/src/app/routes.tsx`; Modify `web/src/App.tsx`

- [ ] **Step 1: StubPage** — create `web/src/design/StubPage.jsx` (design-styled "not built yet"):
```jsx
import { Icon, icons, Eyebrow } from '@/design/components';

export default function StubPage({ title }) {
  return (
    <main style={{ flex: 1, overflow: 'auto', padding: '32px 40px', fontFamily: 'var(--font-sans)' }}>
      <Eyebrow>QAgenticCow</Eyebrow>
      <h1 style={{ fontFamily: 'var(--font-display)', fontSize: 24, fontWeight: 700, color: 'var(--cw-gray-950)', margin: '6px 0 4px' }}>{title}</h1>
      <p style={{ color: 'var(--cw-gray-700)', fontSize: 14, maxWidth: 520 }}>
        This screen is not built yet. It is a placeholder in the navigation so the full console layout is visible.
      </p>
      <div style={{ marginTop: 20, display: 'inline-flex', alignItems: 'center', gap: 8, padding: '8px 12px', border: '1px solid var(--cw-gray-400)', borderRadius: 4, color: 'var(--cw-gray-700)', fontSize: 12.5 }}>
        <Icon path={icons.clock} size={16} /> Planned
      </div>
    </main>
  );
}
```

- [ ] **Step 2: Canary route wrappers** — create `web/src/features/canary/routes.tsx` (adapts router ↔ the design pages' callback props). The page imports resolve once Task 5 lands the pages; in subagent-driven order Task 5 follows, so add placeholder pages if building Task 4 standalone (see note). Content:
```tsx
import { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import RunsList from '@/features/canary/RunsList';
import RunDetail from '@/features/canary/RunDetail';
import NewRunModal from '@/features/canary/NewRunModal';

export function CanaryRoute() {
  const navigate = useNavigate();
  const [modalOpen, setModalOpen] = useState(false);
  return (
    <>
      <RunsList
        onOpenRun={(id: string) => navigate(`/runs/${id}`)}
        onNewRun={() => setModalOpen(true)}
        density="compact"
      />
      <NewRunModal open={modalOpen} onClose={() => setModalOpen(false)} onCreate={() => setModalOpen(false)} />
    </>
  );
}

export function RunDetailRoute() {
  const navigate = useNavigate();
  const { id } = useParams();
  return <RunDetail runId={id} onBack={() => navigate('/canary')} showRailGroups />;
}
```

- [ ] **Step 3: Router table** — create `web/src/app/routes.tsx`:
```tsx
import { createBrowserRouter, Navigate } from 'react-router-dom';
import AppShell from '@/app/AppShell';
import StubPage from '@/design/StubPage';
import { CanaryRoute, RunDetailRoute } from '@/features/canary/routes';
import { NodeTracker, RackTracker } from '@/features/tracking/Trackers';
import Jira from '@/features/jira/Jira';
import Knowledge from '@/features/knowledge/Knowledge';
import GrafanaLinks from '@/features/grafana/GrafanaLinks';
import Silencer from '@/features/silences/Silencer';
import WebPush from '@/features/notifications/WebPush';

export const router = createBrowserRouter([
  {
    path: '/',
    element: <AppShell />,
    children: [
      { index: true, element: <Navigate to="/canary" replace /> },
      { path: 'canary', element: <CanaryRoute /> },
      { path: 'runs/:id', element: <RunDetailRoute /> },
      { path: 'node-tracker', element: <NodeTracker /> },
      { path: 'rack-tracker', element: <RackTracker /> },
      { path: 'jira', element: <Jira onOpenRun={() => {}} /> },
      { path: 'knowledge', element: <Knowledge /> },
      { path: 'grafana', element: <GrafanaLinks /> },
      { path: 'silences', element: <Silencer /> },
      { path: 'notifications', element: <WebPush /> },
      { path: 'l11-fielddiag', element: <StubPage title="L11 FieldDiag" /> },
      { path: 'templates', element: <StubPage title="Templates" /> },
      { path: 'probes', element: <StubPage title="Probes" /> },
      { path: 'agents', element: <StubPage title="Agents" /> },
      { path: 'history', element: <StubPage title="History" /> },
      { path: 'reports', element: <StubPage title="Reports" /> },
      { path: 'integrations', element: <StubPage title="Integrations" /> },
      { path: 'settings', element: <StubPage title="Settings" /> },
      { path: '*', element: <StubPage title="Not found" /> },
    ],
  },
]);
```
(`Jira` takes `onOpenRun`; pass a no-op for Phase 1 — dead clicks are acceptable. `NodeTracker`/`RackTracker` take no props.)

- [ ] **Step 4: App.tsx** — replace `web/src/App.tsx`:
```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from 'react-router-dom';
import { Toaster } from 'sonner';
import { router } from '@/app/routes';

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 5_000 } },
});

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
      <Toaster position="top-right" />
    </QueryClientProvider>
  );
}
```

- [ ] **Step 5: Defer build to Task 5** — routes import pages that Task 5 creates. Do NOT build/commit standalone; this task's files are committed together with Task 5 (or create temporary placeholder page files that Task 5 overwrites). **Chosen approach:** commit Task 4 files now WITHOUT building, then Task 5 adds the pages and runs the first full build. Commit:
```bash
git add web/src/design/StubPage.jsx web/src/features/canary/routes.tsx web/src/app/routes.tsx web/src/App.tsx
git commit -m "qac: web Phase 1 — router table + stub page + canary route wrappers"
```

---

## Task 5: port all seed pages

**Files:** Create under `web/src/features/`:
- `canary/RunsList.jsx`, `canary/RunDetail.jsx`, `canary/NewRunModal.jsx`
- `tracking/Trackers.jsx`
- `jira/Jira.jsx`, `knowledge/Knowledge.jsx`, `grafana/GrafanaLinks.jsx`, `silences/Silencer.jsx`, `notifications/WebPush.jsx`
- `web/src/auth/Login.jsx`, `web/src/auth/LoginCodec.jsx` (overwrites the Task 3 placeholder)

- [ ] **Step 1: Copy each page** from `web/design-handoff/` to its target path (RunsList.jsx→canary/, RunDetail.jsx→canary/, NewRunModal.jsx→canary/, Trackers.jsx→tracking/, Jira.jsx→jira/, Knowledge.jsx→knowledge/, GrafanaLinks.jsx→grafana/, Silencer.jsx→silences/, WebPush.jsx→notifications/, Login.jsx→auth/, LoginCodec.jsx→auth/).

- [ ] **Step 2: Apply the Conversion recipe to each** (top of this plan). Per file, add `import React, { …hooks }` , the `@/design/components` import for primitives it uses, the `@/design/seed` import for seed consts it reads, delete the `Object.assign(window,…)`, and `export default` the page component. `Trackers.jsx` exports **named** `NodeTracker` and `RackTracker` (it defines both). 

- [ ] **Step 3: Guard Knowledge's AI Ask box** — in `knowledge/Knowledge.jsx`, the Ask panel calls `await window.claude.complete({messages})`, which won't exist. Replace that call site with a guard so the page never throws:
```jsx
// window.claude is a design-host-only helper; absent in this build.
const complete = typeof window !== 'undefined' && window.claude && window.claude.complete;
if (!complete) {
  setAnswer('Model not connected. The knowledge base (glossary and sources) is shown above; live Q&A is wired in a later iteration.');
  setBusy(false);
  return;
}
// (existing call:) const res = await complete({ messages });
```
Keep `GLOSSARY`, `KB_SOURCES`, `rankSources`, `renderMd` as-is (in-file consts/helpers). The glossary + source list still render from those consts.

- [ ] **Step 4: Asset URLs** — in `LoginCodec.jsx` (and anywhere else), change relative `src="assets/…"` to absolute `src="/assets/…"` (Vite serves `public/assets/` at `/assets/`). Grep the copied pages: `grep -rn 'src="assets/' web/src/features web/src/auth`.

- [ ] **Step 5: First full build** — from `web/`: `yarn build`. Expected: succeeds. Fix any unresolved import (a primitive/const the recipe missed → add it to that file's import). Re-run until clean.

- [ ] **Step 6: Manual smoke** (optional but recommended) — `yarn dev`, click every sidebar item: seed pages render with real-looking data; stubs show the placeholder; theme toggle flips; lock button shows the codec overlay. Dead in-page buttons are acceptable.

- [ ] **Step 7: Commit**
```bash
git add web/src/features web/src/auth
git commit -m "qac: web Phase 1 — port all design pages (seed data) + guard Knowledge AI box"
```

---

## Task 6: remove obsolete code + smoke tests + full verification

**Files:** Delete old `web/src/features/runs/{RunsList,RunDetail}.tsx`, `web/src/features/runs/wizard/*`, their `*.test.tsx`, `web/src/components/{Badge,Dialog}.tsx` (+ tests); Create `web/src/app/AppShell.test.tsx`, `web/src/app/routes.test.tsx`

- [ ] **Step 1: Identify what's safe to delete** — the retained Phase-2 data layer is `web/src/lib/api.ts` + `web/src/features/runs/use*.ts` hooks + `web/src/mocks/`. Delete only the **presentational** old code + tests:
```bash
cd web
git rm src/features/runs/RunsList.tsx src/features/runs/RunDetail.tsx
git rm -r src/features/runs/wizard
git rm src/components/Badge.tsx src/components/Dialog.tsx 2>/dev/null || true
# old tests tied to the above:
git rm src/features/runs/RunsList.test.tsx 2>/dev/null || true
git rm src/features/runs/wizard/NewRunWizard.test.tsx 2>/dev/null || true
```
(Run `grep -rn "statusToTone\|runs.types\|@/components/Badge\|@/components/Dialog" src` first; keep `runs.types.ts`/`statusToTone.ts`/the hooks if anything retained imports them. Delete a hook only if nothing imports it.)

- [ ] **Step 2: Write smoke tests** — create `web/src/app/routes.test.tsx`:
```tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { RouterProvider, createMemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import AppShell from '@/app/AppShell';
import StubPage from '@/design/StubPage';
import { CanaryRoute } from '@/features/canary/routes';

function renderAt(path: string, element: React.ReactNode) {
  const router = createMemoryRouter(
    [{ path: '/', element: <AppShell />, children: [{ path: path.slice(1) || 'canary', element }] }],
    { initialEntries: [path] },
  );
  const qc = new QueryClient();
  return render(
    <QueryClientProvider client={qc}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

describe('app shell', () => {
  it('renders the sidebar with QA Workflows and Canary FW Release', () => {
    renderAt('/canary', <CanaryRoute />);
    expect(screen.getByText('QA Workflows')).toBeInTheDocument();
    expect(screen.getByText('Canary FW Release')).toBeInTheDocument();
  });

  it('renders a stub page without crashing', () => {
    renderAt('/templates', <StubPage title="Templates" />);
    expect(screen.getByText('Templates')).toBeInTheDocument();
    expect(screen.getByText(/not built yet/i)).toBeInTheDocument();
  });
});
```
(If `useNavigate` inside `Sidebar` requires a router — it does; `createMemoryRouter` provides it. Adjust the nested route `path` handling if the helper's slicing is awkward — the goal is: AppShell mounts with a child element, sidebar text is present, no throw.)

- [ ] **Step 3: Run tests** — from `web/`: `yarn test --run`. Expected: the two smoke tests pass; no references to deleted components remain. Fix any leftover import from a deleted file.

- [ ] **Step 4: Full verification**
  - `cd web && yarn build` — succeeds.
  - `cd web && yarn lint` — clean (`.jsx` unlinted by config; `.ts`/`.tsx` pass).
  - `cd web && yarn test --run` — green.
  - From repo root: `make build` — produces `../bin/qac`; `make test` — Go + web green; `make lint` — green.
  - Run the binary and curl the SPA: `./bin/qac serve --addr 127.0.0.1:8099 &` then `curl -s localhost:8099/ | grep -i '<div id="root"'` (the SPA shell serves). Stop the server.

- [ ] **Step 5: Commit**
```bash
git add -A web/src
git commit -m "qac: web Phase 1 — remove superseded components + add shell/route smoke tests"
```

---

## Self-review notes
- **Spec coverage:** §3.1 routing → Tasks 3-4; §3.2 file structure → Tasks 2-5; §3.3 seams (Babel→Vite, globals→modules) → Task 1 (no unpkg, Vite already) + recipe in 2/3/5; §3.4 tokens/fonts/assets → Task 1; §3.5 Knowledge guard + dead clicks + auth-as-overlay → Task 5 step 3 + AppShell `onLock`; §4 build/embed → Task 6 step 4; §6 tests → Task 6; §7 acceptance → Task 6 step 4.
- **Decisions honored:** react-router + TanStack retained (App.tsx); pages `.jsx` (recipe); login hidden-not-gated (AppShell `onLock`→codec overlay, no startup gate); tweaks panel dropped (not copied); porsche asset kept (copied to public/assets in Task 1).
- **Type consistency:** `navKeyToPath`/`pathToNavKey` (nav.ts) used by AppShell; `CanaryRoute`/`RunDetailRoute` (canary/routes.tsx) used by routes.tsx; every page `export default` except `Trackers.jsx` (named `NodeTracker`,`RackTracker`) — matches the routes.tsx imports.
- **Ordering caveat:** Task 4 commits before its imported pages exist; the first green build is Task 5 step 5. Acceptable for subagent-driven (the controller runs tasks in order); each task still commits, but the tree first builds clean at Task 5. If a task must build in isolation, add placeholder page files. Flagged so the executor expects a red build between Task 4 and Task 5 step 5.
- **Open detail for the implementer:** whether `data.jsx` contains JSX (→ name it `seed.jsx`); the exact primitive/const import set per page (read each file); confirm `window.claude` guard matches Knowledge's actual state setters (`setAnswer`/`setBusy` names may differ — read the file).
