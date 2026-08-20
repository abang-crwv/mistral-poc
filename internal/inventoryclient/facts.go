package inventoryclient

import (
	"context"

	"qac/internal/facts"
)

// RackFacts is the structured fact set extracted from a rack-kind Hit's
// Labels. Field tags carry the registered fact-key string so ToFactBatch
// can build a map[facts.Key]any without a hand-written switch.
type RackFacts struct {
	InstanceType string `fact:"instance_type"`
	SKU          string `fact:"sku"`
	Variant      string `fact:"variant"`
	GBGeneration string `fact:"gb_generation"`
	Region       string `fact:"region"`
	Cluster      string `fact:"cluster"`
}

// ResolveRack performs a rack-scoped lookup and extracts RackFacts from
// the rack-kind Hit's Labels. Returns ErrNotFound if no rack-kind hit
// matches; AmbiguousError if multiple rack-kind hits are present with
// distinct regions.
func ResolveRack(ctx context.Context, r Resolver, rackID string) (RackFacts, error) {
	hits, err := r.Lookup(ctx, rackID)
	if err != nil {
		return RackFacts{}, err
	}
	if len(hits) == 0 {
		return RackFacts{}, ErrNotFound
	}

	// Find rack-kind hits. Ignore BMN/CDU/etc.
	rackHits := hits[:0:0]
	for i := range hits {
		if hits[i].Kind == KindRack {
			rackHits = append(rackHits, hits[i])
		}
	}
	if len(rackHits) == 0 {
		return RackFacts{}, ErrNotFound
	}
	if len(rackHits) > 1 {
		// Multiple rack-kind hits — if they're in different regions, it's
		// ambiguous. (Same region with duplicate hits would just be the
		// same rack twice; we'd pick the first.)
		regions := map[string]bool{}
		for _, h := range rackHits {
			regions[h.Region] = true
		}
		if len(regions) > 1 {
			return RackFacts{}, &AmbiguousError{ID: rackID, Hits: rackHits}
		}
	}
	return rackFactsFromLabels(rackHits[0].Labels), nil
}

// rackFactsFromLabels reads each tagged field by name from labels.
// Missing labels yield empty strings — preserved as the canonical
// fact value rather than dropped (callers can filter if they want).
func rackFactsFromLabels(labels map[string]string) RackFacts {
	return RackFacts{
		InstanceType: labels["instance_type"],
		SKU:          labels["sku"],
		Variant:      labels["variant"],
		GBGeneration: labels["gb_generation"],
		Region:       labels["region"],
		Cluster:      labels["cluster"],
	}
}

// ToFactBatch converts the RackFacts to a map[facts.Key]any suitable
// for facts.Emit. Empty-string fields are still included — callers
// (operators, agents) see explicit "" rather than a missing key.
func (rf RackFacts) ToFactBatch() map[facts.Key]any {
	return map[facts.Key]any{
		facts.KeyInstanceType: rf.InstanceType,
		facts.KeySKU:          rf.SKU,
		facts.KeyVariant:      rf.Variant,
		facts.KeyGBGeneration: rf.GBGeneration,
		facts.KeyRegion:       rf.Region,
		facts.KeyCluster:      rf.Cluster,
	}
}
