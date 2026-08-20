package lifecycleclient

import (
	"context"
	"testing"
)

func TestMapClient_QueryRLCCIgnored_Hit(t *testing.T) {
	c := NewMapClient(map[RackKey][]IgnoredBMN{
		{Rack: "dh3-r012-us-east-01a", Zone: "us-east-01a"}: {
			{Deviceslot: "dh3-r012-node-07-us-east-01a", BMNName: "s90txs57"},
		},
	})
	out, err := c.QueryRLCCIgnored(context.Background(),
		RackKey{Rack: "dh3-r012-us-east-01a", Zone: "us-east-01a"})
	if err != nil {
		t.Fatalf("QueryRLCCIgnored: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	if out[0].Deviceslot != "dh3-r012-node-07-us-east-01a" {
		t.Errorf("Deviceslot = %q", out[0].Deviceslot)
	}
}

func TestMapClient_QueryRLCCIgnored_NoIgnores(t *testing.T) {
	c := NewMapClient(map[RackKey][]IgnoredBMN{})
	out, err := c.QueryRLCCIgnored(context.Background(),
		RackKey{Rack: "dh3-r012-us-east-01a", Zone: "us-east-01a"})
	if err != nil {
		t.Fatalf("QueryRLCCIgnored: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("len = %d, want 0", len(out))
	}
}

func TestMapClient_QueryRLCCIgnored_EmptyConstructor(t *testing.T) {
	c := NewMapClient(nil)
	out, err := c.QueryRLCCIgnored(context.Background(),
		RackKey{Rack: "anything", Zone: "any"})
	if err != nil {
		t.Fatalf("QueryRLCCIgnored: %v", err)
	}
	if out != nil {
		t.Errorf("out = %v, want nil", out)
	}
}

func TestMapClient_QueryActionState_DefaultThreeTicks(t *testing.T) {
	c := NewMapClient(nil)
	dss := []string{"ct-01", "ct-02"}
	// Ticks 1 and 2: not yet complete.
	for tick := 1; tick <= 2; tick++ {
		got, err := c.QueryActionState(context.Background(), dss, "l11-fielddiag")
		if err != nil {
			t.Fatalf("tick %d: QueryActionState: %v", tick, err)
		}
		for _, ds := range dss {
			if got[ds] {
				t.Errorf("tick %d: %s completed early", tick, ds)
			}
		}
	}
	// Tick 3: complete (default completeAfter == 3).
	got, err := c.QueryActionState(context.Background(), dss, "l11-fielddiag")
	if err != nil {
		t.Fatalf("tick 3: QueryActionState: %v", err)
	}
	for _, ds := range dss {
		if !got[ds] {
			t.Errorf("tick 3: %s not completed", ds)
		}
	}
}

func TestMapClient_QueryActionState_SetCompleteAfter(t *testing.T) {
	c := NewMapClient(nil)
	c.SetActionCompleteAfter("zap", 1)
	got, err := c.QueryActionState(context.Background(), []string{"ct-01"}, "zap")
	if err != nil {
		t.Fatalf("QueryActionState: %v", err)
	}
	if !got["ct-01"] {
		t.Error("completeAfter=1 should complete on first tick")
	}
}

func TestMapClient_QueryFLCCState_DefaultAndOverride(t *testing.T) {
	c := NewMapClient(nil)
	dss := []string{"ds-a", "ds-b"}

	// Default: unseeded deviceslots report a zero-value observation.
	got, err := c.QueryFLCCState(context.Background(), dss)
	if err != nil {
		t.Fatalf("QueryFLCCState: %v", err)
	}
	if got["ds-a"].State != "" {
		t.Errorf("unseeded ds-a state = %q, want empty", got["ds-a"].State)
	}

	// Seeded observation is returned verbatim.
	c.SetFLCCObservation("ds-a", FLCCObservation{State: "fail", Step: "fielddiag", PrevStep: "fielddiag", Workflow: "provision"})
	got, err = c.QueryFLCCState(context.Background(), dss)
	if err != nil {
		t.Fatalf("QueryFLCCState: %v", err)
	}
	if got["ds-a"].State != "fail" || got["ds-a"].Step != "fielddiag" {
		t.Errorf("ds-a = %+v, want state=fail step=fielddiag", got["ds-a"])
	}
	if got["ds-b"].State != "" {
		t.Errorf("ds-b = %+v, want zero", got["ds-b"])
	}
}
