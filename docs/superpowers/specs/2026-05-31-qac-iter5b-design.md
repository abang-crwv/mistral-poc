# qac iter-5b — per-CT enumeration and ignore foundation

**Status:** draft
**Branch:** `wp/add-qac` (continues iter-5a)
**Predecessor:** [iter-5a spec](2026-05-30-qac-iter5a-design.md), iter-5a HEAD at `d991d3a`
**PR posture:** still local. iter-5b is foundational schema + events + projection work; no UI changes, no probe changes, no engine changes.

## 1. Purpose

iter-5a taught qac to read the upstream RLCC/FLCC chart workflows. iter-5b teaches it that **a rack is not a single point of observation** — it contains compute trays (BMNs / CTs) that progress through FLCC workflows independently, fail independently, and can be operator-ignored or RLCC-ignored. Without this layer qac can only ever produce rack-level pass/fail verdicts; the canary's actual job is to surface "17 of 18 CTs reached l10-test-loop; CT-7 stuck in fielddiag."

After iter-5b ships:

- Run-create enumerates the BMNs in each canary rack and persists them per (run, rack, deviceslot)
- Operator-supplied `ignored_bmns` lands as `BMNIgnored` events at run-create
- RLCC-ignored BMNs (`baremetal_node_rlcc_provisioning_ignored == 1`) are auto-detected at run-create and recorded as `BMNIgnored(source=rlcc)`
- `evidence.deviceslot` exists as a nullable column ready for iter-5e's `rlcc_action_probe` to populate
- `GET /api/runs/{id}` response carries `ct_count` per rack so the UI can render "36 CTs across 2 racks"

No wizard rewrite (iter-5c). No template materialization (iter-5d). No `rlcc_action_probe` (iter-5e). iter-5b is purely the foundation those iters build on.

## 2. Scope decisions

| Decision | Value | Why |
|---|---|---|
| BMN enumeration source | **Inventory (via extended `inventoryclient.Resolver`)** | Inventory is the canonical "what BMNs should be in this rack" record. PromQL only sees BMNs currently emitting metrics — a BMN that should exist but is silent is exactly the case the canary needs to flag, not hide. |
| `BMNDiscovered` cardinality | **One event per rack, payload carries `bmns: []` slice** | Matches `FactsDiscovered`'s per-rack cardinality. 36 BMN × N rack events would dwarf the rest of the event log. |
| Composite ignore key | **`(run_id, deviceslot, source)`** | A CT can carry operator + RLCC ignores simultaneously. Removing one source doesn't un-ignore it if the other still holds. |
| RLCC ignore detection at run-create | **Yes, via PromQL** | `baremetal_node_rlcc_provisioning_ignored{rack="<r>"} == 1` — surfaced in iter-5c's wizard preview so operators see auto-ignored CTs before submit. |
| RLCC ignore detection mid-run | **Deferred to iter-5e** | iter-5b only catches RLCC ignores at run-create. iter-5e's `rlcc_action_probe` re-checks per tick and emits `BMNIgnored(source=rlcc)` for newly-ignored CTs. |
| `ignored_bmns` wire shape | **Top-level on `POST /api/runs` body** | Operator-ignore is a canary-run concern, not a template-input. Keeping it out of `inputs` means template schemas don't have to know about it. |
| Reason field | **Free-text, optional, on `BMNIgnored` payload** | Operator-supplied ignores have a `reason: string` for the timeline ("known bad GPU"); RLCC-source ignores leave it nil. |
| PromClient surface | **`internal/lifecycleclient/` interface + MapClient + minimal PromClient for the one query** | Interface lands in iter-5b so the run-create handler can call it; full lifecycle polling impl is iter-5e. The MapClient is the test backend; the PromClient is a thin VM client port good enough for the ignore query. |
| Auth surface | **`AWXCTL_VMAUTH_USERNAME` + `AWXCTL_VMAUTH_PASSWORD` from env, Doppler-sourced** | Matches the iter-5a `AWXCTL_SOURCEGRAPH_TOKEN` pattern. Empty env → MapClient fallback with warning. |
| `evidence.deviceslot` column | **Nullable TEXT, idempotent `addColumnIfMissing` migration** | Lands now so iter-5e can write to it without a separate migration. Existing iter-4b rows scan as NULL. |
| `rack_deviceslots` projection table | **New table, `CREATE TABLE IF NOT EXISTS`** | Projection target for `BMNDiscovered`. Mirror of the `facts` table pattern. Rebuildable from events. |
| `bmn_ignores` projection table | **New table, composite PK** | Projection target for `BMNIgnored`. Carries `source` and `reason`. |
| Partial index on `evidence.deviceslot` | **Deferred to iter-5e** | No queries hit it in iter-5b. Land alongside the first writes that populate the column. |
| Wizard preview endpoint (`/api/inventory/preview`) | **Land in iter-5b** | Used by iter-5c's wizard. Returns rack-level info plus deviceslot list plus RLCC-ignored deviceslot list. Backend-only in iter-5b; UI consumption is iter-5c. |
| Frontend changes | **None** | iter-5c handles wizard rewrite. iter-5b is backend-only. |
| Engine changes | **None** | Engine doesn't read `bmn_ignores` until iter-5e. |

## 3. Discovery pipeline (extended)

The iter-4a discovery pipeline in `createRunHandler` runs `operator facts → ResolveRack → per-rack FactsDiscovered → engine.Kick`. iter-5b inserts a per-CT phase between FactsDiscovered and Kick. The whole window stays all-or-nothing: if any phase fails, the run is not created.

```
createRunHandler:
  step 1  operator facts
  step 2  for each canary rack: inventoryclient.ResolveRack → RackFacts
  step 3  emit FactsDiscovered(scope="rack:<r>") per rack       [iter-4a]
  step 4  for each canary rack:
          (a) inventoryclient.ResolveBMNs(rack) → []BMN
              emit BMNDiscovered(rack, bmns:[...])              [NEW]
          (b) lifecycleclient.QueryRLCCIgnored(rack) → []deviceslot
              for each ignored deviceslot:
                emit BMNIgnored(rack, deviceslot, source="rlcc") [NEW]
  step 5  for each operator-supplied ignore:
              emit BMNIgnored(rack, deviceslot, source="operator", reason) [NEW]
  step 6  engine.Kick                                            [iter-4a]
```

If `ResolveBMNs` fails for any rack: emit `BMNDiscoveryFailed(rack, source="inventory")` and return 422 with `error_code: "bmns_unresolved"`. No `RunCreated`.

If `QueryRLCCIgnored` fails for any rack: log a warning, treat as "no auto-ignores for this rack," continue. RLCC ignore detection is best-effort at run-create; operators can manually mark BMNs in iter-5c's wizard. The metric being unavailable should not fail run creation.

Operator-supplied `ignored_bmns` is validated against the just-enumerated BMNs — if an operator specifies a deviceslot that isn't in the rack, return 400 with `error_code: "invalid_request"`. This catches typos before they become silent canary gaps.

## 4. New + extended types

### 4.1 Inventory client extension

```go
// internal/inventoryclient/inventoryclient.go — extends the existing Resolver
type Resolver interface {
    Lookup(ctx context.Context, id string) ([]Hit, error)
    ResolveBMNs(ctx context.Context, rackID string) ([]BMN, error) // NEW
}

// internal/inventoryclient/bmn.go (new)
type BMN struct {
    Deviceslot string `json:"deviceslot"`           // canonical key: dh1-r037-node-07-us-east-04b
    BMNName    string `json:"bmn_name"`             // short identifier: s90txs64
    Rack       string `json:"rack"`                 // parent rack id
    Zone       string `json:"zone"`                 // VM region for downstream PromQL
    CTPosition int    `json:"ct_position,omitempty"`// derived from deviceslot suffix
}
```

`MapResolver` gains a `BMNs map[string][]BMN` keyed by rack id; `SeedDemoFixtures()` populates ~18 BMNs per demo rack so the integ test exercises a realistic count.

### 4.2 Lifecycle client (foundation)

```go
// internal/lifecycleclient/types.go
type RackKey struct {
    Rack string
    Zone string
}

type IgnoredBMN struct {
    Deviceslot string
    BMNName    string
}

// internal/lifecycleclient/client.go
type Client interface {
    QueryRLCCIgnored(ctx context.Context, rack RackKey) ([]IgnoredBMN, error)
    // PollLifecycle and ListBMNsInRack land in iter-5e
}

var ErrAuth = errors.New("AWXCTL_VMAUTH credentials missing or rejected")

// internal/lifecycleclient/mapclient.go
type MapClient struct {
    Ignored map[RackKey][]IgnoredBMN
}
func NewMapClient() *MapClient { ... }

// internal/lifecycleclient/promclient.go
type PromClient struct {
    httpDo  func(*http.Request) (*http.Response, error)
    auth    string // basic auth header value
    baseURL string
}
func NewPromClient() (*PromClient, error)  // reads AWXCTL_VMAUTH_USERNAME/PASSWORD
```

The PromClient in iter-5b implements only `QueryRLCCIgnored` — one PromQL query, one VM endpoint hit. The full lifecycle polling surface (5 queries, soft-fail fanout) lands in iter-5e. Both clients satisfy the same `Client` interface; iter-5e extends the interface, not the impl.

`internal/lifecycleclient/queries/rlcc_ignored.promql` is embedded via `//go:embed`:

```promql
baremetal_node_rlcc_provisioning_ignored{rack="{rack}"} == 1
```

`{rack}` is the only token, substituted via the dumb `strings.ReplaceAll` pattern ported from tiphys/inventory/signal.go.

### 4.3 Event kinds

`BMNDiscovered` — one per rack at run-create:

```go
type bmnDiscoveredPayload struct {
    Rack         string    `json:"rack"`
    Zone         string    `json:"zone"`
    Source       string    `json:"source"`        // "inventory"
    DiscoveredAt time.Time `json:"discovered_at"`
    BMNs         []BMN     `json:"bmns"`          // inventoryclient.BMN slice
}
```

Projection writes one row per BMN into `rack_deviceslots` in the same transaction.

`BMNDiscoveryFailed` — emitted when inventory enumeration fails for a rack:

```go
type bmnDiscoveryFailedPayload struct {
    Rack    string `json:"rack"`
    Source  string `json:"source"`   // "inventory"
    Message string `json:"message"`
}
```

`BMNIgnored` — fires at run-create for both operator and RLCC sources:

```go
type bmnIgnoredPayload struct {
    Rack       string    `json:"rack"`
    Deviceslot string    `json:"deviceslot"`
    Source     string    `json:"source"`            // "operator" | "rlcc"
    Reason     string    `json:"reason,omitempty"`  // operator-supplied free-text; empty for rlcc
    MarkedAt   time.Time `json:"marked_at"`
}
```

Projection writes one row into `bmn_ignores` in the same transaction. The composite PK `(run_id, deviceslot, source)` means re-emitting the same ignore (same source) is an UPSERT — last write wins on `reason`/`marked_at`.

### 4.4 Request body extension

`POST /api/runs` accepts an optional top-level `ignored_bmns`:

```json
{
  "template_id": "firmware-release-canary",
  "inputs": { ... },
  "ignored_bmns": [
    {"rack": "dh1-r037-us-east-04b", "deviceslot": "dh1-r037-node-07-us-east-04b", "reason": "known bad GPU"}
  ]
}
```

(iter-5b keeps the existing `fw-release-canary` template id; the `source_sha` field for pinning RLCC chart snapshots arrives in iter-5d's materialization. The `ignored_bmns` field is orthogonal to which template runs.)

`ignored_bmns` is an array, possibly empty, possibly absent. Each entry requires `rack` and `deviceslot`; `reason` is optional. Validation rules:

- `rack` must appear in `inputs.canary_racks`
- `deviceslot` must appear in the rack's enumerated BMN list (caught after step 4a runs)
- Duplicate (rack, deviceslot) entries fail with `error_code: "invalid_request"` — operator should not double-ignore the same CT

## 5. Schema changes

Migrations land via the idempotent `addColumnIfMissing` / `CREATE TABLE IF NOT EXISTS` patterns already established in `internal/store/migrations.go`.

### 5.1 `evidence.deviceslot` nullable column

```go
// internal/store/migrations.go
func (s *Store) addDeviceslotColumnIfMissing(ctx context.Context) error {
    // SELECT 1 FROM pragma_table_info('evidence') WHERE name='deviceslot';
    // if absent: ALTER TABLE evidence ADD COLUMN deviceslot TEXT
}
```

Called from `migrate()` after `addInputsColumnIfMissing`. Pre-iter5 rows scan as NULL. `Evidence` struct in `internal/store/evidence.go` gains:

```go
type Evidence struct {
    // ...
    Deviceslot *string  // nullable; iter-5e populates, iter-5b reads as nil for existing rows
}
```

`PutEvidence` INSERT grows from 6 to 7 columns; `GetEvidence` SELECT scans into `sql.NullString` then assigns to the pointer. Existing call sites in `internal/probe/alertprobe` and `internal/probe/factprobe` pass nil — iter-5b does not change probe behavior.

### 5.2 `rack_deviceslots` projection table

```sql
CREATE TABLE IF NOT EXISTS rack_deviceslots (
  run_id        TEXT NOT NULL,
  rack          TEXT NOT NULL,
  deviceslot    TEXT NOT NULL,
  bmn_name      TEXT NOT NULL,
  zone          TEXT NOT NULL,
  ct_position   INTEGER,
  discovered_at TEXT NOT NULL,
  PRIMARY KEY (run_id, rack, deviceslot)
);
```

UPSERT'd by `internal/engine/project.go` on `BMNDiscovered`. `ON CONFLICT (run_id, rack, deviceslot) DO UPDATE SET discovered_at = excluded.discovered_at, bmn_name = excluded.bmn_name` — idempotent in case of replay.

### 5.3 `bmn_ignores` projection table

```sql
CREATE TABLE IF NOT EXISTS bmn_ignores (
  run_id     TEXT NOT NULL,
  rack       TEXT NOT NULL,
  deviceslot TEXT NOT NULL,
  source     TEXT NOT NULL,            -- "operator" | "rlcc"
  reason     TEXT,                     -- nullable
  marked_at  TEXT NOT NULL,
  PRIMARY KEY (run_id, deviceslot, source)
);
```

Composite PK `(run_id, deviceslot, source)` lets the same CT carry operator + RLCC ignores simultaneously. `rack` is denormalized for query convenience (`SELECT * FROM bmn_ignores WHERE run_id = ? AND rack = ?`).

### 5.4 Events table

No DDL change. `BMNDiscovered`, `BMNDiscoveryFailed`, `BMNIgnored` are new event kinds; `internal/store/events.go` `AppendEvent` gains three new case branches (projection writes happen in the same transaction as the event append). `internal/engine/project.go` reads them in `loadTemplateFromEvents` so the engine sees the per-CT enumeration on replay.

## 6. HTTP surface

### 6.1 `POST /api/runs` — body extension

Already documented in §4.4. No new route; same handler accepts the optional `ignored_bmns` field. Validation cases above.

### 6.2 `GET /api/runs/{id}` — response extension

Existing response gets a `racks` field with per-rack rollup including CT counts and ignore counts:

```json
{
  "run": { ... existing fields ... },
  "events": [ ... existing ... ],
  "racks": [
    {
      "rack": "dh1-r037-us-east-04b",
      "ct_count": 18,
      "ignored": [
        {"deviceslot": "dh1-r037-node-04-us-east-04b", "source": "rlcc"},
        {"deviceslot": "dh1-r037-node-07-us-east-04b", "source": "operator", "reason": "known bad GPU"}
      ]
    }
  ]
}
```

The `racks` array is derived from `rack_deviceslots` + `bmn_ignores` projections via a single SQL join per rack. Empty arrays when no BMNs were discovered yet (run created before iter-5b, or run with no canary_racks).

### 6.3 `GET /api/inventory/preview` — new route

```
GET /api/inventory/preview?racks=dh1-r037-us-east-04b,dh1-r038-us-east-04b
200 OK
{
  "racks": [
    {
      "rack": "dh1-r037-us-east-04b",
      "zone": "us-east-04b",
      "instance_type": "gb200-4x",
      "sku": "gb200-nvl72",
      "bmns": [
        {"deviceslot": "dh1-r037-node-01-us-east-04b", "bmn_name": "s90txs64", "ct_position": 1},
        ...
      ],
      "rlcc_ignored": [
        {"deviceslot": "dh1-r037-node-04-us-east-04b", "bmn_name": "s90txs67"}
      ]
    }
  ],
  "source": { "inventory_at": "2026-05-31T18:42:00Z", "vm_at": "2026-05-31T18:42:01Z" }
}
```

Called by iter-5c's wizard to render the BMN checkbox UI. Fails partially: if `inventoryclient.ResolveBMNs` fails for one rack, return 200 with that rack absent from the `racks` array and the failure listed under `errors[]`. If `lifecycleclient.QueryRLCCIgnored` fails for any rack, return the BMNs without the `rlcc_ignored` field for that rack and log the failure — wizard render does not block on the auto-detect.

This endpoint is read-only and idempotent. Cache TTL: 60 seconds per (rack, query-time) tuple. Operator hitting refresh on the wizard re-queries.

### 6.4 Stable error codes

Existing codes preserved. New code:

- `bmns_unresolved` (422) — inventory enumeration failed for a rack at run-create

## 7. Startup wiring

`cmd/qac/serve.go` gains a `lifecycleclient.Client` parallel to the iter-5a `rlccclient.Client` / `flccclient.Client`:

```go
vmUser := os.Getenv("AWXCTL_VMAUTH_USERNAME")
vmPass := os.Getenv("AWXCTL_VMAUTH_PASSWORD")
vmBackend := os.Getenv("QAC_VM_BACKEND") // "" or "map"

var lifeC lifecycleclient.Client
switch {
case vmBackend == "map":
    slog.Info("using map backend per QAC_VM_BACKEND=map")
    lifeC = lifecycleclient.NewMapClient()
case vmUser == "" || vmPass == "":
    slog.Warn("using map backend; AWXCTL_VMAUTH_USERNAME/PASSWORD unset (run source_passkeys for live PromQL)")
    lifeC = lifecycleclient.NewMapClient()
default:
    pc, err := lifecycleclient.NewPromClient()
    if err != nil {
        return fmt.Errorf("lifecycle prom client: %w", err)
    }
    lifeC = pc
    slog.Info("using prom backend for lifecycle queries")
}
```

`api.Router` grows one new arg: `lifecycle lifecycleclient.Client`. The existing `internal/server/server.New` and `internal/api/api_test.go newTestServer` adapt the same way iter-5a's Router signature change handled.

## 8. Tests

| File | Asserts |
|---|---|
| `internal/inventoryclient/bmn_test.go` (new) | ResolveBMNs returns the SeedDemoFixtures BMN list; unknown rack returns ErrNotFound; BMN positions derived correctly |
| `internal/lifecycleclient/parse_test.go` (new) | rlcc_ignored.promql template substitution; vector parsing |
| `internal/lifecycleclient/client_test.go` (new) | MapClient.QueryRLCCIgnored returns seeded ignores; PromClient.QueryRLCCIgnored hits httptest endpoint with correct query |
| `internal/store/migrations_test.go` (extend) | After migrate(): evidence.deviceslot exists; rack_deviceslots + bmn_ignores tables present; running migrate twice is idempotent (no diff) |
| `internal/store/events_test.go` (extend) | AppendEvent + projection for BMNDiscovered writes N rows to rack_deviceslots; BMNIgnored writes to bmn_ignores; replay produces same projection |
| `internal/api/runs_test.go` (extend) | POST /api/runs with operator ignored_bmns emits BMNIgnored events; ignore validation rejects unknown rack or deviceslot; GET /api/runs/{id} response includes racks[] rollup |
| `internal/api/inventory_preview_test.go` (new) | GET /api/inventory/preview returns BMN list + rlcc_ignored; partial failure path returns 200 with errors[] |
| `internal/api/canonical_integ_test.go` (extend) | After full happy-path run-create: one BMNDiscovered per resolved rack; deviceslots non-empty; if MapClient seeds RLCC ignores, one BMNIgnored per ignored CT |

`MapClient` fixtures power most tests. `httptest.NewServer` powers PromClient tests (mirror of iter-5a sourcegraph test pattern).

## 9. Risks and open questions

**Risk: BMN cardinality.** A 36-CT rack means 36 rows into `rack_deviceslots` per run-create. Multi-rack runs scale linearly. At qac's current run volume this is trivial; document the upper bound (a single Run can carry hundreds of BMN rows) so iter-5e's probe-tick fan-out doesn't surprise anyone.

**Risk: Inventory drift between preview and run-create.** A BMN appears in `/api/inventory/preview` but is removed before `POST /api/runs` runs. The validation step in §3 ("deviceslot must appear in the enumerated BMN list") catches this — but the wizard's pre-checked state will silently include the now-missing BMN. Mitigation: wizard re-fetches `/api/inventory/preview` on submit; if the enumeration changed, surface a diff modal. iter-5c spec captures this; iter-5b ships the endpoints both modes need.

**Risk: VM endpoint outage at run-create.** RLCC ignore auto-detect best-effort failing means a CT that RLCC marked as ignored gets included in the canary. Operator can still manually mark it via `ignored_bmns`. Document this in the slog warning; iter-5e probe re-detects so the failure window is bounded by the next probe tick.

**Open question:** should `ct_position` (derived from the deviceslot suffix) live on the `rack_deviceslots` table, or be computed at read time? **Decided:** store it. Tiny denormalization for fast UI sort. Computed via regex `node-(\d+)-` against the deviceslot string.

**Open question:** `inventoryclient.ResolveBMNs` — does the production resolver (port from tiphys, iter-4a) support BMN enumeration today, or only rack lookup? **Decided for iter-5b:** add the method to the interface; `MapResolver` implements it from a seeded `BMNs` map; the real-resolver implementation is deferred (still on iter-4a's MapClient until production inventory lands). If production resolver only supports id-lookup, we extend it as a separate concern in iter-6 — does not block iter-5b.

**Open question:** PromQL response shape for vector queries — `prometheus`-style `{status: "success", data: {resultType: "vector", result: [{metric: {...}, value: [ts, "1"]}]}}`. Confirmed via the iter-5a sourcegraph GraphQL pattern; the lifecycleclient parser mirrors that shape.

## 10. Out of scope

- Wizard rewrite (iter-5c)
- `/api/inventory/preview` consumption in any UI (iter-5c)
- Template materialization at pinned source_sha (iter-5d)
- `rlcc_action_probe` (iter-5e)
- Per-CT lifecycle polling (`PollLifecycle`, `ListBMNsInRack`) (iter-5e)
- Per-tick RLCC ignore re-detection (iter-5e)
- Per-CT UI surface — checkbox lists, badges, progress grids (iter-5c wizard + iter-5f run-detail)
- `BMNObserved` event kind (iter-5e)
- Partial index on `evidence(run_id, deviceslot)` (iter-5e)
- Rename to QAgenticCow (deferred; tracked separately)

## 11. Acceptance criteria

After iter-5b is merged on `wp/add-qac`:

1. `make test` and `make lint` are green
2. `go generate ./...` remains idempotent
3. `qac serve` starts cleanly with `QAC_VM_BACKEND=map` and no VMauth env vars; log line confirms map backend
4. `qac serve` with `AWXCTL_VMAUTH_USERNAME` and `AWXCTL_VMAUTH_PASSWORD` set hits a configured VM endpoint at run-create; log line confirms prom backend
5. `POST /api/runs` with two canary racks and a 1-entry `ignored_bmns` array emits: 2× `BMNDiscovered`, 1× `BMNIgnored(source=operator)`, plus any number of `BMNIgnored(source=rlcc)` from auto-detect — all in the same transaction as the `RunCreated` event
6. `GET /api/runs/{id}` response includes `racks[]` with `ct_count` and `ignored[]` rolled up per rack
7. `GET /api/inventory/preview?racks=...` returns rack-level info plus BMN list plus RLCC-ignored list; partial failure returns 200 with `errors[]`
8. `POST /api/runs` with an `ignored_bmns` entry referencing a deviceslot not in the enumerated BMN list returns 400 with `error_code: "invalid_request"` and a clear message identifying the bad deviceslot
9. Replaying events for an existing run rebuilds the `rack_deviceslots` and `bmn_ignores` projections to bit-identical state — verified by an integ test that wipes both tables, calls a projection-rebuild helper, and asserts equality
10. No regressions: all iter-4c, iter-5a tests pass unchanged

## 12. Forward references

| iter | Adds |
|---|---|
| **5c** | Wizard rewrite. `NewRunModal` dropdown driven by `/api/rlcc/workflows`; `newRunSchema.ts` accepts `{template_id, source_sha, inputs, ignored_bmns}`. New BMN checkbox UI consumes `/api/inventory/preview`. MSW handlers for both new endpoints. |
| **5d** | `createRunHandler` materialization at pinned `source_sha`; splice RLCC spine into transient Template; embed in `RunCreated.payload`. `loadTemplateFromEvents` reads from event payload. `templates/firmware-release-canary.yaml` retires or downgrades. |
| **5e** | `rlcc_action_probe`. `lifecycleclient.PollLifecycle` + `ListBMNsInRack` impls. Per-tick PromQL fanout (5 queries, soft-fail). Per-CT evidence rows with `deviceslot` populated. Per-tick RLCC ignore re-detection emits new `BMNIgnored(source=rlcc)` events mid-run. `BMNObserved` event kind. Verdict math respects ignored set. Partial index `idx_evidence_run_slot ON evidence(run_id, deviceslot) WHERE deviceslot IS NOT NULL`. |
| **5f** | Per-CT UI surface. Run-detail renders "17 of 18 CTs reached l10-test-loop; CT-7 stuck in fielddiag" grid. Per-CT badges: "ignored by operator", "ignored by RLCC", "ignored by both". Timeline entries for `BMNIgnored` events. |
| **6+** | Real production inventory resolver. `workflowOverrides` resolution. RLCC `triggers:` block. Dispatch capability. Rename to QAgenticCow.
