package flccclient

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const fourFLCCWorkflowYAML = `
flcc:
  config:
    workflows:
      - name: gb200-rack-provision-v4
        class: rack-provision
        description: Node+DPU vaultify, seatrial then zap both dpu and host.
        phases:
          - name: node-vaultify
            state: node-vaultify
            failState: fail
            timeout: 1h
          - name: l10-test-loop
            state: l10-test-loop
      - name: gb200-rack-power-cycle
        class: rack-provision
        phases:
          - name: power-cycle
            state: power-cycle
            failState: fail
            timeout: 1h
            parameters: {instant-start: "true"}
          - name: hold
            state: hold
      - name: gb200-rack-hpc-verification-v4
        class: rack-provision
        phases:
          - name: l11-test
            state: l11-test
            failState: fail
            timeout: 10h
          - name: l12-test-loop
            state: l12-test-loop
      - name: ready
        class: node-provision
        phases:
          - name: ready
            state: ready
`

func TestParseValuesYAML_DecodesAndDerives(t *testing.T) {
	m, err := ParseValuesYAML(strings.NewReader(fourFLCCWorkflowYAML))
	if err != nil {
		t.Fatalf("ParseValuesYAML: %v", err)
	}
	if got, want := len(m), 4; got != want {
		t.Errorf("len(m) = %d, want %d", got, want)
	}
}

func TestParseValuesYAML_DerivesPhaseCount(t *testing.T) {
	m, _ := ParseValuesYAML(strings.NewReader(fourFLCCWorkflowYAML))
	if got := m["gb200-rack-provision-v4"].PhaseCount; got != 2 {
		t.Errorf("PhaseCount = %d, want 2", got)
	}
}

func TestParseValuesYAML_DerivesTerminalPhase(t *testing.T) {
	m, _ := ParseValuesYAML(strings.NewReader(fourFLCCWorkflowYAML))
	cases := map[string]string{
		"gb200-rack-provision-v4":        "l10-test-loop",
		"gb200-rack-power-cycle":         "hold",
		"gb200-rack-hpc-verification-v4": "l12-test-loop",
		"ready":                          "ready",
	}
	for name, want := range cases {
		if got := m[name].TerminalPhase; got != want {
			t.Errorf("%s TerminalPhase = %q, want %q", name, got, want)
		}
	}
}

func TestParseValuesYAML_PhaseShape(t *testing.T) {
	m, _ := ParseValuesYAML(strings.NewReader(fourFLCCWorkflowYAML))
	g := m["gb200-rack-power-cycle"]
	// Phase 0 has parameters.instant-start = "true"
	if got := g.Phases[0].Parameters["instant-start"]; got != "true" {
		t.Errorf("instant-start = %q, want true", got)
	}
	// Phase 1 (hold) has empty failState
	if got := g.Phases[1].FailState; got != "" {
		t.Errorf("hold FailState = %q, want empty", got)
	}
}

func TestMapClient_GetWorkflow(t *testing.T) {
	c := NewMapClient()
	g, _, err := c.GetWorkflow(context.Background(), "gb200-rack-provision-v4")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if g.TerminalPhase != "l10-test-loop" {
		t.Errorf("TerminalPhase = %q, want l10-test-loop", g.TerminalPhase)
	}
}

func TestMapClient_TerminalPhase(t *testing.T) {
	c := NewMapClient()
	cases := map[string]string{
		"gb200-rack-provision-v4":        "l10-test-loop",
		"gb200-rack-power-cycle":         "hold",
		"gb200-rack-hpc-verification-v4": "l12-test-loop",
		"ready":                          "ready",
	}
	for name, want := range cases {
		got, err := c.TerminalPhase(context.Background(), name)
		if err != nil {
			t.Errorf("TerminalPhase(%q): %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("TerminalPhase(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestMapClient_TerminalPhase_Unknown(t *testing.T) {
	c := NewMapClient()
	_, err := c.TerminalPhase(context.Background(), "no-such-flcc-workflow")
	if !errors.Is(err, ErrWorkflowNotFound) {
		t.Errorf("err = %v, want ErrWorkflowNotFound", err)
	}
}
