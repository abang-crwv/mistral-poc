# qac Iteration 2 Implementation Plan — run-detail + new-run modal

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `POST /api/runs`, a `bundle` column on the `runs` projection, a Radix modal for creating runs from the list page, and a real `/runs/:id` detail page replacing the iter-1 stub.

**Architecture:** Purely additive to the walking skeleton — no refactors. Backend grows by one endpoint + one validation file + one DB column (added via idempotent migration). Frontend grows by one shared Dialog primitive + four new files under `features/runs/` (schema, modal, mutation hook, detail page) + one hook for the detail fetch. Rack regex is hand-mirrored between Go and TS; comments cross-reference.

**Tech Stack:**
- **Backend additions:** `regexp` (stdlib), `ulid.Make()` (already a dep), `pragma_table_info` (SQLite intrinsic), Go 1.22 ServeMux `POST` patterns
- **Frontend additions:** `@radix-ui/react-dialog` (new dep), `@hookform/resolvers/zod` (already in package.json), `react-hook-form` (already in package.json)
- **Branch:** `wp/add-qac` (continuation; walking-skeleton commits remain in history)
- **Module path:** bare `qac` (unchanged, no `replace` directives)
- **Project root:** `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/`

**Spec:** `docs/superpowers/specs/2026-05-28-qac-run-detail-design.md`

**For the Go beginner:** new Go concepts appear in 🟦 callouts in the relevant tasks. Concepts introduced this iteration: `regexp.MustCompile` (compile-once package-level vars), `strings.TrimSpace` + `strings.Split`, multi-return functions with a sentinel string for user-facing errors, `json.NewDecoder(r.Body).Decode(...)`, `http.StatusCreated` (201) vs 200 conventions, `pragma_table_info` introspection for idempotent schema migration.

---

## File structure

**Top-level changes:**

| Path | Action | Responsibility |
|---|---|---|
| `internal/store/migrations.go` | Modify | Add idempotent `addBundleColumnIfMissing` step |
| `internal/store/events.go` | Modify | Add `Bundle` to `runCreatedPayload`; include bundle in INSERT |
| `internal/store/runs.go` | Modify | Add `Bundle` to `Run` struct; update `ListRuns` SELECT |
| `internal/store/store_test.go` | Modify | Add `TestMigrate_AddsBundleColumn` |
| `internal/api/runs_validate.go` | Create | `rackPattern` regex + `ValidateRacks` function |
| `internal/api/runs_validate_test.go` | Create | Table-driven tests for `ValidateRacks` |
| `internal/api/runs.go` | Modify | Add `createRunHandler` |
| `internal/api/api.go` | Modify | Register `POST /api/runs` |
| `internal/api/api_test.go` | Modify | Add 5 POST tests |
| `web/package.json` | Modify | Add `@radix-ui/react-dialog` dep |
| `web/src/lib/api.ts` | Modify | Add `apiPost<TRes, TBody>` |
| `web/src/lib/api.test.ts` | Modify | Add 2 apiPost tests |
| `web/src/components/Dialog.tsx` | Create | Radix Dialog wrapper |
| `web/src/features/runs/runs.types.ts` | Modify | Add `bundle` to `Run`; add `Event` type |
| `web/src/features/runs/newRunSchema.ts` | Create | Zod schema with mirrored rack regex |
| `web/src/features/runs/useCreateRun.ts` | Create | TanStack Query mutation |
| `web/src/features/runs/useRun.ts` | Create | TanStack Query hook for `/api/runs/:id` |
| `web/src/features/runs/NewRunModal.tsx` | Create | Form + dialog + submit |
| `web/src/features/runs/NewRunModal.test.tsx` | Create | 4 tests |
| `web/src/features/runs/RunDetail.tsx` | Create | Header + event log |
| `web/src/features/runs/RunDetail.test.tsx` | Create | 3 tests |
| `web/src/features/runs/RunsList.tsx` | Modify | Swap `template_id` for `bundle` column; add New run button + modal mount |
| `web/src/features/runs/RunsList.test.tsx` | Modify | Add 2 tests |
| `web/src/mocks/handlers.ts` | Modify | Add bundle to fixtures; add POST + GET-by-id handlers |
| `web/src/App.tsx` | Modify | Replace stub element with `<RunDetail />` |

---

## Task 1: Add `@radix-ui/react-dialog` dependency

**Files:**
- Modify: `web/package.json`
- Modify: `web/yarn.lock`

- [ ] **Step 1.1: Verify existing deps**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web
grep -E "@radix-ui/react-dialog|@hookform/resolvers|react-hook-form|zod" package.json
```

Expected: lines for `@hookform/resolvers`, `react-hook-form`, `zod` (added in iter-1). NO line for `@radix-ui/react-dialog`.

- [ ] **Step 1.2: Add the dialog dep at the cloud-console-pinned version**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web
yarn add @radix-ui/react-dialog@1.1.15
```

Expected: yarn.lock updated, `@radix-ui/react-dialog` appears under `dependencies` in package.json.

- [ ] **Step 1.3: Verify the install**

```bash
yarn tsc --noEmit
yarn test --run
```

Expected: tsc clean (no new errors), 7 existing tests still pass.

- [ ] **Step 1.4: Commit**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac
git add web/package.json web/yarn.lock
git commit -m "qac: web — add @radix-ui/react-dialog dep"
```

---

## Task 2: Store — idempotent bundle column migration (TDD)

**Files:**
- Modify: `internal/store/migrations.go`
- Modify: `internal/store/store_test.go`

🟦 **Go concept:** `pragma_table_info('runs')` is a SQLite virtual table that returns one row per column in the named table (columns: `cid, name, type, notnull, dflt_value, pk`). It's the standard way to introspect a schema in pure SQL without ALTER errors. We use it because `ALTER TABLE ADD COLUMN` errors if the column exists, and we want migrations to be safe to run on every Open.

- [ ] **Step 2.1: Write the failing test**

Append to `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/internal/store/store_test.go`:

```go
func TestMigrate_AddsBundleColumn(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "qac.db")

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Verify bundle column exists after first open.
	rows, err := s.DB().QueryContext(ctx, `SELECT name FROM pragma_table_info('runs')`)
	if err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	hasBundle := false
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatalf("scan: %v", err)
		}
		if name == "bundle" {
			hasBundle = true
		}
	}
	rows.Close()
	if !hasBundle {
		t.Fatal("bundle column missing after Open")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen — migration must be idempotent.
	s2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if err := s2.Close(); err != nil {
		t.Fatalf("Close 2: %v", err)
	}
}
```

- [ ] **Step 2.2: Run test — expect failure**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac
go test ./internal/store/... -run TestMigrate_AddsBundleColumn -v
```

Expected: FAIL with `bundle column missing after Open`.

- [ ] **Step 2.3: Add idempotent migration step**

Replace the body of `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/internal/store/migrations.go` with:

```go
package store

import (
	"context"
	"fmt"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS runs (
  id            TEXT PRIMARY KEY,
  template_id   TEXT NOT NULL,
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL,
  status        TEXT NOT NULL,
  rack          TEXT NOT NULL,
  created_by    TEXT NOT NULL DEFAULT 'wpena'
);

CREATE TABLE IF NOT EXISTS events (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id        TEXT NOT NULL REFERENCES runs(id),
  seq           INTEGER NOT NULL,
  kind          TEXT NOT NULL,
  payload       TEXT NOT NULL,
  occurred_at   INTEGER NOT NULL,
  UNIQUE(run_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_events_run ON events(run_id, seq);
CREATE INDEX IF NOT EXISTS idx_runs_updated ON runs(updated_at DESC);
`

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("apply base schema: %w", err)
	}
	if err := s.addBundleColumnIfMissing(ctx); err != nil {
		return fmt.Errorf("add bundle column: %w", err)
	}
	return nil
}

// addBundleColumnIfMissing adds the `bundle` column to the `runs` table
// when it isn't already present. Uses pragma_table_info introspection
// so this is safe to call on every Open without an ALTER duplicate-column
// error. When iteration 3 needs another migration, replace this with a
// real migrations table.
func (s *Store) addBundleColumnIfMissing(ctx context.Context) error {
	row := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('runs') WHERE name = 'bundle'`)
	var n int
	if err := row.Scan(&n); err != nil {
		return fmt.Errorf("introspect: %w", err)
	}
	if n > 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		`ALTER TABLE runs ADD COLUMN bundle TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("alter: %w", err)
	}
	return nil
}
```

- [ ] **Step 2.4: Run test — expect PASS**

```bash
go test ./internal/store/... -v
```

Expected: all 5 tests PASS (4 existing + new `TestMigrate_AddsBundleColumn`).

- [ ] **Step 2.5: Commit**

```bash
git add internal/store/migrations.go internal/store/store_test.go
git commit -m "qac: store — idempotent bundle column migration"
```

---

## Task 3: Store — bundle in events.go (TDD-ish: extend existing test)

**Files:**
- Modify: `internal/store/events.go`
- Modify: `internal/store/store_test.go`

🟦 **Go concept (struct tag JSON decode):** Adding a new field to a struct used by `json.Unmarshal` is non-breaking — JSON without the field decodes the new field to its zero value. So old payloads (without `bundle`) decode to `Bundle: ""`. No data migration needed for existing rows.

- [ ] **Step 3.1: Extend the existing test to assert bundle is stored**

Find `TestAppendEvent_RunCreated_UpsertsRun` in `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/internal/store/store_test.go`. Replace the `json.Marshal` line and add bundle assertions. The full updated test:

```go
func TestAppendEvent_RunCreated_UpsertsRun(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "qac.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000A"
	payload, _ := json.Marshal(map[string]string{
		"template_id": "firmware-release-canary",
		"bundle":      "gb200-fw-2026-05-canary-3",
		"rack":        "dh3-r012-us-east-01a",
		"created_by":  "wpena",
	})
	if err := s.AppendEvent(ctx, runID, "RunCreated", payload); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	var status, rack, bundle string
	row := s.DB().QueryRowContext(ctx,
		`SELECT status, rack, bundle FROM runs WHERE id = ?`, runID)
	if err := row.Scan(&status, &rack, &bundle); err != nil {
		t.Fatalf("scan run: %v", err)
	}
	if status != "pending" {
		t.Fatalf("status = %q, want pending", status)
	}
	if rack != "dh3-r012-us-east-01a" {
		t.Fatalf("rack = %q, want dh3-r012-us-east-01a", rack)
	}
	if bundle != "gb200-fw-2026-05-canary-3" {
		t.Fatalf("bundle = %q, want gb200-fw-2026-05-canary-3", bundle)
	}

	var seq int
	row = s.DB().QueryRowContext(ctx, `SELECT seq FROM events WHERE run_id = ?`, runID)
	if err := row.Scan(&seq); err != nil {
		t.Fatalf("scan event: %v", err)
	}
	if seq != 1 {
		t.Fatalf("seq = %d, want 1", seq)
	}
}
```

- [ ] **Step 3.2: Run test — expect failure**

```bash
go test ./internal/store/... -run TestAppendEvent_RunCreated_UpsertsRun -v
```

Expected: FAIL — either `no such column: bundle` (if Task 2 wasn't run) or `bundle = "", want ...` (because INSERT doesn't write to the column yet).

- [ ] **Step 3.3: Update `events.go`**

Replace `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/internal/store/events.go` with:

```go
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// runCreatedPayload mirrors the JSON shape of the RunCreated event.
// Only the fields we need to populate the runs projection are decoded.
type runCreatedPayload struct {
	TemplateID string `json:"template_id"`
	Bundle     string `json:"bundle"`
	Rack       string `json:"rack"`
	CreatedBy  string `json:"created_by"`
}

// AppendEvent inserts an event and updates the runs projection in one
// transaction. For RunCreated, it inserts the runs row. For other kinds
// (StepStarted, StepCompleted, OperatorAction), it updates status +
// updated_at on the existing row. Unknown kinds are stored but do not
// alter the projection.
func (s *Store) AppendEvent(ctx context.Context, runID, kind string, payload []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	switch kind {
	case "RunCreated":
		var p runCreatedPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("decode RunCreated payload: %w", err)
		}
		createdBy := p.CreatedBy
		if createdBy == "" {
			createdBy = "wpena"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO runs (id, template_id, bundle, created_at, updated_at, status, rack, created_by)
			VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)
			ON CONFLICT(id) DO NOTHING`,
			runID, p.TemplateID, p.Bundle, now, now, p.Rack, createdBy,
		); err != nil {
			return fmt.Errorf("insert run: %w", err)
		}

	case "StepStarted":
		if _, err := tx.ExecContext(ctx, `
			UPDATE runs SET status = 'running', updated_at = ? WHERE id = ?`,
			now, runID,
		); err != nil {
			return fmt.Errorf("update run (StepStarted): %w", err)
		}

	case "StepCompleted":
		if _, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = ? WHERE id = ?`, now, runID); err != nil {
			return fmt.Errorf("update run (StepCompleted): %w", err)
		}

	case "OperatorAction":
		if _, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = ? WHERE id = ?`, now, runID); err != nil {
			return fmt.Errorf("update run (OperatorAction): %w", err)
		}

	default:
		// Store the event anyway; the projection just ignores it.
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO events (run_id, seq, kind, payload, occurred_at)
		VALUES (?, COALESCE((SELECT MAX(seq) FROM events WHERE run_id = ?), 0) + 1, ?, ?, ?)`,
		runID, runID, kind, string(payload), now,
	); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
```

The only changes from iter-1: `Bundle` field on the struct, `bundle` in the INSERT column list, `p.Bundle` in the VALUES, and a corresponding `?` placeholder.

- [ ] **Step 3.4: Run all store tests — expect PASS**

```bash
go test ./internal/store/... -v
```

Expected: all 5 tests PASS.

- [ ] **Step 3.5: Commit**

```bash
git add internal/store/events.go internal/store/store_test.go
git commit -m "qac: store — store bundle from RunCreated event payload"
```

---

## Task 4: Store — bundle in Run struct + ListRuns SELECT

**Files:**
- Modify: `internal/store/runs.go`

🟦 **Go concept (struct field JSON tag):** Adding a new field with a `json:"bundle"` tag means `json.Marshal` writes it to the wire and `json.Unmarshal` reads it. For decoding into anonymous structs in tests, missing tags or fields are silently ignored — so iter-1 API tests continue to pass even as the `Run` shape grows.

- [ ] **Step 4.1: Update `runs.go`**

Replace `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/internal/store/runs.go` with:

```go
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Run is the projection row for a single run.
type Run struct {
	ID         string    `json:"id"`
	Bundle     string    `json:"bundle"`
	TemplateID string    `json:"template_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Status     string    `json:"status"`
	Rack       string    `json:"rack"`
	CreatedBy  string    `json:"created_by"`
}

// Event is a single event log entry.
type Event struct {
	Seq        int64           `json:"seq"`
	Kind       string          `json:"kind"`
	Payload    json.RawMessage `json:"payload"`
	OccurredAt time.Time       `json:"occurred_at"`
}

// ListRuns returns all runs sorted by updated_at desc.
func (s *Store) ListRuns(ctx context.Context) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, bundle, template_id, created_at, updated_at, status, rack, created_by
		FROM runs
		ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query runs: %w", err)
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		var r Run
		var createdAt, updatedAt int64
		if err := rows.Scan(&r.ID, &r.Bundle, &r.TemplateID, &createdAt, &updatedAt, &r.Status, &r.Rack, &r.CreatedBy); err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		r.CreatedAt = time.Unix(createdAt, 0).UTC()
		r.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iter: %w", err)
	}
	return runs, nil
}

// LoadEvents returns the event log for a run, ordered by seq ascending.
func (s *Store) LoadEvents(ctx context.Context, runID string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT seq, kind, payload, occurred_at
		FROM events
		WHERE run_id = ?
		ORDER BY seq ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var payload string
		var occurredAt int64
		if err := rows.Scan(&e.Seq, &e.Kind, &payload, &occurredAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		e.Payload = json.RawMessage(payload)
		e.OccurredAt = time.Unix(occurredAt, 0).UTC()
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iter: %w", err)
	}
	return events, nil
}
```

Changes from iter-1: `Bundle` field on the `Run` struct (second position, after `ID`), and `bundle` in both the SELECT column list and the Scan target list — order matters and must match the SELECT order.

- [ ] **Step 4.2: Run all tests — expect PASS**

```bash
go test ./...
```

Expected: all 21 Go tests pass (the API tests still use anonymous decode structs that only check specific fields, so the new `bundle` field is transparent to them).

- [ ] **Step 4.3: Commit**

```bash
git add internal/store/runs.go
git commit -m "qac: store — expose bundle on Run struct + ListRuns"
```

---

## Task 5: API — ValidateRacks (TDD)

**Files:**
- Create: `internal/api/runs_validate.go`
- Create: `internal/api/runs_validate_test.go`

🟦 **Go concept (`regexp.MustCompile`):** Package-level vars initialized via `regexp.MustCompile(pattern)` compile the regex once at startup. The "Must" variant panics if the pattern is malformed — fine for a hardcoded literal because a bad regex is a programmer error caught immediately at package load. The compiled `*regexp.Regexp` is safe for concurrent use.

🟦 **Go concept (multi-return with sentinel string):** `ValidateRacks` returns three values: the canonical joined form, the parsed slice, and an empty-string-means-OK error message. We use a string (not `error`) because the caller wants a user-facing message for the API response envelope, not a wrappable error chain.

- [ ] **Step 5.1: Write the failing tests**

Create `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/internal/api/runs_validate_test.go`:

```go
package api

import "testing"

func TestValidateRacks(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		wantCanon   string
		wantInvalid bool
	}{
		{"single canonical", "dh3-r012-us-east-01a", "dh3-r012-us-east-01a", false},
		{"two racks comma+space", "dh3-r012-us-east-01a, dh4-r066-us-east-11a", "dh3-r012-us-east-01a,dh4-r066-us-east-11a", false},
		{"trailing comma tolerated", "dh3-r012-us-east-01a,", "dh3-r012-us-east-01a", false},
		{"empty input rejected", "", "", true},
		{"whitespace only rejected", "   ", "", true},
		{"capitals rejected", "DH3-R012-US-EAST-01A", "", true},
		{"as1-r07 dummy rejected", "as1-r07", "", true},
		{"CP5 rejected", "CP5-v2a", "", true},
		{"one bad in mixed list rejected", "dh3-r012-us-east-01a, oops", "", true},
		{"missing zero pad rejected", "dh3-r12-us-east-01a", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			canon, _, errMsg := ValidateRacks(tc.input)
			if tc.wantInvalid {
				if errMsg == "" {
					t.Fatalf("expected error, got canon=%q", canon)
				}
				return
			}
			if errMsg != "" {
				t.Fatalf("unexpected error: %s", errMsg)
			}
			if canon != tc.wantCanon {
				t.Fatalf("canon = %q, want %q", canon, tc.wantCanon)
			}
		})
	}
}
```

- [ ] **Step 5.2: Run — expect compile failure**

```bash
go test ./internal/api/... -run TestValidateRacks
```

Expected: `undefined: ValidateRacks`.

- [ ] **Step 5.3: Write `runs_validate.go`**

Create `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/internal/api/runs_validate.go`:

```go
package api

import (
	"fmt"
	"regexp"
	"strings"
)

// rackPattern enforces the canonical CoreWeave rack name format
// (post-2025-07-02): <datahall>-<rack>-<zone>, e.g. dh3-r012-us-east-01a.
// MIRRORED on the frontend in web/src/features/runs/newRunSchema.ts —
// keep them in lockstep. Source: Glean FPA-1509 + Jordan Dahmen's
// "Rack Naming Convention" doc.
var rackPattern = regexp.MustCompile(`^dh\d+-r\d{3}-[a-z]+-[a-z]+-\d{2}[a-z]$`)

// ValidateRacks splits a comma-separated rack string, trims whitespace,
// validates each token against rackPattern, and returns the canonical
// joined form (no spaces) along with the parsed slice. Returns a
// user-facing error message on the first invalid token. Empty string
// means OK.
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

- [ ] **Step 5.4: Run — expect PASS**

```bash
go test ./internal/api/... -run TestValidateRacks -v
```

Expected: all 10 sub-test cases PASS.

- [ ] **Step 5.5: Commit**

```bash
git add internal/api/runs_validate.go internal/api/runs_validate_test.go
git commit -m "qac: api — ValidateRacks with canonical CoreWeave regex"
```

---

## Task 6: API — createRunHandler + POST tests (TDD)

**Files:**
- Modify: `internal/api/runs.go`
- Modify: `internal/api/api.go`
- Modify: `internal/api/api_test.go`

🟦 **Go concept (`json.NewDecoder(r.Body).Decode(...)`):** Streaming JSON decode from the request body. Equivalent to `io.ReadAll` then `json.Unmarshal` but allocates less. Returns an error on malformed JSON OR if the body is empty.

🟦 **Go concept (HTTP status codes):** `http.StatusCreated` (201) is the convention for "request succeeded and a new resource was created". `200 OK` is correct for GET. Returning 201 makes the API contract clearer to anyone reading the wire — they know a side-effect occurred.

🟦 **Go concept (`ulid.Make()`):** The `oklog/ulid/v2` v2.1.x API offers `ulid.Make()` as a convenience using the current time + default entropy source. Equivalent to `ulid.MustNew(ulid.Timestamp(time.Now()), ulid.Monotonic(rand.Reader, 0))` but readable.

- [ ] **Step 6.1: Append failing POST tests**

Append to `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/internal/api/api_test.go`. You may need to add `"strings"` to the imports.

```go
func TestCreateRun_HappyPath(t *testing.T) {
	srv, _ := newTestServer(t)
	body := `{"bundle":"gb200-fw-2026-05-canary-3","rack":"dh3-r012-us-east-01a"}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	var out struct {
		Run struct {
			ID, Bundle, Rack, Status string
		} `json:"run"`
	}
	if err := encjson.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Run.ID == "" {
		t.Fatal("id is empty")
	}
	if out.Run.Bundle != "gb200-fw-2026-05-canary-3" {
		t.Fatalf("bundle = %q, want gb200-fw-2026-05-canary-3", out.Run.Bundle)
	}
	if out.Run.Rack != "dh3-r012-us-east-01a" {
		t.Fatalf("rack = %q, want dh3-r012-us-east-01a", out.Run.Rack)
	}
	if out.Run.Status != "pending" {
		t.Fatalf("status = %q, want pending", out.Run.Status)
	}
}

func TestCreateRun_InvalidRackFormat(t *testing.T) {
	srv, _ := newTestServer(t)
	body := `{"bundle":"b","rack":"as1-r07"}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
	var env struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := encjson.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error.Code != "invalid_request" {
		t.Fatalf("code = %q, want invalid_request", env.Error.Code)
	}
}

func TestCreateRun_EmptyBundle(t *testing.T) {
	srv, _ := newTestServer(t)
	body := `{"bundle":"","rack":"dh3-r012-us-east-01a"}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestCreateRun_BundleTooLong(t *testing.T) {
	srv, _ := newTestServer(t)
	long := strings.Repeat("x", 201)
	body := `{"bundle":"` + long + `","rack":"dh3-r012-us-east-01a"}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestCreateRun_MalformedJSON(t *testing.T) {
	srv, _ := newTestServer(t)
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(`{not json`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}
```

Note: the iter-1 tests aliased `encoding/json` as `encjson` to avoid a name clash. If you see "undefined: encjson", confirm the import alias is still present at the top of the file.

- [ ] **Step 6.2: Run — expect failures**

```bash
go test ./internal/api/... -run TestCreateRun
```

Expected: all 5 tests FAIL (likely 404 from `POST` since the route isn't registered yet, or compile-fail if `strings` import isn't present).

- [ ] **Step 6.3: Register the route in `api.go`**

Replace `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/internal/api/api.go` with:

```go
package api

import (
	"net/http"

	"qac/internal/store"
)

// Router builds the HTTP handler tree for the /api/* surface.
// dbPath is reported by GET /api/health for ops sanity.
func Router(s *store.Store, dbPath string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", healthHandler(dbPath))

	mux.HandleFunc("GET /api/runs", listRunsHandler(s))
	mux.HandleFunc("POST /api/runs", createRunHandler(s))
	mux.HandleFunc("GET /api/runs/{id}", getRunHandler(s))

	return mux
}
```

- [ ] **Step 6.4: Add `createRunHandler` to `runs.go`**

Append to `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/internal/api/runs.go`. The file currently imports `net/http` and `qac/internal/store`. Update imports to also include `encoding/json` and `strings`, plus `github.com/oklog/ulid/v2`. After the existing handlers, append:

```go
// (Add imports at the top: "encoding/json", "strings", "github.com/oklog/ulid/v2")

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

		// Fetch the just-created run from the projection so the response
		// shape matches GET /api/runs/{id}.
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

For clarity, the full `runs.go` after this edit should have:

```go
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/oklog/ulid/v2"

	"qac/internal/store"
)

func listRunsHandler(s *store.Store) http.HandlerFunc { /* unchanged */ }
func getRunHandler(s *store.Store) http.HandlerFunc { /* unchanged */ }
func createRunHandler(s *store.Store) http.HandlerFunc { /* new — see above */ }
```

- [ ] **Step 6.5: Run — expect PASS**

```bash
go test ./internal/api/... -v
```

Expected: all 12 tests PASS (7 from iter-1 + 5 new POST tests).

- [ ] **Step 6.6: Commit**

```bash
git add internal/api/api.go internal/api/runs.go internal/api/api_test.go
git commit -m "qac: api — POST /api/runs with bundle + rack validation"
```

---

## Task 7: Frontend — `apiPost` helper (TDD)

**Files:**
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/lib/api.test.ts`

- [ ] **Step 7.1: Append failing tests**

Append to `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/lib/api.test.ts` (the file already imports `describe, it, expect`, `http`, `HttpResponse`, and `server`). Add `apiPost` to the existing `import { apiGet, ApiException } from './api';` line.

```ts
// Add this test block inside the existing `describe('apiGet', ...)` file,
// at the bottom (or in a new `describe('apiPost', ...)` block):

describe('apiPost', () => {
  it('returns the parsed body on success', async () => {
    server.use(
      http.post('/api/echo', async ({ request }) => {
        const body = (await request.json()) as { hello: string };
        return HttpResponse.json({ echoed: body.hello }, { status: 201 });
      }),
    );
    const result = await apiPost<{ echoed: string }, { hello: string }>('/echo', { hello: 'world' });
    expect(result.echoed).toBe('world');
  });

  it('throws ApiException with code and message on 400 envelope', async () => {
    server.use(
      http.post('/api/bad', () =>
        HttpResponse.json(
          { error: { code: 'invalid_request', message: 'Bundle is required' } },
          { status: 400 },
        ),
      ),
    );
    await expect(apiPost('/bad', {})).rejects.toMatchObject({
      code: 'invalid_request',
      message: 'Bundle is required',
    });
    await expect(apiPost('/bad', {})).rejects.toBeInstanceOf(ApiException);
  });
});
```

- [ ] **Step 7.2: Run — expect compile failure**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web
yarn test --run
```

Expected: TypeScript fails — `apiPost` not exported.

- [ ] **Step 7.3: Add `apiPost` to `api.ts`**

Append to `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/lib/api.ts`:

```ts
export async function apiPost<TRes, TBody>(path: string, body: TBody): Promise<TRes> {
  let res: Response;
  try {
    res = await fetch(`/api${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify(body),
    });
  } catch (e) {
    throw new ApiException('internal', e instanceof Error ? e.message : 'Network error');
  }

  let parsed: unknown = null;
  try {
    parsed = await res.json();
  } catch {
    // Response wasn't JSON; fall through to error handling.
  }

  if (!res.ok) {
    const err = (parsed as { error?: ApiError } | null)?.error;
    throw new ApiException(err?.code ?? 'internal', err?.message ?? 'Request failed');
  }
  return parsed as TRes;
}
```

- [ ] **Step 7.4: Run — expect PASS**

```bash
yarn test --run
```

Expected: 9 tests pass (7 from iter-1 + 2 new apiPost tests).

- [ ] **Step 7.5: Commit**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac
git add web/src/lib/api.ts web/src/lib/api.test.ts
git commit -m "qac: web — apiPost helper with shared error envelope"
```

---

## Task 8: Frontend — types + mocks (combined for type-safety)

**Files:**
- Modify: `web/src/features/runs/runs.types.ts`
- Modify: `web/src/mocks/handlers.ts`

This task combines two file changes because adding `bundle` to the `Run` type makes the existing fixture entries fail TypeScript without simultaneously adding the field there. Two changes; one commit.

- [ ] **Step 8.1: Update `runs.types.ts`**

Replace `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/features/runs/runs.types.ts` with:

```ts
export type RunStatus =
  | 'pending'
  | 'running'
  | 'passed'
  | 'warning'
  | 'failed'
  | 'signed_off';

export type Run = {
  id: string;
  bundle: string;
  template_id: string;
  created_at: string;
  updated_at: string;
  status: RunStatus;
  rack: string;
  created_by: string;
};

export type Event = {
  seq: number;
  kind: string;
  occurred_at: string;
  // payload shape is keyed by `kind`; consumers narrow at the point of use.
  payload: unknown;
};
```

- [ ] **Step 8.2: Update `mocks/handlers.ts`**

Replace `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/mocks/handlers.ts` with:

```ts
import { http, HttpResponse } from 'msw';
import type { Run } from '@/features/runs/runs.types';

// `let` (not `const`) so the POST handler can prepend new runs.
export let fixtureRuns: Run[] = [
  {
    id: '01HZX0000000000000000000A',
    bundle: 'gb200-fw-2026-05-canary-3',
    template_id: 'fw-release-canary',
    created_at: '2026-05-27T15:04:05Z',
    updated_at: '2026-05-27T15:04:05Z',
    status: 'pending',
    rack: 'dh3-r012-us-east-01a',
    created_by: 'wpena',
  },
  {
    id: '01HZX0000000000000000000B',
    bundle: 'gb200-fw-2026-05-canary-2',
    template_id: 'fw-release-canary',
    created_at: '2026-05-27T14:00:00Z',
    updated_at: '2026-05-27T14:30:00Z',
    status: 'passed',
    rack: 'dh4-r066-us-east-11a',
    created_by: 'wpena',
  },
  {
    id: '01HZX0000000000000000000C',
    bundle: 'gb200-fw-2026-05-canary-1',
    template_id: 'fw-release-canary',
    created_at: '2026-05-27T13:00:00Z',
    updated_at: '2026-05-27T13:15:00Z',
    status: 'failed',
    rack: 'dh3-r012-us-east-01a',
    created_by: 'wpena',
  },
];

export const handlers = [
  http.get('/api/health', () =>
    HttpResponse.json({ ok: true, version: 'test', db_path: '/tmp/test.db' }),
  ),
  http.get('/api/runs', () => HttpResponse.json({ runs: fixtureRuns })),
  http.get('/api/runs/:id', ({ params }) => {
    const run = fixtureRuns.find((r) => r.id === params.id);
    if (!run) {
      return HttpResponse.json(
        { error: { code: 'not_found', message: 'Run not found' } },
        { status: 404 },
      );
    }
    return HttpResponse.json({
      run,
      events: [
        {
          seq: 1,
          kind: 'RunCreated',
          occurred_at: run.created_at,
          payload: {},
        },
      ],
    });
  }),
  http.post('/api/runs', async ({ request }) => {
    const body = (await request.json()) as { bundle: string; rack: string };
    const newRun: Run = {
      id: '01HZX0000000000000000000Z',
      bundle: body.bundle,
      template_id: 'fw-release-canary',
      rack: body.rack,
      status: 'pending',
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
      created_by: 'wpena',
    };
    fixtureRuns = [newRun, ...fixtureRuns];
    return HttpResponse.json({ run: newRun }, { status: 201 });
  }),
];
```

Notes:
- The two existing rack strings (`as1-r07`, `as2-r11`) are replaced with canonical names so the rack-filter test still passes (`r11` matches `dh4-r066-us-east-11a`).
- `RunsList.test.tsx` currently asserts `getByText('as2-r11')` — Task 11 updates that assertion.

- [ ] **Step 8.3: Verify compile + tests**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web
yarn tsc --noEmit
yarn test --run
```

Expected: tsc clean. Tests likely fail in `RunsList.test.tsx` (it asserts `as2-r11`). That's expected; Task 11 fixes the assertions. For now, the relevant breakage is type-safety: tsc must be clean.

Actually — to keep this commit green-tests, also update the iter-1 assertions in `RunsList.test.tsx` right now:

Find the existing `RunsList.test.tsx` and replace its `as1-r07` and `as2-r11` references with the new canonical names. The two changes are:
```ts
// Was: const r07 = await screen.findAllByText('as1-r07');
const r07 = await screen.findAllByText('dh3-r012-us-east-01a');
// Was: expect(await screen.findByText('as2-r11')).toBeInTheDocument();
expect(await screen.findByText('dh4-r066-us-east-11a')).toBeInTheDocument();

// In the filter test:
// Was: await screen.findByText('as2-r11');
await screen.findByText('dh4-r066-us-east-11a');
// Was: const filter = ... await user.type(filter, 'r11');
const filter = screen.getByPlaceholderText(/filter by rack/i);
await user.type(filter, 'r066');
// Was: expect(screen.queryByText('as1-r07')).not.toBeInTheDocument();
expect(screen.queryByText('dh3-r012-us-east-01a')).not.toBeInTheDocument();
// Was: expect(screen.getByText('as2-r11')).toBeInTheDocument();
expect(screen.getByText('dh4-r066-us-east-11a')).toBeInTheDocument();
```

Re-run:

```bash
yarn test --run
```

Expected: 9 tests pass (7 from iter-1, updated; 2 from apiPost).

- [ ] **Step 8.4: Commit**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac
git add web/src/features/runs/runs.types.ts web/src/mocks/handlers.ts web/src/features/runs/RunsList.test.tsx
git commit -m "qac: web — Run.bundle + Event type, canonical rack fixtures"
```

---

## Task 9: Frontend — `Dialog` primitive

**Files:**
- Create: `web/src/components/Dialog.tsx`

- [ ] **Step 9.1: Write `Dialog.tsx`**

Create `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/components/Dialog.tsx`:

```tsx
import * as RadixDialog from '@radix-ui/react-dialog';
import { X } from 'lucide-react';
import type { ReactNode } from 'react';

type DialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  children: ReactNode;
};

export function Dialog({ open, onOpenChange, title, children }: DialogProps) {
  return (
    <RadixDialog.Root open={open} onOpenChange={onOpenChange}>
      <RadixDialog.Portal>
        <RadixDialog.Overlay className="fixed inset-0 bg-black/40 data-[state=open]:animate-in data-[state=open]:fade-in" />
        <RadixDialog.Content className="fixed left-1/2 top-1/2 w-[28rem] max-w-[95vw] -translate-x-1/2 -translate-y-1/2 rounded-lg bg-white p-6 shadow-xl">
          <div className="mb-4 flex items-center justify-between">
            <RadixDialog.Title className="text-lg font-semibold">{title}</RadixDialog.Title>
            <RadixDialog.Close
              className="rounded p-1 text-zinc-500 hover:bg-zinc-100"
              aria-label="Close"
            >
              <X size={16} />
            </RadixDialog.Close>
          </div>
          {children}
        </RadixDialog.Content>
      </RadixDialog.Portal>
    </RadixDialog.Root>
  );
}
```

- [ ] **Step 9.2: Compile check**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web
yarn tsc --noEmit
```

Expected: no output (clean).

- [ ] **Step 9.3: Commit**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac
git add web/src/components/Dialog.tsx
git commit -m "qac: web — Dialog component (Radix wrapper)"
```

---

## Task 10: Frontend — `newRunSchema` + hooks (`useCreateRun`, `useRun`)

**Files:**
- Create: `web/src/features/runs/newRunSchema.ts`
- Create: `web/src/features/runs/useCreateRun.ts`
- Create: `web/src/features/runs/useRun.ts`

- [ ] **Step 10.1: Write `newRunSchema.ts`**

Create `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/features/runs/newRunSchema.ts`:

```ts
import { z } from 'zod';

// MIRRORED in internal/api/runs_validate.go (rackPattern).
// If you change one, change the other in the same commit.
const rackPattern = /^dh\d+-r\d{3}-[a-z]+-[a-z]+-\d{2}[a-z]$/;

export const newRunSchema = z.object({
  bundle: z
    .string()
    .trim()
    .min(1, 'Bundle is required')
    .max(200, 'Bundle is too long (max 200 characters)'),
  rack: z
    .string()
    .trim()
    .min(1, 'At least one rack is required')
    .refine(
      (val) =>
        val
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean)
          .every((r) => rackPattern.test(r)),
      'Rack format example: dh3-r012-us-east-01a',
    ),
});

export type NewRunInput = z.infer<typeof newRunSchema>;
```

- [ ] **Step 10.2: Write `useCreateRun.ts`**

Create `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/features/runs/useCreateRun.ts`:

```ts
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiPost } from '@/lib/api';
import type { NewRunInput } from './newRunSchema';
import type { Run } from './runs.types';

export function useCreateRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: NewRunInput) =>
      apiPost<{ run: Run }, NewRunInput>('/runs', input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['runs'] });
    },
  });
}
```

- [ ] **Step 10.3: Write `useRun.ts`**

Create `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/features/runs/useRun.ts`:

```ts
import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import type { Run, Event } from './runs.types';

export function useRun(id: string) {
  return useQuery({
    queryKey: ['runs', id],
    queryFn: () => apiGet<{ run: Run; events: Event[] }>(`/runs/${id}`),
    enabled: !!id,
    staleTime: 5_000,
  });
}
```

- [ ] **Step 10.4: Compile check**

```bash
yarn tsc --noEmit
```

Expected: no errors.

- [ ] **Step 10.5: Commit**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac
git add web/src/features/runs/newRunSchema.ts web/src/features/runs/useCreateRun.ts web/src/features/runs/useRun.ts
git commit -m "qac: web — newRunSchema, useCreateRun, useRun"
```

---

## Task 11: Frontend — `NewRunModal` + tests (TDD)

**Files:**
- Create: `web/src/features/runs/NewRunModal.tsx`
- Create: `web/src/features/runs/NewRunModal.test.tsx`

- [ ] **Step 11.1: Write the failing tests**

Create `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/features/runs/NewRunModal.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NewRunModal } from './NewRunModal';

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>();
  return { ...actual, useNavigate: () => mockNavigate };
});

function renderModal(open = true) {
  const onOpenChange = vi.fn();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <NewRunModal open={open} onOpenChange={onOpenChange} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { onOpenChange };
}

describe('NewRunModal', () => {
  it('renders the bundle and rack fields when open', () => {
    renderModal(true);
    expect(screen.getByLabelText(/firmware bundle/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/rack/i)).toBeInTheDocument();
  });

  it('shows an error when rack is invalid', async () => {
    const user = userEvent.setup();
    renderModal(true);
    await user.type(screen.getByLabelText(/firmware bundle/i), 'gb200-fw-2026-05');
    await user.type(screen.getByLabelText(/rack/i), 'as1-r07');
    await user.click(screen.getByRole('button', { name: /create/i }));
    expect(await screen.findByText(/Rack format example/i)).toBeInTheDocument();
  });

  it('shows an error when bundle is empty', async () => {
    const user = userEvent.setup();
    renderModal(true);
    await user.type(screen.getByLabelText(/rack/i), 'dh3-r012-us-east-01a');
    await user.click(screen.getByRole('button', { name: /create/i }));
    expect(await screen.findByText(/Bundle is required/i)).toBeInTheDocument();
  });

  it('submits valid input, closes, and navigates to the new run', async () => {
    const user = userEvent.setup();
    const { onOpenChange } = renderModal(true);
    await user.type(screen.getByLabelText(/firmware bundle/i), 'gb200-fw-2026-05');
    await user.type(screen.getByLabelText(/rack/i), 'dh3-r012-us-east-01a');
    await user.click(screen.getByRole('button', { name: /create/i }));

    await vi.waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/runs/01HZX0000000000000000000Z');
    });
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
```

- [ ] **Step 11.2: Run — expect compile failure**

```bash
yarn test --run NewRunModal
```

Expected: `Cannot find module './NewRunModal'`.

- [ ] **Step 11.3: Write `NewRunModal.tsx`**

Create `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/features/runs/NewRunModal.tsx`:

```tsx
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useNavigate } from 'react-router-dom';
import { toast } from 'sonner';
import { Dialog } from '@/components/Dialog';
import { newRunSchema, type NewRunInput } from './newRunSchema';
import { useCreateRun } from './useCreateRun';

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function NewRunModal({ open, onOpenChange }: Props) {
  const navigate = useNavigate();
  const createRun = useCreateRun();
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
    reset,
  } = useForm<NewRunInput>({ resolver: zodResolver(newRunSchema) });

  const onSubmit = handleSubmit(async (values) => {
    try {
      const { run } = await createRun.mutateAsync(values);
      reset();
      onOpenChange(false);
      navigate(`/runs/${run.id}`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create run');
    }
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange} title="New canary run">
      <form onSubmit={onSubmit} className="space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium" htmlFor="bundle">
            Firmware bundle
          </label>
          <input
            id="bundle"
            type="text"
            className="w-full rounded border border-zinc-300 px-3 py-2 text-sm"
            placeholder="gb200-fw-2026-05-canary-3"
            {...register('bundle')}
          />
          {errors.bundle && (
            <p className="mt-1 text-xs text-red-700">{errors.bundle.message}</p>
          )}
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium" htmlFor="rack">
            Rack
          </label>
          <input
            id="rack"
            type="text"
            className="w-full rounded border border-zinc-300 px-3 py-2 text-sm"
            placeholder="dh3-r012-us-east-01a"
            {...register('rack')}
          />
          <p className="mt-1 text-xs text-zinc-500">Comma-separate for multiple racks.</p>
          {errors.rack && <p className="mt-1 text-xs text-red-700">{errors.rack.message}</p>}
        </div>
        <div className="flex justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={() => onOpenChange(false)}
            className="rounded border border-zinc-300 px-3 py-1.5 text-sm hover:bg-zinc-50"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={isSubmitting}
            className="rounded bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-800 disabled:opacity-50"
          >
            {isSubmitting ? 'Creating' : 'Create'}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
```

- [ ] **Step 11.4: Run — expect PASS**

```bash
yarn test --run NewRunModal
```

Expected: all 4 tests PASS.

- [ ] **Step 11.5: Commit**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac
git add web/src/features/runs/NewRunModal.tsx web/src/features/runs/NewRunModal.test.tsx
git commit -m "qac: web — NewRunModal with zod validation and create flow"
```

---

## Task 12: Frontend — `RunDetail` + tests (TDD)

**Files:**
- Create: `web/src/features/runs/RunDetail.tsx`
- Create: `web/src/features/runs/RunDetail.test.tsx`

- [ ] **Step 12.1: Write the failing tests**

Create `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/features/runs/RunDetail.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { http, HttpResponse } from 'msw';
import { server } from '@/mocks/server';
import { RunDetail } from './RunDetail';

function renderAt(path: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/runs/:id" element={<RunDetail />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('RunDetail', () => {
  it('renders the run header and one event row', async () => {
    renderAt('/runs/01HZX0000000000000000000A');
    expect(await screen.findByText('01HZX0000000000000000000A')).toBeInTheDocument();
    expect(screen.getByText('gb200-fw-2026-05-canary-3')).toBeInTheDocument();
    expect(screen.getByText('dh3-r012-us-east-01a')).toBeInTheDocument();
    expect(await screen.findByText('RunCreated')).toBeInTheDocument();
  });

  it('shows the not-found state when the run is missing', async () => {
    renderAt('/runs/does-not-exist');
    expect(await screen.findByText(/Run not found/i)).toBeInTheDocument();
  });

  it('shows the error state when the API returns 500', async () => {
    server.use(
      http.get('/api/runs/:id', () =>
        HttpResponse.json(
          { error: { code: 'internal', message: 'Failed to load run' } },
          { status: 500 },
        ),
      ),
    );
    renderAt('/runs/01HZX0000000000000000000A');
    expect(await screen.findByText(/Failed to load run/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 12.2: Run — expect compile failure**

```bash
yarn test --run RunDetail
```

Expected: `Cannot find module './RunDetail'`.

- [ ] **Step 12.3: Write `RunDetail.tsx`**

Create `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/features/runs/RunDetail.tsx`:

```tsx
import { useParams, Link } from 'react-router-dom';
import { format, parseISO } from 'date-fns';
import { Badge } from '@/components/Badge';
import { useRun } from './useRun';
import { statusToTone } from './statusToTone';

export function RunDetail() {
  const { id = '' } = useParams<{ id: string }>();
  const { data, isLoading, isError, error } = useRun(id);

  if (isLoading) return <p className="text-zinc-500">Loading run</p>;

  if (isError) {
    const code = (error as { code?: string } | null)?.code;
    if (code === 'not_found') {
      return (
        <div className="space-y-2">
          <p className="text-zinc-700">Run not found.</p>
          <Link to="/" className="text-blue-700 hover:underline">
            Back to runs
          </Link>
        </div>
      );
    }
    return (
      <p className="text-red-700">{(error as Error | null)?.message ?? 'Failed to load run'}</p>
    );
  }

  if (!data) return null;

  const { run, events } = data;
  return (
    <div className="space-y-6">
      <div>
        <Link to="/" className="text-sm text-blue-700 hover:underline">
          ← Runs
        </Link>
      </div>
      <header className="space-y-3">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">Run</h1>
          <Badge tone={statusToTone(run.status)}>{run.status}</Badge>
        </div>
        <dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-1 text-sm">
          <dt className="text-zinc-500">Id</dt>
          <dd className="font-mono">{run.id}</dd>
          <dt className="text-zinc-500">Bundle</dt>
          <dd>{run.bundle}</dd>
          <dt className="text-zinc-500">Rack</dt>
          <dd>{run.rack}</dd>
          <dt className="text-zinc-500">Template</dt>
          <dd>{run.template_id}</dd>
          <dt className="text-zinc-500">Created</dt>
          <dd>{format(parseISO(run.created_at), 'PPpp')}</dd>
          <dt className="text-zinc-500">Created by</dt>
          <dd>{run.created_by}</dd>
        </dl>
      </header>
      <section>
        <h2 className="mb-2 text-lg font-medium">Events</h2>
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-zinc-200 text-left">
              <th className="px-3 py-2 font-medium text-zinc-700">Seq</th>
              <th className="px-3 py-2 font-medium text-zinc-700">Kind</th>
              <th className="px-3 py-2 font-medium text-zinc-700">When</th>
            </tr>
          </thead>
          <tbody>
            {events.map((e) => (
              <tr key={e.seq} className="border-b border-zinc-100">
                <td className="px-3 py-2 font-mono text-zinc-600">{e.seq}</td>
                <td className="px-3 py-2">{e.kind}</td>
                <td className="px-3 py-2 text-zinc-600">
                  {format(parseISO(e.occurred_at), 'PPpp')}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </div>
  );
}
```

- [ ] **Step 12.4: Run — expect PASS**

```bash
yarn test --run RunDetail
```

Expected: all 3 tests PASS.

- [ ] **Step 12.5: Commit**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac
git add web/src/features/runs/RunDetail.tsx web/src/features/runs/RunDetail.test.tsx
git commit -m "qac: web — RunDetail page with header + event log"
```

---

## Task 13: Frontend — `RunsList` updates (New run button + Bundle column)

**Files:**
- Modify: `web/src/features/runs/RunsList.tsx`
- Modify: `web/src/features/runs/RunsList.test.tsx`

- [ ] **Step 13.1: Append failing tests for the new behaviors**

Append to `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/features/runs/RunsList.test.tsx`:

```tsx
import userEvent from '@testing-library/user-event'; // already imported, kept for reference

// Append inside the existing describe('RunsList', ...) block:

it('opens the new-run modal when the New run button is clicked', async () => {
  const user = userEvent.setup();
  renderWithProviders(<RunsList />);
  await user.click(screen.getByRole('button', { name: /new run/i }));
  expect(await screen.findByText(/New canary run/i)).toBeInTheDocument();
});

it('shows the bundle column with values from the API', async () => {
  renderWithProviders(<RunsList />);
  expect(await screen.findByText('gb200-fw-2026-05-canary-3')).toBeInTheDocument();
});
```

- [ ] **Step 13.2: Run — expect failure**

```bash
yarn test --run RunsList
```

Expected: the new tests fail (no `New run` button, no `bundle` column rendered).

- [ ] **Step 13.3: Update `RunsList.tsx`**

Replace `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/features/runs/RunsList.tsx`. The full file (changes: import `useState` and `NewRunModal`; replace `template_id` column with `bundle`; add New-run button and modal state):

```tsx
import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
  useReactTable,
  type SortingState,
} from '@tanstack/react-table';
import { formatDistanceToNow, parseISO } from 'date-fns';
import { Badge } from '@/components/Badge';
import { useRuns } from './useRuns';
import { statusToTone } from './statusToTone';
import { NewRunModal } from './NewRunModal';
import type { Run } from './runs.types';

const columnHelper = createColumnHelper<Run>();

const columns = [
  columnHelper.accessor('id', {
    header: 'Id',
    cell: (info) => (
      <Link
        to={`/runs/${info.getValue()}`}
        className="font-mono text-blue-700 hover:underline"
      >
        {info.getValue()}
      </Link>
    ),
  }),
  columnHelper.accessor('bundle', { header: 'Bundle' }),
  columnHelper.accessor('rack', { header: 'Rack' }),
  columnHelper.accessor('status', {
    header: 'Status',
    cell: (info) => <Badge tone={statusToTone(info.getValue())}>{info.getValue()}</Badge>,
  }),
  columnHelper.accessor('created_at', {
    header: 'Created',
    cell: (info) => formatDistanceToNow(parseISO(info.getValue()), { addSuffix: true }),
  }),
  columnHelper.accessor('created_by', { header: 'Created by' }),
];

export function RunsList() {
  const { data, isLoading, isError, error, refetch } = useRuns();
  const [rackFilter, setRackFilter] = useState('');
  const [sorting, setSorting] = useState<SortingState>([{ id: 'created_at', desc: true }]);
  const [modalOpen, setModalOpen] = useState(false);

  const filtered = useMemo(() => {
    if (!data) return [];
    if (!rackFilter) return data;
    const q = rackFilter.toLowerCase();
    return data.filter((r) => r.rack.toLowerCase().includes(q));
  }, [data, rackFilter]);

  const table = useReactTable({
    data: filtered,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
  });

  if (isError) {
    return (
      <div className="rounded border border-red-200 bg-red-50 p-4 text-red-800">
        <p className="font-medium">{error?.message ?? 'Failed to load runs'}</p>
        <button
          type="button"
          className="mt-2 rounded bg-red-100 px-3 py-1 text-sm hover:bg-red-200"
          onClick={() => refetch()}
        >
          Retry
        </button>
      </div>
    );
  }

  if (isLoading) {
    return <p className="text-zinc-500">Loading runs</p>;
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <input
          type="text"
          placeholder="Filter by rack"
          value={rackFilter}
          onChange={(e) => setRackFilter(e.target.value)}
          className="w-64 rounded border border-zinc-300 px-3 py-1 text-sm"
        />
        <button
          type="button"
          onClick={() => setModalOpen(true)}
          className="rounded bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-800"
        >
          New run
        </button>
      </div>

      {!data || data.length === 0 ? (
        <p className="text-zinc-500">No runs yet. Create one to get started.</p>
      ) : (
        <table className="w-full border-collapse text-sm">
          <thead>
            {table.getHeaderGroups().map((hg) => (
              <tr key={hg.id} className="border-b border-zinc-200 text-left">
                {hg.headers.map((h) => (
                  <th
                    key={h.id}
                    className="cursor-pointer px-3 py-2 font-medium text-zinc-700"
                    onClick={h.column.getToggleSortingHandler()}
                  >
                    {flexRender(h.column.columnDef.header, h.getContext())}
                    {h.column.getIsSorted() === 'asc'
                      ? ' ↑'
                      : h.column.getIsSorted() === 'desc'
                        ? ' ↓'
                        : ''}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {table.getRowModel().rows.map((row) => (
              <tr key={row.id} className="border-b border-zinc-100 hover:bg-zinc-50">
                {row.getVisibleCells().map((cell) => (
                  <td key={cell.id} className="px-3 py-2">
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <NewRunModal open={modalOpen} onOpenChange={setModalOpen} />
    </div>
  );
}
```

Note: the empty-state condition moved INSIDE the main return tree so the "New run" button is always visible (even when the table is empty — that's exactly when the user needs the button most).

- [ ] **Step 13.4: Run — expect PASS**

```bash
yarn test --run
```

Expected: all 18 tests PASS (9 from prior + 4 NewRunModal + 3 RunDetail + 2 new RunsList).

- [ ] **Step 13.5: Commit**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac
git add web/src/features/runs/RunsList.tsx web/src/features/runs/RunsList.test.tsx
git commit -m "qac: web — RunsList shows bundle column and New run button"
```

---

## Task 14: Frontend — Wire `RunDetail` into `App.tsx`

**Files:**
- Modify: `web/src/App.tsx`

- [ ] **Step 14.1: Replace `App.tsx`**

Replace `/Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web/src/App.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { Toaster } from 'sonner';
import { RunsList } from '@/features/runs/RunsList';
import { RunDetail } from '@/features/runs/RunDetail';

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 5_000 } },
});

const router = createBrowserRouter([
  {
    path: '/',
    element: (
      <main className="mx-auto max-w-6xl p-6">
        <h1 className="mb-4 text-2xl font-semibold">Runs</h1>
        <RunsList />
      </main>
    ),
  },
  {
    path: '/runs/:id',
    element: (
      <main className="mx-auto max-w-6xl p-6">
        <RunDetail />
      </main>
    ),
  },
]);

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
      <Toaster position="top-right" />
    </QueryClientProvider>
  );
}
```

- [ ] **Step 14.2: Build the SPA**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac/web
yarn build
ls dist/
```

Expected: `dist/index.html` + `dist/assets/*` regenerated.

- [ ] **Step 14.3: Commit**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac
git add web/src/App.tsx
git commit -m "qac: web — wire RunDetail into /runs/:id route"
```

---

## Task 15: Acceptance verification (spec §14)

This task walks each of the 13 acceptance criteria from the spec. Report PASS/FAIL per criterion with evidence. Do not try to fix failures here — report only.

- [ ] **Step 15.1 — Criterion 1: `make build` succeeds from clean**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac
make clean
make build
test -x ../bin/qac && echo OK
```

Expected: `OK`.

- [ ] **Step 15.2 — Criterion 2: `make test` passes — 21 Go + 18 Vitest**

```bash
make test 2>&1 | tail -40
```

Expected:
- Go: `ok qac/internal/api` (12), `ok qac/internal/engine` (4 + subtests), `ok qac/internal/store` (5)
- Vitest: `Tests  18 passed (18)`

- [ ] **Step 15.3 — Criterion 3: `make lint` clean**

```bash
make lint 2>&1 | tail -20
```

Expected: exit 0, no errors.

- [ ] **Step 15.4 — Criterion 4: server boots cleanly on a fresh AND an iter-1 DB**

```bash
# Fresh DB:
rm -f /tmp/qac-iter2-fresh.db
../bin/qac serve --addr 127.0.0.1:8080 --db /tmp/qac-iter2-fresh.db &
SERVE_PID=$!
sleep 1
kill $SERVE_PID 2>/dev/null
wait $SERVE_PID 2>/dev/null

# Reopening the same DB — migration must be idempotent:
../bin/qac serve --addr 127.0.0.1:8080 --db /tmp/qac-iter2-fresh.db &
SERVE_PID=$!
sleep 1
curl -s http://127.0.0.1:8080/api/health
kill $SERVE_PID 2>/dev/null
wait $SERVE_PID 2>/dev/null
```

Expected: no errors on either boot; `/api/health` returns `{ok: true, ...}` after the reopen.

- [ ] **Step 15.5 — Criterion 5+6+7: Browser smoke (the modal flow)**

Manual:
```bash
rm -f /tmp/qac-iter2-smoke.db
../bin/qac serve --addr 127.0.0.1:8080 --db /tmp/qac-iter2-smoke.db &
open http://127.0.0.1:8080
```

Confirm visually:
- Page shows runs list (empty state copy: "No runs yet. Create one to get started.")
- "New run" button is visible on the right of the rack filter
- Click "New run" — modal opens, title is "New canary run", two labeled fields (Firmware bundle, Rack), Cancel + Create buttons
- Fill bundle = `gb200-fw-2026-05-canary-3`, rack = `dh3-r012-us-east-01a`, click Create
- Modal closes, URL changes to `/runs/<ulid>`
- Detail page shows the run's metadata header with `pending` badge and an Events table containing one row (Seq=1, Kind=RunCreated)

Then re-open the modal, type rack = `as1-r07`, click Create — modal stays open, inline error "Rack format example: dh3-r012-us-east-01a" appears. Network log shows NO `POST /api/runs` request.

Kill the server when done.

- [ ] **Step 15.6 — Criterion 8: `curl` invalid rack returns 400 envelope**

```bash
../bin/qac serve --addr 127.0.0.1:8080 --db /tmp/qac-iter2-curl.db &
SERVE_PID=$!
sleep 1
curl -s -w '\n%{http_code}\n' -X POST http://127.0.0.1:8080/api/runs \
  -H 'Content-Type: application/json' \
  -d '{"bundle":"b","rack":"DH3"}'
kill $SERVE_PID 2>/dev/null
wait $SERVE_PID 2>/dev/null
rm -f /tmp/qac-iter2-curl.db
```

Expected: response body has `{"error":{"code":"invalid_request",...}}` and HTTP status 400.

- [ ] **Step 15.7 — Criterion 9: GET /api/runs includes the new run with bundle**

```bash
../bin/qac serve --addr 127.0.0.1:8080 --db /tmp/qac-iter2-curl.db &
SERVE_PID=$!
sleep 1
curl -s -X POST http://127.0.0.1:8080/api/runs \
  -H 'Content-Type: application/json' \
  -d '{"bundle":"gb200-fw-2026-05-canary-3","rack":"dh3-r012-us-east-01a"}' \
  | python3 -m json.tool
curl -s http://127.0.0.1:8080/api/runs | python3 -m json.tool
kill $SERVE_PID 2>/dev/null
wait $SERVE_PID 2>/dev/null
rm -f /tmp/qac-iter2-curl.db
```

Expected: POST returns the created run with the bundle field. GET shows the run with `bundle`, `template_id: "firmware-release-canary"`, `status: "pending"`.

- [ ] **Step 15.8 — Criterion 11: GET /runs/does-not-exist shows not-found state**

```bash
../bin/qac serve --addr 127.0.0.1:8080 --db /tmp/qac-iter2-nf.db &
SERVE_PID=$!
sleep 1
# Backend 404:
curl -s -w '\n%{http_code}\n' http://127.0.0.1:8080/api/runs/does-not-exist
# Manual browser confirmation: open http://127.0.0.1:8080/runs/does-not-exist
#   Expected: "Run not found." + "Back to runs" link, no spinner.
kill $SERVE_PID 2>/dev/null
wait $SERVE_PID 2>/dev/null
rm -f /tmp/qac-iter2-nf.db
```

Expected: 404 with `{"error":{"code":"not_found","message":"Run not found"}}`.

- [ ] **Step 15.9 — Criterion 12: Copy guide compliance**

```bash
# No exclamation points in UI text:
grep -rn '!' web/src/ --include='*.tsx' --include='*.ts' \
  | grep -v 'test\.' \
  | grep -E '"[^"]*!' || echo "PASS: no exclamation in user-facing strings"

# No standalone & in UI text (excluding JSX expression braces, code, etc.):
grep -rn '\\&' web/src/ --include='*.tsx' || echo "PASS: no escaped & in UI"
```

Manual eyeballing: column headers ("Bundle", "Rack", "Status", "Template", "Created", "Created by") all sentence case; button labels ("New run", "Cancel", "Create") all sentence case; modal title "New canary run".

- [ ] **Step 15.10 — Criterion 13: Self-contained**

```bash
grep -E "^replace" go.mod || echo "no replace directives"
grep -rE "coreweave/(tiphys|qac)|argo/" --include="*.go" --include="*.ts" --include="*.tsx" . 2>/dev/null || echo "no sibling imports"
```

Expected: both echo lines fire.

- [ ] **Step 15.11 — Cleanup**

```bash
rm -f /tmp/qac-iter2-*.db /tmp/qac-iter2-*.db-shm /tmp/qac-iter2-*.db-wal
```

- [ ] **Step 15.12 — Final status**

Report **N/13 PASS, M/13 FAIL**. If any failed, list them and the evidence. Do not attempt fixes in this task — controller decides next steps.

---

## Plan self-review

**1. Spec coverage:**
- §2 Scope items 1–5 — all covered: POST endpoint (Tasks 5–6), schema (Task 2), modal (Tasks 9–11), detail page (Task 12), list-page changes (Task 13).
- §5 Rack format — regex defined in Task 5 (Go) and Task 10 (TS); cross-reference comments in both.
- §6 Architecture — file changes match the table at the top of this plan exactly.
- §7 Schema migration — Task 2 with idempotency test.
- §8 API surface — Tasks 5 (validate) + 6 (handler + registration); error envelope reused from iter-1.
- §9 Validation — Task 5.
- §10 Handler — Task 6.
- §11 Frontend additions — Tasks 7 (apiPost), 8 (types+mocks), 9 (Dialog), 10 (schema+hooks), 11 (modal), 12 (detail), 13 (list), 14 (App).
- §12 Error handling — surfaces match: zod for client-side, regex for server-side, error envelope from Task 6 of iter-1 (unchanged).
- §13 Testing — counts match: 21 Go + 18 Vitest.
- §14 Acceptance criteria — Task 15 walks each criterion.

**2. Placeholder scan:** No "TBD" / "TODO" / "implement later" / "fill in details" left in the plan. Every code block is complete. Every command has its expected output. The Task 13 RunsList full file is complete (no "rest unchanged" gaps).

**3. Type consistency:**
- `ValidateRacks(input string) (canonical string, racks []string, errMsg string)` — declared in Task 5, used identically in Task 6. ✓
- `apiPost<TRes, TBody>(path: string, body: TBody): Promise<TRes>` — declared in Task 7, used in Task 10 (`useCreateRun`). ✓
- `NewRunInput` from `z.infer<typeof newRunSchema>` — declared in Task 10, used in Task 10 (`useCreateRun`) and Task 11 (`NewRunModal`). ✓
- `Run` shape — type updated in Task 8 (TS), Go struct updated in Task 4, both have `bundle: string` / `Bundle string`. ✓
- `Event` type — TS declared in Task 8, Go declared in Task 4 (already existed from iter-1). ✓
- `useRun(id)` returns `{ run, events }` — declared in Task 10, used in Task 12. ✓

No drift detected.

---

## Notes for the Go beginner (read in order)

1. **Task 2 — `pragma_table_info`:** SQLite's introspection table. Avoids the ALTER duplicate-column error. The pattern (`SELECT COUNT(*) FROM pragma_table_info('runs') WHERE name = 'bundle'`) is the canonical idiom for "does this column exist yet?".
2. **Task 5 — `regexp.MustCompile`:** Package-level vars + the "Must" panic-on-error variant. Standard Go idiom for hardcoded patterns.
3. **Task 5 — Multi-return functions:** Three return values, last is a sentinel string. Differs from the common `(T, error)` shape because the caller wants the message for the user-facing API response, not a wrappable error chain.
4. **Task 6 — `json.NewDecoder(r.Body).Decode(...)`:** Streaming JSON decode. Allocates less than `io.ReadAll` + `json.Unmarshal` for large bodies; for small ones the perf difference is noise but the pattern is the convention.
5. **Task 6 — `http.StatusCreated`:** 201 vs 200. Convention: 201 when a new resource is created and returned. Carries semantic weight in REST clients.
6. **Task 6 — `ulid.Make()`:** Convenience constructor for ULIDs (current time, default entropy). Lexicographic ordering = chronological ordering.
