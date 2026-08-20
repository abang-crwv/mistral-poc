package sourcegraph

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClient_ReadsTokenFromEnv(t *testing.T) {
	t.Setenv("AWXCTL_SOURCEGRAPH_TOKEN", "tok-abc")
	c, err := NewClient("https://sourcegraph.coreweave.com")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.token != "tok-abc" {
		t.Errorf("token = %q, want %q", c.token, "tok-abc")
	}
}

func TestNewClient_MissingTokenReturnsErrNoToken(t *testing.T) {
	t.Setenv("AWXCTL_SOURCEGRAPH_TOKEN", "")
	_, err := NewClient("https://sourcegraph.coreweave.com")
	if !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}
}

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	t.Setenv("AWXCTL_SOURCEGRAPH_TOKEN", "tok")
	c, err := NewClient("https://sourcegraph.coreweave.com/")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if strings.HasSuffix(c.baseURL, "/") {
		t.Errorf("baseURL = %q, want no trailing slash", c.baseURL)
	}
}

func TestFetch_HeaderPath_ReturnsBodyAndSHA(t *testing.T) {
	want := "workflows:\n  - name: gb200-rack-bringup-v4\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token tok-abc" {
			t.Errorf("Authorization = %q, want %q", got, "token tok-abc")
		}
		if got, want := r.URL.Path, "/github.com/coreweave/rack-lifecycle-controller@main/-/raw/chart/values.yaml"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		w.Header().Set("X-Sourcegraph-Resolved-Revision", "abc123def")
		_, _ = w.Write([]byte(want))
	}))
	defer srv.Close()
	t.Setenv("AWXCTL_SOURCEGRAPH_TOKEN", "tok-abc")
	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	res, err := c.Fetch(context.Background(), "github.com/coreweave/rack-lifecycle-controller", "chart/values.yaml", "main")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(res.Body) != want {
		t.Errorf("Body = %q, want %q", string(res.Body), want)
	}
	if res.SHA != "abc123def" {
		t.Errorf("SHA = %q, want abc123def", res.SHA)
	}
}

func TestFetch_404ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("repo not found"))
	}))
	defer srv.Close()
	t.Setenv("AWXCTL_SOURCEGRAPH_TOKEN", "tok-abc")
	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = c.Fetch(context.Background(), "github.com/missing/repo", "x.yaml", "main")
	if err == nil {
		t.Fatal("Fetch: want error, got nil")
	}
}

func TestFetch_GraphQLFallback_WhenHeaderAbsent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/-/raw/"):
			// no X-Sourcegraph-Resolved-Revision header
			_, _ = w.Write([]byte("ok"))
		case r.URL.Path == "/.api/graphql":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"repository":{"commit":{"oid":"def456"}}}}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("AWXCTL_SOURCEGRAPH_TOKEN", "tok-abc")
	c, _ := NewClient(srv.URL)
	res, err := c.Fetch(context.Background(), "github.com/coreweave/rack-lifecycle-controller", "chart/values.yaml", "main")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(res.Body) != "ok" {
		t.Errorf("Body = %q, want ok", string(res.Body))
	}
	if res.SHA != "def456" {
		t.Errorf("SHA = %q, want def456 from GraphQL fallback", res.SHA)
	}
}

func TestFetch_GraphQLFallbackError_DoesNotFailFetch(t *testing.T) {
	// If the header is missing AND GraphQL fails, we still return the body;
	// SHA is left empty. Callers can decide whether to treat empty SHA as
	// fatal. Iter-5a treats it as "OK, degrade quietly" — the run will not
	// be pinnable, but the wizard render shouldn't fail.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.api/graphql" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("body"))
	}))
	defer srv.Close()
	t.Setenv("AWXCTL_SOURCEGRAPH_TOKEN", "tok-abc")
	c, _ := NewClient(srv.URL)
	res, err := c.Fetch(context.Background(), "any/repo", "x.yaml", "main")
	if err != nil {
		t.Fatalf("Fetch should not error when GraphQL fails: %v", err)
	}
	if string(res.Body) != "body" {
		t.Errorf("Body = %q, want body", string(res.Body))
	}
	if res.SHA != "" {
		t.Errorf("SHA = %q, want empty", res.SHA)
	}
}
