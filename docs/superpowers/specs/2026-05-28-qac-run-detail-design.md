# qac iteration 2 — run-detail page and new-run modal

**Date:** 2026-05-28
**Status:** Draft, pending user sign-off
**Predecessor spec:** `2026-05-27-qac-rebuild-design.md` (walking skeleton)
**Branch:** `wp/add-qac` (continuing — walking-skeleton PR not yet opened)

---

## 1. Purpose

Replace the stub `/runs/:id` page that ships in the walking skeleton, and add a "New run" affordance so the operator can create canary runs from the browser (today the only way is `qac seed-demo` from the CLI).

The walking skeleton proved the spine (Go binary → embedded SPA → JSON API → SQLite event log → runs list page). This iteration makes the app **navigable as a real tool**: create a run, then see what was created. It does not yet add probes (no real fleet integration); the goal is "the UX shape exists end-to-end" rather than "the canary actually verifies firmware".

## 2. Scope (what this iteration ships)

1. **Backend:** one new endpoint `POST /api/runs` that validates `(bundle, rack)`, generates a ULID, emits a `RunCreated` event, and returns the created run.
2. **Schema:** add a `bundle TEXT NOT NULL DEFAULT ''` column to the `runs` table, with an idempotent migration that tolerates pre-existing databases.
3. **Frontend new-run modal:** a Radix dialog with two fields (firmware bundle, rack). Submits → navigates to the new run's detail page.
4. **Frontend run-detail page:** replaces the stub at `/runs/:id`. Shows the run's metadata header plus an event log table.
5. **Frontend list-page changes:** add a "New run" button that opens the modal; replace the (always-`fw-release-canary`) `Template` column with a `Bundle` column.

## 3. Out of scope

Carried forward from the walking-skeleton spec §12, minus what's now in this iteration:

- Template loader (`qac template load`)
- First real probe (e.g., `baseline_pre` against a stubbed inventory)
- SSE for live event streaming on the detail page
- The five RLCC/FLCC checklists
- Per-node evidence table
- AI summary + operator sign-off → Slack draft text
- Multi-template support (the form hard-codes `fw-release-canary`)
- Bundle uniqueness constraints (same `(bundle, rack)` can be canaried multiple times)

## 4. Constraints carried forward

1. **Self-contained.** No `replace` directives, no sibling-repo imports. Module path remains bare `qac`.
2. **Single binary.** `make build` still produces one executable at `team/wpena/bin/qac`.
3. **Cloud Console copy guide.** All new UI text follows sentence-case, "and" not "&", no exclamation points, technical identifiers preserved verbatim. New strings introduced in this iteration:
   - "New run" (button label)
   - "New canary run" (modal title)
   - "Firmware bundle" (form label)
   - "Rack" (form label)
   - "Comma-separate for multiple racks." (helper text)
   - "Bundle is required" / "Bundle is too long (max 200 characters)" / "At least one rack is required" / "Rack format example: dh3-r012-us-east-01a" (validation messages)
   - "Run not found." (detail page empty state)
   - "Back to runs" (link)
   - "Run" (detail page heading)
   - "Events" (section heading)
4. **Pinned stack.** No new top-level dependencies beyond `@radix-ui/react-dialog`, which is already in the cloud-console subset we adopted (just not yet installed in `package.json`).

## 5. Rack name format

Validation accepts the canonical CoreWeave rack format (post-2025-07-02 rename, per Glean: FPA-1509 + Jordan Dahmen's "Rack Naming Convention" Google Doc):

```
<datahall>-<rack>-<zone>
dh<N>-r<NNN>-<region>-<NN><letter>
```

Examples:
- `dh3-r012-us-east-01a`
- `dh4-r066-us-east-11a`

**Regex (mirrored on both sides):**

```
^dh\d+-r\d{3}-[a-z]+-[a-z]+-\d{2}[a-z]$
```

Deliberately rejects:
- The pre-2025 format (`us-east-01a-dh3-r012`)
- Test/dummy names (`as1-r07`)
- CP5 construction-phase racks (`CP5-v2a`) — they're pre-production and not valid canary targets
- Mixed-case identifiers (the canonical form is lowercase everywhere)

If a CP5 rack ever needs to be canaried, the regex grows an OR-prefix. Tracked as a future revisit.

## 6. Architecture

Iteration 2 is purely **additive** to the walking skeleton. No existing files are renamed or restructured. The boundaries from the prior spec (api → engine → store; features → lib in the frontend) hold without change.

Three categories of file change:

1. **Backend new files:** request validator + new POST handler colocated with existing runs handlers.
2. **Backend modifications:** `migrations.go` (add idempotent ALTER), `runs.go` (add `createRunHandler`), `api.go` (register POST), `events.go` (store `bundle` in the `RunCreated` branch), `runs.go` projection (include `bundle` in `Run` struct + queries).
3. **Frontend new + modified files:** modal + detail page + mutation hook + dialog primitive + zod schema; existing list page gets a "New run" button and a column swap.

### Boundaries (unchanged from walking skeleton)

- `internal/api` is the only thing crossing HTTP → engine.
- `internal/engine` is the only thing reaching `internal/store`.
- The new `runs_validate.go` is a sibling helper in the `api` package — request-validation concerns colocate with the handler that owns them.
- Frontend features own their hooks/types/components. `Dialog.tsx` is shared infrastructure (`components/`), `NewRunModal.tsx` is domain (`features/runs/`).

### File changes

```
team/wpena/qac/
├── internal/store/
│   ├── migrations.go              # MODIFY: add idempotent bundle column ALTER
│   ├── store_test.go              # ADD: TestMigrate_AddsBundleColumn
│   ├── events.go                  # MODIFY: store bundle in runs row on RunCreated
│   └── runs.go                    # MODIFY: include bundle in Run struct + queries
├── internal/api/
│   ├── runs.go                    # MODIFY: add createRunHandler
│   ├── api.go                     # MODIFY: register POST /api/runs
│   ├── runs_validate.go           # NEW: rack regex + ValidateRacks func
│   ├── runs_validate_test.go      # NEW: table-driven validator tests
│   └── api_test.go                # ADD: 4 POST tests
└── web/src/
    ├── components/
    │   └── Dialog.tsx                       # NEW: Radix Dialog wrapper
    ├── features/runs/
    │   ├── runs.types.ts                    # MODIFY: add `bundle` to Run; add Event type
    │   ├── newRunSchema.ts                  # NEW: zod schema
    │   ├── NewRunModal.tsx                  # NEW: form + dialog + mutation submit
    │   ├── NewRunModal.test.tsx             # NEW: 4 cases
    │   ├── useCreateRun.ts                  # NEW: TanStack Query mutation
    │   ├── useRun.ts                        # NEW: TanStack Query hook for detail
    │   ├── RunDetail.tsx                    # NEW: header + event log
    │   ├── RunDetail.test.tsx               # NEW: 3 cases
    │   └── RunsList.tsx                     # MODIFY: New-run button; Bundle column
    ├── lib/
    │   ├── api.ts                           # ADD: apiPost<TRes, TBody>
    │   └── api.test.ts                      # ADD: 2 apiPost cases
    ├── mocks/
    │   └── handlers.ts                      # MODIFY: POST + GET-by-id + fixture bundles
    └── App.tsx                              # MODIFY: replace /runs/:id stub with <RunDetail />
```

## 7. Schema migration

The walking skeleton's `migrations.go` uses `CREATE TABLE IF NOT EXISTS`, which does not add columns to a pre-existing table. We need an explicit `ALTER` for the new `bundle` column, AND it must be safe to run twice (operator may have a database from the walking-skeleton era).

Approach: introspect `pragma_table_info('runs')` for an existing `bundle` column; only run `ALTER TABLE ... ADD COLUMN` when missing.

```go
func (s *Store) migrate(ctx context.Context) error {
    if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
        return fmt.Errorf("apply base schema: %w", err)
    }
    if err := s.addBundleColumnIfMissing(ctx); err != nil {
        return fmt.Errorf("add bundle column: %w", err)
    }
    return nil
}

func (s *Store) addBundleColumnIfMissing(ctx context.Context) error {
    row := s.db.QueryRowContext(ctx,
        `SELECT COUNT(*) FROM pragma_table_info('runs') WHERE name = 'bundle'`)
    var n int
    if err := row.Scan(&n); err != nil {
        return err
    }
    if n > 0 {
        return nil
    }
    _, err := s.db.ExecContext(ctx,
        `ALTER TABLE runs ADD COLUMN bundle TEXT NOT NULL DEFAULT ''`)
    return err
}
```

Test (`TestMigrate_AddsBundleColumn`) opens a fresh DB, verifies the column exists, closes, reopens, verifies no error on the second open.

When iteration 3 ships, a real migrations table is the right next step (we already accept that the current single-statement migration won't scale). Tracked as a future revisit.

## 8. API surface additions

### `POST /api/runs`

**Request:**

```json
{ "bundle": "gb200-fw-2026-05-canary-3", "rack": "dh3-r012-us-east-01a, dh4-r066-us-east-11a" }
```

- `bundle`: required, non-empty after trim, max 200 characters
- `rack`: required, non-empty after trim, each comma-separated token must match the canonical regex

Server canonicalizes `rack` (trims tokens, joins with `","` and no spaces).

**Response (success — 201 Created):**

```json
{
  "run": {
    "id": "01HZX...",
    "bundle": "gb200-fw-2026-05-canary-3",
    "template_id": "firmware-release-canary",
    "rack": "dh3-r012-us-east-01a,dh4-r066-us-east-11a",
    "status": "pending",
    "created_at": "...",
    "updated_at": "...",
    "created_by": "wpena"
  }
}
```

**Response (validation failure — 400):**

```json
{ "error": { "code": "invalid_request", "message": "Rack \"as1-r07\" is not in the expected format (example: dh3-r012-us-east-01a)" } }
```

`code` values:
- `invalid_request` for any 400 (malformed JSON, missing field, regex mismatch, length limits)
- `internal` for 500 (store failure)

`template_id` is server-set to `"firmware-release-canary"`; `created_by` is server-set to `"wpena"`. Both are baked in for iteration 2 — multi-template + auth are deferred.

### Existing endpoints — minor changes

- `GET /api/runs` and `GET /api/runs/:id` continue to return the same JSON shape, now with a `bundle` field on every `Run`. Existing tests in `api_test.go` need a small update: `TestGetRuns_ReturnsSeededList` must seed via the new schema (the `seed-demo` payload remains unchanged at the wire level; the new `events.go` handler reads `bundle` from the payload if present and defaults to `""` otherwise — that's the migration for old data).
- `GET /api/health` unchanged.

## 9. Validation (`runs_validate.go`)

```go
package api

import (
    "fmt"
    "regexp"
    "strings"
)

var rackPattern = regexp.MustCompile(`^dh\d+-r\d{3}-[a-z]+-[a-z]+-\d{2}[a-z]$`)

// ValidateRacks splits a comma-separated rack string, trims whitespace,
// validates each token against rackPattern, and returns the canonical
// joined form along with the parsed slice. Returns a user-facing error
// message on the first invalid token. Empty string means OK.
func ValidateRacks(input string) (canonical string, racks []string, errMsg string) {
    parts := strings.Split(input, ",")
    out := make([]string, 0, len(parts))
    for _, p := range parts {
        p = strings.TrimSpace(p)
        if p == "" {
            continue
        }
        if !rackPattern.MatchString(p) {
            return "", nil, fmt.Sprintf("Rack %q is not in the expected format (example: dh3-r012-us-east-01a)", p)
        }
        out = append(out, p)
    }
    if len(out) == 0 {
        return "", nil, "At least one rack is required"
    }
    return strings.Join(out, ","), out, ""
}
```

**Bundle validation** is inline in `createRunHandler` (trivial: trim, check non-empty, check `len <= 200`). Not worth a separate helper for now.

## 10. Handler (`runs.go`)

```go
func createRunHandler(s *store.Store) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req struct {
            Bundle string `json:"bundle"`
            Rack   string `json:"rack"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeError(w, http.StatusBadRequest, codeInvalidRequest, "Invalid JSON body")
            return
        }

        bundle := strings.TrimSpace(req.Bundle)
        if bundle == "" {
            writeError(w, http.StatusBadRequest, codeInvalidRequest, "Bundle is required")
            return
        }
        if len(bundle) > 200 {
            writeError(w, http.StatusBadRequest, codeInvalidRequest, "Bundle is too long (max 200 characters)")
            return
        }

        canonical, _, errMsg := ValidateRacks(req.Rack)
        if errMsg != "" {
            writeError(w, http.StatusBadRequest, codeInvalidRequest, errMsg)
            return
        }

        runID := ulid.Make().String()
        payload, _ := json.Marshal(map[string]string{
            "template_id": "firmware-release-canary",
            "bundle":      bundle,
            "rack":        canonical,
            "created_by":  "wpena",
        })
        if err := s.AppendEvent(r.Context(), runID, "RunCreated", payload); err != nil {
            writeError(w, http.StatusInternalServerError, codeInternal, "Failed to create run")
            return
        }

        // Fetch the just-created run from the projection.
        runs, err := s.ListRuns(r.Context())
        if err != nil {
            writeError(w, http.StatusInternalServerError, codeInternal, "Run created but failed to load")
            return
        }
        for i := range runs {
            if runs[i].ID == runID {
                writeJSON(w, http.StatusCreated, map[string]any{"run": runs[i]})
                return
            }
        }
        writeError(w, http.StatusInternalServerError, codeInternal, "Run not found after create")
    }
}
```

### `events.go` change

The existing `runCreatedPayload` struct grows a `Bundle` field:

```go
type runCreatedPayload struct {
    TemplateID string `json:"template_id"`
    Bundle     string `json:"bundle"`
    Rack       string `json:"rack"`
    CreatedBy  string `json:"created_by"`
}
```

The `RunCreated` branch in `AppendEvent` writes `bundle` to the new column:

```sql
INSERT INTO runs (id, template_id, bundle, created_at, updated_at, status, rack, created_by)
VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)
ON CONFLICT(id) DO NOTHING
```

Old payloads without `bundle` decode to the zero value `""` — preserves backwards compatibility with the walking-skeleton `seed-demo`.

### `runs.go` (projection) change

`Run` struct grows a `Bundle` field with the `json:"bundle"` tag. `ListRuns` SQL is updated to `SELECT id, template_id, bundle, created_at, ...`.

### Router registration (`api.go`)

```go
mux.HandleFunc("POST /api/runs", createRunHandler(s))
```

## 11. Frontend additions

### `lib/api.ts` — `apiPost`

Symmetric to `apiGet`. Same error envelope handling. Wire shape:

```ts
export async function apiPost<TRes, TBody>(path: string, body: TBody): Promise<TRes>
```

### `components/Dialog.tsx`

Thin wrapper around `@radix-ui/react-dialog`. Generic; takes `{open, onOpenChange, title, children}`. Renders portal + overlay + content + close button. Domain knowledge (which form, what title) lives in consumers.

### `features/runs/newRunSchema.ts`

```ts
import { z } from 'zod';

const rackPattern = /^dh\d+-r\d{3}-[a-z]+-[a-z]+-\d{2}[a-z]$/;

export const newRunSchema = z.object({
  bundle: z.string().trim().min(1, 'Bundle is required').max(200, 'Bundle is too long (max 200 characters)'),
  rack: z
    .string()
    .trim()
    .min(1, 'At least one rack is required')
    .refine(
      (val) => val.split(',').map((s) => s.trim()).filter(Boolean).every((r) => rackPattern.test(r)),
      'Rack format example: dh3-r012-us-east-01a',
    ),
});

export type NewRunInput = z.infer<typeof newRunSchema>;
```

The TS regex is **mirrored** from Go's `rackPattern`. Both files carry a comment cross-referencing the other. If the format ever evolves, both files must be edited in lockstep. Generating one from the other adds build complexity not yet worth paying.

### `features/runs/useCreateRun.ts`

TanStack Query mutation. Invalidates the `['runs']` query key on success so the list refetches.

### `features/runs/NewRunModal.tsx`

Form built with `react-hook-form` + `@hookform/resolvers/zod`. Two labeled inputs, helper text under "Rack", inline error messages under each field, Cancel + Create buttons. On submit:
1. Call `createRun.mutateAsync(values)`
2. On success: `reset()`, close modal via `onOpenChange(false)`, `navigate('/runs/' + run.id)`
3. On failure: `toast.error(err.message)` and keep the modal open

### `features/runs/useRun.ts`

TanStack Query hook for `GET /api/runs/:id`. Returns `{ run, events }`. `enabled: !!id` guards against running with an empty param.

### `features/runs/RunDetail.tsx`

Replaces the stub at `/runs/:id`. Layout:

```
← Runs
Run                            [Badge: status]
┌─ Metadata (definition list) ─────┐
│ Id          01HZX...              │
│ Bundle      gb200-fw-...          │
│ Rack        dh3-r012-us-east-01a  │
│ Template    fw-release-canary     │
│ Created     May 28, 2026 ...      │
│ Created by  wpena                 │
└────────────────────────────────────┘

Events
┌────┬──────────────┬──────────────────────┐
│ Seq│ Kind         │ When                 │
├────┼──────────────┼──────────────────────┤
│ 1  │ RunCreated   │ May 28, 2026, ...    │
└────┴──────────────┴──────────────────────┘
```

States:
- Loading: `<p>Loading run</p>`
- Not found (404 envelope with `code:"not_found"`): `<p>Run not found.</p>` + back link
- Generic error: `<p>{error.message}</p>` in red
- Success: header + events table as above

### `features/runs/RunsList.tsx` modifications

- Remove the `template_id` column from the columns array
- Add a `bundle` column right after `id`
- Add a "New run" button to the right of the rack-filter input
- Add `useState` for `modalOpen`
- Render `<NewRunModal open={modalOpen} onOpenChange={setModalOpen} />` at the end of the component

### `App.tsx`

Replace the stub element for `/runs/:id` with `<RunDetail />`.

### `mocks/handlers.ts`

Adds:
- `http.get('/api/runs/:id', ...)` — looks up in `fixtureRuns`, returns 404 envelope if missing, else `{run, events:[{seq:1,kind:'RunCreated',occurred_at:run.created_at,payload:{}}]}`
- `http.post('/api/runs', ...)` — creates a synthetic `Run` from the body, prepends to `fixtureRuns`, returns 201 with `{run}`
- Updates existing fixtures to include a `bundle` field

`fixtureRuns` becomes a `let` (was `const`) so the POST handler can mutate it. Tests rely on `server.resetHandlers()` between cases for isolation (already wired in `setupTests.ts`).

## 12. Error handling

| Source | Surface | Code | UX |
|---|---|---|---|
| Malformed JSON in POST body | Backend | `invalid_request` | Modal won't trigger this — wire request always sends valid JSON. `curl` consumers see clear message. |
| Empty bundle | Frontend + Backend | `invalid_request` | Frontend zod fires before request. Backend belt-and-suspenders for `curl`. |
| Bundle > 200 chars | Frontend + Backend | `invalid_request` | Same. |
| Invalid rack format | Frontend + Backend | `invalid_request` | Same; error names the offending token. |
| Store write failure | Backend | `internal` | Modal stays open; Sonner toast shows the message; user can retry. |
| Detail page: run not found | Backend | `not_found` | Dedicated UI: "Run not found." + back link. Not a toast. |
| Detail page: generic load failure | Backend | `internal` | Inline error text in red. |
| Network error (fetch threw) | Frontend | `internal` (synthesized by `apiPost`/`apiGet`) | Toast in the modal flow; inline in the detail flow. |

## 13. Testing

**Go (15 → 21 tests total):**

- `internal/store`: +1 (`TestMigrate_AddsBundleColumn` — checks column exists and migration is idempotent)
- `internal/api`: +5
  - `TestValidateRacks` (table-driven, 10 cases) — counted as one test
  - `TestCreateRun_HappyPath` — 201, returned run has correct bundle, status pending
  - `TestCreateRun_InvalidRackFormat` — 400, `invalid_request`
  - `TestCreateRun_EmptyBundle` — 400
  - `TestCreateRun_BundleTooLong` — 400
  - `TestCreateRun_MalformedJSON` — 400

**Frontend (7 → 18 Vitest tests):**

- `lib/api.test.ts`: +2 (`apiPost` happy, `apiPost` 400 envelope)
- `features/runs/NewRunModal.test.tsx`: +4 (renders fields, invalid-rack validation, empty-bundle validation, happy submit + navigate)
- `features/runs/RunDetail.test.tsx`: +3 (renders header + event log, not-found state, generic error state)
- `features/runs/RunsList.test.tsx`: +2 (New-run button opens modal, bundle column renders)

`make test` runs both stacks via the existing Makefile target.

## 14. Acceptance criteria

The iteration is done when all of the following are true:

1. `make build` from clean produces `team/wpena/bin/qac`.
2. `make test` passes — all 21 Go tests + 18 Vitest tests (39 total).
3. `make lint` passes — `go vet` clean, ESLint clean.
4. Starting the server fresh on an existing walking-skeleton DB (or a brand-new one) opens cleanly; migration log shows no errors; restarting the server is a no-op for the schema.
5. Visiting `http://127.0.0.1:8080` shows the runs list with a visible "New run" button to the right of the rack filter.
6. Clicking "New run" opens a modal titled "New canary run" with two labeled fields ("Firmware bundle", "Rack") and Cancel + Create buttons.
7. Submitting the modal with `bundle="gb200-fw-2026-05-canary-3"`, `rack="dh3-r012-us-east-01a"`:
   - Modal closes
   - Browser URL changes to `/runs/<new-ulid>`
   - Page renders the run's metadata header (id, bundle, rack, template, created/updated, created by) with a `pending` status badge
   - Events section shows one row: `Seq=1, Kind=RunCreated, When=<timestamp>`
8. Submitting the modal with an invalid rack (e.g., `as1-r07`) keeps the modal open, shows the validation error inline, and does NOT call the API (no log entry).
9. `curl -X POST http://127.0.0.1:8080/api/runs -H 'Content-Type: application/json' -d '{"bundle":"b","rack":"DH3"}'` returns 400 with `{"error":{"code":"invalid_request",...}}`.
10. `curl -s http://127.0.0.1:8080/api/runs` includes the newly created run with `bundle` field populated.
11. Visiting `http://127.0.0.1:8080/runs/does-not-exist` shows "Run not found." plus a "Back to runs" link (no crash, no infinite spinner).
12. Copy guide compliance — no `&` symbols (use `and`), no exclamation points, sentence case on every header/label/button/empty-state. Technical identifiers (ULIDs, rack names) preserved verbatim, not sentence-cased.
13. Self-contained still — `grep -E "^replace" go.mod` is empty; no imports of `coreweave/tiphys`, `argo/`, or other sibling repos.

## 15. Decisions log (iteration-2 specific)

- **Scope choice.** Option 1 from the brainstorm ("make the app feel like a real tool first") — chosen over option 2 ("first real probe") because the detail page needs a reason to exist; creating runs from the UI provides that reason without the architectural lift of a probe.
- **Form fields.** Bundle + rack only. `template_id` defaults server-side; multi-template support deferred. `created_by` defaults server-side; auth deferred.
- **Bundle validation.** Free-text, non-empty, max 200 characters. Operator hasn't standardized on a paste format yet; tightening later when we know what they paste.
- **Rack validation.** Strict canonical regex (Glean-confirmed format). Rejects CP5; tracked for future revisit if needed.
- **Form location.** Modal on the list page (Radix dialog), not a dedicated route. Lowest friction for a 2-field form.
- **Post-create UX.** Navigate to `/runs/<id>`. Matches the operator workflow "I just created it because I want to look at it".
- **Validation placement.** Both sides (zod on the frontend for instant feedback, Go regex on the backend for `curl`-resilient correctness).
- **Regex mirroring.** TS regex hand-mirrored from Go's. Cross-reference comments in both files. Single-source-of-truth deferred until the cost of drift outweighs the codegen tooling.
- **Schema migration.** Introspect `pragma_table_info` then `ALTER TABLE ... ADD COLUMN` only when missing. A real migrations table is correct longer-term; deferred to iteration 3 when there's a second migration to motivate it.
- **Bundle uniqueness.** Non-unique. Operators run the same `(bundle, rack)` repeatedly during qualification.
- **Event payload shape.** `RunCreatedPayload` grows a `Bundle` field. Old payloads (no bundle key) decode to `""` — preserves walking-skeleton `seed-demo` behavior with no data migration.

## 16. Integration sources (forward note)

Same as the walking-skeleton spec §13: when iteration 3 (first real probe) lands, port logic from `~/Documents/argo/jason/fwqual/`, `~/Documents/argo/tiphys/inventory`, `~/Documents/argo/cwf`, `~/Documents/argo/awxctl`. Do not import; the self-contained constraint still applies.
