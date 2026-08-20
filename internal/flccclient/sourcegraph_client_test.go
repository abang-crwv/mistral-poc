package flccclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"qac/internal/sourcegraph"
)

func TestSourcegraphClient_GetWorkflow(t *testing.T) {
	body := `
flcc:
  config:
    workflows:
      - name: gb200-rack-provision-v4
        class: rack-provision
        phases:
          - {name: node-vaultify, state: node-vaultify, failState: fail, timeout: 1h}
          - {name: l10-test-loop, state: l10-test-loop}
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Sourcegraph-Resolved-Revision", "sha-flcc-1")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	t.Setenv("AWXCTL_SOURCEGRAPH_TOKEN", "tok")
	sg, _ := sourcegraph.NewClient(srv.URL)
	c := NewSourcegraphClient(sg)
	g, src, err := c.GetWorkflow(context.Background(), "gb200-rack-provision-v4")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if g.TerminalPhase != "l10-test-loop" {
		t.Errorf("TerminalPhase = %q", g.TerminalPhase)
	}
	if src.SHA != "sha-flcc-1" {
		t.Errorf("SHA = %q", src.SHA)
	}
}

func TestSourcegraphClient_TerminalPhase(t *testing.T) {
	body := `
flcc:
  config:
    workflows:
      - name: ready
        class: node-provision
        phases:
          - {name: ready, state: ready}
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	t.Setenv("AWXCTL_SOURCEGRAPH_TOKEN", "tok")
	sg, _ := sourcegraph.NewClient(srv.URL)
	c := NewSourcegraphClient(sg)
	got, err := c.TerminalPhase(context.Background(), "ready")
	if err != nil {
		t.Fatalf("TerminalPhase: %v", err)
	}
	if got != "ready" {
		t.Errorf("TerminalPhase = %q, want ready", got)
	}
}
