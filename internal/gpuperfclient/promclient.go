package gpuperfclient

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"qac/internal/vm"
)

// PromClient is the production backend. It renders each metric's scoped query
// and runs it as an instant query through the shared super-region fan-out
// client (internal/vm), keeping the sample value (the perf number is the
// payload, so QueryVectorSamples is used rather than QueryVector).
type PromClient struct {
	vm *vm.Client
}

// NewPromClient returns a PromClient backed by the shared vm.Client.
func NewPromClient(c *vm.Client) *PromClient { return &PromClient{vm: c} }

var _ Client = (*PromClient)(nil)

// Metrics returns the registry.
func (*PromClient) Metrics() []MetricSpec { return Metrics() }

// Gather satisfies Client. An unknown id returns ErrUnknownMetric; a sparse
// rule (no recent run) returns an empty slice; a transport/upstream failure
// bubbles up (vm.ErrUpstream).
func (c *PromClient) Gather(ctx context.Context, metricID, rack string) ([]MetricRow, error) {
	spec, ok := specByID(metricID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownMetric, metricID)
	}
	samples, err := c.vm.QueryVectorSamples(ctx, renderQuery(spec, rack))
	if err != nil {
		return nil, err
	}
	rows := make([]MetricRow, 0, len(samples))
	for _, s := range samples {
		rows = append(rows, MetricRow{Labels: s.Metric, Value: s.Value})
	}
	sortRows(rows, spec.Dims)
	return rows, nil
}

// sortRows orders rows by their dim values (in dim order) for deterministic
// output and stable cross-run diffs.
func sortRows(rows []MetricRow, dims []string) {
	sort.Slice(rows, func(i, j int) bool {
		return rowKey(rows[i], dims) < rowKey(rows[j], dims)
	})
}

// rowKey builds a sort key from a row's dim values, in dim order.
func rowKey(r MetricRow, dims []string) string {
	parts := make([]string, len(dims))
	for i, d := range dims {
		parts[i] = r.Labels[d]
	}
	return strings.Join(parts, "\x00")
}
