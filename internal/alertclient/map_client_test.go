package alertclient

import (
	"errors"
	"testing"
)

func TestMapAlertClient_ListByRack_Happy(t *testing.T) {
	c := NewMapAlertClient(map[string][]Alert{
		"dh3-r012-us-east-02a": {
			{ID: "a1", Name: "GPUTempHigh", Severity: "warning", FiredAt: 1700000000, Scope: "dh3-r012-us-east-02a"},
		},
	}, nil)
	got, err := c.ListByRack(t.Context(), "dh3-r012-us-east-02a")
	if err != nil {
		t.Fatalf("ListByRack: %v", err)
	}
	if len(got) != 1 || got[0].Severity != "warning" {
		t.Fatalf("got = %+v, want one warning alert", got)
	}
}

func TestMapAlertClient_ListByRack_Miss_ReturnsEmpty(t *testing.T) {
	c := NewMapAlertClient(map[string][]Alert{}, nil)
	got, err := c.ListByRack(t.Context(), "no-such-rack")
	if err != nil {
		t.Fatalf("ListByRack: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want empty slice", got)
	}
}

func TestMapAlertClient_ListByRack_FailingSimulator(t *testing.T) {
	// The "failing-source-rack" sentinel id triggers ErrSourceUnavailable.
	c := NewMapAlertClient(SeedDemoAlerts(), nil)
	_, err := c.ListByRack(t.Context(), "failing-source-rack")
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err = %v, want ErrSourceUnavailable", err)
	}
}

func TestSeedDemoAlerts_HasExpectedRacks(t *testing.T) {
	a := SeedDemoAlerts()
	for _, rack := range []string{
		"dh3-r012-us-east-01a",
		"dh3-r012-us-east-02a",
		"dh4-r020-us-west-01b",
		"dh4-r020-us-west-02b",
		"dh9-r999-ambig-test-01a",
	} {
		if _, ok := a[rack]; !ok {
			t.Errorf("SeedDemoAlerts missing rack %q", rack)
		}
	}
	// dh3-r012-us-east-02a has one warning; dh4-r020-us-west-02b has one critical.
	if len(a["dh3-r012-us-east-02a"]) != 1 || a["dh3-r012-us-east-02a"][0].Severity != "warning" {
		t.Errorf("dh3-r012-us-east-02a = %+v, want one warning", a["dh3-r012-us-east-02a"])
	}
	if len(a["dh4-r020-us-west-02b"]) != 1 || a["dh4-r020-us-west-02b"][0].Severity != "critical" {
		t.Errorf("dh4-r020-us-west-02b = %+v, want one critical", a["dh4-r020-us-west-02b"])
	}
	if len(a["dh3-r012-us-east-01a"]) != 0 {
		t.Errorf("dh3-r012-us-east-01a should be empty (clean rack)")
	}
}

func TestMapAlertClient_ListByDeviceslot_Happy(t *testing.T) {
	ds := "dh3-r012-node-03-us-east-01a"
	c := NewMapAlertClient(nil, map[string][]Alert{
		ds: {{ID: "a1", Name: "NVLinkLinkDown", Severity: "critical", Scope: "dh3-r012-us-east-01a", Deviceslot: ds}},
	})
	got, err := c.ListByDeviceslot(t.Context(), ds)
	if err != nil {
		t.Fatalf("ListByDeviceslot: %v", err)
	}
	if len(got) != 1 || got[0].Deviceslot != ds {
		t.Fatalf("got %+v, want 1 alert on %s", got, ds)
	}
}

func TestMapAlertClient_ListByDeviceslot_Miss_ReturnsEmpty(t *testing.T) {
	c := NewMapAlertClient(nil, nil)
	got, err := c.ListByDeviceslot(t.Context(), "no-such-slot")
	if err != nil {
		t.Fatalf("ListByDeviceslot: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d, want 0", len(got))
	}
}

func TestMapAlertClient_ListByDeviceslot_FailingSentinel(t *testing.T) {
	c := NewMapAlertClient(nil, map[string][]Alert{failingDeviceslotID: {}})
	if _, err := c.ListByDeviceslot(t.Context(), failingDeviceslotID); !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err = %v, want ErrSourceUnavailable", err)
	}
}

// Compile-time satisfaction check.
var _ Client = (*MapAlertClient)(nil)
