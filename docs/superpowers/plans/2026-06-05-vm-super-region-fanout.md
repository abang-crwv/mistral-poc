# VM super-region fan-out Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make qac resolve racks in every super-region (us-east, us-west, eu-south, us-lab) by fanning VictoriaMetrics queries across all of them, with an unauthenticated vmui fallback when VMauth creds are missing or an authed query fails.

**Architecture:** A new qac-owned package `internal/vm` (ported from `tiphys/inventory`, referencing it not at all) owns the super-region endpoint maps, basic-auth, the `/api/v1/query` HTTP plumbing, and the parallel fan-out + per-region vmui fallback. The existing `inventoryclient.VMResolver` and `lifecycleclient.PromClient` are refactored to delegate all HTTP to a shared `*vm.Client`; their domain logic (rack shaping, query rendering) is untouched. `serve.go` builds one shared client and passes it to both.

**Tech Stack:** Go 1.26 (`net/http`, `encoding/json`, `sync`), bare `qac` module (no external module path, no `replace`). Tests: stdlib `testing` + `net/http/httptest`.

---

## File Structure

**Create:**
- `internal/vm/vm.go` — `Defaults()`, `Config`, `Client`, `New`, `QueryVector`, `queryEndpoint`, `doQuery`, `ErrUpstream`, `vmResponse`. The single home for VM endpoints + HTTP.
- `internal/vm/warn.go` — one-time unauthed-fallback warning (`maybeWarnUnauthed`) with test hooks (`SetWarnWriter`, `ResetWarnOnce`).
- `internal/vm/vm_test.go` — fan-out, fallback, ErrUpstream, unauthed-warning, trailing-slash tests.

**Modify:**
- `internal/inventoryclient/vm_resolver.go` — strip HTTP plumbing + `ErrAuth`; hold a `*vm.Client`; `NewVMResolver(*vm.Client)`.
- `internal/inventoryclient/vm_resolver_test.go` — rebuild test helper on `vm.Client`; delete the two creds/trailing-slash tests now owned by `internal/vm`.
- `internal/lifecycleclient/promclient.go` — strip HTTP plumbing; hold a `*vm.Client`; `NewPromClient(*vm.Client)`.
- `internal/lifecycleclient/types.go` — remove the now-unused `ErrAuth` var.
- `internal/lifecycleclient/promclient_test.go` — rebuild construction on `vm.Client`; delete the creds test.
- `cmd/qac/serve.go` — build one shared `*vm.Client`; pass to both constructors; collapse the `ErrAuth` demo-map branches; flip the `--vm-url` default to empty (override-only).

---

## Task 1: New `internal/vm` package

**Files:**
- Create: `internal/vm/vm.go`
- Create: `internal/vm/warn.go`
- Test: `internal/vm/vm_test.go`

- [ ] **Step 1: Write the failing test for `Defaults`**

Create `internal/vm/vm_test.go`:

```go
package vm

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

func TestDefaults_FourSuperRegionsBoth(t *testing.T) {
	authed, unauthed := Defaults()
	want := []string{"eu-south", "us-east", "us-lab", "us-west"}

	var gotA []string
	for k := range authed {
		gotA = append(gotA, k)
	}
	sort.Strings(gotA)
	if strings.Join(gotA, ",") != strings.Join(want, ",") {
		t.Errorf("authed keys = %v, want %v", gotA, want)
	}
	if authed["us-lab"] != "http://vmauth.us-lab.int.coreweave.com:8427/prometheus" {
		t.Errorf("authed us-lab = %q", authed["us-lab"])
	}

	var gotU []string
	for k := range unauthed {
		gotU = append(gotU, k)
	}
	sort.Strings(gotU)
	if strings.Join(gotU, ",") != strings.Join(want, ",") {
		t.Errorf("unauthed keys = %v, want %v", gotU, want)
	}
	if unauthed["eu-south"] != "http://vmui.eu-south.int.coreweave.com/select/0/prometheus" {
		t.Errorf("unauthed eu-south = %q", unauthed["eu-south"])
	}
}
```

- [ ] **Step 2: Run it to confirm it fails (package doesn't compile yet)**

Run: `go test ./internal/vm/ -run TestDefaults -v`
Expected: FAIL — `undefined: Defaults` (or build failure: no non-test Go files).

- [ ] **Step 3: Create `internal/vm/vm.go` with the full implementation**

Create `internal/vm/vm.go`:

```go
// Package vm fans out PromQL instant queries across CoreWeave's
// VictoriaMetrics super-regions, with per-super-region unauthenticated
// (vmui) fallback. It is the single home for VM endpoints, basic-auth,
// and the /api/v1/query HTTP plumbing shared by the inventory resolver
// (internal/inventoryclient) and the lifecycle client
// (internal/lifecycleclient). Ported from tiphys/inventory — qac owns
// this copy outright (bare `qac` module: port, don't import).
package vm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrUpstream is wrapped by QueryVector when EVERY configured
// super-region failed (transport error, non-200, or non-"success"
// body). A clean miss — zero series with at least one super-region
// answering — returns (nil, nil) instead, so callers can distinguish
// "rack not found" from "VM unreachable".
var ErrUpstream = errors.New("vm: all super-regions failed")

// Defaults returns qac's authoritative super-region endpoint maps:
// authed VMauth (port 8427, HTTP basic auth) and unauthed vmui
// (multitenant /select/0 path, no auth). Both cover all four
// super-regions. VMauth is plain HTTP — an https:// scheme yields a
// "server gave HTTP response to HTTPS client" TLS error.
func Defaults() (authed, unauthed map[string]string) {
	authed = map[string]string{
		"us-east":  "http://vmauth.us-east.int.coreweave.com:8427/prometheus",
		"us-west":  "http://vmauth.us-west.int.coreweave.com:8427/prometheus",
		"eu-south": "http://vmauth.eu-south.int.coreweave.com:8427/prometheus",
		"us-lab":   "http://vmauth.us-lab.int.coreweave.com:8427/prometheus",
	}
	unauthed = map[string]string{
		"us-east":  "http://vmui.us-east.int.coreweave.com/select/0/prometheus",
		"us-west":  "http://vmui.us-west.int.coreweave.com/select/0/prometheus",
		"eu-south": "http://vmui.eu-south.int.coreweave.com/select/0/prometheus",
		"us-lab":   "http://vmui.us-lab.int.coreweave.com/select/0/prometheus",
	}
	return authed, unauthed
}

// Config configures a Client. Nil AuthedURLs AND nil UnauthedURLs fall
// back to Defaults(). An empty Username or Password puts the Client in
// unauthed mode (vmui only) and emits a one-time warning.
type Config struct {
	AuthedURLs   map[string]string
	UnauthedURLs map[string]string
	Username     string
	Password     string
	HTTPClient   *http.Client
}

// endpoint is one super-region's URL pair (either side may be empty).
type endpoint struct {
	name        string
	authedURL   string
	unauthedURL string
}

// Client fans out instant queries across super-regions. Goroutine-safe.
type Client struct {
	endpoints []endpoint
	user      string
	pass      string
	unauthed  bool // creds absent: skip authed attempts entirely
	httpDo    func(*http.Request) (*http.Response, error)
}

// New builds a Client. It never errors: an empty endpoint set simply
// makes QueryVector return ErrUpstream. When creds are absent it logs a
// one-time unauthed-fallback warning.
func New(cfg Config) *Client {
	authed, unauthed := cfg.AuthedURLs, cfg.UnauthedURLs
	if authed == nil && unauthed == nil {
		authed, unauthed = Defaults()
	}
	// Union of super-region keys, sorted for deterministic iteration.
	keySet := map[string]struct{}{}
	for k := range authed {
		keySet[k] = struct{}{}
	}
	for k := range unauthed {
		keySet[k] = struct{}{}
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	eps := make([]endpoint, 0, len(keys))
	for _, k := range keys {
		eps = append(eps, endpoint{
			name:        k,
			authedURL:   strings.TrimRight(authed[k], "/"),
			unauthedURL: strings.TrimRight(unauthed[k], "/"),
		})
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	noCreds := cfg.Username == "" || cfg.Password == ""
	if noCreds {
		maybeWarnUnauthed("AWXCTL_VMAUTH_USERNAME / _PASSWORD unset")
	}
	return &Client{
		endpoints: eps,
		user:      cfg.Username,
		pass:      cfg.Password,
		unauthed:  noCreds,
		httpDo:    httpClient.Do,
	}
}

// QueryVector runs q as an instant query against every super-region in
// parallel and returns the UNION of result-series label maps. Per
// super-region: try the authed URL (when creds present); on failure
// retry the unauthed vmui URL. A super-region errors only when both
// attempts fail. Returns (nil, ErrUpstream) only when every
// super-region errored.
func (c *Client) QueryVector(ctx context.Context, q string) ([]map[string]string, error) {
	if len(c.endpoints) == 0 {
		return nil, fmt.Errorf("%w: no super-regions configured", ErrUpstream)
	}
	type result struct {
		series []map[string]string
		err    error
	}
	results := make([]result, len(c.endpoints))
	var wg sync.WaitGroup
	for i, ep := range c.endpoints {
		i, ep := i, ep
		wg.Add(1)
		go func() {
			defer wg.Done()
			series, err := c.queryEndpoint(ctx, ep, q)
			results[i] = result{series: series, err: err}
		}()
	}
	wg.Wait()

	var union []map[string]string
	allErrored := true
	for _, r := range results {
		if r.err == nil {
			allErrored = false
			union = append(union, r.series...)
		}
	}
	if allErrored {
		return nil, fmt.Errorf("%w", ErrUpstream)
	}
	return union, nil
}

// queryEndpoint tries the authed URL first (when creds present), then
// falls back to the unauthed vmui URL. Returns the first success, or the
// last error when every attempt fails.
func (c *Client) queryEndpoint(ctx context.Context, ep endpoint, q string) ([]map[string]string, error) {
	var lastErr error
	if !c.unauthed && ep.authedURL != "" {
		series, err := c.doQuery(ctx, ep.authedURL, q, true)
		if err == nil {
			return series, nil
		}
		lastErr = err
	}
	if ep.unauthedURL != "" {
		series, err := c.doQuery(ctx, ep.unauthedURL, q, false)
		if err == nil {
			return series, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("vm %s: no usable endpoint", ep.name)
	}
	return nil, lastErr
}

// vmResponse is the standard Prometheus instant-query response. Only the
// per-series label map is read; the sample value is ignored (both
// callers ignore it).
type vmResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
		} `json:"result"`
	} `json:"data"`
}

// doQuery issues one GET <base>/api/v1/query?query=<q>. useAuth toggles
// the basic-auth header (true for VMauth, false for vmui). A 10s
// per-request timeout is layered onto the caller's context.
func (c *Client) doQuery(ctx context.Context, base, q string, useAuth bool) ([]map[string]string, error) {
	u := fmt.Sprintf("%s/api/v1/query?query=%s", base, url.QueryEscape(q))
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if useAuth {
		req.SetBasicAuth(c.user, c.pass)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpDo(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("vm %s: status %d: %s", base, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var vr vmResponse
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return nil, fmt.Errorf("decode vm response: %w", err)
	}
	if vr.Status != "success" {
		return nil, fmt.Errorf("vm status: %s", vr.Status)
	}
	out := make([]map[string]string, 0, len(vr.Data.Result))
	for _, s := range vr.Data.Result {
		out = append(out, s.Metric)
	}
	return out, nil
}
```

- [ ] **Step 4: Create `internal/vm/warn.go`**

Create `internal/vm/warn.go`:

```go
package vm

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// warnOnce ensures the unauthed-fallback warning is emitted at most once
// per process, even when multiple Clients are built.
var (
	warnOnce     sync.Once
	warnWriterMu sync.Mutex
	warnWriterOv io.Writer
)

// maybeWarnUnauthed emits the one-time unauthed-fallback warning.
func maybeWarnUnauthed(reason string) {
	warnOnce.Do(func() {
		if w := warnWriter(); w != nil {
			fmt.Fprintf(w, "vm: no VMauth credentials (%s); falling back to unauthed vmui endpoints\n", reason)
		}
	})
}

func warnWriter() io.Writer {
	warnWriterMu.Lock()
	defer warnWriterMu.Unlock()
	if warnWriterOv != nil {
		return warnWriterOv
	}
	return os.Stderr
}

// SetWarnWriter overrides the unauthed-warning destination (nil to
// suppress). Returns the previous writer. Intended for tests.
func SetWarnWriter(w io.Writer) io.Writer {
	warnWriterMu.Lock()
	defer warnWriterMu.Unlock()
	prev := warnWriterOv
	warnWriterOv = w
	return prev
}

// ResetWarnOnce resets the once-gate so tests can observe the warning
// across subtests. Intended for tests.
func ResetWarnOnce() { warnOnce = sync.Once{} }
```

- [ ] **Step 5: Run the Defaults test to verify it passes**

Run: `go test ./internal/vm/ -run TestDefaults -v`
Expected: PASS.

- [ ] **Step 6: Add the fan-out / fallback / warning tests**

Append to `internal/vm/vm_test.go`:

```go
// vmServer returns an httptest server that replies to /api/v1/query with
// the given series label maps. wantAuth toggles a basic-auth assertion.
func vmServer(t *testing.T, wantAuth bool, series ...map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("path = %q, want /api/v1/query", r.URL.Path)
		}
		if _, _, ok := r.BasicAuth(); ok != wantAuth {
			t.Errorf("basic auth present = %v, want %v", ok, wantAuth)
		}
		rows := make([]map[string]any, 0, len(series))
		for _, m := range series {
			rows = append(rows, map[string]any{"metric": m, "value": []any{0, "1"}})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "vector", "result": rows},
		})
	}))
}

func TestQueryVector_UnionAcrossSuperRegions(t *testing.T) {
	east := vmServer(t, true, map[string]string{"deviceslot": "e-1"})
	defer east.Close()
	west := vmServer(t, true, map[string]string{"deviceslot": "w-1"})
	defer west.Close()

	c := New(Config{
		AuthedURLs: map[string]string{"us-east": east.URL, "us-west": west.URL},
		Username:   "u", Password: "p",
		HTTPClient: east.Client(),
	})
	got, err := c.QueryVector(context.Background(), "up")
	if err != nil {
		t.Fatalf("QueryVector: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (union of both regions)", len(got))
	}
	slots := map[string]bool{got[0]["deviceslot"]: true, got[1]["deviceslot"]: true}
	if !slots["e-1"] || !slots["w-1"] {
		t.Errorf("union = %v, want e-1 and w-1", got)
	}
}

func TestQueryVector_AuthedFailsFallsBackToVMUI(t *testing.T) {
	authed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer authed.Close()
	vmui := vmServer(t, false, map[string]string{"deviceslot": "via-vmui"})
	defer vmui.Close()

	c := New(Config{
		AuthedURLs:   map[string]string{"us-west": authed.URL},
		UnauthedURLs: map[string]string{"us-west": vmui.URL},
		Username:     "u", Password: "p",
		HTTPClient: vmui.Client(),
	})
	got, err := c.QueryVector(context.Background(), "up")
	if err != nil {
		t.Fatalf("QueryVector: %v", err)
	}
	if len(got) != 1 || got[0]["deviceslot"] != "via-vmui" {
		t.Fatalf("got %v, want one series via-vmui", got)
	}
}

func TestQueryVector_AllErroredReturnsErrUpstream(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad.Close()

	c := New(Config{
		AuthedURLs: map[string]string{"us-west": bad.URL},
		Username:   "u", Password: "p",
		HTTPClient: bad.Client(),
	})
	_, err := c.QueryVector(context.Background(), "up")
	if !errorsIsUpstream(err) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}

func TestQueryVector_MissDistinctFromOutage(t *testing.T) {
	empty := vmServer(t, true) // 200 success, zero series
	defer empty.Close()

	c := New(Config{
		AuthedURLs: map[string]string{"us-west": empty.URL},
		Username:   "u", Password: "p",
		HTTPClient: empty.Client(),
	})
	got, err := c.QueryVector(context.Background(), "up")
	if err != nil {
		t.Fatalf("clean miss should be nil error, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestNew_NoCredsUsesUnauthedAndWarnsOnce(t *testing.T) {
	var buf bytes.Buffer
	prev := SetWarnWriter(&buf)
	defer SetWarnWriter(prev)
	ResetWarnOnce()
	t.Cleanup(ResetWarnOnce)

	vmui := vmServer(t, false, map[string]string{"deviceslot": "noauth"})
	defer vmui.Close()

	c := New(Config{
		UnauthedURLs: map[string]string{"us-west": vmui.URL},
		// no Username/Password → unauthed mode
		HTTPClient: vmui.Client(),
	})
	got, err := c.QueryVector(context.Background(), "up")
	if err != nil {
		t.Fatalf("QueryVector: %v", err)
	}
	if len(got) != 1 || got[0]["deviceslot"] != "noauth" {
		t.Fatalf("got %v, want one series noauth (no basic auth sent)", got)
	}
	if !strings.Contains(buf.String(), "falling back to unauthed vmui") {
		t.Errorf("warning not emitted: %q", buf.String())
	}
	// Building a second unauthed client must NOT warn again.
	buf.Reset()
	_ = New(Config{UnauthedURLs: map[string]string{"us-west": vmui.URL}, HTTPClient: vmui.Client()})
	if buf.Len() != 0 {
		t.Errorf("second warning emitted: %q", buf.String())
	}
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	srv := vmServer(t, true, map[string]string{"deviceslot": "x"})
	defer srv.Close()
	// A trailing slash on the base URL must not double the / before
	// /api/v1/query — the request still reaches the handler.
	c := New(Config{
		AuthedURLs: map[string]string{"us-west": srv.URL + "/"},
		Username:   "u", Password: "p",
		HTTPClient: srv.Client(),
	})
	got, err := c.QueryVector(context.Background(), "up")
	if err != nil || len(got) != 1 {
		t.Fatalf("got (%v, %v), want one series and nil err", got, err)
	}
}

// errorsIsUpstream is a tiny local helper so the test file needs no
// extra import; QueryVector wraps ErrUpstream with fmt.Errorf("%w").
func errorsIsUpstream(err error) bool {
	return err != nil && strings.Contains(err.Error(), "all super-regions failed")
}
```

- [ ] **Step 7: Run the full vm package test suite**

Run: `go test ./internal/vm/ -v`
Expected: PASS for all tests (Defaults, Union, FallsBack, AllErrored, Miss, NoCreds warn, TrimsTrailingSlash).

- [ ] **Step 8: Commit**

```bash
git add internal/vm/
git commit -m "qac: add internal/vm — super-region fan-out client with vmui fallback"
```

---

## Task 2: Refactor `inventoryclient.VMResolver` onto `*vm.Client`

**Files:**
- Modify: `internal/inventoryclient/vm_resolver.go`
- Test: `internal/inventoryclient/vm_resolver_test.go`

- [ ] **Step 1: Update the test helper and delete the two relocated tests**

In `internal/inventoryclient/vm_resolver_test.go`, add `"qac/internal/vm"` to the import block, then replace the tail of `newVMTestServer` (the `t.Setenv` + `NewVMResolver(srv.URL)` lines, currently lines ~63-66) so the function ends like this:

```go
	t.Cleanup(srv.Close)
	r := NewVMResolver(vm.New(vm.Config{
		AuthedURLs: map[string]string{"test": srv.URL},
		Username:   "u",
		Password:   "p",
		HTTPClient: srv.Client(),
	}))
	return r, srv
}
```

Delete the entire `TestNewVMResolver_MissingCredsReturnsErrAuth` function (it asserted `ErrAuth`, which no longer exists — unauthed mode is now covered by `internal/vm`).

Delete the entire `TestNewVMResolver_TrimsTrailingSlash` function (it read `r.baseURL`, a field that no longer exists — trailing-slash trimming is now covered by `internal/vm`'s `TestNew_TrimsTrailingSlash`).

In the query-anchor test (around line 182-190), replace its construction:

```go
	t.Cleanup(srv.Close)
	r := NewVMResolver(vm.New(vm.Config{
		AuthedURLs: map[string]string{"test": srv.URL},
		Username:   "u",
		Password:   "p",
		HTTPClient: srv.Client(),
	}))
	_, _ = r.Lookup(context.Background(), "dh1000-r003-ca-east-01a")
```

(The `errors` import stays — it is still used by the `ErrNotFound` assertions at lines ~164/169.)

- [ ] **Step 2: Run the inventory tests to confirm they fail to compile**

Run: `go test ./internal/inventoryclient/ -run TestVMResolver -v`
Expected: FAIL — build error: `NewVMResolver` still takes a `string`, and references to removed fields. This proves the tests now exercise the new shape.

- [ ] **Step 3: Rewrite `vm_resolver.go` to delegate to `*vm.Client`**

In `internal/inventoryclient/vm_resolver.go`:

Replace the import block with:

```go
import (
	"context"
	"fmt"
	"sort"
	"strings"

	"qac/internal/vm"
)
```

Delete the `ErrAuth` var and its doc comment (lines ~17-21).

Replace the `VMResolver` struct and `NewVMResolver` (lines ~39-65) with:

```go
// VMResolver is the production Resolver: it answers Lookup / ResolveBMNs
// from VictoriaMetrics instead of the in-memory demo map. It runs a
// single PromQL instant query per rack (matching every node-class
// deviceslot in the rack) via the shared super-region fan-out client and
// shapes the result two ways — a rack-kind Hit carrying the dominant
// hardware labelset (for ResolveRack facts) and one BMN per deviceslot
// (for ResolveBMNs).
type VMResolver struct {
	vm *vm.Client
}

// compile-time assertion that VMResolver satisfies the Resolver contract.
var _ Resolver = (*VMResolver)(nil)

// NewVMResolver returns a Resolver backed by the shared vm.Client. The
// client owns endpoint selection, basic-auth, and the unauthed vmui
// fallback; this resolver only renders the per-rack query and shapes the
// response.
func NewVMResolver(c *vm.Client) *VMResolver {
	return &VMResolver{vm: c}
}
```

Replace the body of `queryRack` (its current implementation calls `r.queryVector`) with the vm-client call:

```go
// queryRack issues the single instant query that matches every
// node-class deviceslot in the rack and returns each series' label map,
// fanned out across all super-regions by the shared vm.Client.
func (r *VMResolver) queryRack(ctx context.Context, rackID string) ([]map[string]string, error) {
	prefix, suffix, ok := splitRackID(rackID)
	if !ok {
		// Malformed rack id can't match any deviceslot — treat as a
		// clean miss rather than an error so callers surface
		// "unresolved" instead of "internal".
		return nil, nil
	}
	// Anchor on the deviceslot shape <prefix>-node-<N>-<suffix>. The
	// node infix excludes CDU / power-shelf / NVLink slots, leaving only
	// compute trays.
	q := fmt.Sprintf(`%s{deviceslot=~"^%s-node-[0-9]+-%s$"}`, invMetric, prefix, suffix)
	return r.vm.QueryVector(ctx, q)
}
```

Delete the now-dead HTTP plumbing in this file: the `vmResponse` type (lines ~163-172) and the `queryVector` method (lines ~174-208). Everything else (`Lookup`, `ResolveBMNs`, `dominantLabels`, `splitRackID`, `deriveVariant`, `deriveGBGeneration`) stays byte-for-byte.

- [ ] **Step 4: Run inventory tests + vet**

Run: `go test ./internal/inventoryclient/ -v && go vet ./internal/inventoryclient/`
Expected: PASS for all resolver tests; vet clean (no unused imports).

- [ ] **Step 5: Commit**

```bash
git add internal/inventoryclient/vm_resolver.go internal/inventoryclient/vm_resolver_test.go
git commit -m "qac: route inventory VMResolver through shared internal/vm client"
```

---

## Task 3: Refactor `lifecycleclient.PromClient` onto `*vm.Client`

**Files:**
- Modify: `internal/lifecycleclient/promclient.go`
- Modify: `internal/lifecycleclient/types.go`
- Test: `internal/lifecycleclient/promclient_test.go`

- [ ] **Step 1: Rewrite the test construction sites**

In `internal/lifecycleclient/promclient_test.go`:

Change the import block: remove `"errors"` (its only use is the creds test, deleted below) and add `"qac/internal/vm"`. The block becomes:

```go
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"qac/internal/vm"
)
```

Add this helper near the top of the file (after the imports):

```go
// newTestPromClient builds a PromClient whose single "test" super-region
// points at srv, with creds present so the authed path is exercised.
func newTestPromClient(srv *httptest.Server) *PromClient {
	return NewPromClient(vm.New(vm.Config{
		AuthedURLs: map[string]string{"test": srv.URL},
		Username:   "u",
		Password:   "p",
		HTTPClient: srv.Client(),
	}))
}
```

Replace the construction in `TestPromClient_QueryRLCCIgnored_HappyPath` (the `t.Setenv(...)` pair + `c, err := NewPromClient(srv.URL)` + the `if err != nil` block) with:

```go
	c := newTestPromClient(srv)
```

Delete the entire `TestNewPromClient_MissingCredsReturnsErrAuth` function.

In `TestPromClient_QueryActionState_BuildsQueryAndAuth`, replace the `t.Setenv(...)` pair and the `c, err := NewPromClient(srv.URL)` + `if err != nil` block with:

```go
	c := newTestPromClient(srv)
```

(The handler's basic-auth assertion `gotAuth != ""` still holds: creds are present, so the authed path is used and never falls back.)

In `TestPromClient_QueryFLCCState`, replace the literal construction `c := &PromClient{baseURL: srv.URL, httpDo: http.DefaultClient.Do, user: "u", pass: "p"}` with:

```go
	c := newTestPromClient(srv)
```

In `TestPromClient_ServerError_ReturnsErr`, replace the `t.Setenv(...)` pair and `c, _ := NewPromClient(srv.URL)` with:

```go
	c := newTestPromClient(srv)
```

(The single endpoint returns 500, so all-errored → `QueryVector` returns `ErrUpstream`; the existing `if err == nil { t.Errorf(...) }` assertion still passes.)

- [ ] **Step 2: Run lifecycle tests to confirm they fail to compile**

Run: `go test ./internal/lifecycleclient/ -run TestPromClient -v`
Expected: FAIL — build error: `NewPromClient` still takes a `string` and returns `(…, error)`; `&PromClient{baseURL:…}` references removed fields.

- [ ] **Step 3: Rewrite `promclient.go` to delegate to `*vm.Client`**

In `internal/lifecycleclient/promclient.go`:

Replace the import block with:

```go
import (
	"context"
	"fmt"

	"qac/internal/vm"
)
```

Replace the `PromClient` struct and `NewPromClient` (lines ~17-39) with:

```go
// PromClient is the production lifecycleclient backend. It renders
// lifecycle PromQL and runs it through the shared super-region fan-out
// client (internal/vm), which owns endpoint selection, basic-auth, and
// the unauthed vmui fallback.
type PromClient struct {
	vm *vm.Client
}

// NewPromClient returns a PromClient backed by the shared vm.Client.
func NewPromClient(c *vm.Client) *PromClient {
	return &PromClient{vm: c}
}
```

Keep the `promResponse`? No — it is no longer used (decoding moved to `internal/vm`). Delete the `promResponse` type (lines ~42-51). Keep the `promSample` type (still used by the Query* methods).

Replace the `queryVector` method and the `withTimeout` helper (lines ~118-160) with a thin adapter over the vm client:

```go
// queryVector runs q through the shared vm.Client fan-out and adapts the
// returned label maps to promSample (the sample value is unused by every
// caller, so an empty-value sample is sufficient).
func (c *PromClient) queryVector(ctx context.Context, q string) ([]promSample, error) {
	series, err := c.vm.QueryVector(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]promSample, 0, len(series))
	for _, m := range series {
		out = append(out, promSample{Metric: m})
	}
	return out, nil
}
```

`QueryRLCCIgnored`, `QueryActionState`, and `QueryFLCCState` are unchanged — they already call `c.queryVector` and read `.Metric`. (`fmt` stays: used by the `fmt.Errorf` wraps in `QueryActionState` / `QueryFLCCState`.)

- [ ] **Step 4: Remove the now-unused `ErrAuth` from `types.go`**

In `internal/lifecycleclient/types.go`, delete the `ErrAuth` var and its doc comment (lines ~23-27). If `"errors"` is now unused in that file, remove it from the import block.

Run: `go build ./internal/lifecycleclient/`
Expected: build succeeds; if it complains about an unused `"errors"` import in `types.go`, remove that import line and rebuild.

- [ ] **Step 5: Run lifecycle tests + vet**

Run: `go test ./internal/lifecycleclient/ -v && go vet ./internal/lifecycleclient/`
Expected: PASS for all PromClient tests; vet clean.

- [ ] **Step 6: Commit**

```bash
git add internal/lifecycleclient/promclient.go internal/lifecycleclient/types.go internal/lifecycleclient/promclient_test.go
git commit -m "qac: route lifecycle PromClient through shared internal/vm client"
```

---

## Task 4: Wire `serve.go` to the shared client + flip the flag default

**Files:**
- Modify: `cmd/qac/serve.go`

- [ ] **Step 1: Add the vm import**

In `cmd/qac/serve.go`, add `"qac/internal/vm"` to the import block (alongside the other `qac/internal/...` imports).

- [ ] **Step 2: Build one shared `*vm.Client` and rewrite the two backend-selection blocks**

Replace the inventory + lifecycle backend-selection blocks (the current lines ~119-168, from the `// Inventory backend selection.` comment through the end of the `vmBackend` switch) with:

```go
				// Shared VM client: fans out across all super-regions with
				// per-request vmui fallback. Creds present → authed VMauth;
				// creds absent → unauthed vmui (real data) + a one-time
				// warning. A non-empty --vm-url pins a single authed
				// "override" super-region (testing / escape hatch); empty
				// uses the built-in four-region maps. One client is shared by
				// the inventory resolver and the lifecycle client.
				vmCfg := vm.Config{
					Username: os.Getenv("AWXCTL_VMAUTH_USERNAME"),
					Password: os.Getenv("AWXCTL_VMAUTH_PASSWORD"),
				}
				if vmBaseURL != "" {
					vmCfg.AuthedURLs = map[string]string{"override": vmBaseURL}
				}
				vmClient := vm.New(vmCfg)

				// Inventory backend selection. QAC_INV_BACKEND=map forces the
				// demo MapResolver; otherwise the VMResolver resolves any real
				// rack from VictoriaMetrics via the shared fan-out client.
				invBackend := os.Getenv("QAC_INV_BACKEND")
				var resolver inventoryclient.Resolver
				if invBackend == "map" {
					slog.Info("using demo inventory map per QAC_INV_BACKEND=map")
					resolver = inventoryclient.NewMapResolverWithBMNs(inventoryclient.SeedDemoFixtures(), inventoryclient.SeedDemoBMNs())
				} else {
					resolver = inventoryclient.NewVMResolver(vmClient)
					slog.Info("using vm backend for inventory resolution")
				}

				// Lifecycle backend selection. QAC_VM_BACKEND=map forces the
				// demo MapClient; otherwise the PromClient queries live
				// lifecycle state via the same shared fan-out client.
				vmBackend := os.Getenv("QAC_VM_BACKEND")
				var lifeC lifecycleclient.Client
				if vmBackend == "map" {
					slog.Info("using map backend per QAC_VM_BACKEND=map")
					lifeC = lifecycleclient.NewMapClient(nil)
				} else {
					lifeC = lifecycleclient.NewPromClient(vmClient)
					slog.Info("using prom backend for lifecycle queries")
				}
```

(Note: this removes both `errors.Is(err, …ErrAuth)` branches. The `"errors"` import is still used elsewhere in `serve.go` — `errors.Is(err, http.ErrServerClosed)` in the shutdown select — so leave the import in place.)

- [ ] **Step 3: Flip the `--vm-url` default to empty and update its help/comment**

Replace the flag registration block (current lines ~241-247) with:

```go
	// --vm-url is an optional single-endpoint override: when set, qac
	// pins one authed "override" super-region instead of the built-in
	// four-region maps. Empty (the default) uses the built-in maps and
	// fans out across us-east / us-west / eu-south / us-lab, with an
	// unauthed vmui fallback when VMauth creds are absent or a query
	// fails. VMauth is plain HTTP (an https:// scheme fails TLS).
	cmd.Flags().StringVar(&vmBaseURL, "vm-url", "", "VictoriaMetrics single-endpoint override (default: built-in super-region maps)")
```

- [ ] **Step 4: Build the whole module**

Run: `go build ./...`
Expected: build succeeds with no errors.

- [ ] **Step 5: Run the full Go test suite + vet**

Run: `go test ./... && go vet ./...`
Expected: all packages PASS; vet clean.

- [ ] **Step 6: Manual smoke — boot with no creds (unauthed-mode warning) then health-check**

Run:

```bash
unset AWXCTL_VMAUTH_USERNAME AWXCTL_VMAUTH_PASSWORD
go run ./cmd/qac serve --addr 127.0.0.1:8099 --db /tmp/qac-fanout-smoke.db &
SMOKE_PID=$!
sleep 2
curl -s http://127.0.0.1:8099/api/health
kill $SMOKE_PID
```

Expected: stderr contains `vm: no VMauth credentials (...); falling back to unauthed vmui endpoints` exactly once; `/api/health` returns its healthy JSON body; process shuts down cleanly. (Real rack resolution may still fail without network/creds — that's fine; this only verifies the wiring boots and warns.)

- [ ] **Step 7: Commit**

```bash
git add cmd/qac/serve.go
git commit -m "qac: serve — share one super-region vm.Client; --vm-url now an override"
```

---

## Self-Review

**Spec coverage:**
- Decision 1 (qac-owned `internal/vm`, both clients route through it) → Tasks 1-3. ✓
- Decision 2 (4-region table, both maps) → Task 1 `Defaults()` + `TestDefaults_FourSuperRegionsBoth`. ✓
- Decision 3 (always fan out, union, no regionToSuper) → Task 1 `QueryVector` + `TestQueryVector_UnionAcrossSuperRegions`; `ErrUpstream` vs clean miss in `TestQueryVector_AllErroredReturnsErrUpstream` / `TestQueryVector_MissDistinctFromOutage`. ✓
- Decision 4 (creds-missing → vmui + one-time warning; per-request authed-failure → vmui retry; demo map only via `*_BACKEND=map`) → Task 1 `queryEndpoint` + `TestQueryVector_AuthedFailsFallsBackToVMUI` + `TestNew_NoCredsUsesUnauthedAndWarnsOnce`; Task 4 collapses the demo-map fallback to explicit overrides. ✓
- `ErrAuth` removal → Task 2 (inventory), Task 3 (lifecycle types.go). ✓
- `--vm-url` override + default flip → Task 4. ✓
- Out-of-scope honored: no `baremetal_node_physical_topology_labels` work; no rack-id regex change. ✓

**Type consistency:** `vm.Config` fields (`AuthedURLs`, `UnauthedURLs`, `Username`, `Password`, `HTTPClient`) are identical across Tasks 1-4. `NewVMResolver(*vm.Client)` and `NewPromClient(*vm.Client)` signatures match their call sites in Task 4. `QueryVector(ctx, q) ([]map[string]string, error)` is the single primitive both `queryRack` (Task 2) and `queryVector` (Task 3) consume. `ErrUpstream` defined once (Task 1), asserted via substring helper (no cross-package import needed).

**Placeholder scan:** No TBD/TODO; every code step shows complete code; every run step shows the command and expected result.
