# qac — persistent daily-refreshed local Sourcegraph cache (design)

**Date:** 2026-06-04
**Status:** approved (three forks confirmed with user)
**Predecessor:** the RLCC/FLCC workflow catalogs come from `internal/sourcegraph` (live fetch of each chart's `values.yaml`). The cache is **in-memory only** (`ttlCache`, 5-min TTL) and the server only selects the Sourcegraph backend when `AWXCTL_SOURCEGRAPH_TOKEN` is set — otherwise it falls back to the vendored 3-workflow fixture (`MapClient`). So a tokenless local run shows only 3 RLCC workflows.

## 1. Problem / goal

Keep a **persistent local copy** of the Sourcegraph-fetched RLCC + FLCC workflow catalogs (survives restarts) and **refresh it daily**. Net effect: run `qac` once with a token (`source_passkeys`) and it fetches the real catalogs and writes them to disk; subsequent runs — even tokenless/offline — serve that local copy (the New-run picker shows the real workflow list), auto-refreshed every 24h while running.

## 2. Decisions (confirmed with user)
1. **Refresh mechanism:** in-process — load disk copy on startup; if missing or >24h old and a token is present, refresh; while running, a 24h background ticker refreshes + rewrites the disk copy.
2. **Offline behavior:** serve the local disk copy even without a token this session; fall back to the vendored fixture only when there is **no** disk copy at all.
3. **Scope:** RLCC **and** FLCC (both use the same `sourcegraph.Client` + cache).

## 3. Design

### 3.1 Disk-backed cache (`internal/sourcegraph`)
- `sourcegraph.Client` gains an optional **cache directory** (e.g. `$XDG_DATA_HOME/qac/sourcegraph/`, sibling to `qac.db`). New constructor `NewClientWithCache(baseURL, cacheDir string)` (keep `NewClient` for the no-disk/test path).
- **Persisted entry** (one JSON file per fetch key): `{ key, body (base64/[]byte), sha, fetched_at (unix) }`. Filename = `sha256(key)` hex + `.json`. Fetch key = `repo + "@" + ref + "/" + path` (existing).
- **On construction:** `loadDisk()` reads every `*.json` in the cache dir into the in-memory `ttlCache` via `putAt(key, FetchResult{Body,SHA}, fetchedAt)` — preserving real age so freshness math works across restarts.
- **On successful `fetchNetwork`:** also write the entry to disk (atomically: temp file + rename). Memory + disk stay in lockstep.
- **TTL = 24h** (the daily-fresh window). `NewClientWithCache` sets `newTTLCache(24 * time.Hour)`. (`NewClient` may keep 5-min for tests, or take a ttl param — implementer's call; the persistent client uses 24h.)

### 3.2 Offline tolerance (`Client.Fetch`)
Today, on `cacheMiss`/`cacheStaleOld`, `Fetch` does a synchronous network fetch and **errors** if it fails. Change: on network failure, **fall back to any cached entry** (even stale-old) and return it without error; only return the error when nothing is cached. This lets a tokenless/offline run serve the last-fetched disk copy. (Stale-young already returns cached + background-refreshes — unchanged.)

### 3.3 Server wiring (`cmd/qac/serve.go`)
Replace the token-gated selection. Always build the cache dir under the resolved data dir. Decision tree:
- `QAC_RLCC_BACKEND=map` → `MapClient` (explicit fixture; unchanged).
- else → construct `sourcegraph.NewClientWithCache(url, cacheDir)` (loads disk). Use the `SourcegraphClient` for RLCC + FLCC **when** a token is present **or** the disk copy already has the RLCC/FLCC entries; otherwise (no token AND empty disk) fall back to `MapClient` + warn (so the API always serves).
- `liveBackend` (drives `X-Qac-Degraded`) = true whenever the SourcegraphClient is used (disk or live); the served `source.sha` is then the real SHA, not `vendored`.
- `purgeSourcegraph` stays wired to the client's cache purge (also clears disk).

### 3.4 Daily refresh (in-process)
When a token is present, after wiring:
- **Startup refresh-if-stale:** for the RLCC and FLCC fetch keys, if missing or >24h old, fetch now (populates disk). Non-fatal on failure (serve whatever's cached).
- **24h ticker:** a goroutine (`time.NewTicker(24*time.Hour)`) re-fetches both catalogs (RLCC values.yaml @ `main`, FLCC values.yaml @ `develop`) and rewrites disk; stopped on server shutdown (wire into the existing graceful-shutdown path). Refresh = call `ListWorkflows(ctx)` on each client (which Fetch→network→disk) or a small `Client.Warm(reqs)` helper.
- No token → no ticker, no startup fetch; serve the disk copy (or fixture). A later tokenful run refreshes it.

The known refresh targets (constants already in the sub-clients):
- RLCC: `github.com/coreweave/rack-lifecycle-controller`, `chart/rack-lifecycle-controller/values.yaml`, ref `main`.
- FLCC: `github.com/coreweave/fleet-lifecycle-controller`, `chart/values.yaml`, ref `develop`.

## 4. Out of scope
- An external scheduler / `qac sourcegraph refresh` subcommand (in-process only, per decision 1).
- Frontend changes (the picker already shows whatever `/api/rlcc/workflows` returns; filtering to walkable workflows is a separate tweak).
- Auth/token acquisition (still `source_passkeys` → `AWXCTL_SOURCEGRAPH_TOKEN`).
- Caching anything beyond the RLCC/FLCC `values.yaml` fetches.

## 5. Testing
- **`internal/sourcegraph`:** disk round-trip (fetch → file written → new Client loads it → cache hit without network); age preserved across reload (a >24h-old file reads as stale); offline fallback (network fn errors → stale-old entry served, no error; empty cache + error → error); atomic write (temp+rename).
- **Refresh/Warm:** a `Warm`/refresh call fetches the configured keys and writes disk; tolerant of per-key failure.
- **serve.go selection** (focused, if feasible): `QAC_RLCC_BACKEND=map` → fixture; disk copy present + no token → SourcegraphClient serves disk (not degraded); no token + empty disk → fixture (degraded). May be exercised via a small unit on the selection helper rather than full server boot.
- Full `go test ./...` green; `make build`. No web changes.

## 6. Acceptance criteria
1. A tokenful run fetches RLCC + FLCC catalogs and writes a local copy under `$XDG_DATA_HOME/qac/sourcegraph/`.
2. A subsequent **tokenless** run serves that local copy — `/api/rlcc/workflows` returns the real (not 3-item fixture) list, `X-Qac-Degraded` reflects real source (sha ≠ `vendored`).
3. While running, the copy refreshes every 24h (ticker) and on startup when >24h stale; the ticker stops on shutdown.
4. No disk copy and no token → the vendored fixture still serves (no regression, no error).
5. `go test ./...` + `make build` green.

## 7. Parallelism
Sequential: disk persistence + offline tolerance in `internal/sourcegraph` (with tests) → server wiring + refresh ticker. Subagent-driven.
