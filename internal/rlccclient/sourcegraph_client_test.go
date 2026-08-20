package rlccclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"qac/internal/sourcegraph"
)

func TestSourcegraphClient_ListWorkflows(t *testing.T) {
	body := `
rlcc:
  workflows:
    - name: gb200-rack-bringup-v4
      lifecycle: stable
      desired-state: ready
      actions:
        - {name: a1, handler: await-checkpoint}
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Sourcegraph-Resolved-Revision", "sha-rlcc-1")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	t.Setenv("AWXCTL_SOURCEGRAPH_TOKEN", "tok")
	sg, err := sourcegraph.NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c := NewSourcegraphClient(sg)
	sums, src, err := c.ListWorkflows(context.Background())
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(sums) != 1 || sums[0].Name != "gb200-rack-bringup-v4" {
		t.Errorf("sums = %+v", sums)
	}
	if src.SHA != "sha-rlcc-1" {
		t.Errorf("SHA = %q, want sha-rlcc-1", src.SHA)
	}
}

func TestSourcegraphClient_GetWorkflow_Unknown(t *testing.T) {
	body := `
rlcc:
  workflows:
    - name: only-one
      lifecycle: stable
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	t.Setenv("AWXCTL_SOURCEGRAPH_TOKEN", "tok")
	sg, _ := sourcegraph.NewClient(srv.URL)
	c := NewSourcegraphClient(sg)
	_, _, err := c.GetWorkflow(context.Background(), "no-such-thing")
	if err == nil {
		t.Fatal("GetWorkflow(unknown): want error")
	}
}
