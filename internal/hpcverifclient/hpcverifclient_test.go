package hpcverifclient

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

func TestRenderFailuresQuery_SubstitutesRackNameOnly(t *testing.T) {
	got := renderFailuresQuery("dh202-r185-us-central-08a")
	if !strings.Contains(got, `label_ds_coreweave_com_physical_topology_rack_name="dh202-r185-us-central-08a"`) {
		t.Errorf("rendered query missing rack_name selector:\n%s", got)
	}
	if strings.Contains(got, "{rack_name}") {
		t.Errorf("rendered query still has unsubstituted {rack_name}:\n%s", got)
	}
	// The bare `rack_name` label refs (group_left / label_replace target) must
	// survive untouched — only the braced placeholder is replaced.
	for _, want := range []string{
		"group_left(rack_name, deviceslot)",
		`"rack_name","$1"`,
		"kube_node_annotations",
		"annotation_hpc_verification_coreweave_cloud_message",
		`label_node_coreweave_cloud_state=~"test|test-fail|production|flcc"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered query missing %q:\n%s", want, got)
		}
	}
}

func TestMapClient_ReturnsSeededFailures(t *testing.T) {
	m := NewMapClient(SeedDemoHPCFailures())
	rows, err := m.VerificationFailures(context.Background(), "dh3-r012-us-east-01a")
	if err != nil {
		t.Fatalf("VerificationFailures: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].Testcase != "nvlink_bandwidth" || rows[0].Deviceslot == "" || rows[0].Message == "" {
		t.Errorf("row missing fields: %+v", rows[0])
	}
}

func TestMapClient_HealthyRackIsEmptyNotError(t *testing.T) {
	m := NewMapClient(SeedDemoHPCFailures())
	rows, err := m.VerificationFailures(context.Background(), "dh3-r012-us-east-02a")
	if err != nil {
		t.Fatalf("VerificationFailures: %v (healthy rack must not error)", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %d, want 0 for a healthy rack", len(rows))
	}
}

func TestMapClient_FailingSourceRack(t *testing.T) {
	m := NewMapClient(SeedDemoHPCFailures())
	_, err := m.VerificationFailures(context.Background(), FailingSourceRack)
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err = %v, want ErrSourceUnavailable", err)
	}
}

func TestRenderRosterQuery_SubstitutesRackNameOnly(t *testing.T) {
	got := renderRosterQuery("dh202-r185-us-central-08a")
	if !strings.Contains(got, `label_ds_coreweave_com_physical_topology_rack_name="dh202-r185-us-central-08a"`) {
		t.Errorf("rendered roster query missing rack_name selector:\n%s", got)
	}
	if strings.Contains(got, "{rack_name}") {
		t.Errorf("rendered roster query still has unsubstituted {rack_name}:\n%s", got)
	}
	// The roster is built on the heartbeat gauge (one series per tested node),
	// scoped by the same FLCC/state/topology join as failures.
	for _, want := range []string{
		"kube_node_hpc_verification_last_heartbeat_time",
		"group_left(rack_name, deviceslot)",
		`label_flcc_coreweave_com_state!~"rma|broken|triage|debug|dev|hold|onboard|fail|storage"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered roster query missing %q:\n%s", want, got)
		}
	}
}

func TestMapClient_ReturnsRoster(t *testing.T) {
	m := NewMapClient(nil).WithRoster(map[string][]NodeStatus{
		"rack-a": {
			{Node: "n1", Deviceslot: "rack-a-node-01", LastHeartbeatUnix: 1783200000},
			{Node: "n2", Deviceslot: "rack-a-node-02", LastHeartbeatUnix: 1783100000},
		},
	})
	roster, err := m.VerificationRoster(context.Background(), "rack-a")
	if err != nil {
		t.Fatalf("VerificationRoster: %v", err)
	}
	if len(roster) != 2 {
		t.Fatalf("roster = %d, want 2", len(roster))
	}
	if roster[0].Node != "n1" || roster[0].LastHeartbeatUnix != 1783200000 {
		t.Errorf("roster[0] = %+v", roster[0])
	}
}

func TestMapClient_EmptyRosterIsNotError(t *testing.T) {
	m := NewMapClient(SeedDemoHPCFailures()) // no roster set
	roster, err := m.VerificationRoster(context.Background(), "rack-untested")
	if err != nil {
		t.Fatalf("VerificationRoster: %v (empty roster must not error)", err)
	}
	if len(roster) != 0 {
		t.Errorf("roster = %d, want 0 for a rack with no verification data", len(roster))
	}
}

func TestMapClient_RosterFailingSourceRack(t *testing.T) {
	m := NewMapClient(nil)
	_, err := m.VerificationRoster(context.Background(), FailingSourceRack)
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err = %v, want ErrSourceUnavailable", err)
	}
}

func TestPromClient_RosterMapsSamples(t *testing.T) {
	var gotQuery string
	srv := vectorServer(t, &gotQuery, []map[string]any{
		{"metric": map[string]string{
			"node": "s14w3nb4", "deviceslot": "dh202-r185-node-09-us-central-08a",
		}, "value": []any{1700000000, "1783200000"}},
		{"metric": map[string]string{
			"node": "s44v3nb4", "deviceslot": "dh202-r185-node-17-us-central-08a",
		}, "value": []any{1700000000, "1783100000"}},
	})
	defer srv.Close()

	c := NewPromClient(vm.New(vm.Config{
		AuthedURLs: map[string]string{"us-west": srv.URL},
		Username:   "u", Password: "p",
		HTTPClient: srv.Client(),
	}))
	roster, err := c.VerificationRoster(context.Background(), "dh202-r185-us-central-08a")
	if err != nil {
		t.Fatalf("VerificationRoster: %v", err)
	}
	if len(roster) != 2 {
		t.Fatalf("roster = %d, want 2", len(roster))
	}
	// sorted by deviceslot, so node-09 first; heartbeat parsed to unix int64.
	if roster[0].Node != "s14w3nb4" || roster[0].LastHeartbeatUnix != 1783200000 {
		t.Errorf("roster[0] = %+v, want node-09 / 1783200000", roster[0])
	}
	if !strings.Contains(gotQuery, `rack_name="dh202-r185-us-central-08a"`) {
		t.Errorf("rendered roster query not scoped to rack: %s", gotQuery)
	}
}

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

func TestPromClient_MapsLabelsToFailures(t *testing.T) {
	var gotQuery string
	srv := vectorServer(t, &gotQuery, []map[string]any{
		{"metric": map[string]string{
			"node": "s14w3nb4", "deviceslot": "dh202-r185-node-09-us-central-08a", "testcase": "nvlink_bandwidth",
			"annotation_hpc_verification_coreweave_cloud_message":             "[6B0B] [nvlink_bandwidth] NVLink bandwidth check failed",
			"annotation_hpc_verification_coreweave_cloud_last_heartbeat_time": "2026-05-20T18:42:10Z",
		}, "value": []any{1700000000, "1"}},
		{"metric": map[string]string{
			"node": "s44v3nb4", "deviceslot": "dh202-r185-node-17-us-central-08a", "testcase": "gpu_check",
			"annotation_hpc_verification_coreweave_cloud_message": "Health check failed: gpu_check",
		}, "value": []any{1700000000, "1"}},
	})
	defer srv.Close()

	c := NewPromClient(vm.New(vm.Config{
		AuthedURLs: map[string]string{"us-west": srv.URL},
		Username:   "u", Password: "p",
		HTTPClient: srv.Client(),
	}))
	rows, err := c.VerificationFailures(context.Background(), "dh202-r185-us-central-08a")
	if err != nil {
		t.Fatalf("VerificationFailures: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	// sortFailures orders by deviceslot, so node-09 comes first.
	if rows[0].Node != "s14w3nb4" || rows[0].Testcase != "nvlink_bandwidth" {
		t.Errorf("rows[0] = %+v, want node-09 / nvlink_bandwidth first", rows[0])
	}
	if rows[0].LastHeartbeat != "2026-05-20T18:42:10Z" {
		t.Errorf("rows[0].LastHeartbeat = %q", rows[0].LastHeartbeat)
	}
	if !strings.Contains(gotQuery, `rack_name="dh202-r185-us-central-08a"`) {
		t.Errorf("rendered query not scoped to rack: %s", gotQuery)
	}
}
