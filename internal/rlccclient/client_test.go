package rlccclient

import (
	"bytes"
	"os"
	"reflect"
	"testing"
)

// TestFixtureMatchesTestdata is the drift-prevention test. If someone edits
// testdata/rlcc-values.yaml without re-running `go generate ./internal/rlccclient/`,
// this test fails — the committed Fixture map no longer matches the YAML.
// Reverse direction (edit fixture_gen.go by hand) also fails: re-parsing
// testdata won't match the edited Fixture.
func TestFixtureMatchesTestdata(t *testing.T) {
	body, err := os.ReadFile("testdata/rlcc-values.yaml")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	parsed, err := ParseValuesYAML(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("parse testdata: %v", err)
	}
	if !reflect.DeepEqual(parsed, Fixture) {
		t.Fatalf("Fixture drift detected.\n\nparsed (%d entries): %#v\n\nFixture (%d entries): %#v\n\nRun `go generate ./internal/rlccclient/` to refresh Fixture.",
			len(parsed), parsed, len(Fixture), Fixture)
	}
}

func TestFixture_GB200BringupHasExpectedActions(t *testing.T) {
	g, ok := Fixture["gb200-rack-bringup-v4"]
	if !ok {
		t.Fatal("missing gb200-rack-bringup-v4 in Fixture")
	}
	if g.ActionCount != 10 {
		t.Errorf("gb200-rack-bringup-v4 ActionCount = %d, want 10", g.ActionCount)
	}
	// action[3] = gb200-rack-provision-v4 (handler=provision)
	if got := g.Actions[3].Handler; got != HandlerProvision {
		t.Errorf("action[3].Handler = %q, want %q", got, HandlerProvision)
	}
	if got := g.Actions[3].Arguments.Components[ComponentComputeTray].Workflow; got != "gb200-rack-provision-v4" {
		t.Errorf("action[3] computetray.Workflow = %q, want gb200-rack-provision-v4", got)
	}
	// action[4] = await-rack-provision-v4; computetray.FLCCEndState = l10-test-loop
	if got := g.Actions[4].Arguments.Components[ComponentComputeTray].FLCCEndState; got != "l10-test-loop" {
		t.Errorf("action[4] computetray.FLCCEndState = %q, want l10-test-loop", got)
	}
}

func TestFixture_StateMoversHaveZeroActions(t *testing.T) {
	for _, name := range []string{"ready", "checked-in"} {
		g, ok := Fixture[name]
		if !ok {
			t.Errorf("missing %q in Fixture", name)
			continue
		}
		if g.ActionCount != 0 {
			t.Errorf("%s ActionCount = %d, want 0", name, g.ActionCount)
		}
	}
}
