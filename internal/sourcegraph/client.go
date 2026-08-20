// Package sourcegraph is a low-level HTTP client for fetching raw files from
// CoreWeave's Sourcegraph instance. It is shared by internal/rlccclient and
// internal/flccclient, and intentionally knows nothing about RLCC/FLCC
// semantics — it returns bytes plus the resolved commit SHA.
package sourcegraph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ErrNoToken is returned by NewClient when AWXCTL_SOURCEGRAPH_TOKEN is unset.
// Callers (cmd/qac/serve.go) treat this as a signal to fall through to a
// MapClient backend rather than a fatal startup error.
var ErrNoToken = errors.New("AWXCTL_SOURCEGRAPH_TOKEN unset")

// SourceRef is the (chart_tag, sha) pair returned alongside fetched content.
// ChartTag is set by callers from their own knowledge of the chart version
// pinned in testdata (sourcegraph itself does not know about chart tags).
type SourceRef struct {
	ChartTag string `json:"chart_tag"`
	SHA      string `json:"sha"`
}

// FetchResult is the body + resolved SHA returned by Fetch.
type FetchResult struct {
	Body []byte
	SHA  string
}

// Client is the shared raw-file fetcher. baseURL is the Sourcegraph root
// (e.g. "https://sourcegraph.coreweave.com") with no trailing slash. token is
// the bearer token sent on every request. httpDo is swappable for tests.
type Client struct {
	token    string
	baseURL  string
	httpDo   func(*http.Request) (*http.Response, error)
	cache    *ttlCache
	cacheDir string
}

// NewClient constructs a Client. The token is read from
// AWXCTL_SOURCEGRAPH_TOKEN at construction time; an empty value returns
// ErrNoToken. baseURL has any trailing slash trimmed so call sites can
// concatenate paths without doubled "/".
func NewClient(baseURL string) (*Client, error) {
	tok := os.Getenv("AWXCTL_SOURCEGRAPH_TOKEN")
	if tok == "" {
		return nil, ErrNoToken
	}
	return &Client{
		token:   tok,
		baseURL: strings.TrimRight(baseURL, "/"),
		httpDo:  http.DefaultClient.Do,
		cache:   newTTLCache(5 * time.Minute),
	}, nil
}

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

// Has reports whether a (possibly stale) entry exists for the key (used by the
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

// Fetch returns the raw file body at repo@ref/path. It consults the TTL cache
// first: fresh entries are returned immediately; stale-young entries are
// returned with a background refresh scheduled; stale-old or missing entries
// block for a synchronous network fetch.
func (c *Client) Fetch(ctx context.Context, repo, path, ref string) (FetchResult, error) {
	key := repo + "@" + ref + "/" + path
	now := time.Now()
	cached, status := c.cache.lookup(key, now)
	switch status {
	case cacheFresh:
		return cached, nil
	case cacheStaleYoung:
		c.cache.scheduleRefresh(key, func() (FetchResult, error) {
			return c.fetchNetwork(context.Background(), repo, path, ref)
		})
		return cached, nil
	}
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
}

func (c *Client) fetchNetwork(ctx context.Context, repo, path, ref string) (FetchResult, error) {
	// Sourcegraph raw-file endpoint: <base>/<repo>@<ref>/-/raw/<path> returns
	// the file bytes (the /-/blob/ form returns HTML). http.DefaultClient
	// follows the redirect to the resolved revision. Matches the proven
	// awxctl fetch-awx-catalog.sh form (no /.api/repos/ prefix).
	url := fmt.Sprintf("%s/%s@%s/-/raw/%s", c.baseURL, repo, ref, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.token)
	resp, err := c.httpDo(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return FetchResult{}, fmt.Errorf("sourcegraph %s: status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return FetchResult{}, fmt.Errorf("read body: %w", err)
	}
	sha := resp.Header.Get("X-Sourcegraph-Resolved-Revision")
	if sha == "" {
		// Header was absent — fall through to GraphQL. Best effort; on
		// failure we return the body with an empty SHA so the caller can
		// degrade quietly.
		sha = c.resolveSHAViaGraphQL(ctx, repo, ref)
	}
	return FetchResult{Body: body, SHA: sha}, nil
}

// PurgeCache invalidates the whole TTL cache so the next Fetch hits the
// live API. Called by /api/admin/sourcegraph/refresh.
func (c *Client) PurgeCache() {
	c.cache.Purge()
}

func (c *Client) resolveSHAViaGraphQL(ctx context.Context, repo, ref string) string {
	query := fmt.Sprintf(
		`{"query":"query{repository(name:%q){commit(rev:%q){oid}}}"}`,
		repo, ref,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/.api/graphql", strings.NewReader(query))
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpDo(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.ReadAll(io.LimitReader(resp.Body, 512))
		return ""
	}
	var out struct {
		Data struct {
			Repository struct {
				Commit struct {
					OID string `json:"oid"`
				} `json:"commit"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ""
	}
	return out.Data.Repository.Commit.OID
}
