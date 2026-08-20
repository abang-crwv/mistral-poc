# qac frontend Phase 2 — wire Canary FW Release to the live backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`).

**Goal:** Canary FW Release reads + writes the live backend: RunsList + RunDetail show real runs/events/facts/alert-evidence/steps (Stage 1); New-run and Sign-off/advance drive the backend (Stage 2). A pure `.ts` adapter layer maps real API → the design's shapes; TanStack Query hooks in route wrappers fetch + adapt and pass data to the design pages as props.

**Architecture:** `features/canary/adapters.ts` (pure mappers, unit-tested) + `types.ts` (design-shape TS types) + per-entity TanStack hooks; `routes.tsx` wrappers fetch/adapt/handle loading+error and pass props; the 3 design pages drop `@/design/seed` and take props.

**Tech Stack:** React 18 + Vite + react-router 6 + TanStack Query 5 + Vitest + MSW. Spec: `docs/superpowers/specs/2026-06-04-qac-frontend-phase2-design.md`.

---

## Key facts (verified)
- Real `Run` JSON: `{id, bundle, template_id, created_at, updated_at, status, rack, created_by, inputs}` (`inputs` is raw JSON: `{bundle_tag, canary_racks[], requester, rlcc_workflow, …}`). Status vocab: `pending|running|passed|warning|failed|signed_off`.
- `GET /api/runs` → `{runs: Run[]}`. `GET /api/runs/{id}` → `{run, events, racks}`. Event JSON: `{seq, kind, payload(raw), occurred_at}`.
- `GET /api/runs/{id}/facts` → `{facts: {...}}` (keys: `bundle_tag, requester, instance_type, sku, variant, gb_generation, region, cluster`).
- `GET /api/runs/{id}/evidence?step=alert_baseline` → `{evidence: [{deviceslot, payload, …}]}`; the aggregate row (`deviceslot == null`) `payload` is the alert_probe Output = `{probed_at, source, per_rack, max_severity, alerts_clear}` — matches the design's `ALERT_EVIDENCE` shape.
- `GET /api/templates/{id}` → template `{id, version, title, steps: [{id, kind, title, action?:{tool}, probe_type?, timeout?, …}]}`.
- `POST /api/runs` body: `{template_id, inputs: {bundle_tag, canary_racks, requester, rlcc_workflow}}` → 201 `{run:{id,…}}`. Stable error codes via `ApiException`.
- `POST /api/runs/{id}/actions` body: `{step_id, action:'advance'|'signoff', verdict?, signer_name?, signer_role?, notes?}`.
- `GET /api/rlcc/workflows` → workflow list (see `useRlccWorkflows` in `features/runs/useRlccWorkflows.ts`).
- `lib/api.ts`: `apiGet<T>(path)`, `apiGetWithMeta<T>`, `apiPost<TRes,TBody>(path, body)`, `ApiException{code,message,details}`.
- Existing hooks (`features/runs/`): `useRuns`, `useRun`, `useTemplates`, `useCreateRun`, `useRlccWorkflows`, `useInventoryPreview` — reuse `useRlccWorkflows`; the canary hooks below adapt.
- Design pages today (`features/canary/`):
  - `RunsList({onOpenRun,onNewRun,density})` reads `RUNS` (stat-card counts + filter + table), `FLAT_STEPS`/`STEP_INDEX` (only inside the `r.currentStep ?` progress cell), `stepTitle(id)`.
  - `RunDetail({runId,onBack,showRailGroups})` reads `RUNS.find`, `stepStatesFor(run)`, `FACTS[id]`, `ALERT_EVIDENCE[id]`, `EVENTS[id]`, `TEMPLATE.version`, `STEP_GROUPS`/`FLAT_STEPS`/`STEP_INDEX`.
  - `NewRunModal({open,onClose,onCreate})` collects `{bundle, racks[], requester}` → `onCreate({bundle,racks,requester})`. `RunDetail` has a "Sign off" button (shown for status `warning|passed`) + a `SignoffPanel`.
- `design/seed.js` `stepStatesFor(run)` uses module-level `FLAT_STEPS`/`STEP_INDEX` (the 13 seed steps) — Phase 2 needs a parameterized version against the **real** template (15 steps). Adapter provides `stepStatesFor(run, flatSteps, stepIndex)`.

---

# STAGE 1 — reads

## Task 1: types + adapters (pure, unit-tested)

**Files:** Create `web/src/features/canary/types.ts`, `web/src/features/canary/adapters.ts`, `web/src/features/canary/adapters.test.ts`

- [ ] **Step 1: Types** — create `web/src/features/canary/types.ts`:
```ts
export type RunStatus = 'pending' | 'running' | 'passed' | 'warning' | 'failed' | 'signed_off';

export interface CanaryRun {
  id: string;
  bundle: string;
  template_id: string;
  status: RunStatus;
  racks: string[];
  requester: string;
  created_by: string;
  created_at: string;
  updated_at: string;
  currentStep: string | null;
  verdict?: 'pass' | 'fail';
}

export interface RunEvent { seq: number; kind: string; occurred_at: string; detail: string; }
export interface Facts { instance_type?: string; sku?: string; variant?: string; region?: string; nodes?: number; source: string; }
export interface AlertEvidence {
  probed_at?: number; source?: string; alerts_clear?: boolean; max_severity?: string;
  per_rack?: Record<string, { max_severity: string; count_by_severity: Record<string, number>; alerts: Array<Record<string, unknown>> }>;
}
export interface CanaryStep { id: string; title: string; kind: string; tool: string; live: boolean; timeout: string; desc: string; }

// Raw API shapes (what lib/api returns).
export interface RawRun {
  id: string; bundle: string; template_id: string; status: RunStatus;
  rack: string; created_by: string; created_at: string; updated_at: string;
  inputs?: { bundle_tag?: string; canary_racks?: string[]; requester?: string; rlcc_workflow?: string } | null;
}
export interface RawEvent { seq: number; kind: string; payload: unknown; occurred_at: string; }
export interface RawTemplate { id: string; version: number; title: string; steps: Array<{ id: string; kind: string; title: string; action?: { tool?: string } | null; probe_type?: string; timeout?: string }>; }
```

- [ ] **Step 2: Failing adapter test** — create `web/src/features/canary/adapters.test.ts`:
```ts
import { describe, it, expect } from 'vitest';
import {
  deriveVerdict, deriveCurrentStep, adaptRun, detailFor, adaptEvents,
  adaptFacts, adaptAlertEvidence, adaptTemplateSteps, stepStatesFor,
} from './adapters';
import type { RawRun, RawEvent } from './types';

const raw: RawRun = {
  id: 'r1', bundle: 'b1', template_id: 'firmware-release-canary', status: 'running',
  rack: 'dh3-r012-us-east-01a', created_by: 'wpena', created_at: 't0', updated_at: 't1',
  inputs: { bundle_tag: 'b1', canary_racks: ['dh3-r012-us-east-01a', 'dh3-r014-us-east-01a'], requester: 'k.osei', rlcc_workflow: 'wf' },
};

describe('adaptRun', () => {
  it('derives racks/requester from inputs and verdict from status', () => {
    const r = adaptRun(raw, []);
    expect(r.racks).toEqual(['dh3-r012-us-east-01a', 'dh3-r014-us-east-01a']);
    expect(r.requester).toBe('k.osei');
    expect(r.verdict).toBeUndefined(); // running
    expect(r.currentStep).toBeNull(); // no events
  });
  it('falls back to [rack] when inputs.canary_racks is absent', () => {
    const r = adaptRun({ ...raw, inputs: null }, []);
    expect(r.racks).toEqual(['dh3-r012-us-east-01a']);
  });
  it('maps verdict from terminal status', () => {
    expect(adaptRun({ ...raw, status: 'failed' }, []).verdict).toBe('fail');
    expect(adaptRun({ ...raw, status: 'signed_off' }, []).verdict).toBe('pass');
  });
});

describe('deriveCurrentStep', () => {
  it('is the last StepStarted without a later StepCompleted/StepFailed', () => {
    const evs: RawEvent[] = [
      { seq: 1, kind: 'StepStarted', payload: { step_id: 's1' }, occurred_at: 't' },
      { seq: 2, kind: 'StepCompleted', payload: { step_id: 's1' }, occurred_at: 't' },
      { seq: 3, kind: 'StepStarted', payload: { step_id: 's2' }, occurred_at: 't' },
    ];
    expect(deriveCurrentStep(evs)).toBe('s2');
  });
  it('is null when all started steps completed', () => {
    expect(deriveCurrentStep([
      { seq: 1, kind: 'StepStarted', payload: { step_id: 's1' }, occurred_at: 't' },
      { seq: 2, kind: 'StepCompleted', payload: { step_id: 's1' }, occurred_at: 't' },
    ])).toBeNull();
  });
});

describe('detailFor', () => {
  it('formats known kinds and falls back for unknown', () => {
    expect(detailFor('StepStarted', { step_id: 'l11_fielddiag', probe_type: 'rlcc_action_probe' })).toContain('l11_fielddiag');
    expect(detailFor('StepCompleted', { step_id: 'l11_fielddiag', verdict: 'failed' })).toContain('failed');
    expect(detailFor('Mystery', {})).toBe('Mystery');
  });
});

describe('adaptFacts / adaptAlertEvidence / adaptTemplateSteps / stepStatesFor', () => {
  it('maps facts and supplies a source label', () => {
    const f = adaptFacts({ instance_type: 'gb200-4x', sku: 'X', variant: 'v', region: 'US-EAST-01A' }, ['rk1']);
    expect(f.instance_type).toBe('gb200-4x');
    expect(f.source).toMatch(/inventory/);
  });
  it('passes the alert_probe aggregate payload through', () => {
    const e = adaptAlertEvidence({ probed_at: 1, source: 's', max_severity: 'warning', alerts_clear: false, per_rack: {} });
    expect(e.max_severity).toBe('warning');
  });
  it('builds flat steps + index from a template', () => {
    const { flatSteps, stepIndex } = adaptTemplateSteps({
      id: 't', version: 10, title: 'T',
      steps: [{ id: 'a', kind: 'probe', title: 'A', probe_type: 'alert_probe', timeout: '30s' }, { id: 'b', kind: 'signoff', title: 'B' }],
    });
    expect(flatSteps.map((s) => s.id)).toEqual(['a', 'b']);
    expect(stepIndex.b).toBe(1);
    expect(flatSteps[0].tool).toBe('alert_probe');
  });
  it('stepStatesFor marks before-current passed, current running, after pending', () => {
    const flat = [{ id: 'a' }, { id: 'b' }, { id: 'c' }].map((s) => ({ ...s, title: '', kind: '', tool: '', live: false, timeout: '', desc: '' }));
    const idx = { a: 0, b: 1, c: 2 };
    const run = { ...adaptRun(raw, []), currentStep: 'b', status: 'running' as const };
    const st = stepStatesFor(run, flat, idx);
    expect(st).toEqual({ a: 'passed', b: 'running', c: 'pending' });
  });
});
```

- [ ] **Step 2b: Run — expect FAIL:** `cd web && yarn test --run src/features/canary/adapters.test.ts` (module not found).

- [ ] **Step 3: Implement adapters** — create `web/src/features/canary/adapters.ts`:
```ts
import type { RawRun, RawEvent, RawTemplate, CanaryRun, RunEvent, Facts, AlertEvidence, CanaryStep, RunStatus } from './types';

export function deriveVerdict(status: RunStatus): 'pass' | 'fail' | undefined {
  if (status === 'failed') return 'fail';
  if (status === 'passed' || status === 'signed_off') return 'pass';
  return undefined;
}

function stepId(payload: unknown): string | undefined {
  return payload && typeof payload === 'object' ? (payload as Record<string, unknown>).step_id as string | undefined : undefined;
}

// The current step is the last StepStarted whose step_id has no later
// StepCompleted/StepFailed. Returns null if none is open.
export function deriveCurrentStep(events: RawEvent[]): string | null {
  const open = new Set<string>();
  const order: string[] = [];
  for (const e of events) {
    const sid = stepId(e.payload);
    if (!sid) continue;
    if (e.kind === 'StepStarted') { if (!open.has(sid)) order.push(sid); open.add(sid); }
    else if (e.kind === 'StepCompleted' || e.kind === 'StepFailed') open.delete(sid);
  }
  for (let i = order.length - 1; i >= 0; i--) if (open.has(order[i])) return order[i];
  return null;
}

export function adaptRun(raw: RawRun, events: RawEvent[]): CanaryRun {
  const inputs = raw.inputs ?? {};
  const racks = inputs.canary_racks && inputs.canary_racks.length ? inputs.canary_racks : (raw.rack ? [raw.rack] : []);
  return {
    id: raw.id, bundle: raw.bundle, template_id: raw.template_id, status: raw.status,
    racks, requester: inputs.requester ?? raw.created_by, created_by: raw.created_by,
    created_at: raw.created_at, updated_at: raw.updated_at,
    currentStep: deriveCurrentStep(events), verdict: deriveVerdict(raw.status),
  };
}

const p = (payload: unknown) => (payload && typeof payload === 'object' ? payload as Record<string, any> : {});

export function detailFor(kind: string, payload: unknown): string {
  const d = p(payload);
  switch (kind) {
    case 'RunCreated': return `template ${d.template_id ?? '—'}${d.inputs?.canary_racks ? ` · ${d.inputs.canary_racks.length} racks` : ''}${d.inputs?.requester ? ` · requester ${d.inputs.requester}` : ''}`;
    case 'FactsDiscovered': return [d.instance_type, d.sku, d.variant].filter(Boolean).join(' · ') || 'facts discovered';
    case 'StepStarted': return `${d.step_id ?? '—'}${d.probe_type ? ` · ${d.probe_type}` : ''}`;
    case 'StepCompleted': return `${d.step_id ?? '—'}${d.verdict ? ` · verdict ${d.verdict}` : ' · completed'}`;
    case 'StepFailed': return `${d.step_id ?? '—'} · failed${d.error ? `: ${d.error}` : ''}`;
    case 'OperatorAction': return `${d.step_id ?? '—'} · ${d.action ?? 'action'}${d.verdict ? ` · ${d.verdict}` : ''}${d.signer_name ? ` · ${d.signer_name}` : ''}`;
    case 'BMNIgnored': return `${d.deviceslot ?? '—'} ignored${d.source ? ` · ${d.source}` : ''}`;
    case 'FactsDiscoveryFailed': return `facts discovery failed${d.error ? `: ${d.error}` : ''}`;
    case 'BMNDiscoveryFailed': return `BMN discovery failed${d.message ? `: ${d.message}` : ''}`;
    default: return kind;
  }
}

export function adaptEvents(events: RawEvent[]): RunEvent[] {
  return events.map((e) => ({ seq: e.seq, kind: e.kind, occurred_at: e.occurred_at, detail: detailFor(e.kind, e.payload) }));
}

export function adaptFacts(facts: Record<string, any> | null | undefined, racks: string[]): Facts {
  const f = facts ?? {};
  return {
    instance_type: f.instance_type, sku: f.sku, variant: f.variant, region: f.region,
    nodes: typeof f.nodes === 'number' ? f.nodes : (racks.length ? racks.length * 18 : undefined),
    source: 'inventory · cwf where',
  };
}

export function adaptAlertEvidence(aggregatePayload: Record<string, any> | null | undefined): AlertEvidence {
  return (aggregatePayload ?? {}) as AlertEvidence;
}

export function adaptTemplateSteps(tpl: RawTemplate): { flatSteps: CanaryStep[]; stepIndex: Record<string, number> } {
  const liveProbes = new Set(['alert_probe', 'fact_probe', 'rlcc_action_probe', 'fail_cause_probe']);
  const flatSteps: CanaryStep[] = (tpl.steps ?? []).map((s) => ({
    id: s.id, title: s.title, kind: s.kind,
    tool: s.action?.tool ?? s.probe_type ?? '',
    live: !!s.probe_type && liveProbes.has(s.probe_type),
    timeout: s.timeout ?? '', desc: '',
  }));
  const stepIndex: Record<string, number> = {};
  flatSteps.forEach((s, i) => { stepIndex[s.id] = i; });
  return { flatSteps, stepIndex };
}

// Parameterized port of the design's stepStatesFor (was bound to seed FLAT_STEPS).
export function stepStatesFor(run: CanaryRun, flatSteps: CanaryStep[], stepIndex: Record<string, number>): Record<string, string> {
  const cur = run.currentStep ? stepIndex[run.currentStep] ?? -1 : -1;
  const out: Record<string, string> = {};
  flatSteps.forEach((s, i) => {
    if (run.status === 'pending') { out[s.id] = 'pending'; return; }
    if (run.status === 'signed_off') { out[s.id] = 'passed'; return; }
    if (i < cur) out[s.id] = 'passed';
    else if (i === cur) out[s.id] = run.status === 'failed' ? 'failed' : run.status === 'warning' ? 'warning' : 'running';
    else out[s.id] = 'pending';
  });
  return out;
}
```

- [ ] **Step 4: Run — expect PASS:** `cd web && yarn test --run src/features/canary/adapters.test.ts`.

- [ ] **Step 5: Commit**
```bash
git add web/src/features/canary/types.ts web/src/features/canary/adapters.ts web/src/features/canary/adapters.test.ts
git commit -m "qac: web Phase 2 — canary adapter layer (real API → design shapes) + tests"
```

---

## Task 2: read hooks

**Files:** Create `web/src/features/canary/useCanaryRuns.ts`, `useCanaryRun.ts`, `useRunFacts.ts`, `useRunAlertEvidence.ts`, `useCanaryTemplate.ts`

- [ ] **Step 1: Implement the hooks** (no separate unit tests — covered via MSW route tests in Task 4). Create each:

`useCanaryRuns.ts`:
```ts
import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import { adaptRun } from './adapters';
import type { RawRun, CanaryRun } from './types';

export function useCanaryRuns() {
  return useQuery<CanaryRun[]>({
    queryKey: ['canary', 'runs'],
    queryFn: async () => (await apiGet<{ runs: RawRun[] }>('/runs')).runs.map((r) => adaptRun(r, [])),
  });
}
```

`useCanaryRun.ts`:
```ts
import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import { adaptRun, adaptEvents } from './adapters';
import type { RawRun, RawEvent, CanaryRun, RunEvent } from './types';

export interface CanaryRunDetail { run: CanaryRun; events: RunEvent[]; racks: string[]; }

export function useCanaryRun(id: string) {
  return useQuery<CanaryRunDetail>({
    queryKey: ['canary', 'runs', id],
    queryFn: async () => {
      const d = await apiGet<{ run: RawRun; events: RawEvent[]; racks: string[] }>(`/runs/${id}`);
      return { run: adaptRun(d.run, d.events ?? []), events: adaptEvents(d.events ?? []), racks: d.racks ?? [] };
    },
  });
}
```

`useRunFacts.ts`:
```ts
import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import { adaptFacts } from './adapters';
import type { Facts } from './types';

export function useRunFacts(id: string, racks: string[]) {
  return useQuery<Facts>({
    queryKey: ['canary', 'runs', id, 'facts'],
    queryFn: async () => adaptFacts((await apiGet<{ facts: Record<string, any> }>(`/runs/${id}/facts`)).facts, racks),
  });
}
```

`useRunAlertEvidence.ts`:
```ts
import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import { adaptAlertEvidence } from './adapters';
import type { AlertEvidence } from './types';

interface EvidenceRow { deviceslot: string | null; payload: Record<string, any>; }

export function useRunAlertEvidence(id: string) {
  return useQuery<AlertEvidence>({
    queryKey: ['canary', 'runs', id, 'alert-evidence'],
    queryFn: async () => {
      const rows = (await apiGet<{ evidence: EvidenceRow[] }>(`/runs/${id}/evidence?step=alert_baseline`)).evidence ?? [];
      const aggregate = rows.find((r) => r.deviceslot == null);
      return adaptAlertEvidence(aggregate?.payload);
    },
  });
}
```

`useCanaryTemplate.ts`:
```ts
import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import { adaptTemplateSteps } from './adapters';
import type { RawTemplate, CanaryStep } from './types';

export interface CanaryTemplate { version: number; flatSteps: CanaryStep[]; stepIndex: Record<string, number>; }

export function useCanaryTemplate(id: string) {
  return useQuery<CanaryTemplate>({
    queryKey: ['canary', 'template', id],
    queryFn: async () => {
      const tpl = await apiGet<{ template: RawTemplate }>(`/templates/${id}`).then((r) => r.template);
      const { flatSteps, stepIndex } = adaptTemplateSteps(tpl);
      return { version: tpl.version, flatSteps, stepIndex };
    },
  });
}
```
(Verify `GET /api/templates/{id}` envelope is `{template:…}` — check `getTemplateHandler` in `internal/api/templates.go`; if it returns the template bare, drop the `.template`.)

- [ ] **Step 2: Verify build** — `cd web && yarn build`. Expected: succeeds.

- [ ] **Step 3: Commit**
```bash
git add web/src/features/canary/useCanaryRuns.ts web/src/features/canary/useCanaryRun.ts web/src/features/canary/useRunFacts.ts web/src/features/canary/useRunAlertEvidence.ts web/src/features/canary/useCanaryTemplate.ts
git commit -m "qac: web Phase 2 — canary read hooks (runs/run/facts/alert-evidence/template)"
```

---

## Task 3: prop-refactor RunsList + RunDetail; wire route wrappers; loading/error

**Files:** Modify `web/src/features/canary/RunsList.jsx`, `web/src/features/canary/RunDetail.jsx`, `web/src/features/canary/routes.tsx`; Create `web/src/features/canary/Loading.jsx`

- [ ] **Step 1: Shared loading/error fallback** — create `web/src/features/canary/Loading.jsx`:
```jsx
import { Icon, icons } from '@/design/components';

export function Loading({ label = 'Loading…' }) {
  return (
    <main style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--cw-gray-700)', fontFamily: 'var(--font-sans)', gap: 10 }}>
      <span style={{ display: 'inline-flex', animation: 'fwspin 0.8s linear infinite' }}><Icon path={icons.spinner} size={18} /></span>{label}
    </main>
  );
}

export function ErrorPane({ message }) {
  return (
    <main style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', flexDirection: 'column', gap: 8, color: 'var(--status-danger-fg)', fontFamily: 'var(--font-sans)' }}>
      <Icon path={icons.alert} size={22} />
      <div style={{ fontWeight: 600 }}>Could not load</div>
      <div style={{ color: 'var(--cw-gray-700)', fontSize: 13 }}>{message}</div>
    </main>
  );
}
```

- [ ] **Step 2: Refactor RunsList to props** — in `web/src/features/canary/RunsList.jsx`:
  - Remove `import { FLAT_STEPS, STEP_INDEX, RUNS } from '@/design/seed';`.
  - Change signature to `function RunsList({ runs, flatSteps = [], stepIndex = {}, onOpenRun, onNewRun, density = 'regular' })`.
  - Replace every `RUNS` with `runs`; replace `FLAT_STEPS` with `flatSteps` and `STEP_INDEX` with `stepIndex` (the `stepTitle` helper takes `flatSteps` — inline it as `const stepTitle = (id) => (flatSteps.find((s) => s.id === id) || {}).title || '—';` inside the component, or pass flatSteps). Keep all markup/styles.
  - `export default RunsList;` stays.

- [ ] **Step 3: Refactor RunDetail to props** — in `web/src/features/canary/RunDetail.jsx`:
  - Remove the `@/design/seed` import entirely.
  - Change signature to `function RunDetail({ run, events, facts, evidence, steps, stepIndex, states, templateVersion, showRailGroups = false, onBack, onSignoff, onAdvance })`.
  - Replace seed reads: `RUNS.find(...)` → use the `run` prop; `stepStatesFor(run)` → use the `states` prop; `FACTS[run.id]` → `facts`; `ALERT_EVIDENCE[run.id]` → `evidence`; `EVENTS[run.id]` → `events`; `TEMPLATE.version` → `templateVersion`; `FLAT_STEPS` → `steps`; `STEP_INDEX` → `stepIndex`; `STEP_GROUPS` → not needed when `showRailGroups` is false (the StepRail's grouped branch uses STEP_GROUPS — guard it: when `showRailGroups` is false it iterates `steps`). If `StepRail`/`StepRailItem` reference `STEP_GROUPS`/`FLAT_STEPS`/`STEP_INDEX` at module scope, thread them through as props from `RunDetail`.
  - Leave the Sign-off button + SignoffPanel in place; in Stage 1 they can remain inert (Stage 2 wires `onSignoff`/`onAdvance`). Accept the two callbacks as props now (default no-ops) so the signature is stable.
  - `export default RunDetail;` stays.

- [ ] **Step 4: Wire the route wrappers** — replace `web/src/features/canary/routes.tsx`:
```tsx
import { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import RunsList from '@/features/canary/RunsList';
import RunDetail from '@/features/canary/RunDetail';
import NewRunModal from '@/features/canary/NewRunModal';
import { Loading, ErrorPane } from '@/features/canary/Loading';
import { useCanaryRuns } from '@/features/canary/useCanaryRuns';
import { useCanaryRun } from '@/features/canary/useCanaryRun';
import { useRunFacts } from '@/features/canary/useRunFacts';
import { useRunAlertEvidence } from '@/features/canary/useRunAlertEvidence';
import { useCanaryTemplate } from '@/features/canary/useCanaryTemplate';
import { stepStatesFor } from '@/features/canary/adapters';
import { ApiException } from '@/lib/api';

const msg = (e: unknown) => (e instanceof ApiException ? e.message : 'Unexpected error');

export function CanaryRoute() {
  const navigate = useNavigate();
  const [modalOpen, setModalOpen] = useState(false);
  const runs = useCanaryRuns();
  if (runs.isLoading) return <Loading label="Loading runs…" />;
  if (runs.isError) return <ErrorPane message={msg(runs.error)} />;
  return (
    <>
      <RunsList runs={runs.data ?? []} onOpenRun={(id: string) => navigate(`/runs/${id}`)} onNewRun={() => setModalOpen(true)} density="compact" />
      <NewRunModal open={modalOpen} onClose={() => setModalOpen(false)} onCreate={() => setModalOpen(false)} />
    </>
  );
}

export function RunDetailRoute() {
  const navigate = useNavigate();
  const { id = '' } = useParams();
  const detail = useCanaryRun(id);
  const tpl = useCanaryTemplate(detail.data?.run.template_id ?? '');
  const facts = useRunFacts(id, detail.data?.racks ?? []);
  const evidence = useRunAlertEvidence(id);

  if (detail.isLoading || tpl.isLoading) return <Loading label="Loading run…" />;
  if (detail.isError) return <ErrorPane message={msg(detail.error)} />;
  const run = detail.data!.run;
  const flatSteps = tpl.data?.flatSteps ?? [];
  const stepIndex = tpl.data?.stepIndex ?? {};
  const states = stepStatesFor(run, flatSteps, stepIndex);
  return (
    <RunDetail
      run={run}
      events={detail.data!.events}
      facts={facts.data ?? { source: 'inventory · cwf where' }}
      evidence={evidence.data ?? {}}
      steps={flatSteps}
      stepIndex={stepIndex}
      states={states}
      templateVersion={tpl.data?.version ?? 0}
      showRailGroups={false}
      onBack={() => navigate('/canary')}
    />
  );
}
```
(`useCanaryTemplate`/`useRunFacts` are enabled only once `detail.data` exists — they receive `''`/`[]` until then and run after; acceptable since we gate render on `detail.isLoading`. If a hook firing with an empty id is undesirable, add `enabled: !!id` options — optional.)

- [ ] **Step 5: Verify** — `cd web && yarn build` (succeeds) and run against a live backend (`make build && ./bin/qac serve` then open `/canary`): real runs list; clicking a row shows the real detail. `yarn lint`.

- [ ] **Step 6: Commit**
```bash
git add web/src/features/canary/RunsList.jsx web/src/features/canary/RunDetail.jsx web/src/features/canary/routes.tsx web/src/features/canary/Loading.jsx
git commit -m "qac: web Phase 2 — RunsList/RunDetail consume live data via props + loading/error"
```

---

## Task 4: MSW handlers (real shapes) + route tests

**Files:** Modify `web/src/mocks/handlers.ts`; Create `web/src/features/canary/routes.test.tsx`

- [ ] **Step 1: Real-shaped MSW handlers** — in `web/src/mocks/handlers.ts`, provide handlers returning real shapes for: `GET /api/runs` (`{runs:[RawRun]}`), `GET /api/runs/:id` (`{run, events:[RawEvent], racks}`), `GET /api/runs/:id/facts` (`{facts:{instance_type,…}}`), `GET /api/runs/:id/evidence` (honor `?step=`; `{evidence:[{deviceslot:null, payload:{probed_at,source,per_rack,max_severity,alerts_clear}}]}`), `GET /api/templates/:id` (`{template:{id,version,title,steps:[…]}}`), `GET /api/rlcc/workflows`. Read the file's existing `http`/`HttpResponse` (MSW v2) style and match it.

- [ ] **Step 2: Route tests** — create `web/src/features/canary/routes.test.tsx`: render `CanaryRoute` and `RunDetailRoute` inside a `QueryClientProvider` + `MemoryRouter` (MSW active via `setupTests.ts`); assert the list renders a mocked run's bundle; loading shows first; an error handler (override one request to 500) renders the ErrorPane; RunDetailRoute renders the run id/steps/facts. Match `AppShell.test.tsx`'s harness style.

- [ ] **Step 3: Run — expect PASS:** `cd web && yarn test --run`.

- [ ] **Step 4: Commit**
```bash
git add web/src/mocks/handlers.ts web/src/features/canary/routes.test.tsx
git commit -m "qac: web Phase 2 — MSW real-shape handlers + canary route tests (reads)"
```

---

# STAGE 2 — writes

## Task 5: create + action mutation hooks

**Files:** Create `web/src/features/canary/useCreateCanaryRun.ts`, `web/src/features/canary/useRunAction.ts`

- [ ] **Step 1: Implement** —

`useCreateCanaryRun.ts`:
```ts
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiPost } from '@/lib/api';

export interface CreateCanaryInput { bundle: string; racks: string[]; requester: string; rlcc_workflow: string; }

export function useCreateCanaryRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateCanaryInput) =>
      apiPost<{ run: { id: string } }, unknown>('/runs', {
        template_id: 'firmware-release-canary',
        inputs: { bundle_tag: input.bundle, canary_racks: input.racks, requester: input.requester, rlcc_workflow: input.rlcc_workflow },
      }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['canary', 'runs'] }); },
  });
}
```

`useRunAction.ts`:
```ts
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiPost } from '@/lib/api';

export interface RunActionInput {
  step_id: string; action: 'advance' | 'signoff';
  verdict?: string; signer_name?: string; signer_role?: string; notes?: string;
}

export function useRunAction(runId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: RunActionInput) => apiPost<{ run: unknown }, RunActionInput>(`/runs/${runId}/actions`, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['canary', 'runs', runId] });
      qc.invalidateQueries({ queryKey: ['canary', 'runs'] });
    },
  });
}
```

- [ ] **Step 2: Verify build** — `cd web && yarn build`.

- [ ] **Step 3: Commit**
```bash
git add web/src/features/canary/useCreateCanaryRun.ts web/src/features/canary/useRunAction.ts
git commit -m "qac: web Phase 2 — create + operator-action mutation hooks"
```

---

## Task 6: wire New-run (with workflow picker) + Sign-off/advance

**Files:** Modify `web/src/features/canary/NewRunModal.jsx`, `web/src/features/canary/RunDetail.jsx`, `web/src/features/canary/routes.tsx`

- [ ] **Step 1: NewRunModal — workflow picker + typed onCreate** — in `NewRunModal.jsx`:
  - Add a controlled `workflow` state + a `<select>` (design-styled, matching the existing inputs) populated from a `workflows` prop (`string[]`); include the existing `requester`/`bundle`/`racks` fields. Add `workflow` to the validity check.
  - Change `onCreate` to emit `{ bundle, racks, requester, rlcc_workflow: workflow }`.
  - Signature: `function NewRunModal({ open, onClose, onCreate, workflows = [] })`.

- [ ] **Step 2: CanaryRoute — wire create** — in `routes.tsx` `CanaryRoute`:
  - `const create = useCreateCanaryRun();` and `const workflows = useRlccWorkflows();` (`import { useRlccWorkflows } from '@/features/runs/useRlccWorkflows';` — read its return shape and map to a `string[]` of workflow names).
  - Pass `workflows={…}` to `NewRunModal`; set `onCreate={(input) => create.mutate({ ...input }, { onSuccess: () => { setModalOpen(false); toast.success('Run created'); }, onError: (e) => toast.error(msg(e)) })}` (`import { toast } from 'sonner';`).

- [ ] **Step 3: RunDetail — wire sign-off + advance** — in `RunDetail.jsx`:
  - The "Sign off" button calls `onSignoff` with the operator's verdict + signer fields. For a minimal first cut, gather signer name/role + a pass/fail verdict from the existing `SignoffPanel` inputs (or a small prompt) and call `onSignoff({ step_id: <current signoff step id>, verdict, signer_name, signer_role })`.
  - For `review`/`ai_assess` gated steps, surface an "Advance" affordance that calls `onAdvance({ step_id })`.
  - In `routes.tsx` `RunDetailRoute`: `const act = useRunAction(id);` pass `onSignoff={(input) => act.mutate({ action: 'signoff', ...input }, { onSuccess: () => toast.success('Signed off'), onError: (e) => toast.error(msg(e)) })}` and `onAdvance={(input) => act.mutate({ action: 'advance', ...input }, { onError: (e) => toast.error(msg(e)) })}`.

- [ ] **Step 4: Verify** — `cd web && yarn build && yarn lint`; live smoke (`make build && ./bin/qac serve`): create a run end-to-end; advance/sign off a halted run; UI refreshes.

- [ ] **Step 5: Tests** — extend `routes.test.tsx`: NewRunModal submit fires `POST /api/runs` with the mapped body (assert via an MSW handler capturing the request); Sign-off fires `POST /api/runs/:id/actions`. `yarn test --run`.

- [ ] **Step 6: Commit**
```bash
git add web/src/features/canary/NewRunModal.jsx web/src/features/canary/RunDetail.jsx web/src/features/canary/routes.tsx web/src/features/canary/routes.test.tsx
git commit -m "qac: web Phase 2 — wire New-run (workflow picker) + sign-off/advance to backend"
```

---

## Task 7: full verification

- [ ] **Step 1:** `cd web && yarn build` — succeeds.
- [ ] **Step 2:** `cd web && yarn test --run` — green (adapters + routes + Phase 1 smoke).
- [ ] **Step 3:** `cd web && yarn lint` — clean.
- [ ] **Step 4:** repo root `make build` (binary embeds the SPA), `make test`, `make lint` — green.
- [ ] **Step 5:** Live acceptance vs spec §7: `./bin/qac serve`, open `/canary` — real runs; open a run — real detail; create a run; advance/sign off. Confirm other nav items still render seed data.
- [ ] **Step 6:** Confirm no `@/design/seed` import remains in `RunsList.jsx`/`RunDetail.jsx`/`NewRunModal.jsx`: `grep -rn "@/design/seed" web/src/features/canary` → only non-page files (none expected).

---

## Self-review notes
- **Spec coverage:** §3 mapping → Task 1 adapters; §4 hooks/wrappers → Tasks 2-3; Stage 1 reads → Tasks 1-4; Stage 2 writes → Tasks 5-6; §6 testing → Tasks 1/4/6; §7 acceptance → Task 7. Loading/error → Task 3 (Loading.jsx + wrappers).
- **Type consistency:** `CanaryRun`/`RawRun`/`RunEvent`/`Facts`/`AlertEvidence`/`CanaryStep` defined in `types.ts` and used across adapters + hooks; `adaptRun(raw, events)`, `stepStatesFor(run, flatSteps, stepIndex)`, `detailFor(kind, payload)` signatures consistent between Task 1 def and Task 3 use; hook query keys namespaced `['canary',…]`.
- **Page prop signatures** (stable across stages): `RunsList({runs,flatSteps,stepIndex,onOpenRun,onNewRun,density})`; `RunDetail({run,events,facts,evidence,steps,stepIndex,states,templateVersion,showRailGroups,onBack,onSignoff,onAdvance})`; `NewRunModal({open,onClose,onCreate,workflows})`.
- **Open details for the implementer:** confirm `GET /api/templates/{id}` envelope (`{template:…}` vs bare) and `useRlccWorkflows` return shape (map to `string[]`); confirm `RUN_STATUS` in `design/components.jsx` covers `pending/running/passed/warning/failed/signed_off` (add missing pill styles); the design's `StepRail`/`StepRailItem` may reference seed globals at module scope — thread `steps`/`stepIndex` through as props.
