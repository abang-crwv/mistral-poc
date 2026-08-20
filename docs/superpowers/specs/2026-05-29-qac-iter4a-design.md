# qac iteration 4a — design

**Date:** 2026-05-29
**Author:** wpena (with Claude Code)
**Branch:** wp/add-qac
**Predecessor specs:** iter-1 (`2026-05-27-qac-rebuild-design.md`), iter-2 (`2026-05-28-qac-run-detail-design.md`), iter-3a (`2026-05-28-qac-iter3a-design.md`).

---

## 1. Purpose

Lock down the run metadata model before downstream iterations introduce agentic workflows (engine-driven step dispatch, AI summarization, evaluator agents). The metadata schema is iter-4a's product; the inventory port and fact discovery are its first producers.

Every future consumer of qac run state — engine, action handlers, checkpoint evaluator, AI assess, Slack drafter, dashboard widgets — will read against the contract this iteration establishes. Locking the shape now means later iterations add producers and consumers without churning the schema.

## 2. Scope of iter-4

iter-4 is decomposed into three sub-iterations:

| Sub | Slice | "Verifies something"? | PR? |
|---|---|---|---|
| **iter-4a** *(this spec)* | Metadata model + inventory port + fact discovery. Operator inputs are promoted to facts; per-rack inventory facts are discovered at run create. Hard-fail on unresolved rack. | No — no probe execution yet. | Branch held. |
| **iter-4b** | Engine + first stubbed probe. `engine.Advance`, action handler interface, checkpoint evaluator, `baseline_pre` against a deterministic stub. Sync dispatch. New event kinds for step lifecycle. | **Yes** — the canary verifies one thing end-to-end. | **PR-opens.** |
| **iter-4c** | Async probe execution (goroutines). SSE on `/api/runs/{id}/events`. Per-node evidence table in frontend. Operator-declared node excludes. | Yes, with live updates and per-node detail. | Optional second PR. |

This spec covers iter-4a only.

## 3. Scope of iter-4a

**In scope:**
- New `internal/facts/` package: typed `Key` constants, registered-key validation, `Emit` helper.
- New `internal/inventoryclient/` package: ported `Resolver` interface, `Hit` struct, `ErrNotFound`/`AmbiguousError`. `MapResolver` stub backend. `ResolveRack` wrapper extracting `RackFacts` from a Hit's `Labels`.
- New `facts` table + migrations following the existing iter-3a pattern (`CREATE IF NOT EXISTS`, idempotent).
- `POST /api/runs` evolves to: append `RunCreated` → promote operator inputs to facts → resolve each rack via inventory → emit per-rack `FactsDiscovered` → return 201. All-or-nothing semantics on inventory: if any rack fails to resolve, no `FactsDiscovered` events land at all and the response is 422.
- New `GET /api/runs/{id}/facts` endpoint with `scope` and `source` query filters.
- `templates/firmware-release-canary.yaml` edit: drop `instance_type`, `variant`, `nitwit_receipts`, `slack_thread` from `inputs[]`. `requester` stays as `required: false` (iter-3b deferred).
- New event kinds: `FactsDiscovered` (on operator-promotion and per-rack inventory success), `FactsDiscoveryFailed` (on the inventory hard-fail path — its projection sets runs.status="failed").
- Wiring in `cmd/qac/serve.go` to inject `inventoryclient.NewMapResolver(inventoryclient.SeedDemoFixtures())` into the server.
- Tests: ~35 new Go subtests + ~5 updated existing subtests. Target floor: 145 Go subtests + 18 Vitest.

**Out of scope** (deferred):
- Engine, action handlers, checkpoint evaluator → iter-4b.
- Real VM/Netbox-backed `Resolver` → iter-4b/c (the seam is the `Resolver` interface; implementation only).
- Per-node facts written by probes → iter-4c.
- SSE, async probe dispatch, frontend changes → iter-4c.
- Operator-declared node excludes → iter-4c+ (uses the reserved `node:<id>` scope).
- `nitwit_receipts` and `slack_thread` as facts — re-register when they have real producers.
- New top-level Go dependencies (no `go get` needed).

## 4. Metadata model

### 4.1 Scope vocabulary

Three scope values, encoded as strings on every fact row:

| Scope | Format | Meaning | iter-4a use |
|---|---|---|---|
| `run` | literal `"run"` | Canary-wide fact (operator inputs, run-level discovered metadata) | Operator promotion. |
| `rack:<id>` | `"rack:" + rack-id` | Per-rack fact (instance_type, sku, variant, region, …) | Inventory discovery. |
| `node:<id>` | `"node:" + node-id` | Per-node fact (per-BMN evidence, operator excludes) | **Reserved.** iter-4a writes nothing at this scope. iter-4c is the first writer. Registered now so the schema absorbs that work without a migration. |

The scope string is opaque to the storage layer — any string is accepted at the DB. Validation lives at the producer edge (`facts.Emit` rejects empty scopes, others are by convention).

### 4.2 Fact key registry

All facts use a key from a closed registry. Producers must use a registered key; `facts.Emit` rejects unregistered keys with `ErrUnknownFactKey`. Adding a new fact requires a code change to `internal/facts/keys.go` — deliberate friction.

```go
// internal/facts/keys.go
package facts

type Key string

const (
    // Operator-supplied (source="operator", scope="run").
    KeyBundleTag Key = "bundle_tag"
    KeyRequester Key = "requester"

    // Inventory-discovered (source="inventory", scope="rack:<id>").
    KeyInstanceType  Key = "instance_type"
    KeySKU           Key = "sku"
    KeyVariant       Key = "variant"
    KeyGBGeneration  Key = "gb_generation"
    KeyRegion        Key = "region"
    KeyCluster       Key = "cluster"
)

var registered = map[Key]bool{
    KeyBundleTag:    true,
    KeyRequester:    true,
    KeyInstanceType: true,
    KeySKU:          true,
    KeyVariant:      true,
    KeyGBGeneration: true,
    KeyRegion:       true,
    KeyCluster:      true,
}

func IsRegistered(k Key) bool { return registered[k] }
```

**Convention.** Operator-promotion at POST /api/runs walks the request's `inputs` map and promotes any whose id (as a `Key`) is registered. Today that's `bundle_tag` and `requester`. The handler does not promote unregistered input ids — they remain in `runs.inputs` JSON only.

### 4.3 Provenance shape

Every fact row carries:

- `source` (TEXT) — short string identifying the producer: `"operator"`, `"inventory"`. Future values: `"metrics:baseline_pre"`, `"ai:summary.v1"`, etc.
- `source_event_id` (INTEGER, FK to `events.id`) — the event that produced this fact. Full producer detail (raw response, version, args) lives in the event payload; joinable when needed.
- `discovered_at` (INTEGER, unix ms) — when the producer captured the value.

This is enough for an agent to filter by source ("only inventory facts, not operator-supplied"), to audit when a fact was captured, and to retrieve full producer detail via an `events` join. Confidence is not modeled in iter-4a — deferred until an AI/probabilistic producer exists.

### 4.4 Storage: events + projection

Events are the source of truth (full history, replayable). The `facts` table is a last-writer-wins projection updated transactionally with each `FactsDiscovered` event append. Mirrors the existing `events`→`runs` pattern from iter-1/2/3a.

```sql
CREATE TABLE IF NOT EXISTS facts (
  run_id          TEXT    NOT NULL REFERENCES runs(id),
  scope           TEXT    NOT NULL,
  key             TEXT    NOT NULL,
  value           TEXT    NOT NULL,  -- JSON-encoded
  source          TEXT    NOT NULL,
  source_event_id INTEGER NOT NULL REFERENCES events(id),
  discovered_at   INTEGER NOT NULL,
  PRIMARY KEY (run_id, scope, key)
);
CREATE INDEX IF NOT EXISTS idx_facts_run ON facts(run_id);
CREATE INDEX IF NOT EXISTS idx_facts_scope ON facts(run_id, scope);
```

`UpsertFact` uses SQLite's `ON CONFLICT(run_id, scope, key) DO UPDATE SET value=excluded.value, source=excluded.source, source_event_id=excluded.source_event_id, discovered_at=excluded.discovered_at`. Subsequent emits overwrite. History preserved in `events`.

**Projection extensions.** Three event kinds project into row state today:
- `RunCreated` → INSERT into `runs` (existing iter-3a behavior; unchanged).
- `FactsDiscovered` → UPSERT into `facts` (one row per (scope, key) in the event payload). Does not modify `runs`.
- `FactsDiscoveryFailed` → UPDATE `runs SET status='failed', updated_at=...` for the run. Does not touch `facts`.

All projection writes happen inside the same transaction as the event append (existing pattern from iter-3a's `AppendEvent`).

**Value encoding.** `value` is always JSON-encoded — `"gb200-4x"` (string), `42` (number), `true` (bool), `["url1", "url2"]` (list). Internal representation stays as `json.RawMessage` until the API edge; `GET /api/runs/{id}/facts` returns parsed JSON values so consumers don't double-decode.

### 4.5 Event kinds

```
RunCreated            (existing, unchanged from iter-3a)
FactsDiscovered       payload: {scope, source, facts: {key: <JSON value>, ...}}
                      One event per (scope, source) batch. So a 2-rack canary
                      produces: 1 RunCreated, 1 run-scope FactsDiscovered
                      (source=operator), 2 rack-scope FactsDiscovered
                      (source=inventory, one per rack).
FactsDiscoveryFailed  payload: {scope, source, error, unresolved}
                      Emitted when inventory discovery fails to resolve one
                      or more racks. Single event per failed POST /api/runs
                      attempt, with scope="run", source="inventory", and
                      `unresolved` listing the rack ids that didn't resolve.
                      The projection of this event sets runs.status="failed"
                      — that's how the run row's status is updated without
                      bypassing the event-sourced pattern. iter-4b/c may
                      adopt this same kind for soft-fail probe semantics by
                      emitting it at non-run scopes (e.g. scope="rack:X").
```

## 5. HTTP API

### 5.1 `POST /api/runs` (modified)

Request shapes unchanged from iter-3a — accepts both legacy `{bundle, rack}` and new `{template_id, inputs}`. Normalization unchanged.

After validation succeeds and `RunCreated` is appended, the handler runs the discovery pipeline:

```
1. Promote operator inputs to facts (run-scope, source=operator):
   - Walk inputs map.
   - For each (k, v) where k is a registered fact Key:
     - Add to a single FactsDiscovered batch.
   - If batch is non-empty:
     - Append FactsDiscovered event.
     - UPSERT each (run_id, "run", k) → value.

2. Resolve all racks via inventoryclient.ResolveRack:
   - Collect (rack, RackFacts) tuples for resolved racks.
   - Collect unresolved racks into a list.
   - If any racks unresolved:
       Append a single FactsDiscoveryFailed event:
         scope="run", source="inventory", error message, unresolved=[...].
       Projection of FactsDiscoveryFailed sets runs.status="failed".
       Respond 422 inventory_unresolved with details.unresolved = list.
       Do NOT append any rack FactsDiscovered events.
       Do NOT roll back RunCreated or the operator FactsDiscovered.
       The run row is preserved with status="failed" for audit.
   - If all resolved: continue.

3. Emit per-rack FactsDiscovered (rack-scope, source=inventory):
   - For each (rack, RackFacts) tuple:
     - Append FactsDiscovered event with the six fact keys.
     - UPSERT each (run_id, "rack:<id>", k) → value.

4. Respond 201 with run summary.
```

**All-or-nothing inventory rule.** A canary either has full inventory facts for every rack or it has none. Test coverage must assert: a POST with a mix of resolved + unresolved racks results in 422 + no rack-scope FactsDiscovered events.

**Error codes** (additive to iter-3a):
- `inventory_unresolved` (422) — at least one rack failed inventory lookup.
- Existing: `not_found`, `invalid_request`, `internal`, `template_not_found`.

Error body shape:

```json
{
  "error": {
    "code": "inventory_unresolved",
    "message": "racks not found in inventory",
    "details": { "unresolved": ["dh99-r999-z-zz000-99a"] }
  }
}
```

### 5.2 `GET /api/runs/{id}/facts` (new)

```
GET /api/runs/{id}/facts
GET /api/runs/{id}/facts?scope=run
GET /api/runs/{id}/facts?scope=rack:dh01-r003-a-gb200-01a
GET /api/runs/{id}/facts?scope=rack:*
GET /api/runs/{id}/facts?source=inventory
GET /api/runs/{id}/facts?scope=run&source=operator
```

Response:

```json
{
  "facts": [
    {
      "scope": "run",
      "key": "bundle_tag",
      "value": "dell-xe9712-default-2.0.0-rc",
      "source": "operator",
      "source_event_id": 142,
      "discovered_at": 1748582401234
    },
    {
      "scope": "rack:dh01-r003-a-gb200-01a",
      "key": "instance_type",
      "value": "gb200-4x",
      "source": "inventory",
      "source_event_id": 143,
      "discovered_at": 1748582401237
    }
  ]
}
```

- `value` is parsed JSON (not a string of JSON).
- Empty result → `{"facts": []}`, never `null`.
- Unknown run id → 404 `not_found`.
- Filters are AND-combined.
- `scope=rack:*` matches any scope starting with `rack:`. Other wildcards not supported.

### 5.3 Unchanged endpoints

`GET /api/runs`, `GET /api/runs/{id}`, `GET /api/templates`, `GET /api/templates/{id}`, `GET /api/health` — all unchanged.

`GET /api/runs/{id}` does not embed the facts in its response. Consumers that need facts call `/facts` explicitly. (Future iteration may add a `?include=facts` query param if call patterns warrant it.)

## 6. Inventory port

### 6.1 Surface ported from `~/Documents/argo/tiphys/inventory`

Per the self-contained constraint, ported code lives under `internal/inventoryclient/`. The bare module path `qac` does not depend on any sibling project.

`internal/inventoryclient/inventoryclient.go`:

```go
package inventoryclient

import (
    "context"
    "errors"
    "fmt"
)

type Hit struct {
    ID      string
    Region  string
    Kind    string
    Cluster string
    Labels  map[string]string
}

const (
    KindBMN        = "bmn"
    KindRack       = "rack"
    KindCDU        = "cdu"
    KindPowerShelf = "powershelf"
    KindNVLink     = "nvlink"
)

type Resolver interface {
    Lookup(ctx context.Context, id string) ([]Hit, error)
}

var ErrNotFound = errors.New("inventoryclient: not found")

type AmbiguousError struct {
    ID   string
    Hits []Hit
}

func (e *AmbiguousError) Error() string {
    return fmt.Sprintf("inventoryclient: %q resolves to %d regions", e.ID, len(e.Hits))
}

func (e *AmbiguousError) Regions() []string {
    out := make([]string, len(e.Hits))
    for i, h := range e.Hits {
        out[i] = h.Region
    }
    return out
}
```

### 6.2 qac-specific wrapper

`internal/inventoryclient/facts.go`:

```go
type RackFacts struct {
    InstanceType string `fact:"instance_type"`
    SKU          string `fact:"sku"`
    Variant      string `fact:"variant"`
    GBGeneration string `fact:"gb_generation"`
    Region       string `fact:"region"`
    Cluster      string `fact:"cluster"`
}

// ResolveRack performs a rack-scoped lookup and extracts RackFacts from
// the rack-Kind Hit's Labels map. Returns ErrNotFound if no hit matches,
// AmbiguousError if multiple regions match.
//
// The Labels map is today's contract surface (mirrors what tiphys/inventory's
// rack fan-out path emits). When the real VM/Netbox-backed resolver lands
// later, it must populate Labels with the same vocabulary.
func ResolveRack(ctx context.Context, r Resolver, rackID string) (RackFacts, error) {
    hits, err := r.Lookup(ctx, rackID)
    if err != nil {
        return RackFacts{}, err
    }
    if len(hits) == 0 {
        return RackFacts{}, ErrNotFound
    }
    // Pick the rack-kind hit; ignore non-rack hits (BMN, CDU, etc).
    var rackHit *Hit
    for i := range hits {
        if hits[i].Kind == KindRack {
            if rackHit != nil {
                // multiple rack-kind hits — ambiguous
                return RackFacts{}, &AmbiguousError{ID: rackID, Hits: hits}
            }
            rackHit = &hits[i]
        }
    }
    if rackHit == nil {
        return RackFacts{}, ErrNotFound
    }
    return rackFactsFromLabels(rackHit.Labels), nil
}

func rackFactsFromLabels(labels map[string]string) RackFacts {
    return RackFacts{
        InstanceType: labels["instance_type"],
        SKU:          labels["sku"],
        Variant:      labels["variant"],
        GBGeneration: labels["gb_generation"],
        Region:       labels["region"],
        Cluster:      labels["cluster"],
    }
}

// ToFactBatch returns the RackFacts as a map[facts.Key]any suitable for
// facts.Emit. Empty-string fields are included as "" — preserved as the
// canonical fact value rather than dropped (downstream consumers can filter).
func (rf RackFacts) ToFactBatch() map[facts.Key]any {
    return map[facts.Key]any{
        facts.KeyInstanceType:  rf.InstanceType,
        facts.KeySKU:           rf.SKU,
        facts.KeyVariant:       rf.Variant,
        facts.KeyGBGeneration:  rf.GBGeneration,
        facts.KeyRegion:        rf.Region,
        facts.KeyCluster:       rf.Cluster,
    }
}
```

### 6.3 Stub backend for iter-4a

`internal/inventoryclient/map_resolver.go`:

```go
type MapResolver struct {
    table map[string][]Hit
}

func NewMapResolver(table map[string][]Hit) *MapResolver {
    return &MapResolver{table: table}
}

func (m *MapResolver) Lookup(ctx context.Context, id string) ([]Hit, error) {
    if hits, ok := m.table[id]; ok {
        return hits, nil
    }
    return nil, nil
}

// SeedDemoFixtures returns a deterministic table covering the rack ids
// used by `qac seed-demo` and the canonical canary template's example.
// Production wiring (serve cmd) injects this same fixture in iter-4a; a
// later iteration replaces it with a real VM/Netbox-backed Resolver.
func SeedDemoFixtures() map[string][]Hit {
    return map[string][]Hit{
        "dh01-r003-a-gb200-01a": {{
            ID: "dh01-r003-a-gb200-01a", Region: "us-east", Kind: KindRack,
            Cluster: "use-prod-01",
            Labels: map[string]string{
                "instance_type": "gb200-4x",
                "sku":           "nvl72",
                "variant":       "gb200",
                "gb_generation": "blackwell",
                "region":        "us-east",
                "cluster":       "use-prod-01",
            },
        }},
        "dh01-r003-a-gb200-02a": {{ /* second gb200 rack, same cluster */ }},
        "dh02-r004-b-gb300-01a": {{ /* gb300 variant, sku=nvl72, gb_generation=blackwell */ }},
        "dh02-r004-b-gb300-02a": {{ /* second gb300 rack, same cluster */ }},
    }
}
```

**Fixture coverage requirement.** `SeedDemoFixtures` must contain at minimum:
- Two `gb200-4x` racks (sku=nvl72, gb_generation=blackwell), distinct ids in the same cluster.
- Two `gb300-4x` racks (sku=nvl72, gb_generation=blackwell), distinct ids in a different cluster.
- One ambiguous-error fixture entry: an id that returns multiple rack-kind hits with different regions, used by `inventoryclient/facts_test.go::TestResolveRack_Ambiguous`.

The test `internal/inventoryclient/fixture_coverage_test.go` enforces that every rack id referenced by `cmd/qac/seed_demo.go` and every `example:` in `templates/firmware-release-canary.yaml` exists in `SeedDemoFixtures`.

### 6.4 Server wiring

`cmd/qac/serve.go`:

```go
resolver := inventoryclient.NewMapResolver(inventoryclient.SeedDemoFixtures())
srv, err := server.New(s, addr, resolvedDB, qac.DistFS, resolver)
```

`server.New` gains a `Resolver` parameter, passed through to `api.Router`, which threads it into `createRunHandler`.

### 6.5 What's intentionally not ported

Prom, Loki, Grafana, Netbox — none. Those are sub-project C upstream and stay there. The `Resolver` interface is the seam; a future iteration adds `inventoryclient.NewVMResolver(...)` implementing the same interface.

## 7. Template change

`templates/firmware-release-canary.yaml` — only the `inputs:` block changes.

```yaml
# Removed: instance_type (now inventory-discovered)
# Removed: variant       (now inventory-discovered)
# Removed: nitwit_receipts (no producer in iter-4a; re-add when failure-evidence flow exists)
# Removed: slack_thread    (no producer in iter-4a; re-add when post-run notifier exists)

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
    # Canonical CoreWeave rack format (post-2025-07-02 rename). Mirrored
    # in internal/api/runs_validate.go (Go) and
    # web/src/features/runs/newRunSchema.ts (TS).
    validate: "^dh\\d+-r\\d{3}-[a-z]+-[a-z]+-\\d{2}[a-z]$"

  - id: requester
    label: "Requester (from #havock)"
    type: text
    required: false
    # required:false until iter-3b's dynamic form lands; the iter-2
    # legacy {bundle, rack} modal can't supply it. Flips to required:true
    # in iter-3b.
```

`steps:` block unchanged. Template `version:` bumps from 1 to 2 (body content changed → version must bump per the iter-3a versioning convention).

## 8. File map

```
NEW
  internal/facts/keys.go                   Key type, registered constants, IsRegistered
  internal/facts/keys_test.go              Registry tests
  internal/facts/emit.go                   Emit helper: validate + event append + UPSERT
  internal/facts/emit_test.go              Emit tests (transactional, JSON encoding, unknown key)
  internal/inventoryclient/inventoryclient.go  Resolver, Hit, errors (ported from tiphys/inventory)
  internal/inventoryclient/inventoryclient_test.go
  internal/inventoryclient/facts.go        RackFacts, ResolveRack, ToFactBatch
  internal/inventoryclient/facts_test.go
  internal/inventoryclient/map_resolver.go MapResolver, SeedDemoFixtures
  internal/inventoryclient/fixture_coverage_test.go  Asserts seed-demo/template ids resolve
  internal/store/facts.go                  UpsertFact, ListFacts
  internal/store/facts_test.go
  internal/api/facts.go                    factsHandler (GET /api/runs/{id}/facts)
  internal/api/facts_test.go
  internal/api/discovery_integ_test.go     End-to-end discovery (operator promotion + inventory)

MODIFIED
  internal/store/migrations.go             Add `facts` table to schemaSQL
  internal/store/migrations_test.go        Add facts-table idempotency test
  internal/store/events.go                 Project FactsDiscovered → UPSERT facts; project
                                           FactsDiscoveryFailed → UPDATE runs.status='failed'
  internal/api/runs.go                     createRunHandler: promote inputs + resolve racks + emit
  internal/api/runs_test.go                Add facts assertions to existing happy/sad cases
  internal/api/api.go                      Register GET /api/runs/{id}/facts route; accept Resolver
  internal/api/canonical_integ_test.go     Assert facts projection populated correctly
  internal/api/errors.go                   Add codeInventoryUnresolved = "inventory_unresolved"
  internal/server/server.go                Accept Resolver param; pass through to api.Router
  cmd/qac/serve.go                      Construct MapResolver, pass into server.New
  templates/firmware-release-canary.yaml         Drop 4 inputs; bump version to 2

UNCHANGED
  internal/template/                       (no schema change)
  internal/seed/                           (template loader unchanged)
  internal/engine/                         (still empty; iter-4b territory)
  web/                                     (frontend untouched)
```

## 9. Boot sequence

`qac serve` order:
1. Resolve DB path.
2. `store.Open(ctx, dbPath)` → migrations apply (now includes `facts` table).
3. `seed.LoadEmbeddedTemplates(ctx, s, qac.TemplatesFS)` → canonical template upserted at version 2.
4. **New:** `resolver := inventoryclient.NewMapResolver(inventoryclient.SeedDemoFixtures())`.
5. `server.New(s, addr, dbPath, qac.DistFS, resolver)` → `api.Router` wired with `resolver`; createRunHandler closes over it.
6. `srv.ListenAndServe()`.

Signal-handling and graceful-shutdown paths unchanged from iter-3a.

## 10. Testing strategy

Target: ≥ 145 Go subtests + 18 Vitest. Iter-3a baseline is 105 Go; iter-4a adds ~35 new + modifies ~5.

### 10.1 Unit tests

| File | Subtests | Coverage |
|---|---|---|
| `internal/inventoryclient/inventoryclient_test.go` | ~6 | `MapResolver.Lookup` happy + miss returns `(nil, nil)`. Fixture round-trip via `SeedDemoFixtures`. `AmbiguousError.Regions` ordering. |
| `internal/inventoryclient/facts_test.go` | ~4 | `ResolveRack` extracts all six fact fields from `Hit.Labels`. Missing label → empty-string fact (not an error). Non-rack-kind hit ignored. Multi-region hits → `AmbiguousError`. |
| `internal/facts/keys_test.go` | ~3 | `IsRegistered` happy + miss. Registry is closed. Every declared const appears in `registered`. |
| `internal/facts/emit_test.go` | ~6 | `Emit` writes event + UPSERTs facts in one tx. Unknown key → `ErrUnknownFactKey`, no DB write. Re-emit overwrites value/source/source_event_id. JSON encoding round-trips for string/number/bool/list. Empty batch → no event appended. |
| `internal/store/facts_test.go` | ~5 | `UpsertFact` happy + UPSERT semantics. `ListFacts` with no filter, `scope=run`, `scope=rack:<id>`, `scope=rack:*`. Empty result → empty slice (not nil). |
| `internal/api/facts_test.go` | ~5 | `GET /facts` happy. Unknown run → 404. `scope` filter. `source` filter. Combined filters. Values arrive parsed-JSON (test reads back `"gb200-4x"`, not `"\"gb200-4x\""`). |

### 10.2 Updated tests

| File | Change |
|---|---|
| `internal/api/runs_test.go` | New cases: POST legacy `{bundle, rack}` emits operator-scope `FactsDiscovered` with `bundle_tag`. POST emits inventory-scope `FactsDiscovered` per rack with all six fact keys. Unresolved rack → 422 `inventory_unresolved`, `FactsDiscoveryFailed` event lands, run row status flips to "failed", events trail = [RunCreated, operator FactsDiscovered, FactsDiscoveryFailed]. Mixed resolved+unresolved → 422 + no rack-scope FactsDiscovered events. |
| `internal/api/canonical_integ_test.go` | Asserts facts projection populated end-to-end against the on-disk template. |
| `internal/store/migrations_test.go` | `facts` table created idempotently; re-running `migrate` is a no-op; iter-3a DB upgrades cleanly. |

### 10.3 Integration test

`internal/api/discovery_integ_test.go` — full pipeline:
1. Spin up test server with `MapResolver(SeedDemoFixtures())`.
2. POST a run with one rack from the fixtures.
3. Assert: 201 returned, 3 events present (`RunCreated`, run-scope `FactsDiscovered`, rack-scope `FactsDiscovered`).
4. GET `/api/runs/{id}/facts` → 7 facts (1 operator + 6 inventory).
5. POST a run with an unknown rack. Assert: 422, `inventory_unresolved`, body lists the rack. GET `/api/runs/{id}` returns the run with status="failed" and three events (`RunCreated`, operator `FactsDiscovered`, `FactsDiscoveryFailed`).

## 11. Acceptance criteria

Live walk-through, no commit:

| # | Check |
|---|---|
| A1 | `make build` clean, no new top-level deps. |
| A2 | `make lint` (`go vet` + `yarn lint`) passes. |
| A3 | `make test` passes; subtest count ≥ 145 Go + 18 Vitest. |
| A4 | `rm -f ~/.local/share/qac/qac.db && make dev` boots clean; `sqlite3 …/qac.db ".schema facts"` shows the table. |
| A5 | `curl /api/health` → 200. |
| A6 | `curl -X POST /api/runs -d '{"bundle":"…","rack":"dh01-r003-a-gb200-01a"}'` → 201, run id returned. |
| A7 | `curl /api/runs/<id>/facts` → 7 facts: bundle_tag at run-scope (source=operator) + 6 facts at rack-scope (source=inventory). |
| A8 | `curl /api/runs/<id>/facts?scope=run` → 1 fact. |
| A9 | `curl /api/runs/<id>/facts?source=inventory` → 6 facts. |
| A10 | POST with unknown rack → 422 `inventory_unresolved`, details.unresolved lists the rack. `GET /api/runs/<id>` shows the run with status="failed" and three events: `RunCreated`, run-scope operator `FactsDiscovered`, then `FactsDiscoveryFailed` (no rack-scope `FactsDiscovered`). |
| A11 | Posting against an iter-3a DB (copied to sandbox path): migration runs, `facts` table appears, existing runs untouched, new POST works end-to-end. |
| A12 | `./bin/qac template load templates/firmware-release-canary.yaml` → "loaded fw-release-canary v2". |
| A13 | `internal/api/canonical_integ_test.go` passes (regression coverage on the live YAML). |
| A14 | Mixed POST (1 resolved + 1 unresolved rack): 422; the resolved rack's facts do NOT land — proves all-or-nothing semantics. |

## 12. Risks

- **All-or-nothing rule misimplemented.** If discovery emits FactsDiscovered for the first 2 racks then fails on the 3rd, the run is half-populated and 422'd. A14 catches this; implementation must collect-then-emit.
- **Operator-promotion key collision.** If a template adds an input whose id collides with a registered fact key by accident, it gets silently promoted. Mitigation: the registry is small (8 keys); documented as a convention in the `internal/facts` package comment.
- **Test fixture rot.** `SeedDemoFixtures` must cover every rack id mentioned in `seed-demo` or the canonical template's `example:`. Mitigation: a single test asserts that every rack id used by `seed-demo` and `templates/firmware-release-canary.yaml` examples resolves in `SeedDemoFixtures`.
- **Label vocabulary drift.** `ResolveRack` extracts six keys from `Hit.Labels`. The real VM/Netbox resolver in a future iteration must emit the same key strings. Mitigation: the key strings live as Go consts in `keys.go` and are referenced by both the stub fixture and `rackFactsFromLabels` — a single source of truth.
- **Iter-2 legacy modal breakage.** Flipping `requester` to required:true would break iter-2. Avoided: `requester` stays required:false in iter-4a; deferred to iter-3b.

## 13. Forward compatibility

Designed to absorb iter-4b and iter-4c without schema migrations:

- **`node:<id>` scope** is registered in the vocabulary now; iter-4c writes per-node evidence at this scope without touching `keys.go` or migrations.
- **`FactsDiscoveryFailed` event kind** is introduced in iter-4a at scope="run" for inventory hard-fail. iter-4b/c can emit it at scope="rack:X" or scope="node:X" to record soft-fail probe semantics without a schema migration.
- **`Resolver` interface** is the only seam to inventory; iter-4b/c swap in a real VM-backed implementation by changing one line in `cmd/qac/serve.go`.
- **`Key` registry** absorbs new producers (engine, AI, notifier) by adding constants — append-only, no breaking changes to existing keys.
- **`source` field** absorbs new producers (e.g., `"metrics:baseline_pre"`) as new string values; no schema change.

## 14. Non-goals

- Engine, action handlers, checkpoint evaluator (iter-4b).
- Real VM/Netbox-backed `Resolver` (iter-4b/c).
- Per-node facts, evidence rows, node excludes (iter-4c+).
- SSE, async dispatch, frontend changes (iter-4c).
- AI-derived facts (later iteration).
- Confidence modeling on facts (later iteration when probabilistic sources exist).
- Schema versioning of the `facts` table beyond migration idempotency.
