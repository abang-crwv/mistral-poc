package sourcegraph

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func httpBody(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

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
	r := &http.Response{StatusCode: 200, Body: http.NoBody, Header: http.Header{"X-Sourcegraph-Resolved-Revision": {"deadbeef"}}}
	r.Body = httpBody(body)
	return r
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
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if string(res.Body) != "body-1" {
		t.Fatalf("body = %q", res.Body)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	// A NEW client over the same dir must serve from disk without a network call.
	c2 := newTestClient(t, dir, func(_ *http.Request) (*http.Response, error) { t.Fatal("should not hit network"); return nil, nil })
	res2, err := c2.Fetch(context.Background(), "repo", "p.yaml", "main")
	if err != nil {
		t.Fatalf("fetch c2: %v", err)
	}
	if string(res2.Body) != "body-1" {
		t.Fatalf("disk body = %q", res2.Body)
	}
}

func TestOfflineFallbackServesStale(t *testing.T) {
	dir := t.TempDir()
	c1 := newTestClient(t, dir, func(_ *http.Request) (*http.Response, error) {
		r := okResp("")
		r.Body = httpBody("cached")
		return r, nil
	})
	if _, err := c1.Fetch(context.Background(), "repo", "p.yaml", "main"); err != nil {
		t.Fatal(err)
	}

	// New client, expired entry, network down → must serve the stale disk copy.
	c2 := newTestClient(t, dir, func(_ *http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	c2.cache.expireAll() // force stale-old
	res, err := c2.Fetch(context.Background(), "repo", "p.yaml", "main")
	if err != nil {
		t.Fatalf("offline fetch should serve stale, got %v", err)
	}
	if string(res.Body) != "cached" {
		t.Fatalf("stale body = %q", res.Body)
	}
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
	c := newTestClient(t, dir, func(_ *http.Request) (*http.Response, error) {
		n++
		r := okResp("")
		r.Body = httpBody("v")
		return r, nil
	})
	if _, err := c.Fetch(context.Background(), "repo", "p.yaml", "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Refresh(context.Background(), "repo", "p.yaml", "main"); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("Refresh should bypass fresh cache; network calls = %d, want 2", n)
	}
	_ = filepath.Join(dir, "x") // dir used
}
