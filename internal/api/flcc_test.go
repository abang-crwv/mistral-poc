package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"qac/internal/flccclient"
)

func TestGetFLCCWorkflowHandler_HappyPath(t *testing.T) {
	c := flccclient.NewMapClient()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/flcc/workflows/{name}", getFLCCWorkflowHandler(c, false))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/flcc/workflows/gb200-rack-provision-v4")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Workflow flccclient.FLCCWorkflowGraph `json:"workflow"`
		Source   flccclient.SourceRef         `json:"source"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Workflow.TerminalPhase != "l10-test-loop" {
		t.Errorf("TerminalPhase = %q", body.Workflow.TerminalPhase)
	}
}

func TestGetFLCCWorkflowHandler_Unknown(t *testing.T) {
	c := flccclient.NewMapClient()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/flcc/workflows/{name}", getFLCCWorkflowHandler(c, false))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/flcc/workflows/no-such-thing")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}
