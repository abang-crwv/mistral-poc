package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"qac/internal/engine"
	"qac/internal/store"
)

// seedPendingRun appends a RunCreated event, which the store projects into a
// 'pending' run — the cancellable precondition.
func seedPendingRun(t *testing.T, s *store.Store, runID string) {
	t.Helper()
	payload := []byte(`{"template_id":"demo","created_by":"wpena","inputs":{"bundle_tag":"b-1","canary_racks":["dh1-r001-us-east-01a"]}}`)
	if err := s.AppendEvent(context.Background(), runID, engine.KindRunCreated, payload); err != nil {
		t.Fatalf("seed RunCreated: %v", err)
	}
}

func postCancel(t *testing.T, url, runID string) (int, store.Run) {
	t.Helper()
	res, err := http.Post(url+"/api/runs/"+runID+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST cancel: %v", err)
	}
	defer res.Body.Close()
	var body struct {
		Run   store.Run `json:"run"`
		Error *struct {
			Code, Message string
		} `json:"error"`
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	return res.StatusCode, body.Run
}

func TestCancelRun_PendingRun_BecomesCancelled(t *testing.T) {
	srv, s, _ := newTestServer(t)
	runID := "01HZX0000000000000000CAN1"
	seedPendingRun(t, s, runID)

	code, run := postCancel(t, srv.URL, runID)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if run.Status != "cancelled" {
		t.Errorf("run.Status = %q, want cancelled", run.Status)
	}

	// The terminal status is durable: a second cancel is rejected.
	code2, _ := postCancel(t, srv.URL, runID)
	if code2 != http.StatusUnprocessableEntity {
		t.Errorf("second cancel status = %d, want 422 (already terminal)", code2)
	}
}

func TestCancelRun_UnknownRun_Returns404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	code, _ := postCancel(t, srv.URL, "01HZX0000000000000000NONE")
	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}
