package firmwareclient

import (
	"context"
	"fmt"
	"strings"
)

// FailingSourceRack is the sentinel rack id MapClient surfaces
// ErrSourceUnavailable for, supporting probe error-path tests.
const FailingSourceRack = "failing-source-rack"

// MapClient is a deterministic in-memory Client for tests and the offline
// (QAC_VM_BACKEND=map) backend. Firmware readings are keyed by rack id;
// bundle-convergence rows are attached via WithBundles.
type MapClient struct {
	byRack       map[string][]FirmwareReading
	byRackBundle map[string][]BundleStatus
}

// NewMapClient returns a MapClient backed by the given rack -> readings table.
// A nil table is normalized to empty (every rack resolves to no rows).
func NewMapClient(byRack map[string][]FirmwareReading) *MapClient {
	if byRack == nil {
		byRack = map[string][]FirmwareReading{}
	}
	return &MapClient{byRack: byRack, byRackBundle: map[string][]BundleStatus{}}
}

// WithBundles attaches a rack -> bundle-convergence table (mirrors
// hpcverifclient's WithRoster). Returns the receiver for chaining.
func (m *MapClient) WithBundles(byRack map[string][]BundleStatus) *MapClient {
	if byRack != nil {
		m.byRackBundle = byRack
	}
	return m
}

var _ Client = (*MapClient)(nil)

// FirmwareInventory satisfies Client. Returns the seeded readings for the rack,
// or an empty slice on miss. The FailingSourceRack sentinel surfaces
// ErrSourceUnavailable.
func (m *MapClient) FirmwareInventory(_ context.Context, rack string) ([]FirmwareReading, error) {
	if rack == FailingSourceRack {
		return nil, ErrSourceUnavailable
	}
	rows := m.byRack[rack]
	out := make([]FirmwareReading, len(rows))
	copy(out, rows)
	return out, nil
}

// FirmwareBundles satisfies Client. Returns the seeded bundle rows for the rack,
// or an empty slice on miss. The FailingSourceRack sentinel surfaces
// ErrSourceUnavailable.
func (m *MapClient) FirmwareBundles(_ context.Context, rack string) ([]BundleStatus, error) {
	if rack == FailingSourceRack {
		return nil, ErrSourceUnavailable
	}
	rows := m.byRackBundle[rack]
	out := make([]BundleStatus, len(rows))
	copy(out, rows)
	return out, nil
}

// SeedDemoFirmware returns a deterministic rack -> readings table for the demo
// racks (those in inventoryclient.SeedDemoBMNs). Each rack gets two slots' worth
// of components, keyed by canonical deviceslot (<prefix>-rNNN-node-NN-<zone>) so
// demo readings line up with the demo BMN/deviceslot naming.
func SeedDemoFirmware() map[string][]FirmwareReading {
	racks := []string{
		"dh3-r012-us-east-01a",
		"dh3-r012-us-east-02a",
		"dh4-r020-us-west-01b",
		"dh4-r020-us-west-02b",
	}
	components := []struct{ comp, ver string }{
		{"ARM golden image", "4.8.0.13249"},
		{"HGX_FW_BIOS_0", "2.04.1"},
		{"HGX_FW_GPU_0", "97.00.3F.00.0A"},
	}
	out := map[string][]FirmwareReading{}
	for _, rk := range racks {
		rows := make([]FirmwareReading, 0, len(components)*2)
		for pos := 1; pos <= 2; pos++ {
			ds := demoDeviceslot(rk, pos)
			for _, c := range components {
				rows = append(rows, FirmwareReading{
					Deviceslot: ds,
					Component:  c.comp,
					Version:    c.ver,
				})
			}
		}
		out[rk] = rows
	}
	return out
}

// SeedDemoFirmwareBundles returns a deterministic rack -> bundle-convergence
// table for the demo racks. Each rack's node-01 is on target; node-02 is left
// on the previous bundle (off target) so the demo surfaces a convergence gap.
func SeedDemoFirmwareBundles() map[string][]BundleStatus {
	racks := []string{
		"dh3-r012-us-east-01a",
		"dh3-r012-us-east-02a",
		"dh4-r020-us-west-01b",
		"dh4-r020-us-west-02b",
	}
	const (
		target = "dell-xe9712-phoenix-ROCE-20260112.01.00.00"
		prev   = "dell-xe9712-phoenix-ROCE-20251109.01.00.00"
		dpu    = "nvidia-bluefield-default-20260123.01.00.00"
	)
	out := map[string][]BundleStatus{}
	for _, rk := range racks {
		out[rk] = []BundleStatus{
			{Deviceslot: demoDeviceslot(rk, 1), CurrentBundle: target, TargetBundle: target, DPUBundle: dpu, OnTarget: true},
			{Deviceslot: demoDeviceslot(rk, 2), CurrentBundle: prev, TargetBundle: target, DPUBundle: dpu, OnTarget: false},
		}
	}
	return out
}

// demoDeviceslot derives the canonical deviceslot for a node position on a demo
// rack: rack "dh3-r012-us-east-01a" + pos 1 -> "dh3-r012-node-01-us-east-01a".
// Mirrors the deviceslot shape inventoryclient.SeedDemoBMNs uses. A rack id that
// doesn't split into <prefix>-<rNNN>-<zone> falls back to a "<rack>-node-NN"
// suffix so the seeder never panics on malformed demo input.
func demoDeviceslot(rack string, pos int) string {
	parts := strings.SplitN(rack, "-", 3)
	if len(parts) < 3 {
		return fmt.Sprintf("%s-node-%02d", rack, pos)
	}
	return fmt.Sprintf("%s-%s-node-%02d-%s", parts[0], parts[1], pos, parts[2])
}
