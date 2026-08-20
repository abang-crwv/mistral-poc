# Design: qac-owned super-region VM fan-out

Date: 2026-06-05
Status: approved (brainstorming)

## Problem

Both VM-backed clients in qac bind a **single** VictoriaMetrics endpoint at
construction:

- `internal/inventoryclient/vm_resolver.go` — `VMResolver`, resolves rack
  hardware facts from `device_slot_instance_types`.
- `internal/lifecycleclient/promclient.go` — `PromClient`, RLCC/FLCC lifecycle
  state.

`cmd/qac/serve.go` passes the same `--vm-url` (default
`http://vmauth.us-west.int.coreweave.com:8427/prometheus`) to both. The
consequence: only racks whose data lives in the **us-west** tenant resolve.
Racks in us-east / eu-south / us-lab are unreachable — the binary cannot even
reach their endpoints. (Confirmed: the `device_slot_instance_types` + regex
query is *not* the blocker; endpoint reach is.)

There is also no unauthenticated fallback: when `AWXCTL_VMAUTH_USERNAME` /
`_PASSWORD` are unset, both clients return `ErrAuth` and `serve.go` falls back
to the **demo** map (fake data), never real data.

## Prior art

`~/Documents/argo/tiphys/inventory` implements exactly this: a `composite`
resolver builds one `vm.Client` per super-region, `vm.Defaults()` ships the
authed/unauthed endpoint maps plus a region→super-region prefix table, and
`MaybeWarnUnauthed` emits a one-time stderr warning when creds are absent.

Per qac's hard constraint (bare `qac` module, **port don't import**), we
**port** the relevant logic into a new qac-owned package and reference tiphys
**not at all**.

## Decisions (locked during brainstorming)

1. **Scope** — a new qac-owned package `internal/vm` (port, no tiphys import);
   **both** `VMResolver` and `PromClient` route through it.
2. **Endpoint maps** — qac's authoritative table: **all four** super-regions
   (`us-east`, `us-west`, `eu-south`, `us-lab`) in **both** authed and unauthed
   maps. (Broader than tiphys, which omits `us-lab` everywhere and `eu-south`
   from unauthed.)
3. **Routing** — **always fan out to all** super-regions in parallel; return the
   **union** of series (no dedupe in the `vm` layer — `VMResolver.ResolveBMNs`
   already dedupes by deviceslot and `dominantLabels` collapses fact spread). No
   `regionToSuper` sniff table: full fan-out makes it unnecessary
   (the us-central→us-west folding gotcha resolves itself — us-west answers, the
   rest come back empty; one physical rack lives in exactly one tenant).
4. **Fallback** — **creds-missing AND per-request failure**:
   - creds absent at startup ⇒ unauthed vmui mode + one-time warning;
   - creds present but an authed query fails ⇒ retry that one query on the
     super-region's vmui URL before counting it as errored.
   The demo map is **not** an automatic fallback — it stays reachable only via
   the explicit `QAC_INV_BACKEND=map` / `QAC_VM_BACKEND=map` overrides.

## Out of scope

- The `baremetal_node_physical_topology_labels` resolver redesign (deferred to a
  dedicated session).
- Any change to rack-id regexes (runs_validate.go, the canary template,
  newRunSchema.ts, NewRunModal.jsx).

## Architecture

### 1. New package `internal/vm`

The single home for VM endpoints, auth, HTTP plumbing, and fan-out. Both
existing clients delegate to it; the duplicated `queryVector` in
`vm_resolver.go` and `promclient.go` is deleted.

```go
package vm

// Defaults returns qac's authoritative super-region endpoint maps.
func Defaults() (authed, unauthed map[string]string)
//  authed[sr]   = http://vmauth.<sr>.int.coreweave.com:8427/prometheus
//  unauthed[sr] = http://vmui.<sr>.int.coreweave.com/select/0/prometheus
//  sr ∈ {us-east, us-west, eu-south, us-lab}

type Config struct {
    AuthedURLs, UnauthedURLs map[string]string // default to Defaults()
    Username, Password       string            // empty ⇒ unauthed startup mode
    HTTPClient               *http.Client      // default: 10s timeout, modest pool
}

type Client struct { /* per-super-region endpoints, creds, httpDo, unauthed flag */ }

func New(cfg Config) *Client

// QueryVector fans q out to ALL super-regions in parallel and returns the
// union of result-series label maps. error (wrapping ErrUpstream) is returned
// ONLY when every super-region errored.
func (c *Client) QueryVector(ctx context.Context, q string) ([]map[string]string, error)
```

**Internal shape per super-region** (`endpoint`): `{name, authedURL,
unauthedURL}`. Endpoints are built from the merged authed/unauthed maps in
sorted-key order for deterministic iteration/tests.

**HTTP plumbing** (ported, single copy): GET `<base>/api/v1/query?query=<esc>`,
`Accept: application/json`, basic auth when authed, 10s per-request timeout,
read the standard Prometheus instant-query response, return each result series'
`metric` label map (sample value ignored — both callers ignore it today).

**One-time unauthed warning**: ported `MaybeWarnUnauthed` equivalent guarded by
a `sync.Once`, with a test-settable warn writer (`SetWarnWriter` /
`ResetWarnOnce`) so tests can observe it deterministically. Emitted once per
process when the client is constructed in unauthed mode.

### 2. Endpoint maps & creds

`Defaults()` hardcodes the 4-region table. Creds remain
`AWXCTL_VMAUTH_USERNAME` / `_PASSWORD`, read in `serve.go` and passed via
`Config`. No `regionToSuper` table.

### 3. Fan-out + fallback semantics

`QueryVector` runs one goroutine per super-region:

- **Creds present** → query that super-region's **authed** VMauth URL. On
  failure (401 / 5xx / transport error / non-`success` body), **retry the same
  query against that super-region's vmui URL**.
- **Creds absent at startup** → skip authed entirely; query vmui across all
  regions; emit the one-time warning.
- A super-region counts as "errored" only when **both** its authed and vmui
  attempts fail.
- Result = **union** of all series across super-regions. `error` (wrapping
  `ErrUpstream`) is returned **only when every super-region errored**, so an
  upstream outage is distinguishable from a clean miss (empty union + nil
  error). `VMResolver.dominantLabels` already collapses any cross-source spread.

### 4. Refactor of the two clients + serve.go

- **`inventoryclient.VMResolver`**: drop `baseURL/user/pass/httpDo`; hold a
  `*vm.Client`. `queryRack` calls `r.vm.QueryVector`. All rack-shaping logic
  (`splitRackID`, `dominantLabels`, `deriveVariant`/`deriveGBGeneration`, BMN
  building, sorting) is unchanged. Constructor becomes
  `NewVMResolver(c *vm.Client) *VMResolver`.
- **`lifecycleclient.PromClient`**: same swap; its `queryVector` becomes a thin
  wrapper mapping `[]map[string]string` → its `promSample` / `IgnoredBMN`
  shapes. Constructor becomes `NewPromClient(c *vm.Client) *PromClient`.
- **`ErrAuth` removed from both packages** — creds-missing is no longer an
  error. The `errors.Is(err, ErrAuth)` → demo-map branches in `serve.go`
  collapse: the default path is always vm-backed (authed or unauthed); the demo
  `MapResolver` / `MapClient` stays reachable only via the explicit
  `QAC_INV_BACKEND=map` / `QAC_VM_BACKEND=map` overrides.
- **`serve.go`**: build one shared `*vm.Client` from env creds + `vm.Defaults()`,
  pass it to both constructors. The `--vm-url` flag becomes an **optional
  single-endpoint override**: when set, the client uses a single authed
  super-region (named `override`) — escape hatch for testing / pinning; when
  unset (the **new default**, flipped from the hardcoded us-west URL) the
  built-in 4-region maps are used. The override sets `AuthedURLs` only (no vmui
  fallback for the override endpoint), so it is intended for the creds-present
  case.

### 5. Testing

- **`internal/vm`** (new): fan-out union across endpoints; per-region
  authed→vmui fallback on injected authed failure; all-regions-errored →
  `ErrUpstream`; creds-missing → unauthed mode + one-time warning (captured via
  settable writer); deterministic endpoint ordering. Fake `httpDo`.
- **`VMResolver` / `PromClient` tests**: construct a one-endpoint `vm.Client`
  pointed at an `httptest` server (replaces `NewVMResolver(srv.URL)` /
  `NewPromClient(srv.URL)`); existing shaping assertions stay. The
  `*_MissingCredsReturnsErrAuth` tests are rewritten to assert unauthed-mode
  behavior (no error; vmui used) instead of `ErrAuth`.
- **Canonical integration tests** (`internal/api/canonical_integ_test.go`,
  `discovery_integ_test.go`) unchanged and still green.

## Behavior change summary

| Situation | Before | After |
|---|---|---|
| Rack in us-east/eu-south/us-lab | unreachable (us-west only) | resolved via fan-out |
| Creds present, authed query fails | error | retried on vmui, then errored |
| Creds absent | demo map (fake data) | unauthed vmui (real data) + 1× warning |
| `QAC_INV_BACKEND=map` / `QAC_VM_BACKEND=map` | demo map | demo map (unchanged) |
| `--vm-url` set | sole endpoint, both clients | sole authed override endpoint |
| `--vm-url` default | hardcoded us-west URL | empty ⇒ built-in 4-region maps |
