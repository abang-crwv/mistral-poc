// Package lifecycleclient is a read-only client for per-CT lifecycle
// signals from Prometheus. iter-5b only implements one query —
// baremetal_node_rlcc_provisioning_ignored == 1 — used at run-create to
// auto-detect RLCC-ignored BMNs. iter-5e extends the surface with full
// FLCC/NLCC lifecycle polling.
package lifecycleclient

// RackKey is the (rack, zone) pair Prometheus queries filter on. Zone
// is derived from the rack id suffix (see inventoryclient.zoneFromRackID).
type RackKey struct {
	Rack string
	Zone string
}

// IgnoredBMN is one row returned by QueryRLCCIgnored.
type IgnoredBMN struct {
	Deviceslot string
	BMNName    string
}

// FLCCObservation is one BareMetalNode's current FLCC lifecycle reading,
// from the flcc_node_state metric labels. iter-5g's rlcc_action_probe reads
// this per tick. State == "fail" means the node failed its current phase.
// State/PrevState are the current/previous lifecycle state; Step/PrevStep
// the current/previous workflow step. Workflow is the FLCC workflow name.
type FLCCObservation struct {
	State     string `json:"state"`
	Step      string `json:"step"`
	PrevState string `json:"prev_state"`
	PrevStep  string `json:"prev_step"`
	Workflow  string `json:"workflow"`
}
