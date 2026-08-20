# New Run Wizard — port into `web/src`

Replaces the flat `NewRunModal` ({bundle, rack}) with a 3-step **New Run wizard** that
drives the canonical create-run flow against your real API:

1. **Inputs** — pick a template (`GET /api/templates`, `GET /api/templates/{id}`) and fill its
   `inputs[]` dynamically (renderer driven by the template's input model).
2. **Inventory** — `GET /api/inventory/preview?racks=…` resolves per-rack facts + BMN/CT
   enumeration; operator can mark extra CTs to ignore (with a reason). RLCC-ignored CTs are shown
   pre-checked and read-only. `GET /api/rlcc/workflows` powers a context panel.
3. **Review** — summary, then `POST /api/runs` with `{template_id, inputs, ignored_bmns}`.

Everything matches your existing conventions: plain Tailwind utilities, `cva` Badge, react-query
`useX` hooks over `apiGet`/`apiPost`, feature-folder layout, `@/` alias, MSW fixtures, Vitest +
Testing Library. No new dependencies.

## Drop-in file map

Copy each file to the matching path under `web/`:

```
src/features/runs/newRun.types.ts            NEW   contract types (mirror Go wire shapes)
src/features/runs/buildInputsSchema.ts       NEW   dynamic zod + coercion (mirrors ValidateInputs)
src/features/runs/useTemplates.ts            NEW   useTemplates() + useTemplate(id)
src/features/runs/useInventoryPreview.ts     NEW   GET /api/inventory/preview
src/features/runs/useRlccWorkflows.ts        NEW   GET /api/rlcc/workflows
src/features/runs/useCreateRun.ts            REPLACE  now posts {template_id, inputs, ignored_bmns}
src/features/runs/wizard/wizardShared.ts     NEW   step constants + ignore helpers
src/features/runs/wizard/StepInputs.tsx      NEW   step 1
src/features/runs/wizard/StepInventory.tsx   NEW   step 2
src/features/runs/wizard/StepReview.tsx      NEW   step 3
src/features/runs/wizard/NewRunWizard.tsx    NEW   orchestrator (replaces NewRunModal)
src/features/runs/wizard/NewRunWizard.test.tsx NEW focused test
src/features/runs/RunsList.tsx               REPLACE  renders <NewRunWizard> instead of <NewRunModal>
src/components/Dialog.tsx                     REPLACE  adds optional size?: 'sm'|'lg' (default 'sm', non-breaking)
src/mocks/handlers.ts                         REPLACE  adds templates / preview / rlcc handlers; canonical POST
src/lib/api.ts                                REPLACE  adds apiGetWithMeta + ApiException.details (backward-compatible)
```

After copying, **delete** the obsolete flat modal and its test:

```
src/features/runs/NewRunModal.tsx
src/features/runs/NewRunModal.test.tsx
```

`newRunSchema.ts` is no longer used by the UI (the wizard builds its schema from the template's
`inputs[]`). It's still mirrored to `internal/api/runs_validate.go`, so leave it unless you also drop
the legacy `{bundle, rack}` server path — your call.

## Contract notes (verify against your backend)

- **POST body:** `{ template_id: string, inputs: Record<string,unknown>, ignored_bmns?: {rack, deviceslot, reason?}[] }`.
  `inputs` is keyed by template input id — for `firmware-release-canary`: `bundle_tag` (string),
  `canary_racks` (string[]), `requester?` (string). Empty optional values are omitted.
- **Rack list key:** the backend's `extractRacks` reads `inputs["canary_racks"]`. That id is hard-coded
  in `wizardShared.ts` as `RACKS_INPUT_ID`; change both together if it ever moves.
- **`ignored_bmns`** only carries *operator* ignores. RLCC-detected ignores (`previewRack.rlcc_ignored`,
  source=rlcc) are applied server-side and shown read-only — don't resend them.
- **Error codes** (`internal/api/errors.go`): the wizard branches on `invalid_request`,
  `template_not_found`, `inventory_unresolved`, `bmns_unresolved`.

## `lib/api.ts` change (backward-compatible)

`apiGet<T>` and `apiPost<TRes,TBody>` keep their exact signatures and return the parsed body, so
every existing call site (`useRuns`, `useRun`, the old `useCreateRun`, `api.test.ts`) is unchanged.
What's added:

- **`apiGetWithMeta<T>(path): Promise<{ data: T; headers: Headers }>`** — for callers that need
  response headers. `useInventoryPreview` and `useRlccWorkflows` use it to read `X-Qac-Degraded`.
- **`ApiException.details?`** — the error envelope's `details` (e.g. `inventory_unresolved`'s
  `{ unresolved: string[] }`) is now parsed in both `apiGet` and `apiPost` and thrown on the
  exception. Old two-arg `new ApiException(code, message)` calls still compile (`details` optional).

## Deliberate deviations / follow-ups

- **Controlled state + zod, not react-hook-form.** Your existing flat modal used RHF + `zodResolver`
  with a *static* schema (`newRunSchema.ts`). The wizard's fields are **dynamic** — they come from the
  selected template's `inputs[]`, with per-input `type` / `required` / `validate` — so it uses local
  controlled state + `validateInputs()` (a zod schema built at runtime from the template) instead of
  RHF's static resolver. zod remains the validator; the split is: **static form → RHF, dynamic
  template-driven form → controlled state.** If you'd rather force RHF here too, it's doable with
  `useForm` + `useFieldArray` over the dynamic ids, at some complexity cost.
- **`X-Qac-Degraded` is surfaced** via `apiGetWithMeta`: the Inventory step shows a "fixture data"
  banner and the RLCC panel tags its source when degraded.
- **`error.details` is surfaced**: `inventory_unresolved` / `bmns_unresolved` details (incl. the
  `unresolved[]` rack list) render on the Review step's error block. The Inventory step also lists
  unresolved racks from the preview's own `errors[]`.
- **Multiple templates:** the picker only renders when `GET /api/templates` returns more than one.
