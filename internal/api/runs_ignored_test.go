package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCreateRun_OperatorIgnoredBMN_EmitsBMNIgnoredEvent(t *testing.T) {
	srv, s, _ := newTestServer(t)
	defer srv.Close()
	body := map[string]any{
		"template_id": "firmware-release-canary",
		"inputs": map[string]any{
			"bundle_tag":   "dell-xe9712-default-2.0.0-rc",
			"canary_racks": []string{"dh3-r012-us-east-01a"},
		},
		"ignored_bmns": []map[string]any{
			{
				"rack":       "dh3-r012-us-east-01a",
				"deviceslot": "dh3-r012-node-07-us-east-01a",
				"reason":     "known bad GPU",
			},
		},
	}
	buf, _ := json.Marshal(body)

	// The test server needs the canonical template seeded.
	upsertCanonicalCanaryTemplate(t, s)

	resp, err := http.Post(srv.URL+"/api/runs", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body=%s", resp.StatusCode, b)
	}
	var rr struct {
		Run struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&rr)

	// Confirm a BMNIgnored event with source=operator landed.
	rows, err := s.DB().QueryContext(context.Background(),
		`SELECT COUNT(*) FROM events WHERE run_id = ? AND kind = 'BMNIgnored'`, rr.Run.ID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	rows.Next()
	var n int
	_ = rows.Scan(&n)
	if n < 1 {
		t.Errorf("BMNIgnored event count = %d, want >=1", n)
	}
}

func TestCreateRun_IgnoredBMN_UnknownDeviceslotReturns400(t *testing.T) {
	srv, s, _ := newTestServer(t)
	defer srv.Close()

	upsertCanonicalCanaryTemplate(t, s)

	body := map[string]any{
		"template_id": "firmware-release-canary",
		"inputs": map[string]any{
			"bundle_tag":   "dell-xe9712-default-2.0.0-rc",
			"canary_racks": []string{"dh3-r012-us-east-01a"},
		},
		"ignored_bmns": []map[string]any{
			{"rack": "dh3-r012-us-east-01a", "deviceslot": "no-such-slot"},
		},
	}
	buf, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/api/runs", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	var eb struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&eb)
	if eb.Error.Code != "invalid_request" {
		t.Errorf("error.code = %q, want invalid_request", eb.Error.Code)
	}
	if !strings.Contains(eb.Error.Message, "no-such-slot") {
		t.Errorf("error.message = %q, want mention of bad deviceslot", eb.Error.Message)
	}
}

func TestCreateRun_IgnoredBMN_UnknownRackReturns400(t *testing.T) {
	srv, s, _ := newTestServer(t)
	defer srv.Close()

	upsertCanonicalCanaryTemplate(t, s)

	body := map[string]any{
		"template_id": "firmware-release-canary",
		"inputs": map[string]any{
			"bundle_tag":   "dell-xe9712-default-2.0.0-rc",
			"canary_racks": []string{"dh3-r012-us-east-01a"},
		},
		"ignored_bmns": []map[string]any{
			{"rack": "not-in-canary", "deviceslot": "x"},
		},
	}
	buf, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/api/runs", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGetRun_RacksRollupIncludesCTCountAndIgnored(t *testing.T) {
	srv, s, _ := newTestServer(t)
	defer srv.Close()

	upsertCanonicalCanaryTemplate(t, s)

	body := map[string]any{
		"template_id": "firmware-release-canary",
		"inputs": map[string]any{
			"bundle_tag":   "dell-xe9712-default-2.0.0-rc",
			"canary_racks": []string{"dh3-r012-us-east-01a"},
		},
		"ignored_bmns": []map[string]any{
			{"rack": "dh3-r012-us-east-01a", "deviceslot": "dh3-r012-node-07-us-east-01a", "reason": "test"},
		},
	}
	buf, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/api/runs", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("POST /api/runs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST status = %d, body=%s", resp.StatusCode, b)
	}
	var cr struct {
		Run struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&cr)

	resp2, err := http.Get(srv.URL + "/api/runs/" + cr.Run.ID)
	if err != nil {
		t.Fatalf("GET /api/runs/%s: %v", cr.Run.ID, err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("GET status = %d, body=%s", resp2.StatusCode, b)
	}
	var gr struct {
		Racks []struct {
			Rack    string `json:"rack"`
			CTCount int    `json:"ct_count"`
			Ignored []struct {
				Deviceslot string  `json:"deviceslot"`
				Source     string  `json:"source"`
				Reason     *string `json:"reason"`
			} `json:"ignored"`
		} `json:"racks"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&gr); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if len(gr.Racks) != 1 {
		t.Fatalf("racks len = %d, want 1", len(gr.Racks))
	}
	if gr.Racks[0].CTCount != 18 {
		t.Errorf("CTCount = %d, want 18", gr.Racks[0].CTCount)
	}
	foundOperator := false
	for _, ig := range gr.Racks[0].Ignored {
		if ig.Source == "operator" && ig.Deviceslot == "dh3-r012-node-07-us-east-01a" {
			foundOperator = true
			if ig.Reason == nil || *ig.Reason != "test" {
				t.Errorf("operator ignore Reason = %v, want 'test'", ig.Reason)
			}
		}
	}
	if !foundOperator {
		t.Errorf("operator-ignored CT not present in racks[].ignored")
	}
}
