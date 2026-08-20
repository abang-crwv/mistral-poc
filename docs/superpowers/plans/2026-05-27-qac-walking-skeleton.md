# qac Walking Skeleton Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the qac walking skeleton end-to-end: a single Go binary that serves an embedded React SPA showing a runs-list page backed by event-sourced SQLite.

**Architecture:** Single Go process. Go owns HTTP server, SQLite store, event-fold projection, and JSON API at `/api/*`. React SPA (built by Vite) is compiled to `web/dist/` and embedded into the Go binary via `//go:embed all:web/dist`. In dev, Vite runs on `:5173` and proxies `/api/*` → `:8080`. Event-sourced data model: `events` table is source of truth, `runs` is a denormalized projection kept current by the store in the same transaction as event appends.

**Tech Stack:**
- **Backend:** Go ≥1.22 — `net/http` + `ServeMux`, `modernc.org/sqlite` (pure-Go, no CGO), `spf13/cobra`, `oklog/ulid/v2`, `embed.FS`
- **Frontend:** React 18 + TypeScript 5 + Vite 5 — Tailwind 3 + `tailwind-styled-components` + Radix UI + `cva`, TanStack Table + TanStack Query, React Router 6, Zustand, react-hook-form + zod, sonner, lucide-react, date-fns, Vitest + Testing Library + MSW, ESLint (airbnb-typescript) + Prettier
- **Tooling:** Yarn 4, Node ≥20.10, GNU Make
- **Module path:** bare `qac` (self-contained, no replace directives)
- **Project root:** `team/wpena/qac/` in `fleet-ops-sandbox`

**Spec:** `docs/superpowers/specs/2026-05-27-qac-rebuild-design.md`

**For the Go beginner:** Several tasks introduce new Go concepts — they're marked with a 🟦 **Go concept** callout. Read the callout, optionally pause and look the concept up, then proceed. Concepts introduced (in order): module init and packages; `context.Context` plumbing; `database/sql` + drivers; struct tags for JSON; error wrapping with `%w`; pure functions for projection; `http.Handler` interface; `httptest`; middleware as `http.Handler` wrapping; `embed.FS`; cobra subcommands.

---

## File structure

**Top-level (`team/wpena/qac/`):**

```
go.mod, go.sum            # Go module manifest
Makefile                  # ui | backend | build | dev | test | lint | fmt | clean
.gitignore                # web/dist (except .gitkeep placeholder), web/node_modules, qac.db*, ../bin/qac
README.md                 # Quickstart + project overview
```

**Go (`cmd/`, `internal/`):**

| Path | Responsibility |
|---|---|
| `cmd/qac/main.go` | Cobra root command, registers subcommands, calls `Execute()` |
| `cmd/qac/serve.go` | `serve` subcommand: parse `--addr` `--db`, build server, listen |
| `cmd/qac/seed_demo.go` | `seed-demo` subcommand: open store, emit one `RunCreated` event |
| `internal/store/store.go` | `Store` struct, `Open`, `Close`, transaction helper |
| `internal/store/migrations.go` | Schema migrations (single embedded SQL string for skeleton) |
| `internal/store/events.go` | `AppendEvent`: insert event + upsert run row in one txn |
| `internal/store/runs.go` | `ListRuns`, `LoadEvents` (read-only queries) |
| `internal/store/store_test.go` | Integration tests with `t.TempDir()` |
| `internal/engine/event.go` | `Event` type, kind constants, payload structs |
| `internal/engine/project.go` | `Project([]Event) RunState` — pure function |
| `internal/engine/project_test.go` | Pure unit tests on `Project` |
| `internal/api/api.go` | `Router(store)` factory returning `http.Handler` |
| `internal/api/errors.go` | Error envelope, `writeError`, `writeJSON` helpers |
| `internal/api/runs.go` | `GET /api/runs`, `GET /api/runs/:id` handlers |
| `internal/api/health.go` | `GET /api/health` handler |
| `internal/api/api_test.go` | `httptest` end-to-end tests against real store |
| `internal/server/server.go` | `New(store, addr) *http.Server` — wires API + static SPA + middleware |
| `internal/server/static.go` | `//go:embed all:web/dist` and `http.FileServer` |
| `internal/server/middleware.go` | `requestLogger` middleware |

**Frontend (`web/`):**

| Path | Responsibility |
|---|---|
| `web/package.json` | Yarn 4 manifest with pinned deps |
| `web/.yarnrc.yml` | Yarn 4 config |
| `web/index.html` | Vite entry HTML |
| `web/vite.config.ts` | Vite config: React plugin, `/api` proxy → `:8080`, build outDir `dist` |
| `web/tailwind.config.ts` | Tailwind paths + theme |
| `web/postcss.config.js` | PostCSS pipeline (autoprefixer) |
| `web/tsconfig.json` | TS config: strict, JSX, `@/*` path alias |
| `web/tsconfig.node.json` | TS config for Vite/Node files |
| `web/eslint.config.js` | ESLint airbnb-typescript + Prettier |
| `web/.prettierrc.json` | Prettier config |
| `web/src/main.tsx` | React entry: render `<App />` |
| `web/src/App.tsx` | Router + QueryClientProvider + `<Toaster />` |
| `web/src/styles/globals.css` | Tailwind directives |
| `web/src/setupTests.ts` | Vitest setup: jest-dom matchers + MSW server |
| `web/src/lib/api.ts` | `apiGet<T>(path)`, `ApiException` |
| `web/src/lib/api.test.ts` | Tests against MSW with success/404/500 |
| `web/src/components/Badge.tsx` | CVA Badge with tone variants |
| `web/src/features/runs/runs.types.ts` | `Run`, `RunStatus` TypeScript types mirroring Go |
| `web/src/features/runs/statusToTone.ts` | `statusToTone(status)` → Badge tone |
| `web/src/features/runs/useRuns.ts` | TanStack Query hook |
| `web/src/features/runs/RunsList.tsx` | The runs list page |
| `web/src/features/runs/RunsList.test.tsx` | Page tests with MSW fixtures |
| `web/src/mocks/handlers.ts` | MSW handlers mirroring Go API |
| `web/src/mocks/browser.ts` | MSW browser worker setup |
| `web/src/mocks/server.ts` | MSW Node server setup for Vitest |
| `web/dist/index.html` | Placeholder so `embed.FS` compiles before first Vite build |

---

## Task 1: Scaffold the project tree

**Files:**
- Create: `team/wpena/qac/.gitignore`
- Create: `team/wpena/qac/go.mod`
- Create: `team/wpena/qac/README.md`
- Create: `team/wpena/qac/web/dist/index.html` (placeholder so embed compiles)

🟦 **Go concept:** A Go *module* is the dependency unit (`go.mod` is the manifest). A *package* is the unit of code (each directory = one package). `go mod init <name>` creates the module manifest. Module path `qac` (bare) means this module isn't published to any registry — fine because nothing imports it externally.

- [ ] **Step 1.1: Confirm working directory**

```bash
pwd
# Expected: /Users/wpena/coreweave/fleet-ops-sandbox/team/wpena/qac
```

- [ ] **Step 1.2: Create `.gitignore`**

```gitignore
# Go binary output goes to the team-level bin/ on $PATH
/bin/

# SQLite files (data, WAL, shared memory)
qac.db
qac.db-shm
qac.db-wal

# Frontend artifacts
web/node_modules/
web/dist/*
!web/dist/.gitkeep
!web/dist/index.html

# Editor / OS
.DS_Store
*.swp
```

- [ ] **Step 1.3: Initialize Go module**

```bash
go mod init qac
```

Expected: creates `go.mod` with `module qac\n\ngo 1.22` (or current toolchain).

- [ ] **Step 1.4: Create placeholder `web/dist/index.html`**

```bash
mkdir -p web/dist
```

`web/dist/index.html` (will be overwritten by `vite build` once the frontend is scaffolded):
```html
<!doctype html><title>qac</title><p>placeholder — run <code>make ui</code> to build the real SPA.</p>
```

- [ ] **Step 1.5: Write README quickstart skeleton**

`README.md`:
````markdown
# qac

Self-contained Go binary serving a React SPA for firmware-release canary verification.

See `docs/superpowers/specs/2026-05-27-qac-rebuild-design.md` for the design.

## Quickstart

```bash
make build         # builds frontend + backend, output to ../bin/qac
./../bin/qac seed-demo
./../bin/qac serve --addr 127.0.0.1:8080
# open http://127.0.0.1:8080
```

## Development

```bash
make dev           # vite on :5173 proxying /api → :8080 (Go)
make test          # go test ./... && yarn --cwd web test
make lint          # go vet ./... && yarn --cwd web lint
```
````

- [ ] **Step 1.6: Verify build prerequisites**

```bash
go version    # Expected: go1.22 or later
node --version  # Expected: v20.10 or later
corepack enable  # ensures yarn 4 is available
```

- [ ] **Step 1.7: Commit**

```bash
git add team/wpena/qac/.gitignore team/wpena/qac/go.mod team/wpena/qac/README.md team/wpena/qac/web/dist/index.html
git commit -m "qac: scaffold module + readme + gitignore"
```

---

## Task 2: Add SQLite driver dependency

**Files:**
- Modify: `team/wpena/qac/go.mod`
- Modify: `team/wpena/qac/go.sum`

🟦 **Go concept:** `database/sql` is the stdlib SQL interface. Drivers (like `modernc.org/sqlite`) register themselves on import. We import the driver for its side effect using the blank identifier: `_ "modernc.org/sqlite"`. The package is *pure Go* (transpiled from C) — no CGO, no build flags, works in CI without system libraries.

- [ ] **Step 2.1: Add the dep**

```bash
cd team/wpena/qac
go get modernc.org/sqlite@latest
go get github.com/oklog/ulid/v2@latest
go get github.com/spf13/cobra@latest
```

- [ ] **Step 2.2: Verify go.mod includes the deps**

```bash
grep -E "modernc.org/sqlite|oklog/ulid|spf13/cobra" go.mod
```

Expected: three lines matching the three modules.

- [ ] **Step 2.3: Commit**

```bash
git add team/wpena/qac/go.mod team/wpena/qac/go.sum
git commit -m "qac: add deps (modernc sqlite, ulid, cobra)"
```

---

## Task 3: Store — Open + migrations (TDD)

**Files:**
- Create: `team/wpena/qac/internal/store/store.go`
- Create: `team/wpena/qac/internal/store/migrations.go`
- Create: `team/wpena/qac/internal/store/store_test.go`

🟦 **Go concept:** `*sql.DB` is a connection *pool*, not a single connection — safe to share across goroutines. `WAL` (Write-Ahead Logging) mode is set via PRAGMA after connection; it lets readers and writers proceed concurrently without blocking each other. Errors are values: we return them (`return err`) rather than throwing.

- [ ] **Step 3.1: Write the failing test**

`internal/store/store_test.go`:
```go
package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpen_CreatesSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "qac.db")
	s, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	row := s.DB().QueryRowContext(context.Background(),
		`SELECT name FROM sqlite_master WHERE type='table' AND name='runs'`)
	var name string
	if err := row.Scan(&name); err != nil {
		t.Fatalf("runs table missing: %v", err)
	}
	if name != "runs" {
		t.Fatalf("got table %q, want %q", name, "runs")
	}
}
```

- [ ] **Step 3.2: Run test — expect compile failure**

```bash
cd team/wpena/qac
go test ./internal/store/...
```

Expected: build error (`undefined: Open`, `undefined: Store`).

- [ ] **Step 3.3: Write `store.go`**

`internal/store/store.go`:
```go
// Package store owns the SQLite persistence layer for qac.
// The events table is the source of truth; the runs table is a denormalized
// projection kept current in the same transaction as event appends.
package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Store wraps a *sql.DB. Safe for concurrent use.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path, applies migrations,
// and enables WAL mode.
func Open(ctx context.Context, path string) (*Store, error) {
	// _journal_mode=WAL and _busy_timeout improve concurrency for the
	// single-process server case. _fk=1 enforces foreign keys.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close releases the underlying DB pool.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the underlying *sql.DB. Used by tests; production code
// should call methods on Store instead.
func (s *Store) DB() *sql.DB { return s.db }
```

- [ ] **Step 3.4: Write `migrations.go`**

`internal/store/migrations.go`:
```go
package store

import "context"

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
	_, err := s.db.ExecContext(ctx, schemaSQL)
	return err
}
```

- [ ] **Step 3.5: Run test — expect PASS**

```bash
go test ./internal/store/... -run TestOpen_CreatesSchema -v
```

Expected: `PASS`.

- [ ] **Step 3.6: Commit**

```bash
git add internal/store/
git commit -m "qac: store — Open + migrations with WAL mode"
```

---

## Task 4: Store — AppendEvent (TDD)

**Files:**
- Create: `team/wpena/qac/internal/store/events.go`
- Modify: `team/wpena/qac/internal/store/store_test.go`

🟦 **Go concept:** A *transaction* (`tx, _ := db.BeginTx(...)`) groups multiple statements atomically. We commit on success, roll back on any error using `defer`. The pattern `defer tx.Rollback()` is safe even after Commit — Rollback on a committed tx is a no-op.

- [ ] **Step 4.1: Append the failing test**

Append to `internal/store/store_test.go`:
```go
import (
	"encoding/json"
)

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
		"rack":        "as1-r07",
		"created_by":  "wpena",
	})
	if err := s.AppendEvent(ctx, runID, "RunCreated", payload); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	// runs row should exist with status=pending
	var status, rack string
	row := s.DB().QueryRowContext(ctx, `SELECT status, rack FROM runs WHERE id = ?`, runID)
	if err := row.Scan(&status, &rack); err != nil {
		t.Fatalf("scan run: %v", err)
	}
	if status != "pending" {
		t.Fatalf("status = %q, want pending", status)
	}
	if rack != "as1-r07" {
		t.Fatalf("rack = %q, want as1-r07", rack)
	}

	// events row should exist with seq=1
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

Update the imports at the top of `store_test.go` to include `"encoding/json"`.

- [ ] **Step 4.2: Run — expect compile failure**

```bash
go test ./internal/store/... -run TestAppendEvent_RunCreated_UpsertsRun
```

Expected: `undefined: AppendEvent`.

- [ ] **Step 4.3: Write `events.go`**

`internal/store/events.go`:
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
			INSERT INTO runs (id, template_id, created_at, updated_at, status, rack, created_by)
			VALUES (?, ?, ?, ?, 'pending', ?, ?)
			ON CONFLICT(id) DO NOTHING`,
			runID, p.TemplateID, now, now, p.Rack, createdBy,
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
		// Walking-skeleton scope does not advance status here; worst-wins fold
		// is applied by engine.Project on the detail endpoint. We still bump
		// updated_at so the list view sorts correctly.
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

	// Insert the event with the next seq for this run.
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

- [ ] **Step 4.4: Run — expect PASS**

```bash
go test ./internal/store/... -v
```

Expected: both `TestOpen_CreatesSchema` and `TestAppendEvent_RunCreated_UpsertsRun` PASS.

- [ ] **Step 4.5: Commit**

```bash
git add internal/store/events.go internal/store/store_test.go
git commit -m "qac: store — AppendEvent with transactional runs projection"
```

---

## Task 5: Store — ListRuns + LoadEvents (TDD)

**Files:**
- Create: `team/wpena/qac/internal/store/runs.go`
- Modify: `team/wpena/qac/internal/store/store_test.go`

🟦 **Go concept:** `sql.Rows` must be `defer rows.Close()`d to return the connection to the pool. The pattern is: `rows, err := db.Query(...)` → check err → `defer rows.Close()` → `for rows.Next() { rows.Scan(...) }` → `rows.Err()`.

- [ ] **Step 5.1: Append failing tests**

Append to `internal/store/store_test.go`:
```go
func TestListRuns_OrdersByUpdatedDesc(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "qac.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Insert two runs; the second is newer by one second via manual UPDATE
	mk := func(id, rack string) {
		p, _ := json.Marshal(map[string]string{"template_id": "t", "rack": rack, "created_by": "wpena"})
		if err := s.AppendEvent(ctx, id, "RunCreated", p); err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
	}
	mk("01HZX0000000000000000000A", "as1-r07")
	mk("01HZX0000000000000000000B", "as2-r11")
	if _, err := s.DB().ExecContext(ctx,
		`UPDATE runs SET updated_at = updated_at + 1 WHERE id = ?`,
		"01HZX0000000000000000000B"); err != nil {
		t.Fatalf("bump: %v", err)
	}

	runs, err := s.ListRuns(ctx)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len = %d, want 2", len(runs))
	}
	if runs[0].ID != "01HZX0000000000000000000B" {
		t.Fatalf("first.ID = %q, want newest first", runs[0].ID)
	}
}

func TestLoadEvents_ReturnsInSeqOrder(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "qac.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	runID := "01HZX0000000000000000000A"
	p, _ := json.Marshal(map[string]string{"template_id": "t", "rack": "as1-r07", "created_by": "wpena"})
	if err := s.AppendEvent(ctx, runID, "RunCreated", p); err != nil {
		t.Fatalf("AppendEvent RunCreated: %v", err)
	}
	if err := s.AppendEvent(ctx, runID, "StepStarted", []byte(`{"step_id":"capacity_check"}`)); err != nil {
		t.Fatalf("AppendEvent StepStarted: %v", err)
	}

	events, err := s.LoadEvents(ctx, runID)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len = %d, want 2", len(events))
	}
	if events[0].Kind != "RunCreated" || events[1].Kind != "StepStarted" {
		t.Fatalf("kinds = [%q, %q], want [RunCreated, StepStarted]", events[0].Kind, events[1].Kind)
	}
}
```

- [ ] **Step 5.2: Run — expect compile failure**

```bash
go test ./internal/store/...
```

Expected: `undefined: ListRuns`, `undefined: LoadEvents`, `undefined: Run`, `undefined: Event`.

- [ ] **Step 5.3: Write `runs.go`**

`internal/store/runs.go`:
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
		SELECT id, template_id, created_at, updated_at, status, rack, created_by
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
		if err := rows.Scan(&r.ID, &r.TemplateID, &createdAt, &updatedAt, &r.Status, &r.Rack, &r.CreatedBy); err != nil {
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

- [ ] **Step 5.4: Run — expect PASS**

```bash
go test ./internal/store/... -v
```

Expected: 4 tests PASS.

- [ ] **Step 5.5: Commit**

```bash
git add internal/store/runs.go internal/store/store_test.go
git commit -m "qac: store — ListRuns + LoadEvents with time conversion"
```

---

## Task 6: Engine — Event types + kind constants

**Files:**
- Create: `team/wpena/qac/internal/engine/event.go`

🟦 **Go concept:** A `const` block can declare multiple constants of the same type. We use `string` constants for event kinds (matches what's serialized) rather than an `iota` enum, because the strings cross the JSON boundary into the frontend.

- [ ] **Step 6.1: Write `event.go`**

`internal/engine/event.go`:
```go
// Package engine projects an event log into a RunState. The Project
// function is pure: no I/O, no DB, no clock.
package engine

import (
	"encoding/json"
	"time"
)

// Event kind constants — string values must match what's persisted by
// internal/store and what the frontend sees on the wire.
const (
	KindRunCreated     = "RunCreated"
	KindStepStarted    = "StepStarted"
	KindStepCompleted  = "StepCompleted"
	KindOperatorAction = "OperatorAction"
)

// Run status values — must match what internal/store writes to runs.status.
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusPassed    = "passed"
	StatusWarning   = "warning"
	StatusFailed    = "failed"
	StatusSignedOff = "signed_off"
)

// Verdict values for StepCompleted.
const (
	VerdictPassed  = "passed"
	VerdictWarning = "warning"
	VerdictFailed  = "failed"
)

// Event is one entry from the event log. Payload is keyed by Kind.
type Event struct {
	Seq        int64
	Kind       string
	OccurredAt time.Time
	Payload    json.RawMessage
}

// Payload structs — one per kind. Used by both the producer (caller of
// store.AppendEvent) and the projection (decoded inside Project).

type RunCreatedPayload struct {
	TemplateID string `json:"template_id"`
	Rack       string `json:"rack"`
	CreatedBy  string `json:"created_by"`
}

type StepStartedPayload struct {
	StepID string `json:"step_id"`
}

type StepCompletedPayload struct {
	StepID  string `json:"step_id"`
	Verdict string `json:"verdict"` // passed | warning | failed
}

type OperatorActionPayload struct {
	Action string `json:"action"` // signoff | ignore_node | retry_step | ...
	Reason string `json:"reason"`
}
```

- [ ] **Step 6.2: Compile check**

```bash
cd team/wpena/qac
go build ./internal/engine/...
```

Expected: no output, exit 0.

- [ ] **Step 6.3: Commit**

```bash
git add internal/engine/event.go
git commit -m "qac: engine — event kinds, statuses, and payload types"
```

---

## Task 7: Engine — Project pure function (TDD)

**Files:**
- Create: `team/wpena/qac/internal/engine/project.go`
- Create: `team/wpena/qac/internal/engine/project_test.go`

🟦 **Go concept:** A *pure function* takes inputs and returns outputs with no side effects. `Project` is a free function (not a method on a struct) because it has no dependencies — that's a signal it's pure. Tests for pure functions are the easiest kind to write: build inputs, call, assert.

🟦 **Go concept:** `switch` on a string in Go is exhaustive *by convention*, not by compiler enforcement. There's no `exhaustive` flag like in some languages. Practice: review the switch when a new kind is added; a linter (`golangci-lint` with `exhaustive`) can catch this later if we want.

- [ ] **Step 7.1: Write failing tests**

`internal/engine/project_test.go`:
```go
package engine

import (
	"encoding/json"
	"testing"
	"time"
)

func mustPayload(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestProject_RunCreated_Pending(t *testing.T) {
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	events := []Event{
		{Seq: 1, Kind: KindRunCreated, OccurredAt: now, Payload: mustPayload(t, RunCreatedPayload{
			TemplateID: "firmware-release-canary",
			Rack:       "as1-r07",
			CreatedBy:  "wpena",
		})},
	}
	st := Project(events)
	if st.Status != StatusPending {
		t.Fatalf("status = %q, want %q", st.Status, StatusPending)
	}
	if st.Rack != "as1-r07" {
		t.Fatalf("rack = %q, want as1-r07", st.Rack)
	}
	if !st.CreatedAt.Equal(now) {
		t.Fatalf("created_at = %v, want %v", st.CreatedAt, now)
	}
}

func TestProject_StepStarted_Running(t *testing.T) {
	events := []Event{
		{Kind: KindRunCreated, Payload: mustPayload(t, RunCreatedPayload{TemplateID: "t", Rack: "r"})},
		{Kind: KindStepStarted, Payload: mustPayload(t, StepStartedPayload{StepID: "capacity_check"})},
	}
	st := Project(events)
	if st.Status != StatusRunning {
		t.Fatalf("status = %q, want %q", st.Status, StatusRunning)
	}
}

func TestProject_WorstWinsAcrossSteps(t *testing.T) {
	cases := []struct {
		name     string
		verdicts []string
		want     string
	}{
		{"all passed", []string{"passed", "passed"}, StatusPassed},
		{"warning beats passed", []string{"passed", "warning"}, StatusWarning},
		{"failed beats warning", []string{"warning", "failed"}, StatusFailed},
		{"failed sticks even if later passed", []string{"failed", "passed"}, StatusFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events := []Event{
				{Kind: KindRunCreated, Payload: mustPayload(t, RunCreatedPayload{TemplateID: "t", Rack: "r"})},
			}
			for _, v := range tc.verdicts {
				events = append(events,
					Event{Kind: KindStepStarted, Payload: mustPayload(t, StepStartedPayload{StepID: "s"})},
					Event{Kind: KindStepCompleted, Payload: mustPayload(t, StepCompletedPayload{StepID: "s", Verdict: v})},
				)
			}
			st := Project(events)
			if st.Status != tc.want {
				t.Fatalf("status = %q, want %q", st.Status, tc.want)
			}
		})
	}
}

func TestProject_Signoff_OverridesAll(t *testing.T) {
	events := []Event{
		{Kind: KindRunCreated, Payload: mustPayload(t, RunCreatedPayload{TemplateID: "t", Rack: "r"})},
		{Kind: KindStepCompleted, Payload: mustPayload(t, StepCompletedPayload{StepID: "s", Verdict: "failed"})},
		{Kind: KindOperatorAction, Payload: mustPayload(t, OperatorActionPayload{Action: "signoff", Reason: "expected failure"})},
	}
	st := Project(events)
	if st.Status != StatusSignedOff {
		t.Fatalf("status = %q, want signed_off", st.Status)
	}
}
```

- [ ] **Step 7.2: Run — expect compile failure**

```bash
go test ./internal/engine/...
```

Expected: `undefined: Project`, `undefined: RunState`.

- [ ] **Step 7.3: Write `project.go`**

`internal/engine/project.go`:
```go
package engine

import (
	"encoding/json"
	"time"
)

// StepState carries per-step verdict + signal data. Empty in the walking
// skeleton; populated once probes are wired in.
type StepState struct {
	StepID  string
	Verdict string
	// Signals []Signal — added when probes ship
}

// RunState is the projected view of a run derived from its event log.
type RunState struct {
	ID         string
	TemplateID string
	Rack       string
	CreatedBy  string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	StepStates map[string]StepState
}

// Project folds a sequence of events into a RunState. Pure function:
// no I/O, no clock, no shared state. Caller passes events in seq order.
func Project(events []Event) RunState {
	st := RunState{
		StepStates: map[string]StepState{},
	}
	// worstVerdict tracks the worst step verdict seen so far.
	// passed < warning < failed; empty means no completed steps yet.
	worstVerdict := ""

	for _, e := range events {
		st.UpdatedAt = e.OccurredAt
		switch e.Kind {

		case KindRunCreated:
			var p RunCreatedPayload
			_ = json.Unmarshal(e.Payload, &p)
			st.TemplateID = p.TemplateID
			st.Rack = p.Rack
			st.CreatedBy = p.CreatedBy
			st.CreatedAt = e.OccurredAt
			st.Status = StatusPending

		case KindStepStarted:
			var p StepStartedPayload
			_ = json.Unmarshal(e.Payload, &p)
			// Don't downgrade from a terminal status.
			if st.Status != StatusSignedOff {
				st.Status = StatusRunning
			}

		case KindStepCompleted:
			var p StepCompletedPayload
			_ = json.Unmarshal(e.Payload, &p)
			st.StepStates[p.StepID] = StepState{StepID: p.StepID, Verdict: p.Verdict}
			worstVerdict = worse(worstVerdict, p.Verdict)
			if st.Status != StatusSignedOff {
				st.Status = verdictToStatus(worstVerdict)
			}

		case KindOperatorAction:
			var p OperatorActionPayload
			_ = json.Unmarshal(e.Payload, &p)
			if p.Action == "signoff" {
				st.Status = StatusSignedOff
			}
			// Other operator actions (ignore_node, retry_step, ...) don't
			// change run-level status in the skeleton.

		default:
			// Unknown kinds are ignored. Reviewers: when adding a new kind,
			// add a case here and update the tests in project_test.go.
		}
	}
	return st
}

// worse returns the worse of two verdicts. Empty string is best (no verdict yet).
func worse(a, b string) string {
	rank := map[string]int{"": 0, VerdictPassed: 1, VerdictWarning: 2, VerdictFailed: 3}
	if rank[a] >= rank[b] {
		return a
	}
	return b
}

func verdictToStatus(verdict string) string {
	switch verdict {
	case VerdictPassed:
		return StatusPassed
	case VerdictWarning:
		return StatusWarning
	case VerdictFailed:
		return StatusFailed
	default:
		return StatusRunning
	}
}
```

- [ ] **Step 7.4: Run — expect PASS**

```bash
go test ./internal/engine/... -v
```

Expected: all 4 tests + subtests PASS.

- [ ] **Step 7.5: Commit**

```bash
git add internal/engine/project.go internal/engine/project_test.go
git commit -m "qac: engine — Project pure function with worst-wins fold"
```

---

## Task 8: API — error envelope + JSON helpers (TDD)

**Files:**
- Create: `team/wpena/qac/internal/api/errors.go`
- Create: `team/wpena/qac/internal/api/errors_test.go`

🟦 **Go concept:** `http.ResponseWriter` is an *interface*. To respond, set headers (`w.Header().Set(...)`) before writing status (`w.WriteHeader(...)`) before writing body (`w.Write(...)` or `json.NewEncoder(w).Encode(...)`). Order matters — after `WriteHeader`, header changes are silently ignored.

- [ ] **Step 8.1: Write failing test**

`internal/api/errors_test.go`:
```go
package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWriteError_Envelope(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, 404, "not_found", "Run not found")

	if w.Code != 404 {
		t.Fatalf("code = %d, want 404", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Fatalf("code = %q, want not_found", body.Error.Code)
	}
	if body.Error.Message != "Run not found" {
		t.Fatalf("message = %q, want Run not found", body.Error.Message)
	}
}

func TestWriteJSON_Body(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, 200, map[string]any{"ok": true})

	if w.Code != 200 {
		t.Fatalf("code = %d, want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("ok = %v, want true", body["ok"])
	}
}
```

- [ ] **Step 8.2: Run — expect compile failure**

```bash
go test ./internal/api/...
```

Expected: `undefined: writeError`, `undefined: writeJSON`.

- [ ] **Step 8.3: Write `errors.go`**

`internal/api/errors.go`:
```go
// Package api exposes the JSON HTTP surface backed by internal/store
// and internal/engine.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// errorBody is the wire format for every non-2xx response.
type errorBody struct {
	Error errorDetail `json:"error"`
}
type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Stable error codes — keep these short and snake_case; the frontend
// branches on them.
const (
	codeNotFound       = "not_found"
	codeInvalidRequest = "invalid_request"
	codeInternal       = "internal"
)

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(errorBody{Error: errorDetail{Code: code, Message: message}}); err != nil {
		slog.Error("encode error envelope", "err", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("encode response", "err", err)
	}
}
```

- [ ] **Step 8.4: Run — expect PASS**

```bash
go test ./internal/api/... -v
```

Expected: 2 tests PASS.

- [ ] **Step 8.5: Commit**

```bash
git add internal/api/errors.go internal/api/errors_test.go
git commit -m "qac: api — error envelope + json helpers"
```

---

## Task 9: API — health endpoint (TDD)

**Files:**
- Create: `team/wpena/qac/internal/api/health.go`
- Create: `team/wpena/qac/internal/api/api.go`
- Create: `team/wpena/qac/internal/api/api_test.go`

🟦 **Go concept:** `http.HandlerFunc` is an *adapter* — a function with signature `func(ResponseWriter, *Request)` that satisfies the `http.Handler` interface. We register handlers on a `*http.ServeMux`; Go 1.22 added method+path patterns like `GET /api/health`. The trailing `/` matters — `/api/health` vs `/api/health/` register different routes.

🟦 **Go concept:** `runtime/debug.ReadBuildInfo()` exposes module version + VCS info embedded by the Go linker at build time. Use it to surface a version string without baking one in by hand.

- [ ] **Step 9.1: Write failing test**

`internal/api/api_test.go`:
```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"qac/internal/store"
)

func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "qac.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	srv := httptest.NewServer(Router(s, "test"))
	t.Cleanup(srv.Close)
	return srv, s
}

func TestGetHealth_OK(t *testing.T) {
	srv, _ := newTestServer(t)

	res, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
		DBPath  string `json:"db_path"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.OK {
		t.Fatalf("ok = false, want true")
	}
	if body.Version == "" {
		t.Fatalf("version is empty")
	}
}
```

Note: `t.Context()` requires Go 1.24+. If the toolchain is older, swap for `context.Background()`.

- [ ] **Step 9.2: Run — expect compile failure**

```bash
go test ./internal/api/...
```

Expected: `undefined: Router`.

- [ ] **Step 9.3: Write `api.go`**

`internal/api/api.go`:
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

	// Health endpoint
	mux.HandleFunc("GET /api/health", healthHandler(dbPath))

	// Runs endpoints
	mux.HandleFunc("GET /api/runs", listRunsHandler(s))
	mux.HandleFunc("GET /api/runs/{id}", getRunHandler(s))

	return mux
}
```

- [ ] **Step 9.4: Write `health.go`**

`internal/api/health.go`:
```go
package api

import (
	"net/http"
	"runtime/debug"
)

func healthHandler(dbPath string) http.HandlerFunc {
	version := readVersion()
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"version": version,
			"db_path": dbPath,
		})
	}
}

// readVersion returns the build version from BuildInfo. Falls back to
// "dev" when running via `go run`, which doesn't embed VCS info.
func readVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			return s.Value[:min(7, len(s.Value))]
		}
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

Note: Go 1.21+ has a builtin `min`; the local definition above is only needed if targeting older toolchains. Remove it if `go.mod` uses 1.21+.

- [ ] **Step 9.5: Stub the runs handlers so api.go compiles**

`internal/api/runs.go`:
```go
package api

import (
	"net/http"

	"qac/internal/store"
)

func listRunsHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotImplemented, codeInternal, "not implemented yet")
	}
}

func getRunHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotImplemented, codeInternal, "not implemented yet")
	}
}
```

- [ ] **Step 9.6: Run — expect PASS for health, others not yet tested**

```bash
go test ./internal/api/... -run TestGetHealth -v
```

Expected: `TestGetHealth_OK` PASS.

- [ ] **Step 9.7: Commit**

```bash
git add internal/api/
git commit -m "qac: api — Router + GET /api/health"
```

---

## Task 10: API — GET /api/runs (TDD)

**Files:**
- Modify: `team/wpena/qac/internal/api/runs.go`
- Modify: `team/wpena/qac/internal/api/api_test.go`

🟦 **Go concept:** `time.Time.MarshalJSON()` defaults to RFC3339 — which is ISO-8601-compatible. Since `store.Run` already has `time.Time` fields with `json:` tags, the encoder will emit the right string format for free. No custom marshaling needed.

- [ ] **Step 10.1: Append failing test**

Append to `internal/api/api_test.go`:
```go
import (
	"context"
	encjson "encoding/json"
)

func TestGetRuns_ReturnsSeededList(t *testing.T) {
	srv, s := newTestServer(t)

	// Seed: one RunCreated event
	payload, _ := encjson.Marshal(map[string]string{
		"template_id": "firmware-release-canary",
		"rack":        "as1-r07",
		"created_by":  "wpena",
	})
	if err := s.AppendEvent(context.Background(), "01HZX0000000000000000000A", "RunCreated", payload); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	res, err := http.Get(srv.URL + "/api/runs")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct {
		Runs []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Rack   string `json:"rack"`
		} `json:"runs"`
	}
	if err := encjson.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(body.Runs))
	}
	if body.Runs[0].Status != "pending" {
		t.Fatalf("status = %q, want pending", body.Runs[0].Status)
	}
}

func TestGetRuns_EmptyWhenNoRuns(t *testing.T) {
	srv, _ := newTestServer(t)

	res, err := http.Get(srv.URL + "/api/runs")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct {
		Runs []any `json:"runs"`
	}
	if err := encjson.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Runs) != 0 {
		t.Fatalf("got %d runs, want 0", len(body.Runs))
	}
}
```

- [ ] **Step 10.2: Run — expect failure**

```bash
go test ./internal/api/... -run TestGetRuns
```

Expected: `TestGetRuns_ReturnsSeededList` fails with status 501.

- [ ] **Step 10.3: Implement `listRunsHandler`**

Replace `listRunsHandler` in `internal/api/runs.go`:
```go
func listRunsHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runs, err := s.ListRuns(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to list runs")
			return
		}
		if runs == nil {
			runs = []store.Run{} // ensure JSON encodes [] not null
		}
		writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
	}
}
```

- [ ] **Step 10.4: Run — expect PASS**

```bash
go test ./internal/api/... -v
```

Expected: all PASS.

- [ ] **Step 10.5: Commit**

```bash
git add internal/api/runs.go internal/api/api_test.go
git commit -m "qac: api — GET /api/runs reads runs projection"
```

---

## Task 11: API — GET /api/runs/:id (TDD)

**Files:**
- Modify: `team/wpena/qac/internal/api/runs.go`
- Modify: `team/wpena/qac/internal/api/api_test.go`

🟦 **Go concept:** Go 1.22 ServeMux supports path patterns with named segments. `r.PathValue("id")` reads the `{id}` segment from the URL.

- [ ] **Step 11.1: Append failing tests**

Append to `internal/api/api_test.go`:
```go
func TestGetRunByID_ReturnsRunAndEvents(t *testing.T) {
	srv, s := newTestServer(t)
	runID := "01HZX0000000000000000000A"
	payload, _ := encjson.Marshal(map[string]string{"template_id": "t", "rack": "as1-r07", "created_by": "wpena"})
	if err := s.AppendEvent(context.Background(), runID, "RunCreated", payload); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	res, err := http.Get(srv.URL + "/api/runs/" + runID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct {
		Run    struct{ ID string `json:"id"` } `json:"run"`
		Events []struct{ Kind string `json:"kind"` } `json:"events"`
	}
	if err := encjson.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Run.ID != runID {
		t.Fatalf("run.id = %q, want %q", body.Run.ID, runID)
	}
	if len(body.Events) != 1 || body.Events[0].Kind != "RunCreated" {
		t.Fatalf("events = %+v, want one RunCreated", body.Events)
	}
}

func TestGetRunByID_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)

	res, err := http.Get(srv.URL + "/api/runs/does-not-exist")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := encjson.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Fatalf("code = %q, want not_found", body.Error.Code)
	}
}
```

- [ ] **Step 11.2: Run — expect failure**

```bash
go test ./internal/api/... -run TestGetRunByID
```

Expected: both fail (handler still returns 501).

- [ ] **Step 11.3: Implement `getRunHandler`**

Replace `getRunHandler` in `internal/api/runs.go`:
```go
func getRunHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "Run id is required")
			return
		}

		// Load events; empty slice = unknown run id.
		events, err := s.LoadEvents(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to load events")
			return
		}
		if len(events) == 0 {
			writeError(w, http.StatusNotFound, codeNotFound, "Run not found")
			return
		}

		// Read the runs projection for the canonical metadata.
		runs, err := s.ListRuns(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to load run")
			return
		}
		var run *store.Run
		for i := range runs {
			if runs[i].ID == id {
				run = &runs[i]
				break
			}
		}
		if run == nil {
			writeError(w, http.StatusNotFound, codeNotFound, "Run not found")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"run":    run,
			"events": events,
		})
	}
}
```

- [ ] **Step 11.4: Run — expect PASS**

```bash
go test ./internal/api/... -v
```

Expected: all tests PASS.

- [ ] **Step 11.5: Commit**

```bash
git add internal/api/runs.go internal/api/api_test.go
git commit -m "qac: api — GET /api/runs/{id} returns run + event log"
```

---

## Task 12: Server — embed.FS + static SPA

**Files:**
- Create: `team/wpena/qac/internal/server/static.go`
- Create: `team/wpena/qac/internal/server/middleware.go`
- Create: `team/wpena/qac/internal/server/server.go`

🟦 **Go concept:** `//go:embed all:web/dist` is a *directive comment* — the Go compiler reads it during the build, includes the matching files in the binary, and assigns them to the `embed.FS` variable below the comment. `all:` includes files starting with `.` or `_`. The path is relative to the *Go source file*, not the project root.

🟦 **Go concept:** *Middleware* is just a function that takes an `http.Handler` and returns a new `http.Handler` that wraps it. Composition is via `mw(otherMw(handler))`. SPA fallback: if a request doesn't match any registered route, serve `index.html` (so client-side routing works on direct URL visits and reloads).

- [ ] **Step 12.1: Write `static.go`**

`internal/server/static.go`:
```go
package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:web/dist
var distFS embed.FS

// spaHandler serves static files from the embedded Vite build output.
// For routes that aren't static assets, it falls back to index.html so
// the client-side router can handle them.
func spaHandler() (http.Handler, error) {
	sub, err := fs.Sub(distFS, "web/dist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Static file? Let the file server handle it.
		// Anything with a "." in the last path segment is a file request.
		if strings.Contains(lastSegment(r.URL.Path), ".") {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Otherwise serve index.html so the SPA router takes over.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	}), nil
}

func lastSegment(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
```

Note: `static.go` lives in `internal/server/`, so `//go:embed all:web/dist` resolves relative to that directory. We need `web/dist/` to be reachable from there — Go's embed walks *up* from the source file only via a path relative to the file. Since `internal/server/static.go` and `web/dist/` are both under the module root, embed's path is relative to the source file's directory unless the path doesn't escape the module.

**Important:** `//go:embed` does NOT allow `..` in the pattern. The embed declaration must live in a package whose directory is an *ancestor* of (or contains) the embedded path. So this file actually needs to live higher up. Fix: put the embed in the module root (a small `embed_dist.go` at the project root in package `qac`) OR put it in `cmd/qac/`. Cleanest: keep one tiny embed file at the module root.

- [ ] **Step 12.2: Move embed to module root**

Delete the embed from `internal/server/static.go` and create `team/wpena/qac/embed_dist.go`:

```go
package qac

import "embed"

//go:embed all:web/dist
var DistFS embed.FS
```

Wait — `qac` is the module path but each package directory has its own package declaration. The module root currently has no Go files, so we need to add one with `package main` (matching `cmd/`) — actually no, that doesn't work either. The cleanest pattern is to put the embed inside `cmd/qac/` (since `cmd/qac/main.go` exists there and its parent walks include the dist tree).

**Corrected approach:** Put the embed in `cmd/qac/embed.go`, expose `DistFS` via a package-level variable, and pass it into `server.New`.

Replace the planned `internal/server/static.go` content with:
```go
package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// SPAHandler serves the SPA from the provided embed.FS rooted at
// "web/dist". The caller (cmd/qac) owns the embed directive because
// the path must be relative to its own package directory.
func SPAHandler(distFS embed.FS) (http.Handler, error) {
	sub, err := fs.Sub(distFS, "web/dist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(lastSegment(r.URL.Path), ".") {
			fileServer.ServeHTTP(w, r)
			return
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	}), nil
}

func lastSegment(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
```

- [ ] **Step 12.3: Write `middleware.go`**

`internal/server/middleware.go`:
```go
package server

import (
	"log/slog"
	"net/http"
	"time"
)

// requestLogger logs every request: method, path, status, duration.
type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (lw *loggingResponseWriter) WriteHeader(status int) {
	lw.status = status
	lw.ResponseWriter.WriteHeader(status)
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(lw, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", lw.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
```

- [ ] **Step 12.4: Write `server.go`**

`internal/server/server.go`:
```go
// Package server wires the API router, embedded SPA, and middleware
// into an http.Server.
package server

import (
	"embed"
	"net/http"

	"qac/internal/api"
	"qac/internal/store"
)

// New returns a configured *http.Server. addr is "host:port".
func New(s *store.Store, addr, dbPath string, distFS embed.FS) (*http.Server, error) {
	apiHandler := api.Router(s, dbPath)
	spa, err := SPAHandler(distFS)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	// API takes /api/* — everything else falls through to the SPA.
	mux.Handle("/api/", apiHandler)
	mux.Handle("/", spa)

	return &http.Server{
		Addr:    addr,
		Handler: requestLogger(mux),
	}, nil
}
```

- [ ] **Step 12.5: Compile check (will fail until cmd/qac exists)**

Skip; the next task adds `cmd/qac` which owns the embed declaration and calls `server.New`.

- [ ] **Step 12.6: Commit**

```bash
git add internal/server/
git commit -m "qac: server — http.Server wiring, SPA handler, logging middleware"
```

---

## Task 13: cmd/qac — cobra root + serve subcommand

**Files:**
- Create: `team/wpena/qac/cmd/qac/main.go`
- Create: `team/wpena/qac/cmd/qac/embed.go`
- Create: `team/wpena/qac/cmd/qac/serve.go`

🟦 **Go concept:** cobra represents each subcommand as a `*cobra.Command`. The root command (`qac`) has children added via `rootCmd.AddCommand(serveCmd)`. Flags are declared on the command they belong to. `RunE` returns an error (preferred over `Run` because it lets cobra surface non-zero exit codes).

🟦 **Go concept:** `os.Getenv` reads an environment variable; we use it to fall back when `--db` isn't set. The XDG Base Directory spec says user data lives under `$XDG_DATA_HOME` (default `~/.local/share`).

- [ ] **Step 13.1: Write `embed.go` (owns the //go:embed directive)**

`cmd/qac/embed.go`:
```go
package main

import "embed"

//go:embed all:../../web/dist
var distFS embed.FS
```

Note: this works because `//go:embed` paths are relative to the Go source file's directory; `../../` from `cmd/qac/` reaches the module root, and `web/dist` is under it.

Actually — `//go:embed` does **not** support `..` in the pattern. The path must be relative and stay inside the module, but Go's `embed` documentation explicitly forbids `..`. Confirm before writing:

```
The path must not begin with `/`, must not contain `..`, and must use
forward slashes...
```

So `cmd/qac/embed.go` cannot embed `../../web/dist`. Two options:
1. Move `web/dist` under `cmd/qac/dist/` and have Vite output there — ugly.
2. Put the embed file at the module root in a top-level package, then re-export.

Use option 2. Create the embed file at the *module root*:

- [ ] **Step 13.1 (corrected): Create `embed_dist.go` at the module root**

Delete `cmd/qac/embed.go` if it was created; create `team/wpena/qac/embed_dist.go`:
```go
// Package qac hosts the //go:embed declaration for the SPA bundle.
// The embed directive's pattern is relative to this file's directory,
// so the file must live at the module root.
package qac

import "embed"

//go:embed all:web/dist
var DistFS embed.FS
```

But a Go file at the module root needs a package name. The convention for a tree with `cmd/<binary>/main.go` is for the module-root package to either not exist (cmd-only modules) or to be named after the module. Here, naming the root package `qac` works — main.go in `cmd/qac/` is `package main`, separate from `package qac` at the root.

- [ ] **Step 13.2: Write `main.go`**

`cmd/qac/main.go`:
```go
// Command qac is the firmware-release canary verification tool.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "qac",
		Short: "Firmware-release canary verification",
	}
	root.AddCommand(serveCmd())
	root.AddCommand(seedDemoCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 13.3: Write `serve.go`**

`cmd/qac/serve.go`:
```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"qac"
	"qac/internal/server"
	"qac/internal/store"
)

func serveCmd() *cobra.Command {
	var addr, dbPath string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedDB, err := resolveDBPath(dbPath)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			s, err := store.Open(ctx, resolvedDB)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer s.Close()

			srv, err := server.New(s, addr, resolvedDB, qac.DistFS)
			if err != nil {
				return fmt.Errorf("build server: %w", err)
			}

			// Graceful shutdown on SIGINT/SIGTERM.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			errCh := make(chan error, 1)
			go func() {
				slog.Info("qac serve", "addr", addr, "db", resolvedDB)
				errCh <- srv.ListenAndServe()
			}()

			select {
			case sig := <-sigCh:
				slog.Info("shutting down", "signal", sig.String())
				shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
				defer c()
				_ = srv.Shutdown(shutdownCtx)
				return nil
			case err := <-errCh:
				if err != nil && err.Error() != "http: Server closed" {
					return err
				}
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8080", "listen address")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite path (default: $XDG_DATA_HOME/qac/qac.db)")
	return cmd
}

func resolveDBPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	dir := filepath.Join(dataHome, "qac")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return filepath.Join(dir, "qac.db"), nil
}
```

- [ ] **Step 13.4: Stub `seedDemoCmd` so `main.go` compiles**

Add to `cmd/qac/serve.go` (or create `cmd/qac/seed_demo.go` with a stub for now):

`cmd/qac/seed_demo.go`:
```go
package main

import "github.com/spf13/cobra"

func seedDemoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "seed-demo",
		Short: "Seed a demo run (implemented in next task)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
```

- [ ] **Step 13.5: Build the binary**

```bash
cd team/wpena/qac
mkdir -p ../bin
go build -o ../bin/qac ./cmd/qac
```

Expected: produces `../bin/qac`.

- [ ] **Step 13.6: Smoke test — serve responds**

```bash
../bin/qac serve --addr 127.0.0.1:8080 --db /tmp/qac-smoke.db &
SERVE_PID=$!
sleep 1
curl -s http://127.0.0.1:8080/api/health
# Expected: {"db_path":"...","ok":true,"version":"..."}
curl -s http://127.0.0.1:8080/api/runs
# Expected: {"runs":[]}
kill $SERVE_PID
rm -f /tmp/qac-smoke.db /tmp/qac-smoke.db-*
```

Expected: both curls return 200 JSON.

- [ ] **Step 13.7: Commit**

```bash
git add embed_dist.go cmd/qac/main.go cmd/qac/serve.go cmd/qac/seed_demo.go
git commit -m "qac: cmd — cobra root + serve subcommand + embed.FS"
```

---

## Task 14: cmd/qac — seed-demo subcommand

**Files:**
- Modify: `team/wpena/qac/cmd/qac/seed_demo.go`

🟦 **Go concept:** ULIDs (Universally Unique Lexicographically Sortable Identifiers) are 128-bit IDs whose first 48 bits are a millisecond timestamp. Sorting ULIDs lexicographically sorts them chronologically — handy for `ORDER BY id` queries. We use `github.com/oklog/ulid/v2`.

- [ ] **Step 14.1: Replace the seed-demo stub**

`cmd/qac/seed_demo.go`:
```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"

	"qac/internal/store"
)

func seedDemoCmd() *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "seed-demo",
		Short: "Seed one demo run",
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedDB, err := resolveDBPath(dbPath)
			if err != nil {
				return err
			}

			ctx := context.Background()
			s, err := store.Open(ctx, resolvedDB)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer s.Close()

			id := ulid.MustNew(ulid.Timestamp(time.Now()), ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0)).String()
			payload, err := json.Marshal(map[string]string{
				"template_id": "firmware-release-canary",
				"rack":        "as1-r07",
				"created_by":  "wpena",
			})
			if err != nil {
				return fmt.Errorf("marshal payload: %w", err)
			}
			if err := s.AppendEvent(ctx, id, "RunCreated", payload); err != nil {
				return fmt.Errorf("append event: %w", err)
			}
			fmt.Printf("Seeded run %s\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite path (default: $XDG_DATA_HOME/qac/qac.db)")
	return cmd
}
```

- [ ] **Step 14.2: Rebuild and test**

```bash
go build -o ../bin/qac ./cmd/qac
../bin/qac seed-demo --db /tmp/qac-demo.db
# Expected: Seeded run 01HZX...
../bin/qac serve --addr 127.0.0.1:8080 --db /tmp/qac-demo.db &
SERVE_PID=$!
sleep 1
curl -s http://127.0.0.1:8080/api/runs
# Expected: {"runs":[{"id":"01HZX...","status":"pending",...}]}
kill $SERVE_PID
rm -f /tmp/qac-demo.db /tmp/qac-demo.db-*
```

- [ ] **Step 14.3: Commit**

```bash
git add cmd/qac/seed_demo.go
git commit -m "qac: cmd — seed-demo creates one RunCreated event"
```

---

## Task 15: Frontend scaffold

**Files:**
- Create: `team/wpena/qac/web/.yarnrc.yml`
- Create: `team/wpena/qac/web/package.json`
- Create: `team/wpena/qac/web/tsconfig.json`
- Create: `team/wpena/qac/web/tsconfig.node.json`
- Create: `team/wpena/qac/web/vite.config.ts`
- Create: `team/wpena/qac/web/tailwind.config.ts`
- Create: `team/wpena/qac/web/postcss.config.js`
- Create: `team/wpena/qac/web/.prettierrc.json`
- Create: `team/wpena/qac/web/eslint.config.js`
- Create: `team/wpena/qac/web/index.html`
- Create: `team/wpena/qac/web/src/main.tsx`
- Create: `team/wpena/qac/web/src/App.tsx`
- Create: `team/wpena/qac/web/src/styles/globals.css`
- Create: `team/wpena/qac/web/src/setupTests.ts`

- [ ] **Step 15.1: Set up Yarn 4**

```bash
cd team/wpena/qac/web
corepack enable
yarn set version 4
```

This creates `web/.yarn/` and updates `web/.yarnrc.yml`. Confirm yarn version:
```bash
yarn --version  # Expected: 4.x.x
```

- [ ] **Step 15.2: Write `package.json`**

`web/package.json`:
```json
{
  "name": "qac-web",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "packageManager": "yarn@4.10.3",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview",
    "test": "vitest run",
    "test:watch": "vitest",
    "lint": "eslint . --ext ts,tsx --max-warnings 0",
    "format": "prettier --write \"src/**/*.{ts,tsx,css,json}\""
  },
  "dependencies": {
    "@radix-ui/react-tooltip": "1.0.7",
    "@tanstack/react-query": "5.59.16",
    "@tanstack/react-table": "8.20.5",
    "class-variance-authority": "0.7.0",
    "date-fns": "3.6.0",
    "lucide-react": "0.453.0",
    "react": "18.2.0",
    "react-dom": "18.2.0",
    "react-router-dom": "6.30.3",
    "sonner": "1.5.0",
    "zod": "3.22.4",
    "zustand": "5.0.11"
  },
  "devDependencies": {
    "@testing-library/jest-dom": "6.5.0",
    "@testing-library/react": "16.0.1",
    "@testing-library/user-event": "14.5.2",
    "@types/react": "18.2.42",
    "@types/react-dom": "18.2.17",
    "@typescript-eslint/eslint-plugin": "6.13.2",
    "@typescript-eslint/parser": "6.13.2",
    "@vitejs/plugin-react-swc": "3.5.0",
    "autoprefixer": "10.4.16",
    "eslint": "8.55.0",
    "eslint-config-airbnb-base": "15.0.0",
    "eslint-config-airbnb-typescript": "17.1.0",
    "eslint-config-prettier": "9.1.0",
    "eslint-plugin-import": "2.29.0",
    "eslint-plugin-prettier": "5.0.1",
    "eslint-plugin-react": "7.33.2",
    "eslint-plugin-react-hooks": "4.6.0",
    "jsdom": "24.1.0",
    "msw": "2.4.9",
    "postcss": "8.4.49",
    "prettier": "3.3.3",
    "tailwind-styled-components": "2.2.0",
    "tailwindcss": "3.4.14",
    "tailwindcss-animate": "1.0.7",
    "typescript": "5.3.3",
    "vite": "5.4.21",
    "vitest": "3.2.4"
  }
}
```

- [ ] **Step 15.3: Install dependencies**

```bash
cd team/wpena/qac/web
yarn install
```

Expected: `Done in ...s`.

- [ ] **Step 15.4: Write TS configs**

`web/tsconfig.json`:
```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "jsx": "react-jsx",
    "strict": true,
    "esModuleInterop": true,
    "allowImportingTsExtensions": false,
    "resolveJsonModule": true,
    "skipLibCheck": true,
    "useDefineForClassFields": true,
    "isolatedModules": true,
    "noEmit": true,
    "types": ["vitest/globals", "@testing-library/jest-dom"],
    "baseUrl": ".",
    "paths": { "@/*": ["src/*"] }
  },
  "include": ["src"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

`web/tsconfig.node.json`:
```json
{
  "compilerOptions": {
    "composite": true,
    "skipLibCheck": true,
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "allowSyntheticDefaultImports": true
  },
  "include": ["vite.config.ts", "tailwind.config.ts", "postcss.config.js"]
}
```

- [ ] **Step 15.5: Write `vite.config.ts`**

`web/vite.config.ts`:
```ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react-swc';
import path from 'node:path';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': path.resolve(__dirname, 'src') },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8080',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/setupTests.ts'],
  },
});
```

- [ ] **Step 15.6: Write Tailwind + PostCSS configs**

`web/tailwind.config.ts`:
```ts
import type { Config } from 'tailwindcss';

const config: Config = {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {},
  },
  plugins: [require('tailwindcss-animate')],
};
export default config;
```

`web/postcss.config.js`:
```js
export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
};
```

- [ ] **Step 15.7: Write ESLint + Prettier configs**

`web/.prettierrc.json`:
```json
{
  "singleQuote": true,
  "trailingComma": "all",
  "printWidth": 100,
  "tabWidth": 2
}
```

`web/eslint.config.js`:
```js
// Minimal ESLint config; airbnb-typescript config used via .eslintrc fallback
// would require legacy eslintrc. Here we use flat config with sensible defaults.
import tsParser from '@typescript-eslint/parser';
import tsPlugin from '@typescript-eslint/eslint-plugin';
import reactPlugin from 'eslint-plugin-react';
import reactHooks from 'eslint-plugin-react-hooks';
import prettierPlugin from 'eslint-plugin-prettier';
import prettierConfig from 'eslint-config-prettier';

export default [
  {
    files: ['src/**/*.{ts,tsx}'],
    languageOptions: {
      parser: tsParser,
      parserOptions: { ecmaVersion: 'latest', sourceType: 'module', ecmaFeatures: { jsx: true } },
    },
    plugins: {
      '@typescript-eslint': tsPlugin,
      react: reactPlugin,
      'react-hooks': reactHooks,
      prettier: prettierPlugin,
    },
    rules: {
      ...tsPlugin.configs.recommended.rules,
      ...reactPlugin.configs.recommended.rules,
      ...reactHooks.configs.recommended.rules,
      ...prettierConfig.rules,
      'prettier/prettier': 'error',
      'react/react-in-jsx-scope': 'off',
    },
    settings: { react: { version: '18.2' } },
  },
];
```

- [ ] **Step 15.8: Write `index.html`**

`web/index.html`:
```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>qac</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 15.9: Write `src/styles/globals.css`**

`web/src/styles/globals.css`:
```css
@tailwind base;
@tailwind components;
@tailwind utilities;

:root { color-scheme: light dark; }
body { @apply bg-zinc-50 text-zinc-900 antialiased; font-family: ui-sans-serif, system-ui, sans-serif; }
```

- [ ] **Step 15.10: Write `src/main.tsx` and minimal `src/App.tsx`**

`web/src/main.tsx`:
```tsx
import React from 'react';
import ReactDOM from 'react-dom/client';
import App from '@/App';
import '@/styles/globals.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
```

`web/src/App.tsx`:
```tsx
export default function App() {
  return <div className="p-6 text-lg">qac — scaffold OK</div>;
}
```

- [ ] **Step 15.11: Write `src/setupTests.ts`**

`web/src/setupTests.ts`:
```ts
import '@testing-library/jest-dom/vitest';
```

- [ ] **Step 15.12: Smoke build**

```bash
yarn build
ls dist/
# Expected: index.html assets/
```

- [ ] **Step 15.13: Commit**

```bash
cd team/wpena/qac
git add web/package.json web/yarn.lock web/.yarnrc.yml web/.yarn/releases web/tsconfig.json web/tsconfig.node.json web/vite.config.ts web/tailwind.config.ts web/postcss.config.js web/.prettierrc.json web/eslint.config.js web/index.html web/src/main.tsx web/src/App.tsx web/src/styles/globals.css web/src/setupTests.ts
git commit -m "qac: web — Vite + React + TS + Tailwind + ESLint scaffold"
```

---

## Task 16: Frontend — `lib/api.ts` (TDD)

**Files:**
- Create: `team/wpena/qac/web/src/lib/api.ts`
- Create: `team/wpena/qac/web/src/lib/api.test.ts`
- Create: `team/wpena/qac/web/src/mocks/handlers.ts`
- Create: `team/wpena/qac/web/src/mocks/server.ts`

- [ ] **Step 16.1: Write MSW server + handlers (minimal — extended in task 19)**

`web/src/mocks/handlers.ts`:
```ts
import { http, HttpResponse } from 'msw';

export const handlers = [
  http.get('/api/health', () =>
    HttpResponse.json({ ok: true, version: 'test', db_path: '/tmp/test.db' }),
  ),
];
```

`web/src/mocks/server.ts`:
```ts
import { setupServer } from 'msw/node';
import { handlers } from './handlers';

export const server = setupServer(...handlers);
```

Update `web/src/setupTests.ts`:
```ts
import '@testing-library/jest-dom/vitest';
import { afterAll, afterEach, beforeAll } from 'vitest';
import { server } from './mocks/server';

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());
```

- [ ] **Step 16.2: Write failing test**

`web/src/lib/api.test.ts`:
```ts
import { describe, it, expect } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '@/mocks/server';
import { apiGet, ApiException } from './api';

describe('apiGet', () => {
  it('returns the parsed body on success', async () => {
    server.use(
      http.get('/api/test', () => HttpResponse.json({ value: 42 })),
    );
    const result = await apiGet<{ value: number }>('/test');
    expect(result.value).toBe(42);
  });

  it('throws ApiException with code and message on 404 envelope', async () => {
    server.use(
      http.get('/api/missing', () =>
        HttpResponse.json({ error: { code: 'not_found', message: 'Run not found' } }, { status: 404 }),
      ),
    );
    await expect(apiGet('/missing')).rejects.toMatchObject({
      code: 'not_found',
      message: 'Run not found',
    });
    await expect(apiGet('/missing')).rejects.toBeInstanceOf(ApiException);
  });

  it('throws ApiException with internal code on 500 without envelope', async () => {
    server.use(
      http.get('/api/broken', () => new HttpResponse(null, { status: 500 })),
    );
    await expect(apiGet('/broken')).rejects.toMatchObject({
      code: 'internal',
    });
  });
});
```

- [ ] **Step 16.3: Run — expect compile failure**

```bash
cd team/wpena/qac/web
yarn test --run
```

Expected: cannot find module `./api`.

- [ ] **Step 16.4: Write `api.ts`**

`web/src/lib/api.ts`:
```ts
export type ApiError = { code: string; message: string };

export class ApiException extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = 'ApiException';
  }
}

export async function apiGet<T>(path: string): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`/api${path}`, { headers: { Accept: 'application/json' } });
  } catch (e) {
    throw new ApiException('internal', e instanceof Error ? e.message : 'Network error');
  }

  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    // Response wasn't JSON; fall through to error handling.
  }

  if (!res.ok) {
    const err = (body as { error?: ApiError } | null)?.error;
    throw new ApiException(err?.code ?? 'internal', err?.message ?? 'Request failed');
  }
  return body as T;
}
```

- [ ] **Step 16.5: Run — expect PASS**

```bash
yarn test --run
```

Expected: all 3 tests PASS.

- [ ] **Step 16.6: Commit**

```bash
cd team/wpena/qac
git add web/src/lib/ web/src/mocks/ web/src/setupTests.ts
git commit -m "qac: web — lib/api with ApiException + MSW setup"
```

---

## Task 17: Frontend — Badge component

**Files:**
- Create: `team/wpena/qac/web/src/components/Badge.tsx`

- [ ] **Step 17.1: Write `Badge.tsx`**

`web/src/components/Badge.tsx`:
```tsx
import { cva, type VariantProps } from 'class-variance-authority';
import type { ReactNode } from 'react';

const badge = cva('inline-flex items-center rounded px-2 py-0.5 text-xs font-medium', {
  variants: {
    tone: {
      neutral: 'bg-zinc-100 text-zinc-700',
      info: 'bg-blue-100 text-blue-700',
      warn: 'bg-amber-100 text-amber-800',
      danger: 'bg-red-100 text-red-700',
      success: 'bg-green-100 text-green-700',
    },
  },
  defaultVariants: { tone: 'neutral' },
});

export type BadgeProps = VariantProps<typeof badge> & { children: ReactNode };

export function Badge({ tone, children }: BadgeProps) {
  return <span className={badge({ tone })}>{children}</span>;
}
```

- [ ] **Step 17.2: Compile check**

```bash
cd team/wpena/qac/web
yarn tsc --noEmit
```

Expected: no errors.

- [ ] **Step 17.3: Commit**

```bash
cd team/wpena/qac
git add web/src/components/Badge.tsx
git commit -m "qac: web — Badge component with CVA tone variants"
```

---

## Task 18: Frontend — runs types + statusToTone

**Files:**
- Create: `team/wpena/qac/web/src/features/runs/runs.types.ts`
- Create: `team/wpena/qac/web/src/features/runs/statusToTone.ts`

- [ ] **Step 18.1: Write types**

`web/src/features/runs/runs.types.ts`:
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
  template_id: string;
  created_at: string;
  updated_at: string;
  status: RunStatus;
  rack: string;
  created_by: string;
};
```

- [ ] **Step 18.2: Write `statusToTone`**

`web/src/features/runs/statusToTone.ts`:
```ts
import type { RunStatus } from './runs.types';
import type { ComponentProps } from 'react';
import type { Badge } from '@/components/Badge';

type BadgeTone = NonNullable<ComponentProps<typeof Badge>['tone']>;

export function statusToTone(status: RunStatus): BadgeTone {
  switch (status) {
    case 'pending':
      return 'neutral';
    case 'running':
      return 'info';
    case 'passed':
      return 'success';
    case 'warning':
      return 'warn';
    case 'failed':
      return 'danger';
    case 'signed_off':
      return 'neutral';
  }
}
```

- [ ] **Step 18.3: Compile check**

```bash
yarn tsc --noEmit
```

Expected: no errors.

- [ ] **Step 18.4: Commit**

```bash
cd team/wpena/qac
git add web/src/features/runs/runs.types.ts web/src/features/runs/statusToTone.ts
git commit -m "qac: web — runs types + statusToTone mapping"
```

---

## Task 19: Frontend — `useRuns` hook + MSW handlers for runs

**Files:**
- Create: `team/wpena/qac/web/src/features/runs/useRuns.ts`
- Modify: `team/wpena/qac/web/src/mocks/handlers.ts`

- [ ] **Step 19.1: Extend MSW handlers**

Replace `web/src/mocks/handlers.ts`:
```ts
import { http, HttpResponse } from 'msw';
import type { Run } from '@/features/runs/runs.types';

export const fixtureRuns: Run[] = [
  {
    id: '01HZX0000000000000000000A',
    template_id: 'fw-release-canary',
    created_at: '2026-05-27T15:04:05Z',
    updated_at: '2026-05-27T15:04:05Z',
    status: 'pending',
    rack: 'as1-r07',
    created_by: 'wpena',
  },
  {
    id: '01HZX0000000000000000000B',
    template_id: 'fw-release-canary',
    created_at: '2026-05-27T14:00:00Z',
    updated_at: '2026-05-27T14:30:00Z',
    status: 'passed',
    rack: 'as2-r11',
    created_by: 'wpena',
  },
  {
    id: '01HZX0000000000000000000C',
    template_id: 'fw-release-canary',
    created_at: '2026-05-27T13:00:00Z',
    updated_at: '2026-05-27T13:15:00Z',
    status: 'failed',
    rack: 'as1-r07',
    created_by: 'wpena',
  },
];

export const handlers = [
  http.get('/api/health', () =>
    HttpResponse.json({ ok: true, version: 'test', db_path: '/tmp/test.db' }),
  ),
  http.get('/api/runs', () => HttpResponse.json({ runs: fixtureRuns })),
];
```

- [ ] **Step 19.2: Write `useRuns.ts`**

`web/src/features/runs/useRuns.ts`:
```ts
import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import type { Run } from './runs.types';

export function useRuns() {
  return useQuery({
    queryKey: ['runs'],
    queryFn: () => apiGet<{ runs: Run[] }>('/runs').then((r) => r.runs),
    staleTime: 5_000,
  });
}
```

- [ ] **Step 19.3: Compile check**

```bash
yarn tsc --noEmit
```

- [ ] **Step 19.4: Commit**

```bash
cd team/wpena/qac
git add web/src/features/runs/useRuns.ts web/src/mocks/handlers.ts
git commit -m "qac: web — useRuns hook + MSW fixtures"
```

---

## Task 20: Frontend — `RunsList` page (TDD)

**Files:**
- Create: `team/wpena/qac/web/src/features/runs/RunsList.tsx`
- Create: `team/wpena/qac/web/src/features/runs/RunsList.test.tsx`

- [ ] **Step 20.1: Write failing test**

`web/src/features/runs/RunsList.test.tsx`:
```tsx
import { describe, it, expect } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { http, HttpResponse } from 'msw';
import { server } from '@/mocks/server';
import { RunsList } from './RunsList';

function renderWithProviders(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('RunsList', () => {
  it('renders rows for each run from the API', async () => {
    renderWithProviders(<RunsList />);

    const r07 = await screen.findAllByText('as1-r07');
    expect(r07.length).toBeGreaterThanOrEqual(1);
    expect(await screen.findByText('as2-r11')).toBeInTheDocument();
  });

  it('filters by rack', async () => {
    const user = userEvent.setup();
    renderWithProviders(<RunsList />);

    // Wait for the table to render
    await screen.findByText('as2-r11');

    const filter = screen.getByPlaceholderText(/filter by rack/i);
    await user.type(filter, 'r11');

    expect(screen.queryByText('as1-r07')).not.toBeInTheDocument();
    expect(screen.getByText('as2-r11')).toBeInTheDocument();
  });

  it('shows the empty state when there are no runs', async () => {
    server.use(http.get('/api/runs', () => HttpResponse.json({ runs: [] })));
    renderWithProviders(<RunsList />);
    expect(await screen.findByText(/No runs yet/i)).toBeInTheDocument();
  });

  it('shows the error state when the API fails', async () => {
    server.use(
      http.get('/api/runs', () =>
        HttpResponse.json({ error: { code: 'internal', message: 'Failed to list runs' } }, { status: 500 }),
      ),
    );
    renderWithProviders(<RunsList />);
    expect(await screen.findByText(/Failed to list runs/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 20.2: Run — expect compile failure**

```bash
yarn test --run RunsList
```

Expected: `Cannot find module './RunsList'`.

- [ ] **Step 20.3: Write `RunsList.tsx`**

`web/src/features/runs/RunsList.tsx`:
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
import type { Run } from './runs.types';

const columnHelper = createColumnHelper<Run>();

const columns = [
  columnHelper.accessor('id', {
    header: 'Id',
    cell: (info) => (
      <Link to={`/runs/${info.getValue()}`} className="font-mono text-blue-700 hover:underline">
        {info.getValue()}
      </Link>
    ),
  }),
  columnHelper.accessor('rack', { header: 'Rack' }),
  columnHelper.accessor('status', {
    header: 'Status',
    cell: (info) => <Badge tone={statusToTone(info.getValue())}>{info.getValue()}</Badge>,
  }),
  columnHelper.accessor('template_id', { header: 'Template' }),
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

  if (!data || data.length === 0) {
    return <p className="text-zinc-500">No runs yet. Create one to get started.</p>;
  }

  return (
    <div className="space-y-3">
      <input
        type="text"
        placeholder="Filter by rack"
        value={rackFilter}
        onChange={(e) => setRackFilter(e.target.value)}
        className="w-64 rounded border border-zinc-300 px-3 py-1 text-sm"
      />
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
                  {h.column.getIsSorted() === 'asc' ? ' ↑' : h.column.getIsSorted() === 'desc' ? ' ↓' : ''}
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
    </div>
  );
}
```

- [ ] **Step 20.4: Run — expect PASS**

```bash
yarn test --run RunsList
```

Expected: all 4 tests PASS.

- [ ] **Step 20.5: Commit**

```bash
cd team/wpena/qac
git add web/src/features/runs/RunsList.tsx web/src/features/runs/RunsList.test.tsx
git commit -m "qac: web — RunsList page with filter, sort, empty + error states"
```

---

## Task 21: Frontend — wire up `App.tsx`

**Files:**
- Modify: `team/wpena/qac/web/src/App.tsx`

- [ ] **Step 21.1: Replace `App.tsx`**

`web/src/App.tsx`:
```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { Toaster } from 'sonner';
import { RunsList } from '@/features/runs/RunsList';

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
        <h1 className="mb-4 text-2xl font-semibold">Run detail</h1>
        <p className="text-zinc-500">Run detail view ships in the next iteration.</p>
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

- [ ] **Step 21.2: Build the SPA**

```bash
cd team/wpena/qac/web
yarn build
```

Expected: `dist/index.html` and `dist/assets/*.js` exist.

- [ ] **Step 21.3: Commit**

```bash
cd team/wpena/qac
git add web/src/App.tsx
git commit -m "qac: web — App.tsx wires Router + QueryClient + Toaster"
```

---

## Task 22: Makefile

**Files:**
- Create: `team/wpena/qac/Makefile`

- [ ] **Step 22.1: Write `Makefile`**

`Makefile`:
```make
.PHONY: ui backend build dev test lint fmt clean

ui:
	cd web && yarn install --immutable && yarn build

backend:
	mkdir -p ../bin
	go build -o ../bin/qac ./cmd/qac

build: ui backend

dev:
	@echo "Go on :8080, Vite on :5173 (proxies /api → :8080)"
	@bash -c 'trap "kill 0" EXIT; \
	  go run ./cmd/qac serve --addr 127.0.0.1:8080 & \
	  (cd web && yarn dev) & \
	  wait'

test:
	go test ./...
	cd web && yarn test --run

lint:
	go vet ./...
	cd web && yarn lint

fmt:
	go fmt ./...
	cd web && yarn format

clean:
	rm -rf web/dist web/node_modules ../bin/qac
```

- [ ] **Step 22.2: Smoke `make build`**

```bash
cd team/wpena/qac
make build
```

Expected: `../bin/qac` exists.

- [ ] **Step 22.3: Smoke `make test`**

```bash
make test
```

Expected: all Go tests + all Vitest tests PASS.

- [ ] **Step 22.4: Commit**

```bash
git add Makefile
git commit -m "qac: Makefile — ui/backend/build/dev/test/lint/fmt/clean"
```

---

## Task 23: Acceptance verification against spec §11

This task validates each acceptance criterion from the spec.

- [ ] **Step 23.1 — Criterion 1: `make build` succeeds from a fresh clone**

```bash
cd team/wpena/qac
make clean
make build
test -x ../bin/qac && echo OK
```

Expected: `OK`.

- [ ] **Step 23.2 — Criterion 2: `./qac seed-demo` creates a demo run**

```bash
rm -f /tmp/qac-accept.db /tmp/qac-accept.db-*
../bin/qac seed-demo --db /tmp/qac-accept.db
```

Expected: prints `Seeded run 01HZX...`.

- [ ] **Step 23.3 — Criterion 3: `serve` answers `/api/runs` with the seeded run**

```bash
../bin/qac serve --addr 127.0.0.1:8080 --db /tmp/qac-accept.db &
SERVE_PID=$!
sleep 1
curl -s http://127.0.0.1:8080/api/runs | python3 -m json.tool
# Expected: { "runs": [ { "id": "01HZX...", "status": "pending", ... } ] }
kill $SERVE_PID
```

- [ ] **Step 23.4 — Criterion 4: SPA renders the seeded run, filter works, copy is compliant**

```bash
../bin/qac serve --addr 127.0.0.1:8080 --db /tmp/qac-accept.db &
SERVE_PID=$!
sleep 1
open http://127.0.0.1:8080  # macOS; use xdg-open on Linux
# Manual checks:
#  - row visible for the seeded run
#  - rack filter narrows
#  - status badge styled (neutral for "pending")
#  - column headers in sentence case
#  - no exclamation points anywhere
#  - ULID preserved verbatim (not sentence-cased)
kill $SERVE_PID
```

- [ ] **Step 23.5 — Criterion 5: `make dev` runs Vite + Go with hot reload**

```bash
make dev
# In another terminal: open http://127.0.0.1:5173
# Edit web/src/App.tsx — change "Runs" → "Runs (dev)"; confirm hot reload
# Ctrl+C to stop
```

- [ ] **Step 23.6 — Criterion 6: `make test` passes**

```bash
make test
```

Expected: all tests PASS, exit 0.

- [ ] **Step 23.7 — Criterion 7: `make lint` passes**

```bash
make lint
```

Expected: `go vet` clean, ESLint clean.

- [ ] **Step 23.8 — Criterion 8: README quickstart works in ≤5 commands**

Verify `README.md` has the quickstart from Task 1; walk through it from a clean state:
```bash
cd team/wpena/qac
make clean
make build
../bin/qac seed-demo
../bin/qac serve --addr 127.0.0.1:8080
```

That's 4 commands (≤5 ✓).

- [ ] **Step 23.9 — Criterion 9: no `replace` directives, no sibling-repo imports**

```bash
cd team/wpena/qac
! grep -E "^replace" go.mod
! grep -rE "coreweave/(tiphys|qac)|argo/" --include="*.go" --include="*.ts" --include="*.tsx" .
echo "Self-contained ✓"
```

Expected: prints `Self-contained ✓`.

- [ ] **Step 23.10: Commit acceptance verification log if desired (optional)**

If anything failed in 23.1–23.9, fix it before the next step. When all 9 are green, the walking skeleton is **done**.

---

## Final commit + branch

- [ ] **Step F.1: Cleanup**

```bash
rm -f /tmp/qac-accept.db /tmp/qac-accept.db-* /tmp/qac-smoke.db /tmp/qac-smoke.db-* /tmp/qac-demo.db /tmp/qac-demo.db-*
```

- [ ] **Step F.2: Final smoke run + screenshot for the PR**

```bash
cd team/wpena/qac
make build
../bin/qac seed-demo
../bin/qac serve --addr 127.0.0.1:8080 &
sleep 1
# Take a screenshot of http://127.0.0.1:8080 for the PR description
kill $!
```

- [ ] **Step F.3: Open PR**

```bash
cd /Users/wpena/coreweave/fleet-ops-sandbox
git push -u origin wp/add-qac
gh pr create --title "qac: walking-skeleton rebuild (self-contained Go + React)" --body "$(cat <<'EOF'
## Summary
- Self-contained Go binary embedding a React SPA; event-sourced SQLite store
- Runs-list page as the spine slice; one feature touches every architectural layer truthfully
- Stack aligns with cloud-console frontend conventions (React+TS+Vite+Tailwind+Radix+CVA+TanStack Table/Query+react-hook-form+zod+sonner)
- See `docs/superpowers/specs/2026-05-27-qac-rebuild-design.md` and `docs/superpowers/plans/2026-05-27-qac-walking-skeleton.md`

## Test plan
- [x] `make build` succeeds from clean
- [x] `seed-demo` creates a run, `serve` returns it via `/api/runs`
- [x] Browser renders runs list, rack filter works, copy passes the cloud-console copy guide
- [x] `make test` (Go + Vitest) passes
- [x] `make lint` (`go vet` + ESLint) passes
- [x] No `replace` directives, no sibling-repo imports

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Plan self-review

**1. Spec coverage:**
- §1 Purpose, §2 Constraints — addressed by the self-contained build (Tasks 1, 13.1, 23.9).
- §3 Stack — Tasks 2, 15 install/pin the exact deps.
- §4 Architecture + directory layout — Tasks 3–13 build the Go side; Tasks 15–21 build the web side.
- §5 Data model — Task 3 (migrations).
- §6 Engine + projection — Tasks 6, 7.
- §7 API surface — Tasks 8 (errors), 9 (health), 10 (list), 11 (detail).
- §8 Frontend feature shape — Tasks 16 (api), 17 (Badge), 18 (types), 19 (useRuns), 20 (RunsList), 21 (App).
- §9 Testing — every TDD task; coverage on store, engine, API, lib/api, RunsList.
- §10 Build pipeline — Task 22.
- §11 Acceptance criteria — Task 23 walks each criterion.
- §13 Forward note — not implemented by code; carried in the spec and memory.

**2. Placeholder scan:** No "TBD" / "TODO" / "fill in details" / "Similar to Task N" left in the plan. The "implemented in next task" stub in Task 13.4 is intentional (lets `main.go` compile before Task 14 lands the real implementation) and is replaced by Task 14.

**3. Type consistency:**
- `Store.AppendEvent(ctx, runID, kind string, payload []byte)` (Task 4) — used consistently in Tasks 5, 9, 10, 11, 13, 14.
- `engine.Project([]Event) RunState` (Task 7) — declared in §6 of spec; not consumed in the skeleton (detail handler reads runs projection from store, not the fold). Documented in spec §5 ("List = scan; detail = fold") — actual fold usage arrives with the run-detail page (out of scope).
- `Run` shape: Go `store.Run` (Task 5) and TS `Run` (Task 18) match field names/types via JSON tags.
- `ApiException` (Task 16) used consistently in `RunsList.test.tsx` (Task 20) via `apiGet`.

Plan is internally consistent.

---

## Notes for the Go beginner

Read these in order as the implementation hits them:

1. **Task 1, 2** — Modules & packages. `go.mod` is the manifest; `go get` adds dependencies.
2. **Task 3** — `database/sql`, drivers, `context.Context`, error wrapping (`%w`), `defer`.
3. **Task 4** — Transactions, `defer tx.Rollback()` pattern.
4. **Task 5** — `sql.Rows` iteration, `time.Unix` for epoch ↔ `time.Time`.
5. **Task 6** — `const` blocks, struct tags (`json:"..."`).
6. **Task 7** — Pure functions, free functions vs methods, switch-on-string.
7. **Task 8, 9** — `http.ResponseWriter`, `http.HandlerFunc`, `httptest`.
8. **Task 9** — Go 1.22 ServeMux patterns, `runtime/debug.ReadBuildInfo`.
9. **Task 11** — `r.PathValue("id")` for URL path params.
10. **Task 12** — `embed.FS`, `fs.Sub`, `//go:embed` constraints (no `..`).
11. **Task 13** — cobra subcommands, graceful shutdown (`signal.Notify`, `srv.Shutdown`).
12. **Task 14** — ULIDs.
