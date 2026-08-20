package gpuperfclient

import (
	"context"
	"fmt"
)

// FailingSourceRack is the sentinel rack id MapClient surfaces
// ErrSourceUnavailable for, supporting probe error-path tests.
const FailingSourceRack = "failing-source-rack"

// MapClient is a deterministic in-memory Client for tests and the offline
// (QAC_VM_BACKEND=map) backend. Rows are keyed metricID -> rack -> rows.
type MapClient struct {
	byMetricRack map[string]map[string][]MetricRow
}

// NewMapClient returns a MapClient backed by the given metricID -> rack -> rows
// table. A nil table is normalized to empty (every metric/rack is sparse).
func NewMapClient(byMetricRack map[string]map[string][]MetricRow) *MapClient {
	if byMetricRack == nil {
		byMetricRack = map[string]map[string][]MetricRow{}
	}
	return &MapClient{byMetricRack: byMetricRack}
}

var _ Client = (*MapClient)(nil)

// Metrics returns the registry.
func (*MapClient) Metrics() []MetricSpec { return Metrics() }

// Gather satisfies Client. Unknown id -> ErrUnknownMetric; FailingSourceRack ->
// ErrSourceUnavailable; otherwise the seeded rows (or empty on a sparse miss).
func (m *MapClient) Gather(_ context.Context, metricID, rack string) ([]MetricRow, error) {
	if _, ok := specByID(metricID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownMetric, metricID)
	}
	if rack == FailingSourceRack {
		return nil, ErrSourceUnavailable
	}
	rows := m.byMetricRack[metricID][rack]
	out := make([]MetricRow, len(rows))
	copy(out, rows)
	return out, nil
}

// SeedDemoGPUPerf returns a deterministic table for the demo rack
// dh3-r012-us-east-01a: GPU Blaze GFLOPS (avg) for two GPUs at two precisions,
// plus a CUDA sync-latency row. Enough for the probe to exercise multiple
// metric shapes; other metrics/racks are sparse (empty), modeling "no run".
func SeedDemoGPUPerf() map[string]map[string][]MetricRow {
	rack := "dh3-r012-us-east-01a"
	node := "s90txs51"
	return map[string]map[string][]MetricRow{
		"gpu_blaze_gflops_avg": {rack: {
			{Labels: map[string]string{"node": node, "gpu": "0", "precision": "fp8e4m3_fp8e4m3_bf16"}, Value: 1365144.125},
			{Labels: map[string]string{"node": node, "gpu": "0", "precision": "bf16_bf16_bf16"}, Value: 683489.5},
			{Labels: map[string]string{"node": node, "gpu": "1", "precision": "fp8e4m3_fp8e4m3_bf16"}, Value: 1364921.875},
		}},
		"nccl_bandwidth_observed_avg": {rack: {
			{Labels: map[string]string{"node": node, "suite": "gb300_long", "nvls_mode": "off"}, Value: 665.126},
		}},
		"nccl_bandwidth_expected_avg": {rack: {
			{Labels: map[string]string{"node": node, "suite": "gb300_long", "nvls_mode": "off"}, Value: 680.0},
		}},
		"sync_latency_avg": {rack: {
			{Labels: map[string]string{"node": node}, Value: 12.5},
		}},
	}
}
