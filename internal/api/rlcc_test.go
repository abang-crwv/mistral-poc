package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"qac/internal/rlccclient"
)

func TestListRLCCWorkflowsHandler_HappyPath(t *testing.T) {
	c := rlccclient.NewMapClient()
	srv := httptest.NewServer(listRLCCWorkflowsHandler(c, false))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Qac-Degraded"); got != "true" {
		t.Errorf("X-Qac-Degraded = %q, want true (MapClient backend)", got)
	}
	var body struct {
		Workflows []rlccclient.WorkflowSummary `json:"workflows"`
		Source    rlccclient.SourceRef         `json:"source"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Workflows) == 0 {
		t.Fatal("workflows empty")
	}
	if body.Source.ChartTag == "" {
		t.Errorf("source.chart_tag empty")
	}
}

func TestGetRLCCWorkflowHandler_HappyPath(t *testing.T) {
	c := rlccclient.NewMapClient()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/rlcc/workflows/{name}", getRLCCWorkflowHandler(c, false))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/rlcc/workflows/gb200-rack-bringup-v4")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Workflow rlccclient.WorkflowGraph `json:"workflow"`
		Source   rlccclient.SourceRef     `json:"source"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Workflow.Name != "gb200-rack-bringup-v4" {
		t.Errorf("Name = %q", body.Workflow.Name)
	}
}

func TestGetRLCCWorkflowHandler_Unknown(t *testing.T) {
	c := rlccclient.NewMapClient()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/rlcc/workflows/{name}", getRLCCWorkflowHandler(c, false))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/rlcc/workflows/no-such-thing")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	var body errorBody
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Error.Code != codeNotFound {
		t.Errorf("error.code = %q, want %q", body.Error.Code, codeNotFound)
	}
}
