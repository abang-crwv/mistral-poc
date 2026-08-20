# qac — persistent daily-refreshed local Sourcegraph cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`).

**Goal:** Persist the Sourcegraph-fetched RLCC + FLCC workflow catalogs to a local disk cache (under `$XDG_DATA_HOME/qac/sourcegraph/`), load it on startup, serve it offline (tokenless), and refresh it every 24h while running.

**Architecture:** Add disk persistence + offline-tolerance + a forced `Refresh` to `internal/sourcegraph.Client` (new `NewClientWithCache`); wire `cmd/qac/serve.go` to always build the disk-backed client (fixture only when no token AND no disk copy), with a startup refresh-if-stale + a 24h ticker stopped on shutdown.

**Tech Stack:** Go 1.23, bare module `qac`. Spec: `docs/superpowers/specs/2026-06-04-qac-sourcegraph-local-cache-design.md`.

---

## Key facts (verified)
- `internal/sourcegraph/client.go`: `Client{token,baseURL,httpDo,cache *ttlCache}`. `NewClient(baseURL)` reads `AWXCTL_SOURCEGRAPH_TOKEN`, returns `ErrNoToken` if unset; sets `newTTLCache(5*time.Minute)`. `Fetch(ctx,repo,path,ref)` keys on `repo+"@"+ref+"/"+path`; cacheFresh→return; staleYoung→return+bg refresh; miss/staleOld→sync `fetchNetwork` (errors on failure). `FetchResult{Body []byte, SHA string}`. `PurgeCache()` → `cache.Purge()`.
- `internal/sourcegraph/cache.go`: `ttlCache{ttl,entries map[string]cacheEntry}`; `cacheEntry{res FetchResult, createdAt time.Time}` (confirm field names by reading). `put(key,res)`, `putAt(key,res,at)`, `lookup(key,now) (FetchResult,cacheStatus)` (cacheMiss/Fresh/StaleYoung/StaleOld), `Purge()`, `scheduleRefresh(key,fn)`.
- RLCC fetch coords (`internal/rlccclient/sourcegraph_client.go`): repo `github.com/coreweave/rack-lifecycle-controller`, path `chart/rack-lifecycle-controller/values.yaml`, ref `main`.
- FLCC (`internal/flccclient/sourcegraph_client.go`): repo `github.com/coreweave/fleet-lifecycle-controller`, path `chart/values.yaml`, ref `develop`.
- `cmd/qac/serve.go` (~line 64-90): token-gated switch sets `rlccC`/`flccC`/`liveBackend`/`purgeSourcegraph`. `resolvedDB` is the db path (its dir is the data dir). Graceful shutdown on SIGINT/SIGTERM is later in the same RunE (read it to wire ticker stop).
- `go:embed`/data dir: `$XDG_DATA_HOME/qac/qac.db` → cache dir = `filepath.Join(filepath.Dir(resolvedDB), "sourcegraph")`.

---

## Task 1: disk persistence + offline tolerance + Refresh (`internal/sourcegraph`)

**Files:** Modify `internal/sourcegraph/cache.go`, `internal/sourcegraph/client.go`; Test `internal/sourcegraph/disk_test.go` (create)

- [ ] **Step 1: Failing test** — create `internal/sourcegraph/disk_test.go`:
```go
package sourcegraph

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

// newTestClient builds a Client with a disk cache dir and a stub httpDo.
func newTestClient(t *testing.T, dir string, do func(*http.Request) (*http.Response, error)) *Client {
	t.Helper()
	c, err := NewClientWithCache("https://sg.example", dir)
	if err != nil {
		t.Fatalf("NewClientWithCache: %v", err)
	}
	c.httpDo = do
	return c
}

func okResp(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: http.NoBody, Header: http.Header{"X-Sourcegraph-Resolved-Revision": {"deadbeef"}}}
}

func TestDiskRoundTrip(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	do := func(_ *http.Request) (*http.Response, error) {
		calls++
		r := okResp("body-1")
		r.Body = httpBody("body-1")
		return r, nil
	}
	c1 := newTestClient(t, dir, do)
	res, err := c1.Fetch(context.Background(), "repo", "p.yaml", "main")
	if err != nil { t.Fatalf("fetch: %v", err) }
	if string(res.Body) != "body-1" { t.Fatalf("body = %q", res.Body) }
	if calls != 1 { t.Fatalf("calls = %d, want 1", calls) }

	// A NEW client over the same dir must serve from disk without a network call.
	c2 := newTestClient(t, dir, func(_ *http.Request) (*http.Response, error) { t.Fatal("should not hit network"); return nil, nil })
	res2, err := c2.Fetch(context.Background(), "repo", "p.yaml", "main")
	if err != nil { t.Fatalf("fetch c2: %v", err) }
	if string(res2.Body) != "body-1" { t.Fatalf("disk body = %q", res2.Body) }
}

func TestOfflineFallbackServesStale(t *testing.T) {
	dir := t.TempDir()
	c1 := newTestClient(t, dir, func(_ *http.Request) (*http.Response, error) { r := okResp(""); r.Body = httpBody("cached"); return r, nil })
	if _, err := c1.Fetch(context.Background(), "repo", "p.yaml", "main"); err != nil { t.Fatal(err) }

	// New client, expired entry, network down → must serve the stale disk copy.
	c2 := newTestClient(t, dir, func(_ *http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	c2.cache.expireAll() // force stale-old
	res, err := c2.Fetch(context.Background(), "repo", "p.yaml", "main")
	if err != nil { t.Fatalf("offline fetch should serve stale, got %v", err) }
	if string(res.Body) != "cached" { t.Fatalf("stale body = %q", res.Body) }
}

func TestOfflineNoCacheErrors(t *testing.T) {
	c := newTestClient(t, t.TempDir(), func(_ *http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	if _, err := c.Fetch(context.Background(), "repo", "p.yaml", "main"); err == nil {
		t.Fatal("expected error with empty cache + offline")
	}
}

func TestRefreshForcesNetwork(t *testing.T) {
	dir := t.TempDir()
	n := 0
	c := newTestClient(t, dir, func(_ *http.Request) (*http.Response, error) { n++; r := okResp(""); r.Body = httpBody("v"); return r, nil })
	if _, err := c.Fetch(context.Background(), "repo", "p.yaml", "main"); err != nil { t.Fatal(err) }
	if _, err := c.Refresh(context.Background(), "repo", "p.yaml", "main"); err != nil { t.Fatal(err) }
	if n != 2 { t.Fatalf("Refresh should bypass fresh cache; network calls = %d, want 2", n) }
	_ = filepath.Join(dir, "x") // dir used
}
```
(Add a tiny helper `httpBody(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }` in the test, and `import ("io";"strings")`. `expireAll()` is a test-only cache helper added in Step 2.)

- [ ] **Step 2: Run — expect FAIL:** `go test ./internal/sourcegraph -run 'Disk|Offline|Refresh' -v` (undefined `NewClientWithCache`/`Refresh`/`expireAll`).

- [ ] **Step 3: Disk persistence in cache.go** — add to `internal/sourcegraph/cache.go`:
```go
import (
	"encoding/json"
	"os"
	"path/filepath"
	"crypto/sha256"
	"encoding/hex"
)

// persisted is the on-disk JSON shape for one cache entry. Body is []byte
// (Go marshals it as base64), so arbitrary file content round-trips.
type persisted struct {
	Key       string `json:"key"`
	Body      []byte `json:"body"`
	SHA       string `json:"sha"`
	FetchedAt int64  `json:"fetched_at"`
}

func keyFile(dir, key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".json")
}

// loadDir reads every *.json entry under dir into the cache, preserving age.
func (c *ttlCache) loadDir(dir string) {
	ents, err := os.ReadDir(dir)
	if err != nil { return } // missing dir is fine — empty cache
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" { continue }
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil { continue }
		var p persisted
		if json.Unmarshal(b, &p) != nil || p.Key == "" { continue }
		c.putAt(p.Key, FetchResult{Body: p.Body, SHA: p.SHA}, time.Unix(p.FetchedAt, 0))
	}
}

// writeFile persists one entry atomically (temp + rename).
func writeFile(dir, key string, res FetchResult, at time.Time) {
	if dir == "" { return }
	_ = os.MkdirAll(dir, 0o755)
	p := persisted{Key: key, Body: res.Body, SHA: res.SHA, FetchedAt: at.Unix()}
	b, err := json.Marshal(p)
	if err != nil { return }
	tmp := keyFile(dir, key) + ".tmp"
	if os.WriteFile(tmp, b, 0o644) != nil { return }
	_ = os.Rename(tmp, keyFile(dir, key))
}

// expireAll backdates every entry past any plausible TTL (test helper).
func (c *ttlCache) expireAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.entries {
		e.createdAt = time.Unix(0, 0)
		c.entries[k] = e
	}
}
```
(Match the cache's existing mutex field name — read `cache.go`; it likely has `mu sync.Mutex`/`RWMutex`. Adjust `expireAll` lock calls to match. If `cacheEntry`'s time field isn't `createdAt`, use the real name.)

- [ ] **Step 4: Disk-backed constructor + offline Fetch + Refresh in client.go** — add to `internal/sourcegraph/client.go`:
```go
// add field to Client:
//   cacheDir string

// NewClientWithCache builds a Client whose TTL cache is backed by a directory
// on disk (loaded on construction, written on every successful network fetch)
// and whose freshness window is 24h. Unlike NewClient, a missing token is NOT
// fatal — a tokenless client still serves the on-disk copy (offline); fetches
// will fail and fall back to cache. cacheDir == "" disables persistence.
func NewClientWithCache(baseURL, cacheDir string) (*Client, error) {
	c := &Client{
		token:    os.Getenv("AWXCTL_SOURCEGRAPH_TOKEN"),
		baseURL:  strings.TrimRight(baseURL, "/"),
		httpDo:   http.DefaultClient.Do,
		cache:    newTTLCache(24 * time.Hour),
		cacheDir: cacheDir,
	}
	if cacheDir != "" {
		c.cache.loadDir(cacheDir)
	}
	return c, nil
}

// HasFresh reports whether a non-expired entry exists for the key (used by the
// server to decide fixture-vs-sourcegraph and startup refresh-if-stale).
func (c *Client) Has(repo, path, ref string) bool {
	_, status := c.cache.lookup(repo+"@"+ref+"/"+path, time.Now())
	return status != cacheMiss
}

// Refresh forces a network fetch (bypassing cache freshness), updates the
// cache, and persists to disk. Used by the daily ticker + startup warm.
func (c *Client) Refresh(ctx context.Context, repo, path, ref string) (FetchResult, error) {
	res, err := c.fetchNetwork(ctx, repo, path, ref)
	if err != nil {
		return FetchResult{}, err
	}
	key := repo + "@" + ref + "/" + path
	c.cache.put(key, res)
	writeFile(c.cacheDir, key, res, time.Now())
	return res, nil
}
```
Then modify `Fetch`'s miss/stale-old tail for persistence + offline tolerance:
```go
	// cacheMiss or cacheStaleOld: synchronous fetch, with offline fallback.
	res, err := c.fetchNetwork(ctx, repo, path, ref)
	if err != nil {
		if status == cacheStaleOld {
			return cached, nil // serve the stale disk/memory copy when offline
		}
		return FetchResult{}, err
	}
	c.cache.put(key, res)
	writeFile(c.cacheDir, key, res, now)
	return res, nil
```
(`cached`/`status` are already in scope from the `lookup` above the switch. Add `cacheDir string` to the `Client` struct definition.)

- [ ] **Step 5: Run — expect PASS:** `go test ./internal/sourcegraph -count=1`.

- [ ] **Step 6: Commit**
```bash
git add internal/sourcegraph/cache.go internal/sourcegraph/client.go internal/sourcegraph/disk_test.go
git commit -m "qac: sourcegraph — disk-backed cache + offline fallback + Refresh (local copy)"
```

---

## Task 2: server wiring — disk-backed selection + daily refresh

**Files:** Modify `cmd/qac/serve.go`

- [ ] **Step 1: Build the disk-backed client + selection** — replace the backend `switch` (~lines 64-90). Compute `cacheDir := filepath.Join(filepath.Dir(resolvedDB), "sourcegraph")` (ensure `path/filepath` imported). Logic:
```go
token := os.Getenv("AWXCTL_SOURCEGRAPH_TOKEN")
backendOverride := os.Getenv("QAC_RLCC_BACKEND")
var rlccC rlccclient.Client
var flccC flccclient.Client
var liveBackend bool
var purgeSourcegraph = func() {}
var sgRefresh func() // nil unless we have a token to refresh with

switch {
case backendOverride == "map":
	slog.Info("using map backend per QAC_RLCC_BACKEND=map")
	rlccC = rlccclient.NewMapClient()
	flccC = flccclient.NewMapClient()
default:
	cacheDir := filepath.Join(filepath.Dir(resolvedDB), "sourcegraph")
	sg, err := sourcegraph.NewClientWithCache("https://sourcegraph.coreweave.com", cacheDir)
	if err != nil {
		return fmt.Errorf("sourcegraph client: %w", err)
	}
	const (
		rlccRepo = "github.com/coreweave/rack-lifecycle-controller"
		rlccPath = "chart/rack-lifecycle-controller/values.yaml"
		rlccRef  = "main"
		flccRepo = "github.com/coreweave/fleet-lifecycle-controller"
		flccPath = "chart/values.yaml"
		flccRef  = "develop"
	)
	haveCopy := sg.Has(rlccRepo, rlccPath, rlccRef) || sg.Has(flccRepo, flccPath, flccRef)
	if token == "" && !haveCopy {
		slog.Warn("using map backend; no Sourcegraph token and no local copy (run `source_passkeys` once to fetch + cache the real workflows)")
		rlccC = rlccclient.NewMapClient()
		flccC = flccclient.NewMapClient()
		break
	}
	rlccC = rlccclient.NewSourcegraphClient(sg)
	flccC = flccclient.NewSourcegraphClient(sg)
	liveBackend = true
	purgeSourcegraph = sg.PurgeCache
	if token != "" {
		slog.Info("using sourcegraph backend (disk-cached)", "cache_dir", cacheDir, "token_len", len(token))
		sgRefresh = func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := sg.Refresh(ctx, rlccRepo, rlccPath, rlccRef); err != nil {
				slog.Warn("rlcc refresh failed", "err", err)
			}
			if _, err := sg.Refresh(ctx, flccRepo, flccPath, flccRef); err != nil {
				slog.Warn("flcc refresh failed", "err", err)
			}
		}
	} else {
		slog.Info("serving local Sourcegraph copy (no token this session; will not refresh)", "cache_dir", cacheDir)
	}
}
```
(Keep `NewSourcegraphClient(sg)` signatures as-is — confirm they accept `*sourcegraph.Client`.)

- [ ] **Step 2: Startup refresh-if-stale + 24h ticker** — after the server is constructed and before/around the graceful-shutdown block, when `sgRefresh != nil`, start a goroutine. Read the existing shutdown path (SIGINT/SIGTERM) and add a `stopRefresh := make(chan struct{})`; close it in the shutdown path. Goroutine:
```go
if sgRefresh != nil {
	go func() {
		sgRefresh() // startup warm (Refresh always re-fetches; bounds the copy to ~now)
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-stopRefresh:
				return
			case <-t.C:
				sgRefresh()
				slog.Info("sourcegraph local copy refreshed (24h tick)")
			}
		}
	}()
}
```
(If wiring a `stopRefresh` channel into the existing shutdown is awkward, derive it from a `context.Context` cancelled on signal — match whatever the RunE already uses. The goroutine MUST stop on shutdown — no leaked ticker.)
Note: `sgRefresh()` on startup always re-fetches (simpler + guarantees a fresh daily copy when a token is present). If you prefer refresh-only-if-stale, gate the startup call on `!sg.Has(...)` or an age check — but unconditional startup refresh is acceptable and matches "refresh every day".

- [ ] **Step 3: Verify build + vet + tests** — `go build ./... && go vet ./... && go test ./internal/sourcegraph ./cmd/qac -count=1`.

- [ ] **Step 4: Commit**
```bash
git add cmd/qac/serve.go
git commit -m "qac: serve — disk-cached Sourcegraph backend + startup/24h refresh (local copy)"
```

---

## Task 3: full verification

- [ ] **Step 1:** `go build ./... && go vet ./...` — clean.
- [ ] **Step 2:** `go test ./...` — all green.
- [ ] **Step 3:** `gofmt -l` on touched files — empty.
- [ ] **Step 4:** Live smoke (no token): `QAC_RLCC_BACKEND=` unset, no token, no prior copy → `./bin/qac serve` → `curl /api/rlcc/workflows` still returns the fixture (3) with `X-Qac-Degraded: true` (no regression). (A true tokenful fetch can't be exercised offline; assert the no-regression path.)
- [ ] **Step 5:** Acceptance review vs spec §6 (note: criteria 1-3 require a real token/network to fully exercise; verify the logic + the no-token/no-copy fallback path here).

---

## Self-review notes
- **Spec coverage:** §3.1 disk cache → Task 1 Steps 3-4; §3.2 offline tolerance → Task 1 Step 4 Fetch tail; §3.3 server selection → Task 2 Step 1; §3.4 refresh ticker → Task 2 Step 2; §5 tests → Task 1 + Task 3; §6 → Task 3.
- **Type consistency:** `NewClientWithCache(baseURL, cacheDir) (*Client, error)`, `Has(repo,path,ref) bool`, `Refresh(ctx,repo,path,ref) (FetchResult,error)`, `writeFile(dir,key,res,at)`, `loadDir(dir)` used consistently across Task 1 + Task 2. Fetch key format `repo+"@"+ref+"/"+path` matches existing.
- **Open details for the implementer:** confirm `ttlCache`'s mutex + time-field names (for `expireAll`/`loadDir`); confirm `NewSourcegraphClient` accepts `*sourcegraph.Client`; confirm the rlcc/flcc workflows handler tolerates a `ListWorkflows` error (token present, network down, empty disk) — pre-existing live behavior, no new handling required; wire `stopRefresh` into the actual shutdown mechanism the RunE uses (channel or ctx).
- **No regression:** `QAC_RLCC_BACKEND=map` and the no-token/no-copy path both still serve the fixture; `PurgeCache` still wired to `/api/admin/sourcegraph/refresh`.
