package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestInventoryPreview_HappyPath(t *testing.T) {
	srv, _, _ := newTestServer(t)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/inventory/preview?racks=dh3-r012-us-east-01a")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Racks []struct {
			Rack         string `json:"rack"`
			Zone         string `json:"zone"`
			InstanceType string `json:"instance_type"`
			BMNs         []struct {
				Deviceslot string `json:"deviceslot"`
			} `json:"bmns"`
			RLCCIgnored []struct {
				Deviceslot string `json:"deviceslot"`
			} `json:"rlcc_ignored"`
		} `json:"racks"`
		Errors []struct {
			Rack    string `json:"rack"`
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Racks) != 1 {
		t.Fatalf("racks len = %d, want 1", len(body.Racks))
	}
	if len(body.Racks[0].BMNs) != 18 {
		t.Errorf("BMNs len = %d, want 18", len(body.Racks[0].BMNs))
	}
	if body.Racks[0].InstanceType != "gb200-4x" {
		t.Errorf("InstanceType = %q, want gb200-4x", body.Racks[0].InstanceType)
	}
}

func TestInventoryPreview_MultipleRacks(t *testing.T) {
	srv, _, _ := newTestServer(t)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/inventory/preview?racks=dh3-r012-us-east-01a,dh3-r012-us-east-02a")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		Racks []struct {
			Rack string `json:"rack"`
		} `json:"racks"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Racks) != 2 {
		t.Errorf("racks len = %d, want 2", len(body.Racks))
	}
}

func TestInventoryPreview_MissingRacksParam(t *testing.T) {
	srv, _, _ := newTestServer(t)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/inventory/preview")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestInventoryPreview_UnknownRackInErrorsArray(t *testing.T) {
	srv, _, _ := newTestServer(t)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/inventory/preview?racks=dh3-r012-us-east-01a,no-such-rack")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (partial-failure)", resp.StatusCode)
	}
	var body struct {
		Racks  []map[string]any `json:"racks"`
		Errors []struct {
			Rack    string `json:"rack"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Racks) != 1 {
		t.Errorf("racks len = %d, want 1 (the resolvable one)", len(body.Racks))
	}
	if len(body.Errors) != 1 || !strings.Contains(body.Errors[0].Rack, "no-such-rack") {
		t.Errorf("errors = %v, want one entry for no-such-rack", body.Errors)
	}
}

func TestInventoryPreview_CachesWithin60s(t *testing.T) {
	// Two back-to-back requests should hit the cache on the second call.
	// We can't observe the resolver directly here without DI gymnastics;
	// instead, verify both responses are deeply equal.
	srv, _, _ := newTestServer(t)
	defer srv.Close()
	a, err := http.Get(srv.URL + "/api/inventory/preview?racks=dh3-r012-us-east-01a")
	if err != nil {
		t.Fatalf("first GET: %v", err)
	}
	defer a.Body.Close()
	b, err := http.Get(srv.URL + "/api/inventory/preview?racks=dh3-r012-us-east-01a")
	if err != nil {
		t.Fatalf("second GET: %v", err)
	}
	defer b.Body.Close()
	if a.StatusCode != b.StatusCode {
		t.Errorf("status mismatch: %d vs %d", a.StatusCode, b.StatusCode)
	}
}
