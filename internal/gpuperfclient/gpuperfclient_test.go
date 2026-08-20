package gpuperfclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"qac/internal/vm"
)

func TestMetrics_RegistryIsStableAndComplete(t *testing.T) {
	m := Metrics()
	if len(m) != 28 {
		t.Fatalf("registry size = %d, want 28", len(m))
	}
	// First entry is GPU Blaze GFLOPS avg; last is the CPU-perf threshold.
	if m[0].ID != "gpu_blaze_gflops_avg" {
		t.Errorf("m[0].ID = %q, want gpu_blaze_gflops_avg", m[0].ID)
	}
	if m[len(m)-1].ID != "cpu_perf_threshold" {
		t.Errorf("last ID = %q, want cpu_perf_threshold", m[len(m)-1].ID)
	}
	seen := map[string]bool{}
	for _, s := range m {
		if s.ID == "" || s.Title == "" || s.Rule == "" || s.Agg == "" || len(s.Dims) == 0 {
			t.Errorf("incomplete spec: %+v", s)
		}
		if seen[s.ID] {
			t.Errorf("duplicate id %q", s.ID)
		}
		seen[s.ID] = true
		if s.Agg != "avg" && s.Agg != "min" && s.Agg != "max" {
			t.Errorf("spec %q has bad agg %q", s.ID, s.Agg)
		}
	}
}

func TestRenderQuery_ScopedJoin(t *testing.T) {
	spec, _ := specByID("gpu_blaze_gflops_avg")
	got := renderQuery(spec, "dh202-r185-us-central-08a")
	for _, want := range []string{
		"avg by (node, gpu, precision)",
		"gpu:hpc_verification_gpu_blaze_gflops:avg5m",
		"on(node, cluster, cluster_org, zone) group_left()",
		`rack_name="dh202-r185-us-central-08a"`,
		// The rack scope excludes FLCC-ignorable nodes (rma/broken/etc.), so a
		// node in RMA or debug can't leak junk perf rows into the pack.
		`state!~"rma|broken|triage|debug|dev|hold|onboard|fail|storage"`,
		`workflow!~"broken-collect|rma.*"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered query missing %q:\n%s", want, got)
		}
	}
}

func TestRegistry_HasCPUMegatronNVLinkGEMMSignals(t *testing.T) {
	cases := []struct {
		id, kind, rulePrefix string
		dims                 []string
	}{
		{"cpu_perf_avg", kindRule, "node:hpc_verification_cpu_perf_average", []string{"node", "testcase"}},
		{"cpu_perf_threshold", kindRule, "node:hpc_verification_cpu_perf_threshold", []string{"node", "testcase"}},
		{"megatron_job_dur_max", kindRule, "node:hpc_verification_megatron_lm_job_dur:max5m", []string{"node", "testcase"}},
		{"megatron_longest_iter_max", kindRule, "node:hpc_verification_megatron_lm_longest_iter:max5m", []string{"node", "testcase"}},
		{"nvlink_gemm_bw_avg", kindHistogramMean, "hpcv_plugin_mnubergemm_group_value_avg_nvlink_bw_gbps", []string{"node", "suite"}},
		{"nvlink_gemm_bw_min", kindHistogramMean, "hpcv_plugin_mnubergemm_group_value_min_nvlink_bw_gbps", []string{"node", "suite"}},
	}
	for _, c := range cases {
		spec, ok := specByID(c.id)
		if !ok {
			t.Errorf("missing registry id %q", c.id)
			continue
		}
		if spec.Kind != c.kind {
			t.Errorf("%q Kind = %q, want %q", c.id, spec.Kind, c.kind)
		}
		if !strings.HasPrefix(spec.Rule, c.rulePrefix) {
			t.Errorf("%q Rule = %q, want prefix %q", c.id, spec.Rule, c.rulePrefix)
		}
		if strings.Join(spec.Dims, ",") != strings.Join(c.dims, ",") {
			t.Errorf("%q Dims = %v, want %v", c.id, spec.Dims, c.dims)
		}
	}
}

func TestRegistry_NCCLIsHistogramBacked(t *testing.T) {
	// The old entries referenced recording rules that do not exist
	// (node:hpc_verification_nccl_allreduce_*:avg5m), so NCCL was always empty.
	for _, dead := range []string{"nccl_allreduce_average_avg", "nccl_allreduce_expected_avg"} {
		if _, ok := specByID(dead); ok {
			t.Errorf("dead NCCL id %q still in registry", dead)
		}
	}
	for _, id := range []string{"nccl_bandwidth_observed_avg", "nccl_bandwidth_expected_avg"} {
		spec, ok := specByID(id)
		if !ok {
			t.Fatalf("expected NCCL id %q in registry", id)
		}
		if spec.Kind != kindHistogramMean {
			t.Errorf("%q Kind = %q, want %q", id, spec.Kind, kindHistogramMean)
		}
		if !strings.HasPrefix(spec.Rule, "hpcv_plugin_nccl_value_") {
			t.Errorf("%q Rule = %q, want an hpcv_plugin_nccl_value_* histogram base", id, spec.Rule)
		}
	}
}

func TestRenderQuery_HistogramMean(t *testing.T) {
	spec, _ := specByID("nccl_bandwidth_observed_avg")
	got := renderQuery(spec, "dh202-r185-us-central-08a")
	for _, want := range []string{
		"avg by (node, suite, nvls_mode)",
		"hpcv_plugin_nccl_value_avg_bandwidth_gbps_sum / hpcv_plugin_nccl_value_avg_bandwidth_gbps_count",
		"on(node, cluster, cluster_org, zone) group_left()",
		`rack_name="dh202-r185-us-central-08a"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered histogram query missing %q:\n%s", want, got)
		}
	}
	// It must not resurrect the nonexistent recording-rule shape.
	for _, bad := range []string{":avg5m", "nccl_allreduce"} {
		if strings.Contains(got, bad) {
			t.Errorf("rendered histogram query unexpectedly contains %q:\n%s", bad, got)
		}
	}
}

func TestMapClient_GatherSeededRowsAndUnknown(t *testing.T) {
	m := NewMapClient(SeedDemoGPUPerf())
	rows, err := m.Gather(context.Background(), "gpu_blaze_gflops_avg", "dh3-r012-us-east-01a")
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0].Value == 0 || rows[0].Labels["gpu"] == "" {
		t.Errorf("row missing value/labels: %+v", rows[0])
	}
	// Sparse metric/rack -> empty, not error.
	empty, err := m.Gather(context.Background(), "ib_bw_avg", "dh3-r012-us-east-01a")
	if err != nil || len(empty) != 0 {
		t.Errorf("sparse metric: rows=%d err=%v, want 0/nil", len(empty), err)
	}
	// Unknown id -> ErrUnknownMetric.
	if _, err := m.Gather(context.Background(), "nope", "r"); !errors.Is(err, ErrUnknownMetric) {
		t.Errorf("unknown id err = %v, want ErrUnknownMetric", err)
	}
}

func TestMapClient_FailingSourceRack(t *testing.T) {
	m := NewMapClient(SeedDemoGPUPerf())
	_, err := m.Gather(context.Background(), "gpu_blaze_gflops_avg", FailingSourceRack)
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err = %v, want ErrSourceUnavailable", err)
	}
}

func samplesServer(t *testing.T, captured *string, rows []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if captured != nil {
			*captured = r.URL.Query().Get("query")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "vector", "result": rows},
		})
	}))
}

func TestPromClient_GatherKeepsValuesAndSorts(t *testing.T) {
	var gotQuery string
	srv := samplesServer(t, &gotQuery, []map[string]any{
		{"metric": map[string]string{"node": "n2", "gpu": "1", "precision": "bf16_bf16_bf16"}, "value": []any{0, strconv.FormatFloat(683489.5, 'f', -1, 64)}},
		{"metric": map[string]string{"node": "n1", "gpu": "0", "precision": "fp8e4m3_fp8e4m3_bf16"}, "value": []any{0, strconv.FormatFloat(1365144.125, 'f', -1, 64)}},
	})
	defer srv.Close()

	c := NewPromClient(vm.New(vm.Config{
		AuthedURLs: map[string]string{"us-west": srv.URL},
		Username:   "u", Password: "p",
		HTTPClient: srv.Client(),
	}))
	rows, err := c.Gather(context.Background(), "gpu_blaze_gflops_avg", "dh202-r185-us-central-08a")
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	// Sorted by dims (node first): n1 before n2.
	if rows[0].Labels["node"] != "n1" || rows[0].Value != 1365144.125 {
		t.Errorf("rows[0] = %+v, want n1 / 1365144.125 first", rows[0])
	}
	if rows[1].Value != 683489.5 {
		t.Errorf("rows[1].Value = %v, want 683489.5", rows[1].Value)
	}
	if !strings.Contains(gotQuery, `rack_name="dh202-r185-us-central-08a"`) {
		t.Errorf("query not scoped to rack: %s", gotQuery)
	}
}
