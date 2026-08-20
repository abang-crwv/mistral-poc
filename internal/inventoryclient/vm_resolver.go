package inventoryclient

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"qac/internal/vm"
)

// Two inventory metrics, two roles, joined on the deviceslot label:
//
//   - topoMetric (baremetal_node_physical_topology_labels) is the AS-BUILT
//     view: exactly one series per node that has actually been discovered
//     and installed. It carries the real node identity (bmn, bmn_serial,
//     serial, bmc_ip) and physical placement (rack_unit, grid_group), and
//     its label_ds_coreweave_com_physical_topology_rack_name label is the
//     authoritative rack-membership key. It does NOT carry instance_type or
//     cw_sku. We enumerate canary trays from here (ResolveBMNs) so a canary
//     targets the nodes that physically exist — a planned-but-undiscovered
//     slot has no node to talk to and simply does not appear.
//
//   - factMetric (device_slot_instance_types) is the PLANNING view, sourced
//     from the DeviceSlot .Spec: it lists every planned slot and carries the
//     hardware facts (instance_type, cw_sku, region, cluster). Its bmn label
//     is absent on slots whose node isn't discovered yet, so it is NOT a
//     reliable tray-identity source — we read only the hardware facts from it
//     (Lookup). Always query it scoped by the deviceslot regex; an unscoped
//     read returns the whole fleet and trips VM's max-series cap.
//
// On a fully-discovered rack both metrics return all 18 GB200 NVL72 trays
// with matching deviceslot/bmn; they diverge only on half-built racks, where
// factMetric keeps the empty-bmn planned slot and topoMetric omits it.
const (
	topoMetric    = "baremetal_node_physical_topology_labels"
	topoRackLabel = "label_ds_coreweave_com_physical_topology_rack_name"
	factMetric    = "device_slot_instance_types"
	// domainMetric carries the NVLink-domain membership of each node:
	// region/zone/cluster/nvlink_domain alongside the node + deviceslot.
	// ResolveDomainScope reads the domain selector labels off it.
	domainMetric = "baremetal_node_nvlink_domain_info"
)

// VMResolver is the production Resolver: it answers Lookup / ResolveBMNs
// from VictoriaMetrics instead of the in-memory demo map, via the shared
// super-region fan-out client. Lookup reads the dominant hardware labelset
// from factMetric (scoped by the node deviceslot regex); ResolveBMNs
// enumerates the as-built trays from topoMetric (scoped by the authoritative
// rack-name label) and returns one BMN per deviceslot.
type VMResolver struct {
	vm *vm.Client
}

// compile-time assertion that VMResolver satisfies the Resolver contract.
var _ Resolver = (*VMResolver)(nil)

// NewVMResolver returns a Resolver backed by the shared vm.Client. The
// client owns endpoint selection, basic-auth, and the unauthed vmui
// fallback; this resolver only renders the per-rack query and shapes the
// response.
func NewVMResolver(c *vm.Client) *VMResolver {
	return &VMResolver{vm: c}
}

// Lookup satisfies Resolver. It runs the per-rack query and returns a
// single rack-kind Hit whose Labels carry the dominant hardware facts
// (instance_type, sku, variant, gb_generation, region, cluster). Zero
// matching series returns (nil, nil) — the miss shape ResolveRack turns
// into ErrNotFound, matching MapResolver.Lookup's contract.
func (r *VMResolver) Lookup(ctx context.Context, id string) ([]Hit, error) {
	series, err := r.queryFacts(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(series) == 0 {
		return nil, nil
	}
	dom := dominantLabels(series)
	region := strings.ToLower(dom["region"])
	return []Hit{{
		ID:      id,
		Region:  region,
		Kind:    KindRack,
		Cluster: dom["cluster"],
		Labels: map[string]string{
			"instance_type": dom["instance_type"],
			// VM names the SKU label cw_sku, not sku — remap so
			// rackFactsFromLabels (which reads "sku") sees it.
			"sku":           dom["cw_sku"],
			"variant":       deriveVariant(dom["instance_type"]),
			"gb_generation": deriveGBGeneration(dom["instance_type"]),
			"region":        region,
			"cluster":       dom["cluster"],
		},
	}}, nil
}

// ResolveBMNs satisfies Resolver. It enumerates the as-built trays from
// topoMetric (scoped by the authoritative rack-name label) and returns one
// BMN per unique deviceslot, sorted by tray position. An empty result
// returns ErrNotFound — a rack with no discovered nodes has nothing to
// canary, so empty means either an unknown rack id or a rack not yet built.
func (r *VMResolver) ResolveBMNs(ctx context.Context, rackID string) ([]BMN, error) {
	if rackID == "" {
		return nil, ErrEmptyRack
	}
	series, err := r.queryTopology(ctx, rackID)
	if err != nil {
		return nil, err
	}
	if len(series) == 0 {
		return nil, ErrNotFound
	}
	// Dedupe by deviceslot: VM can return the same deviceslot on more
	// than one series. One tray = one BMN.
	seen := make(map[string]bool, len(series))
	out := make([]BMN, 0, len(series))
	for _, m := range series {
		slot := m["deviceslot"]
		if slot == "" || seen[slot] {
			continue
		}
		seen[slot] = true
		zone := strings.ToLower(m["zone"])
		if zone == "" {
			zone = zoneFromRackID(rackID)
		}
		// topoMetric is the as-built view, so every series carries the real
		// short BMN ("bmn", e.g. s95t4nb4). Should one ever lack it, leave
		// BMNName empty rather than falling back to the deviceslot — empty
		// honestly signals "no BMN reported" instead of duplicating the slot.
		out = append(out, BMN{
			Deviceslot: slot,
			BMNName:    m["bmn"],
			Rack:       rackID,
			Zone:       zone,
			CTPosition: positionFromDeviceslot(slot),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CTPosition != out[j].CTPosition {
			return out[i].CTPosition < out[j].CTPosition
		}
		return out[i].Deviceslot < out[j].Deviceslot
	})
	return out, nil
}

// queryFacts matches every node-class deviceslot in the rack on factMetric,
// fanned out across all super-regions by the shared vm.Client. The
// "-node-<N>-" infix excludes CDU / power-shelf / NVLink slots, leaving
// only compute trays. A malformed rack id can't match any deviceslot, so
// it returns a clean miss (callers surface "unresolved" rather than
// "internal"). This is the hardware-facts source (instance_type, cw_sku).
func (r *VMResolver) queryFacts(ctx context.Context, rackID string) ([]map[string]string, error) {
	prefix, suffix, ok := splitRackID(rackID)
	if !ok {
		return nil, nil
	}
	q := fmt.Sprintf(`%s{deviceslot=~"^%s-node-[0-9]+-%s$"}`, factMetric, prefix, suffix)
	return r.vm.QueryVector(ctx, q)
}

// queryTopology matches every as-built node in the rack on topoMetric,
// scoped by the authoritative rack-name label and fanned out across all
// super-regions. The rack id is anchored (^...$) so a longer rack name
// can't match as a substring; the id carries no regex metacharacters in
// practice. This is the tray-enumeration + BMN-identity source.
func (r *VMResolver) queryTopology(ctx context.Context, rackID string) ([]map[string]string, error) {
	q := fmt.Sprintf(`%s{%s=~"^%s$"}`, topoMetric, topoRackLabel, rackID)
	return r.vm.QueryVector(ctx, q)
}

// ResolveDomainScope satisfies Resolver. It reads the NVLink-domain selector
// labels (region/zone/cluster/nvlink_domain) for a rack off domainMetric,
// scoped by the rack's node deviceslot regex (same regex shape as queryFacts).
// region/zone are returned RAW (not lower-cased) because the alert join matches
// on the stored label value. Empty result returns ErrNotFound.
func (r *VMResolver) ResolveDomainScope(ctx context.Context, rackID string) (DomainScope, error) {
	if rackID == "" {
		return DomainScope{}, ErrEmptyRack
	}
	series, err := r.queryDomainScope(ctx, rackID)
	if err != nil {
		return DomainScope{}, err
	}
	if len(series) == 0 {
		return DomainScope{}, ErrNotFound
	}
	dom := dominantScope(series)
	return DomainScope{
		Region:       dom["region"],
		Zone:         dom["zone"],
		Cluster:      dom["cluster"],
		NVLinkDomain: dom["nvlink_domain"],
	}, nil
}

// queryDomainScope matches every node in the rack on domainMetric, scoped by
// the node deviceslot regex and fanned out across all super-regions. A
// malformed rack id can't match any deviceslot and returns a clean miss.
func (r *VMResolver) queryDomainScope(ctx context.Context, rackID string) ([]map[string]string, error) {
	prefix, suffix, ok := splitRackID(rackID)
	if !ok {
		return nil, nil
	}
	q := fmt.Sprintf(`%s{deviceslot=~"^%s-node-[0-9]+-%s$"}`, domainMetric, prefix, suffix)
	return r.vm.QueryVector(ctx, q)
}

// dominantScope picks the most frequent (region, zone, cluster, nvlink_domain)
// labelset among a rack's domain series and returns that series' label map. A
// rack belongs to one NVLink domain, so this collapses any stray series down to
// the rack's headline domain scope. Ties broken by key for determinism.
func dominantScope(series []map[string]string) map[string]string {
	type tally struct {
		count  int
		labels map[string]string
	}
	groups := map[string]*tally{}
	for _, m := range series {
		key := m["region"] + "|" + m["zone"] + "|" + m["cluster"] + "|" + m["nvlink_domain"]
		if g, ok := groups[key]; ok {
			g.count++
		} else {
			groups[key] = &tally{count: 1, labels: m}
		}
	}
	var best *tally
	var bestKey string
	for k, g := range groups {
		if best == nil || g.count > best.count || (g.count == best.count && k < bestKey) {
			best, bestKey = g, k
		}
	}
	return best.labels
}

// dominantLabels picks the most frequent (instance_type, cw_sku, region,
// cluster) labelset among the rack's series and returns that series'
// full label map. A rack is hardware-homogeneous, so this collapses the
// minor cluster/sku spread (e.g. a mgmt tray reporting a different
// cw_sku) down to the rack's headline facts without dropping any trays.
func dominantLabels(series []map[string]string) map[string]string {
	type tally struct {
		count  int
		labels map[string]string
	}
	groups := map[string]*tally{}
	for _, m := range series {
		key := m["instance_type"] + "|" + m["cw_sku"] + "|" + m["region"] + "|" + m["cluster"]
		if g, ok := groups[key]; ok {
			g.count++
		} else {
			groups[key] = &tally{count: 1, labels: m}
		}
	}
	var best *tally
	var bestKey string
	for k, g := range groups {
		// Higher count wins; ties broken by key for determinism.
		if best == nil || g.count > best.count || (g.count == best.count && k < bestKey) {
			best, bestKey = g, k
		}
	}
	return best.labels
}

// splitRackID splits a canonical rack id into the deviceslot prefix
// (first two dash components, e.g. "dh1000-r003") and the region suffix
// (everything after, e.g. "ca-east-01a"). Mirrors the boundary
// zoneFromRackID uses. Returns ok=false when the id has fewer than
// three dash components.
func splitRackID(rackID string) (prefix, suffix string, ok bool) {
	parts := strings.SplitN(rackID, "-", 3)
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[0] + "-" + parts[1], parts[2], true
}

// deriveVariant strips the trailing "-4x"/"-8x" sizing from an
// instance_type to get the GPU variant (gb200-4x -> gb200). VM carries
// no explicit variant label, so we derive it. Returns "" for an empty
// or single-segment instance_type.
func deriveVariant(instanceType string) string {
	if i := strings.IndexByte(instanceType, '-'); i > 0 {
		return instanceType[:i]
	}
	return ""
}

// deriveGBGeneration maps a Grace-Blackwell instance_type to its silicon
// generation. VM carries no gb_generation label; the demo fixtures
// stamped "blackwell" for gb200/gb300, so we reproduce that. Unknown
// families return "" (surfaced as an explicit empty fact, not guessed).
func deriveGBGeneration(instanceType string) string {
	switch {
	case strings.HasPrefix(instanceType, "gb200"), strings.HasPrefix(instanceType, "gb300"):
		return "blackwell"
	default:
		return ""
	}
}
