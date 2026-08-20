// Package firmwareclient is a self-contained per-rack firmware-inventory
// source. firmware_inventory_probe uses it to snapshot the firmware a release
// actually moves (the part static inventory facts like sku/variant don't
// capture). Readings are keyed by deviceslot — the stable physical-slot key,
// which survives node swaps — so a cross-run diff pinpoints which slot's which
// component changed version.
//
// Source: the redfish_firmware_version VM metric, joined onto
// baremetal_node_physical_topology_labels (scoped by rack_name) on
// (region, deviceslot) and rolled up by (deviceslot, component, version). The
// rack id we hold IS the topology rack_name, so the query needs only the rack —
// no zone or domain-scope resolution.
//
// Like the sibling ports, the bare `qac` module means we carry the shape we
// need and run through the shared super-region fan-out client (internal/vm).
package firmwareclient

import (
	"context"
	"errors"
)

// FirmwareReading is one firmware-inventory row: a component's version at a
// specific physical slot (deviceslot) on the rack.
type FirmwareReading struct {
	Deviceslot string `json:"deviceslot"`
	Component  string `json:"component"`
	Version    string `json:"version"`
}

// BundleStatus is one node's firmware-bundle convergence: whether it reached
// the target bundle after a zap. CurrentBundle is the installed node bundle
// (status_node_bundle), TargetBundle is the pinned target (spec_node_bundle);
// OnTarget is true when the two match. This is the signal a raw version
// snapshot misses — "did the release actually land on this node" — and
// mirrors the intent of the (blocked) NodeFwBundleMismatch alert / FMA-3195.
// Source: baremetal_node_fwbundle_info.
type BundleStatus struct {
	Deviceslot    string `json:"deviceslot"`
	CurrentBundle string `json:"current_bundle"`
	TargetBundle  string `json:"target_bundle"`
	DPUBundle     string `json:"dpu_bundle,omitempty"`
	OnTarget      bool   `json:"on_target"`
}

// Client reads the firmware inventory for a rack. Implementations must be
// goroutine-safe.
type Client interface {
	// FirmwareInventory returns one row per (deviceslot, component, version) on
	// the rack. A clean miss (rack has no firmware series) returns an empty
	// slice, not an error.
	FirmwareInventory(ctx context.Context, rack string) ([]FirmwareReading, error)

	// FirmwareBundles returns one row per node on the rack with its current vs
	// target firmware bundle (convergence). A clean miss returns an empty
	// slice, not an error.
	FirmwareBundles(ctx context.Context, rack string) ([]BundleStatus, error)
}

// ErrSourceUnavailable signals a source-side failure (network, auth). Callers
// may map it onto a StepFailed event.
var ErrSourceUnavailable = errors.New("firmwareclient: source unavailable")
