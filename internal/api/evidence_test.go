package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"qac/internal/store"
)

func seedEvidence(t *testing.T, s *store.Store, runID, evID string) {
	t.Helper()
	ctx := context.Background()
	// Need a run row first.
	_, _ = s.DB().ExecContext(ctx, `
		INSERT INTO runs (id, template_id, bundle, created_at, updated_at, status, rack, created_by, inputs)
		VALUES (?, ?, '', 0, 0, 'pending', '', 'wpena', '{}')`,
		runID, "firmware-release-canary")
	if err := s.PutEvidence(ctx, store.Evidence{
		ID:          evID,
		RunID:       runID,
		StepID:      "alert_baseline",
		ContentType: "application/json",
		Payload:     []byte(`{"alerts_clear":true}`),
	}); err != nil {
		t.Fatalf("PutEvidence: %v", err)
	}
}

func TestEvidence_Happy_200JSON(t *testing.T) {
	srv, s, _ := newTestServer(t)
	runID := "01HZX0000000000000000RUN1"
	evID := "01HZXEVIDENCE00000000001"
	seedEvidence(t, s, runID, evID)

	res, err := http.Get(srv.URL + "/api/runs/" + runID + "/evidence/" + evID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	body, _ := io.ReadAll(res.Body)
	if string(body) != `{"alerts_clear":true}` {
		t.Errorf("body = %s, want {\"alerts_clear\":true}", body)
	}
}

func TestEvidence_UnknownEid_404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	res, err := http.Get(srv.URL + "/api/runs/01HZX0000000000000000RUN1/evidence/ghost")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
}

func TestEvidence_CrossRun_403(t *testing.T) {
	srv, s, _ := newTestServer(t)
	runID := "01HZX0000000000000000RUN1"
	otherRun := "01HZX0000000000000000RUN2"
	evID := "01HZXEVIDENCE00000000001"
	seedEvidence(t, s, runID, evID)
	// Also seed a second run row so the URL's run_id is plausible.
	_, _ = s.DB().ExecContext(context.Background(), `
		INSERT INTO runs (id, template_id, bundle, created_at, updated_at, status, rack, created_by, inputs)
		VALUES (?, ?, '', 0, 0, 'pending', '', 'wpena', '{}')`,
		otherRun, "firmware-release-canary")

	res, err := http.Get(srv.URL + "/api/runs/" + otherRun + "/evidence/" + evID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("status = %d, want 403", res.StatusCode)
	}
}

// === GET /api/runs/{id}/evidence (list) — iter-5j ===

// seedEvidenceList seeds a run with 3 evidence rows across two steps:
//   - step1: 1 row with deviceslot, 1 row without
//   - step2: 1 row with deviceslot
func seedEvidenceList(t *testing.T, s *store.Store, runID string) {
	t.Helper()
	ctx := context.Background()
	_, _ = s.DB().ExecContext(ctx, `
		INSERT INTO runs (id, template_id, bundle, created_at, updated_at, status, rack, created_by, inputs)
		VALUES (?, ?, '', 0, 0, 'pending', '', 'wpena', '{}')`,
		runID, "firmware-release-canary")

	ds1 := "rack-node-01"
	ds2 := "rack-node-02"
	if err := s.PutEvidence(ctx, store.Evidence{
		ID:          "01HZXEVLIST000000000001",
		RunID:       runID,
		StepID:      "alert_baseline",
		ContentType: "application/json",
		Payload:     []byte(`{"ok":true}`),
		Deviceslot:  &ds1,
	}); err != nil {
		t.Fatalf("PutEvidence[0]: %v", err)
	}
	if err := s.PutEvidence(ctx, store.Evidence{
		ID:          "01HZXEVLIST000000000002",
		RunID:       runID,
		StepID:      "alert_baseline",
		ContentType: "application/json",
		Payload:     []byte(`{"aggregate":true}`),
		// no Deviceslot → aggregate row
	}); err != nil {
		t.Fatalf("PutEvidence[1]: %v", err)
	}
	if err := s.PutEvidence(ctx, store.Evidence{
		ID:          "01HZXEVLIST000000000003",
		RunID:       runID,
		StepID:      "fact_baseline",
		ContentType: "application/json",
		Payload:     []byte(`{"facts":1}`),
		Deviceslot:  &ds2,
	}); err != nil {
		t.Fatalf("PutEvidence[2]: %v", err)
	}
}

func TestEvidenceList_All(t *testing.T) {
	srv, s, _ := newTestServer(t)
	runID := "01HZX0000000000000LIST1"
	seedEvidenceList(t, s, runID)

	res, err := http.Get(srv.URL + "/api/runs/" + runID + "/evidence")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d, want 200; body = %s", res.StatusCode, body)
	}
	var envelope struct {
		Evidence []struct {
			ID          string          `json:"id"`
			StepID      string          `json:"step_id"`
			Deviceslot  *string         `json:"deviceslot"`
			ContentType string          `json:"content_type"`
			CreatedAt   int64           `json:"created_at"`
			Payload     json.RawMessage `json:"payload"`
		} `json:"evidence"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(envelope.Evidence) != 3 {
		t.Fatalf("len evidence = %d, want 3", len(envelope.Evidence))
	}
	// Each item must have a valid JSON payload.
	for i, e := range envelope.Evidence {
		if !json.Valid(e.Payload) {
			t.Errorf("evidence[%d].payload is not valid JSON: %s", i, e.Payload)
		}
		if e.ID == "" {
			t.Errorf("evidence[%d].id is empty", i)
		}
		if e.StepID == "" {
			t.Errorf("evidence[%d].step_id is empty", i)
		}
		if e.ContentType == "" {
			t.Errorf("evidence[%d].content_type is empty", i)
		}
	}
}

func TestEvidenceList_FilterByStep(t *testing.T) {
	srv, s, _ := newTestServer(t)
	runID := "01HZX0000000000000LIST2"
	seedEvidenceList(t, s, runID)

	res, err := http.Get(srv.URL + "/api/runs/" + runID + "/evidence?step=alert_baseline")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d, want 200; body = %s", res.StatusCode, body)
	}
	var envelope struct {
		Evidence []struct {
			StepID string `json:"step_id"`
		} `json:"evidence"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(envelope.Evidence) != 2 {
		t.Fatalf("len evidence = %d, want 2 (alert_baseline only)", len(envelope.Evidence))
	}
	for i, e := range envelope.Evidence {
		if e.StepID != "alert_baseline" {
			t.Errorf("evidence[%d].step_id = %q, want alert_baseline", i, e.StepID)
		}
	}
}

func TestEvidenceList_EmptyRun(t *testing.T) {
	srv, s, _ := newTestServer(t)
	runID := "01HZX0000000000000LIST3"
	// Seed the run row but no evidence.
	_, _ = s.DB().ExecContext(context.Background(), `
		INSERT INTO runs (id, template_id, bundle, created_at, updated_at, status, rack, created_by, inputs)
		VALUES (?, ?, '', 0, 0, 'pending', '', 'wpena', '{}')`,
		runID, "firmware-release-canary")

	res, err := http.Get(srv.URL + "/api/runs/" + runID + "/evidence")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d, want 200; body = %s", res.StatusCode, body)
	}
	var envelope struct {
		Evidence []any `json:"evidence"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Must be an empty array, not null.
	if envelope.Evidence == nil {
		t.Fatal("evidence = null, want empty array []")
	}
	if len(envelope.Evidence) != 0 {
		t.Fatalf("len evidence = %d, want 0", len(envelope.Evidence))
	}
}
