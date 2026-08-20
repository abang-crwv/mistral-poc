package inventoryclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"qac/internal/vm"
)

// vmSeries is a tiny helper to build one device_slot_instance_types
// series' label map in the shape VM actually returns (confirmed against
// US-EAST 2026-06-04).
func vmSeries(deviceslot, instanceType, cwSKU, cluster string) map[string]string {
	return map[string]string{
		"__name__":      "device_slot_instance_types",
		"deviceslot":    deviceslot,
		"name":          deviceslot,
		"instance_type": instanceType,
		"cw_sku":        cwSKU,
		"region":        "CA-EAST-01",
		"zone":          "CA-EAST-01A",
		"cluster":       cluster,
		"cluster_org":   "coreweave",
	}
}

// invSeriesWithBMN is a device_slot_instance_types series that also carries
// the real short bmn (which the metric does in production, but the bare
// vmSeries helper omits). Used to assert ResolveBMNs prefers "bmn" over
// "name" for BMNName.
func invSeriesWithBMN(deviceslot, bmn string) map[string]string {
	m := vmSeries(deviceslot, "gb200-4x", "GPU-GB200-01", "ca-east-01a-prod")
	m["bmn"] = bmn
	return m
}

// newVMTestServer returns an httptest server that responds to
// /api/v1/query with the given series, plus a VMResolver pointed at it.
// Creds are wired directly into vm.Config; no env-var injection needed.
func newVMTestServer(t *testing.T, series []map[string]string) (*VMResolver, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.URL.Path; got != "/api/v1/query" {
			t.Errorf("path = %q, want /api/v1/query", got)
		}
		if user, pass, ok := req.BasicAuth(); !ok || user != "u" || pass != "p" {
			t.Errorf("basic auth = (%q,%q,%v), want (u,p,true)", user, pass, ok)
		}
		resp := map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result": func() []map[string]any {
					rows := make([]map[string]any, 0, len(series))
					for _, m := range series {
						rows = append(rows, map[string]any{"metric": m, "value": []any{0, "1"}})
					}
					return rows
				}(),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	r := NewVMResolver(vm.New(vm.Config{
		AuthedURLs: map[string]string{"test": srv.URL},
		Username:   "u",
		Password:   "p",
		HTTPClient: srv.Client(),
	}))
	return r, srv
}

// gb200Rack builds an 18-node GB200 NVL72 rack: 15 in -prod with the
// dominant sku, plus three trays enrolled in other clusters — exactly
// the spread real VM returns for one physical rack.
func gb200Rack() []map[string]string {
	var s []map[string]string
	for i := 1; i <= 15; i++ {
		s = append(s, vmSeries(fmt.Sprintf("dh1000-r003-node-%02d-ca-east-01a", i), "gb200-4x", "GPU-GB200-01", "ca-east-01a-prod"))
	}
	s = append(s, vmSeries("dh1000-r003-node-16-ca-east-01a", "gb200-4x", "GPU-GB200-01", "ca-east-01a-mgmt"))
	s = append(s, vmSeries("dh1000-r003-node-17-ca-east-01a", "gb200-4x", "GPU-GB200-04", "ca-east-01a-prod"))
	s = append(s, vmSeries("dh1000-r003-node-18-ca-east-01a", "gb200-4x", "GPU-GB200-01", "fleetops-ca-east-01a"))
	return s
}

func TestVMResolver_ResolveRack_DominantFacts(t *testing.T) {
	r, _ := newVMTestServer(t, gb200Rack())
	facts, err := ResolveRack(context.Background(), r, "dh1000-r003-ca-east-01a")
	if err != nil {
		t.Fatalf("ResolveRack: %v", err)
	}
	if facts.InstanceType != "gb200-4x" {
		t.Errorf("InstanceType = %q, want gb200-4x", facts.InstanceType)
	}
	// cw_sku -> sku remap; dominant is GPU-GB200-01 (15 of 18).
	if facts.SKU != "GPU-GB200-01" {
		t.Errorf("SKU = %q, want GPU-GB200-01 (dominant cw_sku)", facts.SKU)
	}
	if facts.Variant != "gb200" {
		t.Errorf("Variant = %q, want gb200 (derived)", facts.Variant)
	}
	if facts.GBGeneration != "blackwell" {
		t.Errorf("GBGeneration = %q, want blackwell (derived)", facts.GBGeneration)
	}
	if facts.Region != "ca-east-01" {
		t.Errorf("Region = %q, want ca-east-01 (lowercased)", facts.Region)
	}
}

func TestVMResolver_ResolveBMNs_AllEighteenTrays(t *testing.T) {
	r, _ := newVMTestServer(t, gb200Rack())
	bmns, err := r.ResolveBMNs(context.Background(), "dh1000-r003-ca-east-01a")
	if err != nil {
		t.Fatalf("ResolveBMNs: %v", err)
	}
	if len(bmns) != 18 {
		t.Fatalf("len(bmns) = %d, want 18 (a GB200 NVL72 rack, no cluster filtering)", len(bmns))
	}
	// Sorted by CT position: first tray is node-01, last is node-18.
	if bmns[0].CTPosition != 1 {
		t.Errorf("bmns[0].CTPosition = %d, want 1", bmns[0].CTPosition)
	}
	if bmns[17].CTPosition != 18 {
		t.Errorf("bmns[17].CTPosition = %d, want 18", bmns[17].CTPosition)
	}
	if bmns[0].Deviceslot != "dh1000-r003-node-01-ca-east-01a" {
		t.Errorf("bmns[0].Deviceslot = %q", bmns[0].Deviceslot)
	}
	if bmns[0].Zone != "ca-east-01a" {
		t.Errorf("bmns[0].Zone = %q, want ca-east-01a (lowercased)", bmns[0].Zone)
	}
	if bmns[0].Rack != "dh1000-r003-ca-east-01a" {
		t.Errorf("bmns[0].Rack = %q", bmns[0].Rack)
	}
}

func TestVMResolver_ResolveBMNs_DedupesByDeviceslot(t *testing.T) {
	// Same deviceslot on two series (different cw_sku rollups) collapses
	// to one tray.
	series := []map[string]string{
		vmSeries("dh1000-r003-node-01-ca-east-01a", "gb200-4x", "GPU-GB200-01", "ca-east-01a-prod"),
		vmSeries("dh1000-r003-node-01-ca-east-01a", "gb200-4x", "GPU-GB200-04", "ca-east-01a-prod"),
	}
	r, _ := newVMTestServer(t, series)
	bmns, err := r.ResolveBMNs(context.Background(), "dh1000-r003-ca-east-01a")
	if err != nil {
		t.Fatalf("ResolveBMNs: %v", err)
	}
	if len(bmns) != 1 {
		t.Fatalf("len(bmns) = %d, want 1 (deduped)", len(bmns))
	}
}

func TestVMResolver_EmptyResult(t *testing.T) {
	r, _ := newVMTestServer(t, nil)
	// Lookup miss -> ResolveRack surfaces ErrNotFound.
	_, err := ResolveRack(context.Background(), r, "dh9-r999-no-such-01a")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ResolveRack err = %v, want ErrNotFound", err)
	}
	// ResolveBMNs miss -> ErrNotFound.
	_, err = r.ResolveBMNs(context.Background(), "dh9-r999-no-such-01a")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ResolveBMNs err = %v, want ErrNotFound", err)
	}
}

func TestVMResolver_QueryShape(t *testing.T) {
	// Lookup reads hardware facts from device_slot_instance_types scoped by
	// the node deviceslot regex; ResolveBMNs enumerates as-built trays from
	// baremetal_node_physical_topology_labels scoped by the anchored rack-name
	// label. Neither is ever unscoped.
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotQuery = req.URL.Query().Get("query")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	t.Cleanup(srv.Close)
	r := NewVMResolver(vm.New(vm.Config{
		AuthedURLs: map[string]string{"test": srv.URL},
		Username:   "u",
		Password:   "p",
		HTTPClient: srv.Client(),
	}))

	wantFacts := `device_slot_instance_types{deviceslot=~"^dh1000-r003-node-[0-9]+-ca-east-01a$"}`
	_, _ = r.Lookup(context.Background(), "dh1000-r003-ca-east-01a")
	if gotQuery != wantFacts {
		t.Errorf("Lookup query =\n %q\nwant\n %q", gotQuery, wantFacts)
	}

	wantTopo := `baremetal_node_physical_topology_labels{label_ds_coreweave_com_physical_topology_rack_name=~"^dh1000-r003-ca-east-01a$"}`
	gotQuery = ""
	_, _ = r.ResolveBMNs(context.Background(), "dh1000-r003-ca-east-01a")
	if gotQuery != wantTopo {
		t.Errorf("ResolveBMNs query =\n %q\nwant\n %q", gotQuery, wantTopo)
	}
}

// BMNName is the real short bmn from the "bmn" label. A node with no bmn
// label resolves to an EMPTY BMNName (not the deviceslot) — empty honestly
// signals "no BMN reported" rather than duplicating the slot.
func TestVMResolver_ResolveBMNs_UsesRealBMN(t *testing.T) {
	noBMN := vmSeries("dh1000-r003-node-02-ca-east-01a", "gb200-4x", "GPU-GB200-01", "ca-east-01a-prod") // vmSeries sets no "bmn"
	series := []map[string]string{
		invSeriesWithBMN("dh1000-r003-node-01-ca-east-01a", "s22l4nb4"),
		noBMN,
	}
	r, _ := newVMTestServer(t, series)
	bmns, err := r.ResolveBMNs(context.Background(), "dh1000-r003-ca-east-01a")
	if err != nil {
		t.Fatalf("ResolveBMNs: %v", err)
	}
	if len(bmns) != 2 {
		t.Fatalf("len(bmns) = %d, want 2", len(bmns))
	}
	if bmns[0].BMNName != "s22l4nb4" {
		t.Errorf("bmns[0].BMNName = %q, want s22l4nb4 (real bmn, not deviceslot)", bmns[0].BMNName)
	}
	if bmns[0].Deviceslot != "dh1000-r003-node-01-ca-east-01a" {
		t.Errorf("bmns[0].Deviceslot = %q", bmns[0].Deviceslot)
	}
	// node-02 has no bmn label → empty, NOT the deviceslot.
	if bmns[1].BMNName != "" {
		t.Errorf("bmns[1].BMNName = %q, want empty (no bmn reported)", bmns[1].BMNName)
	}
}

func TestSplitRackID(t *testing.T) {
	cases := []struct {
		in             string
		prefix, suffix string
		ok             bool
	}{
		{"dh1000-r003-ca-east-01a", "dh1000-r003", "ca-east-01a", true},
		{"dh3-r012-us-east-01a", "dh3-r012", "us-east-01a", true},
		{"dh3-r012", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		p, s, ok := splitRackID(c.in)
		if p != c.prefix || s != c.suffix || ok != c.ok {
			t.Errorf("splitRackID(%q) = (%q,%q,%v), want (%q,%q,%v)", c.in, p, s, ok, c.prefix, c.suffix, c.ok)
		}
	}
}

func TestDeriveVariantAndGeneration(t *testing.T) {
	if v := deriveVariant("gb200-4x"); v != "gb200" {
		t.Errorf("deriveVariant(gb200-4x) = %q, want gb200", v)
	}
	if v := deriveVariant("gb300-4x"); v != "gb300" {
		t.Errorf("deriveVariant(gb300-4x) = %q, want gb300", v)
	}
	if g := deriveGBGeneration("gb300-4x"); g != "blackwell" {
		t.Errorf("deriveGBGeneration(gb300-4x) = %q, want blackwell", g)
	}
	if g := deriveGBGeneration("cd-gp-a192-genoa"); g != "" {
		t.Errorf("deriveGBGeneration(cd-gp-a192-genoa) = %q, want empty", g)
	}
}
