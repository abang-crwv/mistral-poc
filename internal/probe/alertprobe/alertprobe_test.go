package alertprobe

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"qac/internal/alertcategoryclient"
	"qac/internal/inventoryclient"
	"qac/internal/probe"
)

// fakeResolver implements inventoryclient.Resolver with configurable
// ResolveDomainScope behavior. Lookup/ResolveBMNs are unused by alert_probe.
type fakeResolver struct {
	scope    map[string]inventoryclient.DomainScope
	scopeErr map[string]error
}

func (f *fakeResolver) Lookup(context.Context, string) ([]inventoryclient.Hit, error) {
	return nil, nil
}
func (f *fakeResolver) ResolveBMNs(context.Context, string) ([]inventoryclient.BMN, error) {
	return nil, nil
}
func (f *fakeResolver) ResolveDomainScope(_ context.Context, rackID string) (inventoryclient.DomainScope, error) {
	if e, ok := f.scopeErr[rackID]; ok {
		return inventoryclient.DomainScope{}, e
	}
	return f.scope[rackID], nil
}

// evidence mirrors the alert_probe structured output for decoding in tests.
type evidence struct {
	ProbedAt   int64  `json:"probed_at"`
	Lookback   string `json:"lookback"`
	Categories map[string]struct {
		Title   string `json:"title"`
		PerRack map[string]struct {
			Scope struct {
				Region       string `json:"region"`
				Zone         string `json:"zone"`
				Cluster      string `json:"cluster"`
				NVLinkDomain string `json:"nvlink_domain"`
			} `json:"scope"`
			Rows     []map[string]any `json:"rows"`
			RowCount int              `json:"row_count"`
			Error    string           `json:"error"`
		} `json:"per_rack"`
	} `json:"categories"`
}

func decode(t *testing.T, out probe.Output) evidence {
	t.Helper()
	var ev evidence
	if err := json.Unmarshal(out.StructuredData, &ev); err != nil {
		t.Fatalf("decode StructuredData: %v", err)
	}
	return ev
}

func TestAlertProbe_TypeAndCategory(t *testing.T) {
	p := New()
	if p.Type() != "alert_probe" {
		t.Errorf("Type = %q, want alert_probe", p.Type())
	}
	if p.Category() != probe.CategoryGatherer {
		t.Errorf("Category = %q, want gatherer", p.Category())
	}
}

func TestAlertProbe_Run_GathersAllCategoriesPerRack(t *testing.T) {
	const rack = "dh1000-r002-ca-east-01a"
	res := &fakeResolver{scope: map[string]inventoryclient.DomainScope{
		rack: {Region: "CA-EAST-01A", Zone: "CA-EAST-01A", Cluster: "fleetops-ca-east-01a", NVLinkDomain: "DH1000-002-CA-EAST-01A"},
	}}
	rows := map[string]map[string][]alertcategoryclient.CategoryRow{
		"node_alert_history": {
			rack: {{Labels: map[string]string{"alertname": "NodeGPUNVLinkDown", "node": rack + "-node-01"}, FirstSeen: 100, LastSeen: 200, SampleCount: 5}},
		},
	}
	c := probe.Clients{
		AlertCategoryClient: alertcategoryclient.NewMapClient(rows),
		InventoryResolver:   res,
	}
	sc := probe.StepContext{RunID: "r1", StepID: "alert_baseline", Racks: []string{rack}}

	out, err := New().Run(context.Background(), c, sc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	ev := decode(t, out)

	// All 7 categories present as keys.
	if len(ev.Categories) != 7 {
		t.Fatalf("categories = %d, want 7", len(ev.Categories))
	}
	hist, ok := ev.Categories["node_alert_history"]
	if !ok {
		t.Fatalf("missing node_alert_history category")
	}
	if hist.Title == "" {
		t.Errorf("category title is empty")
	}
	rk := hist.PerRack[rack]
	if rk.RowCount != 1 || len(rk.Rows) != 1 {
		t.Fatalf("node_alert_history rows = %d (count %d), want 1", len(rk.Rows), rk.RowCount)
	}
	if rk.Rows[0]["labels"].(map[string]any)["alertname"] != "NodeGPUNVLinkDown" {
		t.Errorf("alertname = %v", rk.Rows[0]["labels"])
	}
	// Scope echoed (raw/upper-cased).
	if rk.Scope.Zone != "CA-EAST-01A" || rk.Scope.NVLinkDomain != "DH1000-002-CA-EAST-01A" {
		t.Errorf("scope = %+v", rk.Scope)
	}
	// A category with no seeded rows still appears, with an empty per-rack entry.
	other := ev.Categories["other_alerts"]
	if _, ok := other.PerRack[rack]; !ok {
		t.Errorf("other_alerts missing per-rack entry for %s", rack)
	}
	if other.PerRack[rack].RowCount != 0 {
		t.Errorf("other_alerts row_count = %d, want 0", other.PerRack[rack].RowCount)
	}
	if out.ProbedAt == 0 {
		t.Errorf("ProbedAt = 0")
	}
	if ev.Lookback != "24h" {
		t.Errorf("lookback = %q, want 24h (default)", ev.Lookback)
	}
}

func TestAlertProbe_Run_LookbackFromConfig(t *testing.T) {
	const rack = "r-a"
	c := probe.Clients{
		AlertCategoryClient: alertcategoryclient.NewMapClient(nil),
		InventoryResolver:   &fakeResolver{scope: map[string]inventoryclient.DomainScope{rack: {Zone: "Z"}}},
	}
	sc := probe.StepContext{
		RunID: "r1", StepID: "alert_baseline", Racks: []string{rack},
		Config: map[string]any{"lookback": "1h"},
	}
	out, err := New().Run(context.Background(), c, sc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ev := decode(t, out); ev.Lookback != "1h" {
		t.Errorf("lookback = %q, want 1h", ev.Lookback)
	}
}

func TestAlertProbe_Run_ScopeMissRecordedPerRack(t *testing.T) {
	const rack = "missing-rack"
	c := probe.Clients{
		AlertCategoryClient: alertcategoryclient.NewMapClient(nil),
		InventoryResolver:   &fakeResolver{scopeErr: map[string]error{rack: inventoryclient.ErrNotFound}},
	}
	sc := probe.StepContext{RunID: "r1", StepID: "alert_baseline", Racks: []string{rack}}
	out, err := New().Run(context.Background(), c, sc)
	if err != nil {
		t.Fatalf("Run should not error on a scope miss: %v", err)
	}
	ev := decode(t, out)
	rk := ev.Categories["node_alert_history"].PerRack[rack]
	if rk.Error == "" {
		t.Errorf("expected per-rack error for unresolved scope, got none")
	}
	if rk.RowCount != 0 {
		t.Errorf("row_count = %d, want 0 for unresolved rack", rk.RowCount)
	}
}

func TestAlertProbe_Run_GatherErrorBubbles(t *testing.T) {
	const rack = alertcategoryclient.FailingSourceRack
	c := probe.Clients{
		AlertCategoryClient: alertcategoryclient.NewMapClient(nil),
		InventoryResolver:   &fakeResolver{scope: map[string]inventoryclient.DomainScope{rack: {Zone: "Z"}}},
	}
	sc := probe.StepContext{RunID: "r1", StepID: "alert_baseline", Racks: []string{rack}}
	_, err := New().Run(context.Background(), c, sc)
	if !errors.Is(err, alertcategoryclient.ErrSourceUnavailable) {
		t.Fatalf("err = %v, want ErrSourceUnavailable", err)
	}
}

func TestAlertProbe_Run_NilClientsError(t *testing.T) {
	sc := probe.StepContext{RunID: "r1", StepID: "alert_baseline", Racks: []string{"r"}}
	if _, err := New().Run(context.Background(), probe.Clients{InventoryResolver: &fakeResolver{}}, sc); err == nil {
		t.Errorf("expected error when AlertCategoryClient is nil")
	}
	if _, err := New().Run(context.Background(), probe.Clients{AlertCategoryClient: alertcategoryclient.NewMapClient(nil)}, sc); err == nil {
		t.Errorf("expected error when InventoryResolver is nil")
	}
}
