package inventoryclient

import (
	"context"
	"errors"
	"testing"
)

func TestResolveBMNs_HappyPath(t *testing.T) {
	r := NewMapResolverWithBMNs(SeedDemoFixtures(), SeedDemoBMNs())
	bmns, err := r.ResolveBMNs(context.Background(), "dh3-r012-us-east-01a")
	if err != nil {
		t.Fatalf("ResolveBMNs: %v", err)
	}
	if len(bmns) != 18 {
		t.Errorf("len = %d, want 18", len(bmns))
	}
	if bmns[0].Rack != "dh3-r012-us-east-01a" {
		t.Errorf("Rack = %q, want dh3-r012-us-east-01a", bmns[0].Rack)
	}
	if bmns[0].Deviceslot == "" {
		t.Errorf("Deviceslot is empty")
	}
	if bmns[0].BMNName == "" {
		t.Errorf("BMNName is empty")
	}
	if bmns[0].Zone != "us-east-01a" {
		t.Errorf("Zone = %q, want us-east-01a", bmns[0].Zone)
	}
}

func TestResolveBMNs_PositionDerivedFromDeviceslot(t *testing.T) {
	r := NewMapResolverWithBMNs(SeedDemoFixtures(), SeedDemoBMNs())
	bmns, _ := r.ResolveBMNs(context.Background(), "dh3-r012-us-east-01a")
	// Deviceslot like "dh3-r012-node-07-us-east-01a" → CTPosition = 7
	found := false
	for _, b := range bmns {
		if b.CTPosition == 7 {
			found = true
			if !contains(b.Deviceslot, "node-07-") {
				t.Errorf("CTPosition=7 maps to deviceslot %q, expected node-07- in name", b.Deviceslot)
			}
		}
	}
	if !found {
		t.Errorf("no BMN with CTPosition=7 found")
	}
}

func TestResolveBMNs_NotFound(t *testing.T) {
	r := NewMapResolver(SeedDemoFixtures())
	_, err := r.ResolveBMNs(context.Background(), "no-such-rack")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestResolveBMNs_EmptyRackID(t *testing.T) {
	r := NewMapResolver(SeedDemoFixtures())
	_, err := r.ResolveBMNs(context.Background(), "")
	if err == nil {
		t.Errorf("ResolveBMNs(\"\"): want error")
	}
}

func TestResolveBMNs_SortedByPosition(t *testing.T) {
	r := NewMapResolverWithBMNs(SeedDemoFixtures(), SeedDemoBMNs())
	bmns, _ := r.ResolveBMNs(context.Background(), "dh3-r012-us-east-01a")
	for i := 1; i < len(bmns); i++ {
		if bmns[i].CTPosition < bmns[i-1].CTPosition {
			t.Errorf("BMNs not sorted by CTPosition: bmns[%d]=%d < bmns[%d]=%d",
				i, bmns[i].CTPosition, i-1, bmns[i-1].CTPosition)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
