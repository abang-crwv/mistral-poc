package alertcategoryclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"qac/internal/vm"
)

func TestCategories_AllSevenRegistered(t *testing.T) {
	cats := Categories()
	if len(cats) != 7 {
		t.Fatalf("category count = %d, want 7", len(cats))
	}
	want := []string{
		"node_alert_history", "other_alerts", "node_nvlink_alerts",
		"node_pcie_alerts", "node_redfish_alerts", "nvlink_domain_alerts",
		"nvlink_switch_alerts",
	}
	for i, id := range want {
		if cats[i].ID != id {
			t.Errorf("category[%d].ID = %q, want %q", i, cats[i].ID, id)
		}
		if cats[i].Title == "" {
			t.Errorf("category %q has empty title", id)
		}
	}
	// Every registered category must have a loadable query template.
	for _, c := range cats {
		if loadQuery(c.queryFile) == "" {
			t.Errorf("category %q: empty/missing query template %q", c.ID, c.queryFile)
		}
	}
}

func TestRenderQuery_SubstitutesScopeAndRack(t *testing.T) {
	scope := DomainScope{
		Region: "CA-EAST-01A", Zone: "CA-EAST-01A",
		Cluster: "fleetops-ca-east-01a", NVLinkDomain: "DH1000-002-CA-EAST-01A",
	}
	// node_alert_history uses region/zone/cluster/nvlink_domain.
	got := renderQuery(loadQuery("node_alert_history.promql"), scope, "dh1000-r002-ca-east-01a")
	for _, want := range []string{
		`region=~"CA-EAST-01A"`, `zone=~"CA-EAST-01A"`,
		`cluster=~"fleetops-ca-east-01a"`, `nvlink_domain=~"DH1000-002-CA-EAST-01A"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered query missing %q\n%s", want, got)
		}
	}
	for _, ph := range []string{"{region}", "{zone}", "{cluster}", "{nvlink_domain}", "{rack}", "{rack_prefix}"} {
		if strings.Contains(got, ph) {
			t.Errorf("rendered query still has unsubstituted placeholder %q:\n%s", ph, got)
		}
	}
}

func TestRenderQuery_EmptyOptionalLabelBecomesWildcard(t *testing.T) {
	scope := DomainScope{NVLinkDomain: "D"} // region/zone/cluster empty
	got := renderQuery(loadQuery("node_alert_history.promql"), scope, "dh1000-r002-ca-east-01a")
	if !strings.Contains(got, `region=~".*"`) || !strings.Contains(got, `cluster=~".*"`) {
		t.Errorf("empty optional labels should render as .*, got:\n%s", got)
	}
}

func TestRenderQuery_SwitchScopesByNVLinkDomain(t *testing.T) {
	// Switch alerts are scoped by nvlink_domain (not a deviceslot prefix):
	// switch deviceslots have two formats and many carry nvlink_domain, while a
	// deviceslot-prefix match cross-matches same-numbered racks in other zones.
	got := renderQuery(loadQuery("nvlink_switch_alerts.promql"),
		DomainScope{NVLinkDomain: "DH1000-002-CA-EAST-01A"}, "dh1000-r002-ca-east-01a")
	if !strings.Contains(got, `nvlink_domain=~"DH1000-002-CA-EAST-01A"`) {
		t.Errorf("switch query should scope by nvlink_domain, got:\n%s", got)
	}
}

func TestMapClient_GatherReturnsSeededRows(t *testing.T) {
	m := NewMapClient(SeedDemoCategories())
	rows, err := m.Gather(context.Background(), "node_alert_history", DomainScope{}, "dh3-r012-us-east-01a", Lookback{})
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Labels["alertname"] != "NodeGPUNVLinkDown" {
		t.Errorf("alertname = %q, want NodeGPUNVLinkDown", rows[0].Labels["alertname"])
	}
	if rows[0].SampleCount == 0 {
		t.Errorf("sample_count should be non-zero")
	}
}

func TestMapClient_FailingSourceRack(t *testing.T) {
	m := NewMapClient(SeedDemoCategories())
	_, err := m.Gather(context.Background(), "node_alert_history", DomainScope{}, FailingSourceRack, Lookback{})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err = %v, want ErrSourceUnavailable", err)
	}
}

func TestMapClient_UnknownCategory(t *testing.T) {
	m := NewMapClient(nil)
	_, err := m.Gather(context.Background(), "does_not_exist", DomainScope{}, "dh3-r012-us-east-01a", Lookback{})
	if !errors.Is(err, ErrUnknownCategory) {
		t.Fatalf("err = %v, want ErrUnknownCategory", err)
	}
}

// rangeServer replies to /api/v1/query_range with one matrix series. It also
// captures the rendered query so the test can assert it was scoped.
func rangeServer(t *testing.T, captured *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if captured != nil {
			*captured = r.URL.Query().Get("query")
		}
		row := map[string]any{
			"metric": map[string]string{"alertname": "NodeGPUNVLinkDown", "node": "n1", "severity": "warning"},
			"values": [][2]any{{1700000000, "1"}, {1700000060, "1"}, {1700000120, "1"}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "matrix", "result": []map[string]any{row}},
		})
	}))
}

func TestPromClient_GatherCollapsesMatrixToRows(t *testing.T) {
	var gotQuery string
	srv := rangeServer(t, &gotQuery)
	defer srv.Close()

	vc := vm.New(vm.Config{
		AuthedURLs: map[string]string{"us-west": srv.URL},
		Username:   "u", Password: "p",
		HTTPClient: srv.Client(),
	})
	c := NewPromClient(vc)
	scope := DomainScope{Region: "CA-EAST-01A", Zone: "CA-EAST-01A", Cluster: "fleetops", NVLinkDomain: "DH1000-002-CA-EAST-01A"}
	rows, err := c.Gather(context.Background(), "node_nvlink_alerts", scope, "dh1000-r002-ca-east-01a", Lookback{Window: time.Hour, Step: 60 * time.Second})
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.FirstSeen != 1700000000 || r.LastSeen != 1700000120 || r.SampleCount != 3 {
		t.Errorf("row first/last/count = %d/%d/%d, want 1700000000/1700000120/3", r.FirstSeen, r.LastSeen, r.SampleCount)
	}
	if r.Labels["alertname"] != "NodeGPUNVLinkDown" {
		t.Errorf("alertname = %q", r.Labels["alertname"])
	}
	// The rendered query must carry the curated allowlist and the domain scope.
	if !strings.Contains(gotQuery, "NodeGPUNVLinkDown") || !strings.Contains(gotQuery, "DH1000-002-CA-EAST-01A") {
		t.Errorf("rendered query not scoped/curated as expected: %s", gotQuery)
	}
}

func TestPromClient_GatherUnknownCategory(t *testing.T) {
	c := NewPromClient(vm.New(vm.Config{Username: "u", Password: "p"}))
	_, err := c.Gather(context.Background(), "nope", DomainScope{}, "r", Lookback{})
	if !errors.Is(err, ErrUnknownCategory) {
		t.Fatalf("err = %v, want ErrUnknownCategory", err)
	}
}
