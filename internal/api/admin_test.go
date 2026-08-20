package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefreshSourcegraphHandler_CallsPurge(t *testing.T) {
	var purged bool
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/admin/sourcegraph/refresh", refreshSourcegraphHandler(func() {
		purged = true
	}))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/admin/sourcegraph/refresh", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if !purged {
		t.Error("refresh handler did not call purge")
	}
	var body struct {
		Invalidated []string `json:"invalidated"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Invalidated) != 2 {
		t.Errorf("invalidated len = %d, want 2", len(body.Invalidated))
	}
}
