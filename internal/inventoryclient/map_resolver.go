package inventoryclient

import (
	"context"
	"fmt"
	"strings"
)

// MapResolver is a deterministic in-memory Resolver for tests and the
// iter-4a default backend. A future iteration replaces it with a real
// VM/Netbox-backed Resolver implementing the same interface — no
// changes to callers required.
type MapResolver struct {
	table map[string][]Hit
	bmns  map[string][]BMN
}

// NewMapResolver returns a Resolver backed by the given lookup table.
// Use NewMapResolverWithBMNs to attach per-rack BMN fixtures; this
// constructor leaves the BMN map empty (ResolveBMNs returns ErrNotFound
// for every rack), which is appropriate for tests that don't exercise
// the per-CT path.
func NewMapResolver(table map[string][]Hit) *MapResolver {
	return &MapResolver{table: table, bmns: map[string][]BMN{}}
}

// NewMapResolverWithBMNs is the construction shape used by
// cmd/qac/serve.go: it seeds both the rack-Hit table and the
// rack→BMN list table from SeedDemoFixtures + SeedDemoBMNs.
func NewMapResolverWithBMNs(table map[string][]Hit, bmns map[string][]BMN) *MapResolver {
	return &MapResolver{table: table, bmns: bmns}
}

// Lookup satisfies Resolver. Returns (nil, nil) on miss — matches the
// tiphys/inventory contract where the absence of hits is not an error.
// AmbiguousError is the caller's (ResolveRack's) concern — Lookup just
// hands back whatever hits the table has.
func (m *MapResolver) Lookup(ctx context.Context, id string) ([]Hit, error) {
	if hits, ok := m.table[id]; ok {
		return hits, nil
	}
	return nil, nil
}

// ResolveBMNs satisfies Resolver. Returns the BMN list for the given rack id.
// Returns ErrEmptyRack when rackID is empty; ErrNotFound when the rack id
// has no BMN list seeded in this MapResolver.
func (m *MapResolver) ResolveBMNs(ctx context.Context, rackID string) ([]BMN, error) {
	if rackID == "" {
		return nil, ErrEmptyRack
	}
	bmns, ok := m.bmns[rackID]
	if !ok {
		return nil, ErrNotFound
	}
	// Return a copy so callers can't mutate the seeded fixture.
	out := make([]BMN, len(bmns))
	copy(out, bmns)
	return out, nil
}

// ResolveDomainScope satisfies Resolver. It derives a deterministic
// DomainScope from the rack id (zone from the id, region/cluster from the
// seeded Hit table when present) and a synthesized uppercase nvlink_domain.
// The demo path never queries a real domain, so the synthesized values only
// need to be internally consistent. Returns ErrEmptyRack for an empty id.
func (m *MapResolver) ResolveDomainScope(ctx context.Context, rackID string) (DomainScope, error) {
	if rackID == "" {
		return DomainScope{}, ErrEmptyRack
	}
	return deriveDemoDomainScope(rackID, m.table[rackID]), nil
}

// deriveDemoDomainScope builds the demo DomainScope for a rack. zone/region
// are upper-cased to mirror how the real metric stores them; nvlink_domain is
// the rack id with the "-r" rack marker dropped, upper-cased
// (dh3-r012-us-east-01a -> DH3-012-US-EAST-01A).
func deriveDemoDomainScope(rackID string, hits []Hit) DomainScope {
	zone := zoneFromRackID(rackID)
	region := zone
	cluster := ""
	if len(hits) > 0 {
		if hits[0].Region != "" {
			region = hits[0].Region
		}
		cluster = hits[0].Cluster
	}
	nvlinkDomain := strings.ToUpper(strings.Replace(rackID, "-r", "-", 1))
	return DomainScope{
		Region:       strings.ToUpper(region),
		Zone:         strings.ToUpper(zone),
		Cluster:      cluster,
		NVLinkDomain: nvlinkDomain,
	}
}

// SeedDemoDomainScopes returns the DomainScope for every rack id in
// SeedDemoBMNs, derived the same way MapResolver.ResolveDomainScope does.
// Useful for fixture-backed alert-category clients and coverage tests.
func SeedDemoDomainScopes() map[string]DomainScope {
	fixtures := SeedDemoFixtures()
	out := map[string]DomainScope{}
	for rid := range SeedDemoBMNs() {
		out[rid] = deriveDemoDomainScope(rid, fixtures[rid])
	}
	return out
}

// SeedDemoBMNs returns a deterministic rack→[]BMN map covering every
// rack id present in SeedDemoFixtures. 18 BMNs per rack matches the
// GB200 NVL72 form factor.
func SeedDemoBMNs() map[string][]BMN {
	rackIDs := []string{
		"dh3-r012-us-east-01a",
		"dh3-r012-us-east-02a",
		"dh4-r020-us-west-01b",
		"dh4-r020-us-west-02b",
	}
	out := map[string][]BMN{}
	for _, rid := range rackIDs {
		zone := zoneFromRackID(rid)
		// Trim the zone suffix to recover the "dh3-r012" prefix.
		prefix := rid[:len(rid)-len(zone)-1]
		bmns := make([]BMN, 0, 18)
		for pos := 1; pos <= 18; pos++ {
			slot := fmt.Sprintf("%s-node-%02d-%s", prefix, pos, zone)
			bmns = append(bmns, BMN{
				Deviceslot: slot,
				BMNName:    fmt.Sprintf("s90txs%02d", pos+50),
				Rack:       rid,
				Zone:       zone,
				CTPosition: pos,
			})
		}
		out[rid] = bmns
	}
	return out
}

// SeedDemoFixtures returns a deterministic table covering the rack ids
// used by `fwqual seed-demo` and the canonical canary template. iter-4a
// production wiring (serve cmd) injects this same fixture; later
// iterations swap in the real resolver.
//
// Coverage guarantee enforced by internal/inventoryclient/fixture_coverage_test.go:
//   - Two gb200-4x racks (sku=nvl72, gb_generation=blackwell)
//   - Two gb300-4x racks (sku=nvl72, gb_generation=blackwell)
//   - One ambiguous-error fixture entry (multi-region) used by
//     facts_test.go's TestResolveRack_Ambiguous
func SeedDemoFixtures() map[string][]Hit {
	gb200Labels := func(rackID, cluster, region string) map[string]string {
		return map[string]string{
			"instance_type": "gb200-4x",
			"sku":           "nvl72",
			"variant":       "gb200",
			"gb_generation": "blackwell",
			"region":        region,
			"cluster":       cluster,
		}
	}
	gb300Labels := func(rackID, cluster, region string) map[string]string {
		return map[string]string{
			"instance_type": "gb300-4x",
			"sku":           "nvl72",
			"variant":       "gb300",
			"gb_generation": "blackwell",
			"region":        region,
			"cluster":       cluster,
		}
	}

	return map[string][]Hit{
		"dh3-r012-us-east-01a": {{
			ID: "dh3-r012-us-east-01a", Region: "us-east", Kind: KindRack,
			Cluster: "use-prod-01",
			Labels:  gb200Labels("dh3-r012-us-east-01a", "use-prod-01", "us-east"),
		}},
		"dh3-r012-us-east-02a": {{
			ID: "dh3-r012-us-east-02a", Region: "us-east", Kind: KindRack,
			Cluster: "use-prod-01",
			Labels:  gb200Labels("dh3-r012-us-east-02a", "use-prod-01", "us-east"),
		}},
		"dh4-r020-us-west-01b": {{
			ID: "dh4-r020-us-west-01b", Region: "us-west", Kind: KindRack,
			Cluster: "usw-prod-02",
			Labels:  gb300Labels("dh4-r020-us-west-01b", "usw-prod-02", "us-west"),
		}},
		"dh4-r020-us-west-02b": {{
			ID: "dh4-r020-us-west-02b", Region: "us-west", Kind: KindRack,
			Cluster: "usw-prod-02",
			Labels:  gb300Labels("dh4-r020-us-west-02b", "usw-prod-02", "us-west"),
		}},
		// Ambiguous fixture: same id resolves to two regions. Used by
		// facts_test.go to assert ResolveRack returns AmbiguousError.
		"dh9-r999-ambig-test-01a": {
			{
				ID: "dh9-r999-ambig-test-01a", Region: "us-east", Kind: KindRack,
				Cluster: "use-prod-01",
				Labels:  gb200Labels("dh9-r999-ambig-test-01a", "use-prod-01", "us-east"),
			},
			{
				ID: "dh9-r999-ambig-test-01a", Region: "us-west", Kind: KindRack,
				Cluster: "usw-prod-02",
				Labels:  gb200Labels("dh9-r999-ambig-test-01a", "usw-prod-02", "us-west"),
			},
		},
	}
}
