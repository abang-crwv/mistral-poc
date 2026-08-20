package hpcverifclient

import (
	"context"
	"sort"

	"qac/internal/vm"
)

// PromClient is the production backend. It renders the failures query and runs
// it as an instant query through the shared super-region fan-out client
// (internal/vm), mapping each result series' labels to a NodeFailure. The
// query emits one series per failing node (value is always 1), so QueryVector's
// label-only result is exactly what we need.
type PromClient struct {
	vm *vm.Client
}

// NewPromClient returns a PromClient backed by the shared vm.Client.
func NewPromClient(c *vm.Client) *PromClient { return &PromClient{vm: c} }

var _ Client = (*PromClient)(nil)

// VerificationFailures satisfies Client. A healthy rack returns an empty slice;
// a transport/upstream failure bubbles up (vm.ErrUpstream).
func (c *PromClient) VerificationFailures(ctx context.Context, rack string) ([]NodeFailure, error) {
	series, err := c.vm.QueryVector(ctx, renderFailuresQuery(rack))
	if err != nil {
		return nil, err
	}
	out := make([]NodeFailure, 0, len(series))
	for _, m := range series {
		out = append(out, NodeFailure{
			Node:          m["node"],
			Deviceslot:    m["deviceslot"],
			Testcase:      m["testcase"],
			Message:       m["annotation_hpc_verification_coreweave_cloud_message"],
			LastHeartbeat: m["annotation_hpc_verification_coreweave_cloud_last_heartbeat_time"],
		})
	}
	sortFailures(out)
	return out, nil
}

// VerificationRoster satisfies Client. It runs the roster query (the heartbeat
// gauge scoped to the rack, one series per tested node) as an instant query,
// keeping the sample value as the last-heartbeat unix time. A rack with no
// recent verification run returns an empty slice; a transport/upstream failure
// bubbles up (vm.ErrUpstream).
func (c *PromClient) VerificationRoster(ctx context.Context, rack string) ([]NodeStatus, error) {
	samples, err := c.vm.QueryVectorSamples(ctx, renderRosterQuery(rack))
	if err != nil {
		return nil, err
	}
	out := make([]NodeStatus, 0, len(samples))
	for _, s := range samples {
		out = append(out, NodeStatus{
			Node:              s.Metric["node"],
			Deviceslot:        s.Metric["deviceslot"],
			LastHeartbeatUnix: int64(s.Value),
		})
	}
	sortRoster(out)
	return out, nil
}

// sortRoster orders roster rows by deviceslot, then node, for deterministic
// output and stable cross-run diffs.
func sortRoster(v []NodeStatus) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].Deviceslot != v[j].Deviceslot {
			return v[i].Deviceslot < v[j].Deviceslot
		}
		return v[i].Node < v[j].Node
	})
}

// sortFailures orders rows by deviceslot, then node, then testcase, for
// deterministic output and stable cross-run diffs.
func sortFailures(v []NodeFailure) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].Deviceslot != v[j].Deviceslot {
			return v[i].Deviceslot < v[j].Deviceslot
		}
		if v[i].Node != v[j].Node {
			return v[i].Node < v[j].Node
		}
		return v[i].Testcase < v[j].Testcase
	})
}
