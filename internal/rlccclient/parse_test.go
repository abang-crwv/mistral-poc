package rlccclient

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const threeWorkflowYAML = `
rlcc:
  workflows:
    - name: gb200-rack-bringup-v4
      description: Rack Bringup V4 for GB200.
      lifecycle: stable
      current-states: [checked-in, onboarding]
      desired-state: ready
      actions:
        - name: await-cdu-ps-init
          handler: await-checkpoint
          description: Wait for CDU and PSes to be inited
          arguments:
            checkpoint: rack
            components:
              coolantdistributionunit:
                checks: [count, init]
        - name: gb200-rack-provision-v4
          handler: provision
          arguments:
            checkpoint: rack
            checkpoint-args-step: await-rack-provision-v4
            components:
              computetray: {workflow: gb200-rack-provision-v4}
        - name: await-rack-provision-v4
          handler: await-checkpoint
          arguments:
            components:
              computetray:
                flcc-end-state: l10-test-loop
                workflow: gb200-rack-provision-v4
                checks: [count, workflow]
    - name: ready
      lifecycle: stable
      desired-state: ready
      # no actions[] — state-mover
    - name: checked-in
      lifecycle: stable
      desired-state: checked-in
`

func TestParseValuesYAML_DecodesCatalog(t *testing.T) {
	m, err := ParseValuesYAML(strings.NewReader(threeWorkflowYAML))
	if err != nil {
		t.Fatalf("ParseValuesYAML: %v", err)
	}
	if got, want := len(m), 3; got != want {
		t.Errorf("len(m) = %d, want %d", got, want)
	}
	for _, name := range []string{"gb200-rack-bringup-v4", "ready", "checked-in"} {
		if _, ok := m[name]; !ok {
			t.Errorf("missing %q in catalog", name)
		}
	}
}

func TestParseValuesYAML_DerivesActionCount(t *testing.T) {
	m, _ := ParseValuesYAML(strings.NewReader(threeWorkflowYAML))
	if got := m["gb200-rack-bringup-v4"].ActionCount; got != 3 {
		t.Errorf("gb200-rack-bringup-v4 ActionCount = %d, want 3", got)
	}
	if got := m["ready"].ActionCount; got != 0 {
		t.Errorf("ready ActionCount = %d, want 0 (state-mover)", got)
	}
}

func TestParseValuesYAML_DerivesRackKindFromPrefix(t *testing.T) {
	m, _ := ParseValuesYAML(strings.NewReader(threeWorkflowYAML))
	if got := m["gb200-rack-bringup-v4"].RackKind; got != "gb200" {
		t.Errorf("gb200 RackKind = %q, want gb200", got)
	}
	if got := m["ready"].RackKind; got != "" {
		t.Errorf("ready RackKind = %q, want empty (no known prefix)", got)
	}
}

func TestParseValuesYAML_PreservesActionComponentsShape(t *testing.T) {
	m, _ := ParseValuesYAML(strings.NewReader(threeWorkflowYAML))
	g := m["gb200-rack-bringup-v4"]
	if len(g.Actions) != 3 {
		t.Fatalf("Actions len = %d, want 3", len(g.Actions))
	}
	// action[2] is await-rack-provision-v4; computetray.flcc-end-state == "l10-test-loop"
	got := g.Actions[2].Arguments.Components[ComponentComputeTray].FLCCEndState
	if got != "l10-test-loop" {
		t.Errorf("computetray.FLCCEndState = %q, want l10-test-loop", got)
	}
	// action[1] handler is provision (typed)
	if g.Actions[1].Handler != HandlerProvision {
		t.Errorf("action[1].Handler = %q, want %q", g.Actions[1].Handler, HandlerProvision)
	}
}

func TestMapClient_ListWorkflows(t *testing.T) {
	c := NewMapClient()
	summaries, src, err := c.ListWorkflows(context.Background())
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(summaries) == 0 {
		t.Fatal("ListWorkflows returned empty")
	}
	if src.ChartTag == "" {
		t.Errorf("SourceRef.ChartTag empty")
	}
}

func TestMapClient_GetWorkflow_Known(t *testing.T) {
	c := NewMapClient()
	g, _, err := c.GetWorkflow(context.Background(), "gb200-rack-bringup-v4")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if g.Name != "gb200-rack-bringup-v4" {
		t.Errorf("Name = %q", g.Name)
	}
}

func TestMapClient_GetWorkflow_Unknown(t *testing.T) {
	c := NewMapClient()
	_, _, err := c.GetWorkflow(context.Background(), "no-such-workflow")
	if err == nil {
		t.Fatal("GetWorkflow(unknown): want error")
	}
	if !errors.Is(err, ErrWorkflowNotFound) {
		t.Errorf("err = %v, want ErrWorkflowNotFound", err)
	}
}
