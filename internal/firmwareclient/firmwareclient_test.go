package firmwareclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"qac/internal/vm"
)

func TestRenderInventoryQuery_SubstitutesRackName(t *testing.T) {
	got := renderInventoryQuery("dh1000-r002-ca-east-01a")
	if !strings.Contains(got, `label_ds_coreweave_com_physical_topology_rack_name="dh1000-r002-ca-east-01a"`) {
		t.Errorf("rendered query missing rack_name match:\n%s", got)
	}
	if strings.Contains(got, "{rack_name}") {
		t.Errorf("rendered query still has unsubstituted {rack_name}:\n%s", got)
	}
	// The query must keep the topology↔firmware join and the rollup grouping.
	for _, want := range []string{
		"redfish_firmware_version",
		"on (region, deviceslot)",
		"count by (deviceslot, component, version)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered query missing %q:\n%s", want, got)
		}
	}
}

func TestMapClient_ReturnsSeededReadings(t *testing.T) {
	m := NewMapClient(SeedDemoFirmware())
	rows, err := m.FirmwareInventory(context.Background(), "dh3-r012-us-east-01a")
	if err != nil {
		t.Fatalf("FirmwareInventory: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("rows = 0, want seeded readings")
	}
	// Readings must be keyed by canonical deviceslot, sorted, and carry a
	// component + version.
	first := rows[0]
	if !strings.HasPrefix(first.Deviceslot, "dh3-r012-node-") || !strings.HasSuffix(first.Deviceslot, "-us-east-01a") {
		t.Errorf("deviceslot = %q, want dh3-r012-node-NN-us-east-01a shape", first.Deviceslot)
	}
	if first.Component == "" || first.Version == "" {
		t.Errorf("reading missing component/version: %+v", first)
	}
}

func TestMapClient_MissReturnsEmpty(t *testing.T) {
	m := NewMapClient(SeedDemoFirmware())
	rows, err := m.FirmwareInventory(context.Background(), "rack-with-no-firmware")
	if err != nil {
		t.Fatalf("FirmwareInventory: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %d, want 0 for a clean miss", len(rows))
	}
}

func TestMapClient_FailingSourceRack(t *testing.T) {
	m := NewMapClient(SeedDemoFirmware())
	_, err := m.FirmwareInventory(context.Background(), FailingSourceRack)
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err = %v, want ErrSourceUnavailable", err)
	}
}

// vectorServer replies to /api/v1/query with the given vector result and
// captures the rendered query.
func vectorServer(t *testing.T, captured *string, result []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if captured != nil {
			*captured = r.URL.Query().Get("query")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "vector", "result": result},
		})
	}))
}

func TestPromClient_MapsLabelsToReadings(t *testing.T) {
	var gotQuery string
	srv := vectorServer(t, &gotQuery, []map[string]any{
		{"metric": map[string]string{"deviceslot": "dh1000-r002-node-02-ca-east-01a", "component": "HGX_FW_BIOS_0", "version": "2.04.1"}, "value": []any{1700000000, "1"}},
		{"metric": map[string]string{"deviceslot": "dh1000-r002-node-01-ca-east-01a", "component": "ARM golden image", "version": "4.8.0.13249"}, "value": []any{1700000000, "1"}},
	})
	defer srv.Close()

	c := NewPromClient(vm.New(vm.Config{
		AuthedURLs: map[string]string{"us-west": srv.URL},
		Username:   "u", Password: "p",
		HTTPClient: srv.Client(),
	}))
	rows, err := c.FirmwareInventory(context.Background(), "dh1000-r002-ca-east-01a")
	if err != nil {
		t.Fatalf("FirmwareInventory: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	// sortReadings orders by deviceslot, so node-01 comes first.
	if rows[0].Deviceslot != "dh1000-r002-node-01-ca-east-01a" || rows[0].Component != "ARM golden image" {
		t.Errorf("rows[0] = %+v, want node-01 / ARM golden image first", rows[0])
	}
	if rows[1].Version != "2.04.1" {
		t.Errorf("rows[1].Version = %q, want 2.04.1", rows[1].Version)
	}
	if !strings.Contains(gotQuery, `rack_name="dh1000-r002-ca-east-01a"`) {
		t.Errorf("rendered query not scoped to rack: %s", gotQuery)
	}
}

func TestRenderBundleQuery_SubstitutesRackAndUsesFwbundleInfo(t *testing.T) {
	q := renderBundleQuery("dh3-r012-us-east-01a")
	for _, want := range []string{
		"baremetal_node_fwbundle_info",
		`label_ds_coreweave_com_physical_topology_rack_name="dh3-r012-us-east-01a"`,
	} {
		if !strings.Contains(q, want) {
			t.Errorf("bundle query missing %q:\n%s", want, q)
		}
	}
}

func TestPromClient_FirmwareBundles_DerivesOnTarget(t *testing.T) {
	srv := vectorServer(t, nil, []map[string]any{
		// node-02 current != target -> off target.
		{"metric": map[string]string{"deviceslot": "dh3-r012-node-02-us-east-01a", "status_node_bundle": "b-1109", "spec_node_bundle": "b-0112", "status_dpu_bundle": "dpu-1"}, "value": []any{1700000000, "1"}},
		// node-01 current == target -> on target.
		{"metric": map[string]string{"deviceslot": "dh3-r012-node-01-us-east-01a", "status_node_bundle": "b-0112", "spec_node_bundle": "b-0112", "status_dpu_bundle": "dpu-1"}, "value": []any{1700000000, "1"}},
	})
	defer srv.Close()

	c := NewPromClient(vm.New(vm.Config{
		AuthedURLs: map[string]string{"us-west": srv.URL},
		Username:   "u", Password: "p",
		HTTPClient: srv.Client(),
	}))
	rows, err := c.FirmwareBundles(context.Background(), "dh3-r012-us-east-01a")
	if err != nil {
		t.Fatalf("FirmwareBundles: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	// Sorted by deviceslot: node-01 first (on target), node-02 second (off).
	if !rows[0].OnTarget || rows[0].CurrentBundle != rows[0].TargetBundle {
		t.Errorf("rows[0] = %+v, want on target", rows[0])
	}
	if rows[1].OnTarget || rows[1].DPUBundle != "dpu-1" {
		t.Errorf("rows[1] = %+v, want off target with dpu bundle", rows[1])
	}
}

func TestMapClient_FirmwareBundles_SeedAndFailing(t *testing.T) {
	m := NewMapClient(SeedDemoFirmware()).WithBundles(SeedDemoFirmwareBundles())
	rows, err := m.FirmwareBundles(context.Background(), "dh3-r012-us-east-01a")
	if err != nil {
		t.Fatalf("FirmwareBundles: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (node-01 on target, node-02 off)", len(rows))
	}
	var offTarget int
	for _, r := range rows {
		if !r.OnTarget {
			offTarget++
		}
	}
	if offTarget != 1 {
		t.Errorf("off-target rows = %d, want 1", offTarget)
	}
	// Failing sentinel bubbles.
	if _, err := m.FirmwareBundles(context.Background(), FailingSourceRack); !errors.Is(err, ErrSourceUnavailable) {
		t.Errorf("FailingSourceRack err = %v, want ErrSourceUnavailable", err)
	}
	// A client with no WithBundles returns empty, not error.
	plain := NewMapClient(SeedDemoFirmware())
	if got, err := plain.FirmwareBundles(context.Background(), "dh3-r012-us-east-01a"); err != nil || len(got) != 0 {
		t.Errorf("no-bundles client: rows=%d err=%v, want 0/nil", len(got), err)
	}
}
