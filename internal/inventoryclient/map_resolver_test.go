package inventoryclient

import (
	"context"
	"testing"
)

func TestMapResolver_LookupHappy(t *testing.T) {
	r := NewMapResolver(SeedDemoFixtures())
	hits, err := r.Lookup(context.Background(), "dh3-r012-us-east-01a")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("len = %d, want 1", len(hits))
	}
	if hits[0].Kind != KindRack {
		t.Fatalf("kind = %q, want rack", hits[0].Kind)
	}
	if hits[0].Labels["instance_type"] != "gb200-4x" {
		t.Fatalf("instance_type = %q, want gb200-4x", hits[0].Labels["instance_type"])
	}
}

func TestMapResolver_LookupMissReturnsNil(t *testing.T) {
	r := NewMapResolver(SeedDemoFixtures())
	hits, err := r.Lookup(context.Background(), "dh99-r999-us-east-99z")
	if err != nil {
		t.Fatalf("Lookup miss should not error: %v", err)
	}
	if hits != nil {
		t.Fatalf("hits = %v, want nil", hits)
	}
}

func TestMapResolver_AmbiguousFixturePresent(t *testing.T) {
	r := NewMapResolver(SeedDemoFixtures())
	hits, err := r.Lookup(context.Background(), "dh9-r999-ambig-test-01a")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(hits) < 2 {
		t.Fatalf("len = %d, want >= 2 (this fixture exists so ResolveRack can return AmbiguousError)", len(hits))
	}
	// Confirm regions differ — that's what would trigger an AmbiguousError
	// when ResolveRack inspects the hits.
	seen := map[string]bool{}
	for _, h := range hits {
		seen[h.Region] = true
	}
	if len(seen) < 2 {
		t.Fatalf("regions = %v, want 2+ distinct", seen)
	}
}
