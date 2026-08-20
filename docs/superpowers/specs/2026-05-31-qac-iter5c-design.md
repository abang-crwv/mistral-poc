# qac iter-5c — wizard rewrite

**Status:** draft
**Branch:** `wp/add-qac` (continues iter-5b)
**Predecessor:** [iter-5b spec](2026-05-31-qac-iter5b-design.md), iter-5b HEAD at `1a8275c`
**PR posture:** still local. iter-5c is the first frontend iter since iter-2 and brings the SPA up to date with iter-3a / iter-5a / iter-5b backend surfaces.

## 1. Purpose

The SPA's `NewRunModal` is still the iter-2 flat `{bundle, rack}` form. The backend has moved through iter-3a (template-driven runs), iter-4 (event-sourced engine, alert + fact probes), iter-5a (RLCC + FLCC workflow clients), and iter-5b (per-CT enumeration + ignore lists). iter-5c catches the frontend up by replacing `NewRunModal` with a workflow-aware, BMN-aware form that exercises every iter-3a-onward surface.

After iter-5c ships:

- Operator picks an RLCC workflow from a dropdown driven by `GET /api/rlcc/workflows` (all 21 entries; state-movers greyed)
- Operator picks one or more canary racks; wizard auto-fetches `GET /api/inventory/preview?racks=...` and renders 18 BMN checkboxes per rack
- RLCC-ignored CTs come back pre-unchecked with a badge
- Operator can uncheck more CTs and supply an optional reason per ignore
- Operator fills bundle_tag and requester
- Submit posts `{template_id: "firmware-release-canary", inputs: {bundle_tag, canary_racks, rlcc_workflow, requester}, ignored_bmns: [...]}`
- Existing `RunsList` continues to render the new runs (no changes needed — `Run` type already carries `template_id`)
- Existing `RunDetail` continues to render flat events list (per-CT view is iter-5f)

No engine changes. No probe changes. No template materialization (iter-5d). The picked workflow lands in `inputs.rlcc_workflow` as a stored field that iter-5d will read when materialization wires it to the engine.

## 2. Scope decisions

| Decision | Value | Why |
|---|---|---|
| Workflow picker | **Load-bearing — selection goes to `inputs.rlcc_workflow`** | Operator's choice has to land somewhere for iter-5d to act on. Storing it in inputs (rather than a new top-level POST field) reuses the iter-3a inputs validation machinery — no backend changes beyond a template input addition. |
| Template input form | **Hardcoded for `fw-release-canary`** | iter-5c renders the four known inputs (bundle_tag, canary_racks, rlcc_workflow, requester) explicitly. Dynamic-form-from-template lands in iter-5d when materialized templates introduce varied inputs that vary per workflow. |
| Legacy modal | **Rip out fully on the frontend** | The legacy `NewRunInput` zod schema and `{bundle, rack}` form fields go away. Backend `normalize()` keeps accepting the legacy shape (seed-demo, integ tests, CLI callers still emit it) — no backend code change. |
| Wizard layout | **Single page** | Five fields. Multi-step adds wizard-state machinery without UX win at this scale. |
| Workflow breadth | **All 21 entries surfaced; state-movers (action_count=0) greyed and not pickable** | Matches iter-5a §2's "hiding decisions live in the UI, not the API." Operator sees the full catalog; the UI filters at render time. |
| Default workflow | **`gb200-rack-bringup-v4` if present in list; else first stable** | Matches the production-canary default. Falls back gracefully if the chart removes the canonical workflow. |
| RLCC-ignored badge | **Pre-unchecked + visible "RLCC ignored" pill on the row** | Iter-5b spec's wizard sketch. Pre-uncheck means the operator's "all checked" default still excludes auto-ignored CTs without manual fiddling. |
| Ignore reason | **One free-text field at the bottom of the BMN block — applies to all operator-unchecked CTs in this submit** | Per-row reason fields are over-engineering for iter-5c. One reason for the whole submit is what the iter-5b spec sketch shows. iter-5e can refine if operators ask. |
| Preview refresh | **Auto-refetch on rack list change, debounced 400ms** | Operator types/pastes racks; we want fresh BMN counts without hammering the endpoint. |
| Submit on stale preview | **Wizard re-fetches preview on submit and shows a diff modal if the BMN set changed** | iter-5b spec §9 risk mitigation. If the same rack now has fewer/different BMNs, the operator approves the new set before submit. |
| MSW handlers | **New handlers for `/api/rlcc/workflows`, `/api/inventory/preview`; existing `POST /api/runs` accepts the new body shape** | Tests run against MSW; the handlers mirror the real backend's response shapes. |
| `newRunSchema.ts` | **Rewrite to `{template_id, inputs: {bundle_tag, canary_racks, rlcc_workflow, requester}, ignored_bmns: [...]}`** | Validation moves from flat fields to a nested structure that mirrors the API contract. The rack regex stays on the canary_racks array's items. |
| Existing RunsList | **No changes** | The `Run` type already carries `template_id`; the list renders runs identically regardless of how they were created. |
| Existing RunDetail | **No changes** | iter-5f rewrites this when iter-5d (materialization) + iter-5e (per-CT probe) have produced per-CT data worth surfacing. |
| `firmware-release-canary.yaml` | **Add `rlcc_workflow` input (`type: text, required: false`)** | Required:false so iter-2 legacy callers (seed-demo, integ test) still validate. iter-5d flips it to required when materialization wires it in. |

## 3. Wizard UX

```
New canary run
─────────────────────────────────────────────────────────────────────

  Workflow
  [ gb200-rack-bringup-v4                                     v ]
  ├─ gb200-rack-bringup-v4               full L10-L12 bringup
  ├─ gb200-rack-bringup-v4-no-l10-test   debug variant
  ├─ gb200-rack-bringup-v4-no-seatrial   debug variant
  ├─ gb200-rack-hpc-verification-v4      hpc verify only
  ├─ b300-rack-bringup-v0                B300 bringup
  └─ ─── state movers (no actions) ──────────────────────── (greyed)
     checked-in · onboarding · ready · triage

  Firmware bundle tag
  [ dell-xe9712-default-2.0.0-rc                                 ]

  Canary racks  (one per line, format dh#-r###-region-zone)
  [ dh3-r012-us-east-01a                                         ]
  [                                                              ]
  [ + add rack ]

  Compute trays  (17 of 18 in scope · 1 RLCC ignored)
  ┌─ dh3-r012-us-east-01a · gb200-4x · 18 CTs ─────────────────┐
  │  [✓] CT-01  s90txs51                                       │
  │  [✓] CT-02  s90txs52                                       │
  │  [✓] CT-03  s90txs53                                       │
  │  [○] CT-04  s90txs54   ⟨RLCC ignored⟩  (pre-unchecked)     │
  │  [✓] CT-05  s90txs55                                       │
  │  ...                                                       │
  │  [✓] CT-18  s90txs68                                       │
  │                                                            │
  │  Reason for any custom ignores (optional)                  │
  │  [                                                       ] │
  └────────────────────────────────────────────────────────────┘

  Requester  (from #havock)
  [ wpena@coreweave.com                                          ]

                                       [ Cancel ]  [ Create run ]
```

States the wizard handles:

- **Initial load** — workflow dropdown populated from `/api/rlcc/workflows`; default selection is `gb200-rack-bringup-v4`; racks list is empty; BMN block hidden until at least one valid rack is entered.
- **Rack typed/pasted** — on debounced change (400ms), wizard fetches `/api/inventory/preview?racks=<comma-joined>`; renders per-rack BMN blocks; pre-unchecks RLCC-ignored rows with the badge.
- **Preview load error** — banner above the BMN block: "Couldn't load BMN list for rack X (try again or proceed without per-CT controls)." The Create button remains enabled (backend re-validates).
- **Workflow change** — refreshes the dropdown's selected value; does NOT re-fetch preview (BMN data is rack-keyed, not workflow-keyed).
- **Submit click** — re-fetches preview synchronously to catch drift since the last debounced fetch; if the BMN set per rack changed, shows a "Inventory changed since you opened this form — review and resubmit" modal with a diff (added/removed BMNs); otherwise POSTs and navigates to the run-detail page.
- **Backend 400 on submit** — renders the error message inline above the form; preserves form state so operator can correct and retry.

## 4. Types and validation

### 4.1 Wire types (`web/src/features/runs/runs.types.ts`)

```typescript
// Workflow catalog entry from GET /api/rlcc/workflows
export interface WorkflowSummary {
  name: string;
  description: string;
  lifecycle: 'stable' | 'deprecated';
  current_states: string[];
  desired_state: string;
  rack_kind: string;       // gb200 | b300 | gbx00 | bx00 | b200 | ''
  action_count: number;    // 0 = state-mover, grey it out
}

// BMN from GET /api/inventory/preview
export interface PreviewBMN {
  deviceslot: string;
  bmn_name: string;
  ct_position?: number;
}

export interface PreviewIgnored {
  deviceslot: string;
  bmn_name?: string;
}

export interface PreviewRack {
  rack: string;
  zone: string;
  instance_type?: string;
  sku?: string;
  bmns: PreviewBMN[];
  rlcc_ignored?: PreviewIgnored[];
}

export interface PreviewError {
  rack: string;
  message: string;
}

export interface PreviewResponse {
  racks: PreviewRack[];
  errors?: PreviewError[];
}
```

### 4.2 `newRunSchema.ts` rewrite (zod)

```typescript
import { z } from 'zod';

const RACK_REGEX = /^dh\d+-r\d{3}-[a-z]+-[a-z]+-\d{2}[a-z]$/;

export const ignoredBMNSchema = z.object({
  rack: z.string(),
  deviceslot: z.string(),
  reason: z.string().optional(),
});

export const newRunSchema = z.object({
  template_id: z.literal('fw-release-canary'),
  inputs: z.object({
    bundle_tag: z.string().min(1, 'Bundle tag is required'),
    canary_racks: z.array(z.string().regex(RACK_REGEX, 'Invalid rack format')).min(1, 'At least one canary rack is required'),
    rlcc_workflow: z.string().min(1, 'Workflow is required'),
    requester: z.string().email('Must be a valid email').optional(),
  }),
  ignored_bmns: z.array(ignoredBMNSchema).default([]),
});

export type NewRunInput = z.infer<typeof newRunSchema>;
```

The shared rack regex stays mirrored against `internal/api/runs_validate.go` (Go) and `templates/firmware-release-canary.yaml` (template input validation). The mirror comment in the template file already calls this out.

## 5. Hooks and data flow

### 5.1 `useRLCCWorkflows.ts` (new)

```typescript
import { useQuery } from '@tanstack/react-query';
import type { WorkflowSummary } from './runs.types';

interface WorkflowsResponse {
  workflows: WorkflowSummary[];
  source: { chart_tag: string; sha: string };
}

export function useRLCCWorkflows() {
  return useQuery({
    queryKey: ['rlcc', 'workflows'],
    queryFn: async () => {
      const res = await fetch('/api/rlcc/workflows');
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      return (await res.json()) as WorkflowsResponse;
    },
    staleTime: 5 * 60 * 1000, // 5 min — matches backend cache
  });
}
```

### 5.2 `useInventoryPreview.ts` (new)

```typescript
import { useQuery } from '@tanstack/react-query';
import type { PreviewResponse } from './runs.types';

export function useInventoryPreview(racks: string[]) {
  const enabled = racks.length > 0 && racks.every(r => r.trim().length > 0);
  const key = racks.join(',');
  return useQuery({
    queryKey: ['inventory', 'preview', key],
    enabled,
    queryFn: async () => {
      const res = await fetch(`/api/inventory/preview?racks=${encodeURIComponent(key)}`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      return (await res.json()) as PreviewResponse;
    },
    staleTime: 60 * 1000, // 60s — matches backend TTL
  });
}
```

### 5.3 `useCreateRun.ts` (modified)

```typescript
import { useMutation, useQueryClient } from '@tanstack/react-query';
import type { NewRunInput } from './newRunSchema';
import type { Run } from './runs.types';

interface CreateRunResponse {
  run: Run;
}

export function useCreateRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: NewRunInput): Promise<CreateRunResponse> => {
      const res = await fetch('/api/runs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error?.message ?? `HTTP ${res.status}`);
      }
      return await res.json();
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['runs'] });
    },
  });
}
```

The mutation `input` type is `NewRunInput` (the new shape). The legacy `{ bundle, rack }` shape is no longer accepted by this hook.

## 6. Template change

`templates/firmware-release-canary.yaml` gets one new input. Bump version `4 → 5`:

```yaml
schema: qac.template/v1
id: fw-release-canary
version: 5     # bumped from 4
title: "FW release canary"

target:
  scope: rack
  instance_types: [gb200-4x, gb300-4x]

inputs:
  - id: bundle_tag
    label: "FW bundle tag"
    type: text
    required: true
    example: "dell-xe9712-default-2.0.0-rc"

  - id: canary_racks
    label: "Canary racks"
    type: multi_text
    required: true
    validate: "^dh\\d+-r\\d{3}-[a-z]+-[a-z]+-\\d{2}[a-z]$"

  # iter-5c addition — wizard picks an RLCC workflow from /api/rlcc/workflows.
  # required:false so iter-2 legacy callers (seed-demo, integ tests) still
  # validate. iter-5d will flip this to required:true when materialization
  # actually reads this value to splice the RLCC spine.
  - id: rlcc_workflow
    label: "RLCC workflow"
    type: text
    required: false
    example: "gb200-rack-bringup-v4"

  - id: requester
    label: "Requester (from #havock)"
    type: text
    required: false

steps:
  # existing 14 steps unchanged
  ...
```

The `internal/template/embedded_test.go` Version assertion bumps from 4 to 5. Step count stays 14.

## 7. Wizard structure (file layout)

```
MODIFY
  web/src/features/runs/NewRunModal.tsx       Full rewrite. Form structure per §3.
  web/src/features/runs/newRunSchema.ts       Rewrite to nested shape per §4.2.
  web/src/features/runs/useCreateRun.ts       Mutation signature change per §5.3.
  web/src/features/runs/runs.types.ts         Add WorkflowSummary, Preview* types per §4.1.
  web/src/mocks/handlers.ts                   New MSW handlers for the two GETs;
                                              POST /api/runs accepts the new shape.
  web/src/features/runs/NewRunModal.test.tsx  Rewrite — replace flat-field assertions
                                              with workflow-dropdown + BMN-checkbox flow.
  templates/firmware-release-canary.yaml            Bump version 4 → 5; add rlcc_workflow input.
  internal/template/embedded_test.go          Version assertion 4 → 5.

CREATE
  web/src/features/runs/useRLCCWorkflows.ts   TanStack Query hook for the workflow catalog.
  web/src/features/runs/useInventoryPreview.ts TanStack Query hook for the BMN preview.
  web/src/features/runs/BMNCheckboxList.tsx   Per-rack BMN checkbox subcomponent.
                                              Renders one rack's BMN list with auto-pre-uncheck
                                              for RLCC-ignored rows.
```

`BMNCheckboxList.tsx` is split out so the testing surface stays tight: one component per behavior (workflow dropdown, BMN list, top-level form). The parent `NewRunModal.tsx` owns form state (react-hook-form), the dropdown, and the submit logic; child components consume controlled inputs.

## 8. Tests

| File | Asserts |
|---|---|
| `NewRunModal.test.tsx` | Workflow dropdown renders entries from MSW; default selection is `gb200-rack-bringup-v4`; state-movers (`action_count: 0`) are present but disabled in the dropdown; typing a rack triggers preview fetch; BMN checkboxes render after fetch; RLCC-ignored rows are pre-unchecked with the badge; uncheck on a CT updates `ignored_bmns` on submit; submit POSTs the new body shape including operator-unchecked BMNs |
| `BMNCheckboxList.test.tsx` (new) | Renders the right count of checkboxes given a `PreviewRack`; RLCC-ignored deviceslots pre-unchecked + badged; user uncheck/check propagates via the controlled `onChange` prop; reason field's value flows through |
| `useRLCCWorkflows.test.ts` (new) | Returns workflow list from MSW; 5min staleTime |
| `useInventoryPreview.test.ts` (new) | Disabled when racks list is empty; fetches when non-empty; cache key includes the joined rack list |
| `internal/template/embedded_test.go` (modify) | Version is now 5; step count still 14 |

Existing `RunsList.test.tsx` doesn't change. Existing backend tests don't change (the new shape was already accepted by `normalize()` since iter-3a; the `rlcc_workflow` input is non-required so existing test bodies pass).

## 9. Risks and open questions

**Risk: drift between wizard preview and run-create.** A BMN appears in preview but is removed before submit. Wizard re-fetches preview on submit-click and shows a diff modal (§3) — operator approves the new state before the POST. iter-5b backend validates again on the server side; this is belt + braces.

**Risk: large rack counts and BMN render performance.** A 36-BMN rack today is fine; if a future GB300 form factor has 72 BMNs the checkbox list grows. iter-5c does no virtualization. If it becomes a problem, `react-virtual` is a one-PR addition in iter-5f or later.

**Risk: workflow dropdown gets long** (21 entries today, more if other rack kinds land). iter-5c uses a native `<select>` (no search). If operators ask for filter/search, swap to a Radix combobox in a follow-up.

**Open question:** should the BMN reason field be per-row or global? **Decided:** global (one field below the list) for iter-5c. Operators can describe the reason for the whole submit. Per-row reason is iter-5f or later.

**Open question:** how does the wizard surface a `lifecycle: "deprecated"` workflow? **Decided:** show in the dropdown with a "deprecated" tag, but allow selection. Operators may want to retest old workflows. We don't filter at the API or the UI.

**Open question:** what if `/api/rlcc/workflows` returns degraded data (the `X-Qac-Degraded: true` header from iter-5a)? **Decided:** wizard reads the header and shows a small "using fixture data" indicator next to the workflow dropdown. Doesn't block submit.

**Open question:** should we render the picked workflow's action list as a preview? **Decided:** not in iter-5c. iter-5d's materialization is what makes that data load-bearing; iter-5c stops at "operator picks the name." A read-only action preview can land in iter-5d or iter-5f.

## 10. Out of scope

- Template materialization at pinned `source_sha` (iter-5d)
- `source_sha` field on the POST body (iter-5d)
- Dynamic input form rendered from `template.inputs[]` (iter-5d)
- `rlcc_action_probe` and per-CT lifecycle polling (iter-5e)
- Per-CT UI on `RunDetail` (iter-5f)
- Mid-run RLCC ignore re-detection display (iter-5f timeline entries)
- Workflow action preview / step rail (iter-5f)
- Searchable workflow dropdown (post-5f if asked for)
- Per-row ignore reason (post-5f if asked for)
- Rename to QAgenticCow (deferred; tracked separately as task #160)

## 11. Acceptance criteria

After iter-5c is merged on `wp/add-qac`:

1. `make test` is green (Go + web)
2. `make lint` is green
3. `cd web && yarn dev` serves the SPA; opening "New run" shows the new wizard, not the legacy form
4. The workflow dropdown populates from `/api/rlcc/workflows` (use `QAC_RLCC_BACKEND=map` to get fixture data); `gb200-rack-bringup-v4` is selected by default
5. The 4 state-mover entries (`checked-in`, `onboarding`, `ready`, `triage`) appear in the dropdown but are disabled (not selectable)
6. Typing `dh3-r012-us-east-01a` in the canary racks field renders an 18-BMN checkbox block within ~500ms (debounce + fetch)
7. With a MapClient that seeds an RLCC ignore for `dh3-r012-node-03-us-east-01a`, that row appears pre-unchecked with the "RLCC ignored" badge
8. Unchecking `dh3-r012-node-07-us-east-01a` and submitting POSTs `{template_id, inputs: {…, rlcc_workflow: "gb200-rack-bringup-v4"}, ignored_bmns: [{rack, deviceslot: "dh3-r012-node-07-us-east-01a"}, {rack, deviceslot: "dh3-r012-node-03-us-east-01a"}]}`
9. `GET /api/runs/{id}` for that run shows `racks[0].ct_count: 18` and 2 `ignored[]` entries (one operator, one rlcc)
10. The iter-2 legacy `{bundle, rack}` form is gone from the frontend; `seed-demo` and the integ tests (which still emit the legacy shape via backend code paths) continue to work — backend `normalize()` is unchanged
11. `internal/template/embedded_test.go` asserts Version == 5

## 12. Forward references

| iter | Adds |
|---|---|
| **5d** | Template materialization: `createRunHandler` re-fetches the picked RLCC workflow at `source_sha`, splices its spine into a transient template, embeds in `RunCreated.payload`. `loadTemplateFromEvents` reads from payload. Frontend adds `source_sha` to the POST body. `rlcc_workflow` input flips to `required: true`. |
| **5e** | `rlcc_action_probe`. Full `lifecycleclient.PollLifecycle` + `ListBMNsInRack` impls. Per-tick PromQL fanout. Per-CT evidence rows with `deviceslot` populated. Per-tick RLCC ignore re-detection. `BMNObserved` event kind. |
| **5f** | Per-CT UI on `RunDetail`. Per-CT badges (ignored by operator / RLCC / both). Timeline entries for `BMNIgnored` events. Per-rack progress grid. Optionally: searchable workflow dropdown, per-row reason field, workflow action preview, BMN-list virtualization. |
| **6+** | Real production resolver. `workflowOverrides`. RLCC `triggers:` block. Dispatch. Rename to QAgenticCow. |
