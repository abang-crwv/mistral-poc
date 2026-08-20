package lifecycleclient

import "context"

// Client exposes per-CT lifecycle reads. iter-5b shipped QueryRLCCIgnored;
// iter-5e adds QueryActionState for per-tick action-completion polling.
// iter-5g adds QueryFLCCState for per-deviceslot FLCC state reads.
type Client interface {
	// QueryRLCCIgnored returns the deviceslots in `rack` that have
	// baremetal_node_rlcc_provisioning_ignored == 1. An empty return
	// + nil error means "no RLCC ignores for this rack" — fwqual treats
	// that as a positive observation, not an error.
	QueryRLCCIgnored(ctx context.Context, rack RackKey) ([]IgnoredBMN, error)

	// QueryActionState reports per-deviceslot completion of an RLCC action
	// state. The returned map is keyed by deviceslot; true means the CT has
	// reached targetState. iter-5e's rlcc_action_probe polls this each tick.
	QueryActionState(ctx context.Context, deviceslots []string, targetState string) (map[string]bool, error)

	// QueryFLCCState returns each deviceslot's current FLCC observation
	// (flcc_node_state labels). Unseeded/unknown deviceslots return a
	// zero-value FLCCObservation. iter-5g's rlcc_action_probe polls this.
	QueryFLCCState(ctx context.Context, deviceslots []string) (map[string]FLCCObservation, error)
}
