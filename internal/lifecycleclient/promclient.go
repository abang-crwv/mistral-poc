package lifecycleclient

import (
	"context"
	"fmt"

	"qac/internal/vm"
)

// PromClient is the production lifecycleclient backend. It renders
// lifecycle PromQL and runs it through the shared super-region fan-out
// client (internal/vm), which owns endpoint selection, basic-auth, and
// the unauthed vmui fallback.
type PromClient struct {
	vm *vm.Client
}

// NewPromClient returns a PromClient backed by the shared vm.Client.
func NewPromClient(c *vm.Client) *PromClient {
	return &PromClient{vm: c}
}

// promSample is the internal adapter shape the Query* methods read; only
// Metric (the label map) is used. vm.Client discards sample values, so
// there is no Value field and no JSON tag (this is no longer a decode
// target — decoding happens in internal/vm).
type promSample struct {
	Metric map[string]string
}

// QueryRLCCIgnored satisfies Client. Issues one HTTP GET against
// /api/v1/query. Returns the vector samples as IgnoredBMN rows.
func (c *PromClient) QueryRLCCIgnored(ctx context.Context, rack RackKey) ([]IgnoredBMN, error) {
	samples, err := c.queryVector(ctx, renderRLCCIgnoredQuery(rack.Rack))
	if err != nil {
		return nil, err
	}
	out := make([]IgnoredBMN, 0, len(samples))
	for _, s := range samples {
		out = append(out, IgnoredBMN{
			Deviceslot: s.Metric["deviceslot"],
			BMNName:    s.Metric["bmn"],
		})
	}
	return out, nil
}

// QueryActionState satisfies Client. Fans out one instant query per
// deviceslot; a non-empty result vector means the CT reached targetState.
func (c *PromClient) QueryActionState(ctx context.Context, deviceslots []string, targetState string) (map[string]bool, error) {
	out := make(map[string]bool, len(deviceslots))
	for _, ds := range deviceslots {
		rack, node := parseActionLabels(ds)
		samples, err := c.queryVector(ctx, renderActionStateQuery(rack, node, targetState))
		if err != nil {
			return nil, fmt.Errorf("query action state %s: %w", ds, err)
		}
		out[ds] = len(samples) > 0
	}
	return out, nil
}

// QueryFLCCState satisfies Client. Fans out one instant query per
// deviceslot against flcc_node_state and reads the lifecycle labels off
// the (at most one) returned series. A deviceslot with no series yields a
// zero-value FLCCObservation.
func (c *PromClient) QueryFLCCState(ctx context.Context, deviceslots []string) (map[string]FLCCObservation, error) {
	out := make(map[string]FLCCObservation, len(deviceslots))
	for _, ds := range deviceslots {
		samples, err := c.queryVector(ctx, renderFLCCStateQuery(ds))
		if err != nil {
			return nil, fmt.Errorf("query flcc state %s: %w", ds, err)
		}
		if len(samples) == 0 {
			out[ds] = FLCCObservation{}
			continue
		}
		m := samples[0].Metric
		out[ds] = FLCCObservation{
			State:     m["state"],
			Step:      m["step"],
			PrevState: m["prev_state"],
			PrevStep:  m["prev_step"],
			Workflow:  m["workflow"],
		}
	}
	return out, nil
}

// queryVector runs q through the shared vm.Client fan-out and adapts the
// returned label maps to promSample (the sample value is unused by every
// caller, so an empty-value sample is sufficient).
func (c *PromClient) queryVector(ctx context.Context, q string) ([]promSample, error) {
	series, err := c.vm.QueryVector(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]promSample, 0, len(series))
	for _, m := range series {
		out = append(out, promSample{Metric: m})
	}
	return out, nil
}
