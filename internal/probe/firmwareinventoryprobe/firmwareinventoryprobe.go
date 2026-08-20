// Package firmwareinventoryprobe implements firmware_inventory_probe — a
// gatherer that snapshots the firmware inventory for each canary rack. Firmware
// is the part a firmware release actually moves, so a cross-run diff of these
// snapshots pinpoints which slot's which component changed. (Static inventory
// facts — instance_type/sku/variant — are captured at run-create time and don't
// change across a zap, so this probe no longer gathers them.)
//
// Readings are keyed by deviceslot (the stable physical-slot key, which survives
// node swaps). Source: the redfish_firmware_version VM metric joined onto
// physical topology by rack_name, via firmwareclient. The rack id IS the
// topology rack_name, so no zone or domain-scope resolution is needed.
//
// It also captures per-node firmware-bundle CONVERGENCE (current vs target
// bundle, from baremetal_node_fwbundle_info) — the "did the release actually
// land on this node" signal a raw version snapshot misses, and the intent
// behind the fleet's NodeFwBundleMismatch alert / FMA-3195. Still a gatherer:
// it records on_target / off_target as evidence, no verdict.
package firmwareinventoryprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"qac/internal/probe"
)

// Probe snapshots the per-deviceslot firmware inventory per rack.
type Probe struct{}

// New returns a zero-value Probe.
func New() *Probe { return &Probe{} }

// Compile-time satisfaction check.
var _ probe.Probe = (*Probe)(nil)

func (*Probe) Type() string             { return "firmware_inventory_probe" }
func (*Probe) Category() probe.Category { return probe.CategoryGatherer }

// componentVersion is one (component, version) reading at a slot. Deviceslot is
// the map key in rackInventory, so it is omitted here.
type componentVersion struct {
	Component string `json:"component"`
	Version   string `json:"version"`
}

// bundleView is one node's firmware-bundle convergence in evidence (keyed by
// deviceslot in rackInventory.Bundles). OnTarget=false means the node did not
// reach its target bundle — the "did the release actually land" signal a raw
// version snapshot misses.
type bundleView struct {
	CurrentBundle string `json:"current_bundle"`
	TargetBundle  string `json:"target_bundle"`
	DPUBundle     string `json:"dpu_bundle,omitempty"`
	OnTarget      bool   `json:"on_target"`
}

// rackInventory is the per-rack body. ByDeviceslot groups readings by the
// stable physical-slot key so a diff pinpoints which slot changed.
// Resolved=false + Error records a per-rack miss (no firmware series). Bundles
// carries per-node convergence; OffTarget lists the deviceslots not on target
// and AllOnTarget summarizes the rack (only meaningful when Bundles is set).
type rackInventory struct {
	Resolved     bool                          `json:"resolved"`
	ByDeviceslot map[string][]componentVersion `json:"by_deviceslot,omitempty"`
	Bundles      map[string]bundleView         `json:"bundles,omitempty"`
	OffTarget    []string                      `json:"off_target,omitempty"`
	AllOnTarget  bool                          `json:"all_on_target"`
	Error        string                        `json:"error,omitempty"`
}

// inventoryEvidence is the structured output.
type inventoryEvidence struct {
	ProbedAt    int64                    `json:"probed_at"`
	Source      string                   `json:"source"`
	PerRack     map[string]rackInventory `json:"per_rack"`
	AllResolved bool                     `json:"all_resolved"`
}

const source = "redfish-firmware-version"

// Run satisfies probe.Probe. For each rack it queries the firmware inventory and
// groups readings by deviceslot. An empty inventory is recorded as per-rack data
// (resolved=false); transport/source errors bubble up to StepFailed.
func (*Probe) Run(ctx context.Context, clients probe.Clients, sc probe.StepContext) (probe.Output, error) {
	if clients.FirmwareClient == nil {
		return probe.Output{}, fmt.Errorf("firmware_inventory_probe: clients.FirmwareClient is nil")
	}

	perRack := make(map[string]rackInventory, len(sc.Racks))
	allResolved := true

	for _, rack := range sc.Racks {
		readings, err := clients.FirmwareClient.FirmwareInventory(ctx, rack)
		if err != nil {
			// Source/transport failure — bubble up so the engine emits
			// StepFailed for this step. The walk continues to the next step.
			return probe.Output{}, fmt.Errorf("firmware_inventory_probe: firmware inventory %s: %w", rack, err)
		}
		bundles, err := clients.FirmwareClient.FirmwareBundles(ctx, rack)
		if err != nil {
			return probe.Output{}, fmt.Errorf("firmware_inventory_probe: firmware bundles %s: %w", rack, err)
		}

		var inv rackInventory
		if len(readings) == 0 {
			inv.Error = "no firmware versions found"
			allResolved = false
		} else {
			inv.Resolved = true
			byDeviceslot := map[string][]componentVersion{}
			for _, r := range readings {
				byDeviceslot[r.Deviceslot] = append(byDeviceslot[r.Deviceslot], componentVersion{
					Component: r.Component,
					Version:   r.Version,
				})
			}
			inv.ByDeviceslot = byDeviceslot
		}

		// Bundle convergence: did each node reach its target bundle? AllOnTarget
		// is only asserted when bundle data is present (client rows are sorted
		// by deviceslot, so OffTarget is deterministic).
		if len(bundles) > 0 {
			bmap := make(map[string]bundleView, len(bundles))
			for _, b := range bundles {
				bmap[b.Deviceslot] = bundleView{
					CurrentBundle: b.CurrentBundle,
					TargetBundle:  b.TargetBundle,
					DPUBundle:     b.DPUBundle,
					OnTarget:      b.OnTarget,
				}
				if !b.OnTarget {
					inv.OffTarget = append(inv.OffTarget, b.Deviceslot)
				}
			}
			inv.Bundles = bmap
			inv.AllOnTarget = len(inv.OffTarget) == 0
		}

		perRack[rack] = inv
	}

	probedAt := time.Now().Unix()
	ev := inventoryEvidence{
		ProbedAt:    probedAt,
		Source:      source,
		PerRack:     perRack,
		AllResolved: allResolved,
	}
	structured, err := json.Marshal(ev)
	if err != nil {
		return probe.Output{}, fmt.Errorf("firmware_inventory_probe: marshal output: %w", err)
	}
	return probe.Output{
		StructuredData: structured,
		Sources:        []string{source},
		ProbedAt:       probedAt,
	}, nil
}
