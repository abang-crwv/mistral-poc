package firmwareinventoryprobe

import (
	"context"
	"encoding/json"
	"testing"

	"qac/internal/firmwareclient"
	"qac/internal/probe"
)

func TestProbe_TypeAndCategory(t *testing.T) {
	p := New()
	if p.Type() != "firmware_inventory_probe" {
		t.Errorf("Type = %q, want firmware_inventory_probe", p.Type())
	}
	if p.Category() != probe.CategoryGatherer {
		t.Errorf("Category = %q, want gatherer", p.Category())
	}
}

// evidence is the decode shape for the structured output.
type evidence struct {
	ProbedAt int64  `json:"probed_at"`
	Source   string `json:"source"`
	PerRack  map[string]struct {
		Resolved     bool `json:"resolved"`
		ByDeviceslot map[string][]struct {
			Component string `json:"component"`
			Version   string `json:"version"`
		} `json:"by_deviceslot"`
		Error string `json:"error"`
	} `json:"per_rack"`
	AllResolved bool `json:"all_resolved"`
}

func TestProbe_Run_GroupsByDeviceslot_AllResolved(t *testing.T) {
	c := probe.Clients{FirmwareClient: firmwareclient.NewMapClient(map[string][]firmwareclient.FirmwareReading{
		"rack-a": {
			{Deviceslot: "rack-a-node-01", Component: "BIOS", Version: "1.0"},
			{Deviceslot: "rack-a-node-01", Component: "BMC", Version: "2.0"},
			{Deviceslot: "rack-a-node-02", Component: "BIOS", Version: "1.0"},
		},
	})}
	sc := probe.StepContext{RunID: "x", StepID: "firmware_baseline", Racks: []string{"rack-a"}}

	out, err := New().Run(context.Background(), c, sc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var body evidence
	if err := json.Unmarshal(out.StructuredData, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.AllResolved {
		t.Errorf("all_resolved = false, want true")
	}
	if body.Source != "redfish-firmware-version" {
		t.Errorf("source = %q, want redfish-firmware-version", body.Source)
	}
	ra := body.PerRack["rack-a"]
	if !ra.Resolved {
		t.Fatalf("rack-a resolved = false, want true")
	}
	if len(ra.ByDeviceslot["rack-a-node-01"]) != 2 {
		t.Errorf("node-01 readings = %d, want 2", len(ra.ByDeviceslot["rack-a-node-01"]))
	}
	if len(ra.ByDeviceslot["rack-a-node-02"]) != 1 {
		t.Errorf("node-02 readings = %d, want 1", len(ra.ByDeviceslot["rack-a-node-02"]))
	}
	if out.ProbedAt == 0 {
		t.Errorf("ProbedAt = 0, want non-zero")
	}
	if len(out.Sources) != 1 || out.Sources[0] != "redfish-firmware-version" {
		t.Errorf("out.Sources = %+v, want [redfish-firmware-version]", out.Sources)
	}
}

func TestProbe_Run_EmptyInventory_RecordedAsUnresolved(t *testing.T) {
	// rack-known has readings; rack-empty has none → resolved=false, all_resolved=false.
	c := probe.Clients{FirmwareClient: firmwareclient.NewMapClient(map[string][]firmwareclient.FirmwareReading{
		"rack-known": {{Deviceslot: "rack-known-node-01", Component: "BIOS", Version: "1.0"}},
	})}
	sc := probe.StepContext{RunID: "x", StepID: "firmware_baseline", Racks: []string{"rack-known", "rack-empty"}}

	out, err := New().Run(context.Background(), c, sc)
	if err != nil {
		t.Fatalf("Run: %v (empty inventory is data, not error)", err)
	}
	var body evidence
	if err := json.Unmarshal(out.StructuredData, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.AllResolved {
		t.Errorf("all_resolved = true, want false (one rack empty)")
	}
	if !body.PerRack["rack-known"].Resolved {
		t.Errorf("rack-known.resolved = false, want true")
	}
	if body.PerRack["rack-empty"].Resolved {
		t.Errorf("rack-empty.resolved = true, want false")
	}
	if body.PerRack["rack-empty"].Error == "" {
		t.Errorf("rack-empty.error empty, want a no-firmware message")
	}
}

func TestProbe_Run_SourceError_Bubbles(t *testing.T) {
	c := probe.Clients{FirmwareClient: firmwareclient.NewMapClient(nil)}
	sc := probe.StepContext{RunID: "x", StepID: "firmware_baseline", Racks: []string{firmwareclient.FailingSourceRack}}

	_, err := New().Run(context.Background(), c, sc)
	if err == nil {
		t.Fatal("Run returned nil error; want bubble-up of source failure")
	}
}

func TestProbe_Run_NilClient_Errors(t *testing.T) {
	_, err := New().Run(context.Background(), probe.Clients{}, probe.StepContext{Racks: []string{"r"}})
	if err == nil {
		t.Fatal("Run returned nil error; want nil-client error")
	}
}

// bundleEvidence decodes the convergence fields the base evidence struct omits.
type bundleEvidence struct {
	PerRack map[string]struct {
		Bundles map[string]struct {
			CurrentBundle string `json:"current_bundle"`
			TargetBundle  string `json:"target_bundle"`
			OnTarget      bool   `json:"on_target"`
		} `json:"bundles"`
		OffTarget   []string `json:"off_target"`
		AllOnTarget bool     `json:"all_on_target"`
	} `json:"per_rack"`
}

func TestProbe_Run_BundleConvergence(t *testing.T) {
	c := probe.Clients{FirmwareClient: firmwareclient.NewMapClient(map[string][]firmwareclient.FirmwareReading{
		"rack-a": {{Deviceslot: "rack-a-node-01", Component: "BIOS", Version: "1.0"}},
	}).WithBundles(map[string][]firmwareclient.BundleStatus{
		"rack-a": {
			{Deviceslot: "rack-a-node-01", CurrentBundle: "b2", TargetBundle: "b2", OnTarget: true},
			{Deviceslot: "rack-a-node-02", CurrentBundle: "b1", TargetBundle: "b2", OnTarget: false},
		},
	})}
	sc := probe.StepContext{Racks: []string{"rack-a"}}

	out, err := New().Run(context.Background(), c, sc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var body bundleEvidence
	if err := json.Unmarshal(out.StructuredData, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	ra := body.PerRack["rack-a"]
	if len(ra.Bundles) != 2 {
		t.Fatalf("bundles = %d, want 2", len(ra.Bundles))
	}
	if !ra.Bundles["rack-a-node-01"].OnTarget || ra.Bundles["rack-a-node-02"].OnTarget {
		t.Errorf("convergence per node wrong: %+v", ra.Bundles)
	}
	if ra.AllOnTarget {
		t.Errorf("all_on_target = true, want false (one node off target)")
	}
	if len(ra.OffTarget) != 1 || ra.OffTarget[0] != "rack-a-node-02" {
		t.Errorf("off_target = %v, want [rack-a-node-02]", ra.OffTarget)
	}
}

func TestProbe_Run_AllOnTarget_WhenConverged(t *testing.T) {
	c := probe.Clients{FirmwareClient: firmwareclient.NewMapClient(nil).WithBundles(map[string][]firmwareclient.BundleStatus{
		"rack-a": {{Deviceslot: "rack-a-node-01", CurrentBundle: "b2", TargetBundle: "b2", OnTarget: true}},
	})}
	out, err := New().Run(context.Background(), c, probe.StepContext{Racks: []string{"rack-a"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var body bundleEvidence
	_ = json.Unmarshal(out.StructuredData, &body)
	ra := body.PerRack["rack-a"]
	if !ra.AllOnTarget || len(ra.OffTarget) != 0 {
		t.Errorf("all_on_target=%v off_target=%v, want true/[]", ra.AllOnTarget, ra.OffTarget)
	}
}
