package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestListProbes(t *testing.T) {
	srv, _, _ := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/probes")
	if err != nil {
		t.Fatalf("GET /api/probes: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Probes []struct {
			Type        string `json:"type"`
			Category    string `json:"category"`
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"probes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// newTestServer registers alert_probe, firmware_inventory_probe,
	// rlcc_action_probe — sorted by type.
	if len(body.Probes) != 3 {
		t.Fatalf("probes = %d, want 3: %+v", len(body.Probes), body.Probes)
	}
	wantOrder := []string{"alert_probe", "firmware_inventory_probe", "rlcc_action_probe"}
	for i, want := range wantOrder {
		if body.Probes[i].Type != want {
			t.Errorf("probes[%d].type = %q, want %q", i, body.Probes[i].Type, want)
		}
	}
	// Category comes from the registry; title/description from the catalog.
	byType := map[string]struct{ cat, title, desc string }{}
	for _, p := range body.Probes {
		byType[p.Type] = struct{ cat, title, desc string }{p.Category, p.Title, p.Description}
	}
	if got := byType["alert_probe"]; got.cat != "gatherer" || got.title == "" || got.title == "alert_probe" || got.desc == "" {
		t.Errorf("alert_probe view = %+v, want gatherer + human title + description", got)
	}
	if got := byType["firmware_inventory_probe"]; got.cat != "gatherer" || got.title == "" {
		t.Errorf("firmware_inventory_probe view = %+v", got)
	}
}
