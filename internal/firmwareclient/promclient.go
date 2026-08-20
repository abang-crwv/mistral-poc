package firmwareclient

import (
	"context"
	"sort"

	"qac/internal/vm"
)

// PromClient is the production backend. It renders the firmware-inventory query
// and runs it as an instant query through the shared super-region fan-out
// client (internal/vm), mapping each result series' labels to a
// FirmwareReading.
type PromClient struct {
	vm *vm.Client
}

// NewPromClient returns a PromClient backed by the shared vm.Client.
func NewPromClient(c *vm.Client) *PromClient { return &PromClient{vm: c} }

var _ Client = (*PromClient)(nil)

// FirmwareInventory satisfies Client. A clean miss returns an empty slice; a
// transport/upstream failure bubbles up (vm.ErrUpstream). The query's count
// value is discarded — the (deviceslot, component, version) label tuple is the
// inventory we keep.
func (c *PromClient) FirmwareInventory(ctx context.Context, rack string) ([]FirmwareReading, error) {
	series, err := c.vm.QueryVector(ctx, renderInventoryQuery(rack))
	if err != nil {
		return nil, err
	}
	out := make([]FirmwareReading, 0, len(series))
	for _, m := range series {
		out = append(out, FirmwareReading{
			Deviceslot: m["deviceslot"],
			Component:  m["component"],
			Version:    m["version"],
		})
	}
	sortReadings(out)
	return out, nil
}

// FirmwareBundles satisfies Client. It runs the fwbundle-convergence query and
// maps each series' labels (status_node_bundle = current, spec_node_bundle =
// target, status_dpu_bundle) to a BundleStatus, deriving OnTarget from an exact
// current==target match (matching how the fleet's fwbundle check does a full
// string compare today). A clean miss returns an empty slice; a transport
// failure bubbles up.
func (c *PromClient) FirmwareBundles(ctx context.Context, rack string) ([]BundleStatus, error) {
	series, err := c.vm.QueryVector(ctx, renderBundleQuery(rack))
	if err != nil {
		return nil, err
	}
	out := make([]BundleStatus, 0, len(series))
	for _, m := range series {
		cur, tgt := m["status_node_bundle"], m["spec_node_bundle"]
		out = append(out, BundleStatus{
			Deviceslot:    m["deviceslot"],
			CurrentBundle: cur,
			TargetBundle:  tgt,
			DPUBundle:     m["status_dpu_bundle"],
			OnTarget:      cur != "" && cur == tgt,
		})
	}
	sortBundles(out)
	return out, nil
}

// sortBundles orders by deviceslot for deterministic output.
func sortBundles(v []BundleStatus) {
	sort.Slice(v, func(i, j int) bool { return v[i].Deviceslot < v[j].Deviceslot })
}

// sortReadings orders rows by deviceslot, then component, then version, for
// deterministic output (and stable cross-run diffs).
func sortReadings(v []FirmwareReading) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].Deviceslot != v[j].Deviceslot {
			return v[i].Deviceslot < v[j].Deviceslot
		}
		if v[i].Component != v[j].Component {
			return v[i].Component < v[j].Component
		}
		return v[i].Version < v[j].Version
	})
}
