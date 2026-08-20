package inventoryclient

import (
	"context"
	"errors"
	"testing"

	"qac/internal/facts"
)

func TestResolveRack_HappyPath(t *testing.T) {
	r := NewMapResolver(SeedDemoFixtures())
	rf, err := ResolveRack(context.Background(), r, "dh3-r012-us-east-01a")
	if err != nil {
		t.Fatalf("ResolveRack: %v", err)
	}
	if rf.InstanceType != "gb200-4x" {
		t.Errorf("InstanceType = %q, want gb200-4x", rf.InstanceType)
	}
	if rf.SKU != "nvl72" {
		t.Errorf("SKU = %q, want nvl72", rf.SKU)
	}
	if rf.Variant != "gb200" {
		t.Errorf("Variant = %q, want gb200", rf.Variant)
	}
	if rf.GBGeneration != "blackwell" {
		t.Errorf("GBGeneration = %q, want blackwell", rf.GBGeneration)
	}
	if rf.Region != "us-east" {
		t.Errorf("Region = %q, want us-east", rf.Region)
	}
	if rf.Cluster != "use-prod-01" {
		t.Errorf("Cluster = %q, want use-prod-01", rf.Cluster)
	}
}

func TestResolveRack_MissingLabelsReturnEmpty(t *testing.T) {
	r := NewMapResolver(map[string][]Hit{
		"dh5-r001-eu-west-01a": {{
			ID:     "dh5-r001-eu-west-01a",
			Region: "eu-west",
			Kind:   KindRack,
			Labels: map[string]string{"instance_type": "gb200-4x"}, // only one label
		}},
	})
	rf, err := ResolveRack(context.Background(), r, "dh5-r001-eu-west-01a")
	if err != nil {
		t.Fatalf("ResolveRack: %v", err)
	}
	if rf.InstanceType != "gb200-4x" {
		t.Errorf("InstanceType = %q", rf.InstanceType)
	}
	if rf.SKU != "" {
		t.Errorf("SKU = %q, want empty", rf.SKU)
	}
}

func TestResolveRack_NotFound(t *testing.T) {
	r := NewMapResolver(SeedDemoFixtures())
	_, err := ResolveRack(context.Background(), r, "dh99-r999-no-such-99z")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestResolveRack_NonRackHitIgnored(t *testing.T) {
	// A BMN-kind hit only (no rack-kind hit) should yield ErrNotFound.
	r := NewMapResolver(map[string][]Hit{
		"dh5-r001-eu-west-01a": {{
			ID:     "dh5-r001-eu-west-01a",
			Region: "eu-west",
			Kind:   KindBMN, // not KindRack
			Labels: map[string]string{"instance_type": "gb200-4x"},
		}},
	})
	_, err := ResolveRack(context.Background(), r, "dh5-r001-eu-west-01a")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (only BMN hit)", err)
	}
}

func TestResolveRack_AmbiguousMultiRegion(t *testing.T) {
	r := NewMapResolver(SeedDemoFixtures())
	_, err := ResolveRack(context.Background(), r, "dh9-r999-ambig-test-01a")
	var ambig *AmbiguousError
	if !errors.As(err, &ambig) {
		t.Fatalf("err type = %T, want *AmbiguousError; err = %v", err, err)
	}
	regs := ambig.Regions()
	if len(regs) != 2 {
		t.Fatalf("regions = %v, want 2", regs)
	}
}

func TestToFactBatch_MapsAllSixKeys(t *testing.T) {
	rf := RackFacts{
		InstanceType: "gb200-4x",
		SKU:          "nvl72",
		Variant:      "gb200",
		GBGeneration: "blackwell",
		Region:       "us-east",
		Cluster:      "use-prod-01",
	}
	batch := rf.ToFactBatch()
	if len(batch) != 6 {
		t.Fatalf("len = %d, want 6", len(batch))
	}
	want := map[facts.Key]string{
		facts.KeyInstanceType: "gb200-4x",
		facts.KeySKU:          "nvl72",
		facts.KeyVariant:      "gb200",
		facts.KeyGBGeneration: "blackwell",
		facts.KeyRegion:       "us-east",
		facts.KeyCluster:      "use-prod-01",
	}
	for k, w := range want {
		v, ok := batch[k]
		if !ok {
			t.Errorf("batch missing %q", k)
			continue
		}
		s, ok := v.(string)
		if !ok || s != w {
			t.Errorf("batch[%q] = %v, want %q", k, v, w)
		}
	}
}
