package template

import (
	"strings"
	"testing"
)

// validTemplate returns a known-good Template for table-driven mutation.
func validTemplate() Template {
	action := &Action{Tool: "metrics", Timeout: "5m"}
	checkpoint := &Checkpoint{Targets: "per_rack", SuccessWhen: map[string]any{"stdout_contains": "ok"}}
	return Template{
		Schema:  "qac.template/v1",
		ID:      "smoke",
		Version: 1,
		Title:   "Smoke test",
		Target:  Target{Scope: "rack", InstanceTypes: []string{"gb200-4x"}},
		Inputs: []Input{
			{ID: "bundle_tag", Label: "FW bundle tag", Type: "text", Required: true},
		},
		Steps: []Step{
			{ID: "noop", Kind: "probe", Title: "no-op", Action: action, Checkpoint: checkpoint},
		},
	}
}

func TestValidate_HappyPath(t *testing.T) {
	if err := Validate(validTemplate()); err != nil {
		t.Fatalf("Validate on happy path: %v", err)
	}
}

func TestValidate_Mutations(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*Template)
		wantMatch string // substring expected in the error message
	}{
		{
			name:      "missing schema",
			mutate:    func(t *Template) { t.Schema = "" },
			wantMatch: "schema",
		},
		{
			name:      "wrong schema",
			mutate:    func(t *Template) { t.Schema = "other/v1" },
			wantMatch: "schema",
		},
		{
			name:      "missing id",
			mutate:    func(t *Template) { t.ID = "" },
			wantMatch: "id",
		},
		{
			name:      "bad id chars",
			mutate:    func(t *Template) { t.ID = "Smoke!" },
			wantMatch: "id",
		},
		{
			name:      "version below 1",
			mutate:    func(t *Template) { t.Version = 0 },
			wantMatch: "version",
		},
		{
			name:      "empty title",
			mutate:    func(t *Template) { t.Title = "" },
			wantMatch: "title",
		},
		{
			name:      "bad target scope",
			mutate:    func(t *Template) { t.Target.Scope = "datacenter" },
			wantMatch: "scope",
		},
		{
			name: "duplicate input ids",
			mutate: func(t *Template) {
				t.Inputs = append(t.Inputs, Input{ID: "bundle_tag", Label: "dup", Type: "text"})
			},
			wantMatch: "duplicate input",
		},
		{
			name:      "empty input id",
			mutate:    func(t *Template) { t.Inputs[0].ID = "" },
			wantMatch: "input",
		},
		{
			name:      "bad input type",
			mutate:    func(t *Template) { t.Inputs[0].Type = "spaceship" },
			wantMatch: "type",
		},
		{
			name: "enum without options",
			mutate: func(t *Template) {
				t.Inputs[0].Type = "enum"
				t.Inputs[0].Options = nil
			},
			wantMatch: "options",
		},
		{
			name:      "bad validate regex",
			mutate:    func(t *Template) { t.Inputs[0].Validate = "[" },
			wantMatch: "validate",
		},
		{
			name:      "zero steps",
			mutate:    func(t *Template) { t.Steps = nil },
			wantMatch: "step",
		},
		{
			name: "duplicate step ids",
			mutate: func(t *Template) {
				s := t.Steps[0]
				t.Steps = append(t.Steps, s)
			},
			wantMatch: "duplicate step",
		},
		{
			name:      "bad step kind",
			mutate:    func(t *Template) { t.Steps[0].Kind = "wave-hands" },
			wantMatch: "kind",
		},
		{
			name:      "bad step id chars",
			mutate:    func(t *Template) { t.Steps[0].ID = "Noop!" },
			wantMatch: "id",
		},
		{
			name:      "probe missing action",
			mutate:    func(t *Template) { t.Steps[0].Action = nil },
			wantMatch: "action",
		},
		{
			name:      "probe missing checkpoint",
			mutate:    func(t *Template) { t.Steps[0].Checkpoint = nil },
			wantMatch: "checkpoint",
		},
		{
			name:      "probe action missing tool",
			mutate:    func(t *Template) { t.Steps[0].Action.Tool = "" },
			wantMatch: "tool",
		},
		{
			name:      "probe checkpoint bad targets",
			mutate:    func(t *Template) { t.Steps[0].Checkpoint.Targets = "cluster" },
			wantMatch: "targets",
		},
		{
			name: "review step empty sources",
			mutate: func(t *Template) {
				t.Steps = []Step{
					{ID: "probe1", Kind: "probe", Title: "p", Action: &Action{Tool: "metrics"}, Checkpoint: &Checkpoint{Targets: "per_rack"}},
					{ID: "review1", Kind: "review", Title: "r", Sources: nil},
				}
			},
			wantMatch: "sources",
		},
		{
			name: "review references unknown step",
			mutate: func(t *Template) {
				t.Steps = []Step{
					{ID: "probe1", Kind: "probe", Title: "p", Action: &Action{Tool: "metrics"}, Checkpoint: &Checkpoint{Targets: "per_rack"}},
					{ID: "review1", Kind: "review", Title: "r", Sources: []string{"nonexistent"}},
				}
			},
			wantMatch: "sources",
		},
		{
			name: "review references later step",
			mutate: func(t *Template) {
				t.Steps = []Step{
					{ID: "review1", Kind: "review", Title: "r", Sources: []string{"probe1"}},
					{ID: "probe1", Kind: "probe", Title: "p", Action: &Action{Tool: "metrics"}, Checkpoint: &Checkpoint{Targets: "per_rack"}},
				}
			},
			wantMatch: "sources",
		},
		{
			name: "ai_assess missing prompt_ref",
			mutate: func(t *Template) {
				t.Steps = append(t.Steps, Step{ID: "ai1", Kind: "ai_assess", Title: "ai"})
			},
			wantMatch: "prompt_ref",
		},
		{
			name: "signoff empty final_verdict_options",
			mutate: func(t *Template) {
				t.Steps = append(t.Steps, Step{ID: "sign1", Kind: "signoff", Title: "sign"})
			},
			wantMatch: "final_verdict_options",
		},
		{
			name: "signoff duplicate field ids",
			mutate: func(t *Template) {
				t.Steps = append(t.Steps, Step{
					ID: "sign1", Kind: "signoff", Title: "sign",
					FinalVerdictOptions: []string{"pass"},
					Fields: []Input{
						{ID: "x", Label: "x", Type: "text"},
						{ID: "x", Label: "y", Type: "text"},
					},
				})
			},
			wantMatch: "duplicate",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tpl := validTemplate()
			tc.mutate(&tpl)
			err := Validate(tpl)
			if err == nil {
				t.Fatalf("Validate returned nil; want error matching %q", tc.wantMatch)
			}
			if !strings.Contains(err.Error(), tc.wantMatch) {
				t.Fatalf("Validate err = %q, want substring %q", err.Error(), tc.wantMatch)
			}
		})
	}
}

func TestValidate_ProbeStep_AcceptsProbeType(t *testing.T) {
	tpl := Template{
		Schema:  "qac.template/v1",
		ID:      "firmware-release-canary",
		Version: 1,
		Title:   "x",
		Target:  Target{Scope: "rack"},
		Steps: []Step{
			{ID: "alert_baseline", Kind: "probe", Title: "x", ProbeType: "alert_probe", Racks: "{{ inputs.canary_racks }}"},
		},
	}
	if err := Validate(tpl); err != nil {
		t.Fatalf("Validate: %v (want nil for probe_type-shaped probe step)", err)
	}
}

func TestValidate_ProbeStep_AcceptsLegacyAction(t *testing.T) {
	tpl := Template{
		Schema:  "qac.template/v1",
		ID:      "firmware-release-canary",
		Version: 1,
		Title:   "x",
		Target:  Target{Scope: "rack"},
		Steps: []Step{
			{ID: "baseline", Kind: "probe", Title: "x",
				Action:     &Action{Tool: "metrics"},
				Checkpoint: &Checkpoint{Targets: "per_rack"}},
		},
	}
	if err := Validate(tpl); err != nil {
		t.Fatalf("Validate: %v (want nil for legacy action-shaped probe step)", err)
	}
}

func TestValidate_ProbeStep_AcceptsProbeTypeWithActionConfig(t *testing.T) {
	// iter-5f: a probe_type step may carry an action as a config carrier
	// (runner maps action.args map → StepContext.Config). No checkpoint or
	// tool requirement applies in that case.
	tpl := Template{
		Schema:  "qac.template/v1",
		ID:      "firmware-release-canary",
		Version: 1,
		Title:   "x",
		Target:  Target{Scope: "rack"},
		Steps: []Step{
			{ID: "l11_fielddiag", Kind: "probe", Title: "x",
				ProbeType: "rlcc_action_probe",
				Racks:     "{{ inputs.canary_racks }}",
				// Tool mirrors the real YAML shape; the validator does not enforce it when ProbeType is set.
				Action: &Action{Tool: "rlcc", Args: map[string]any{
					"match_handler": "l11-fielddiag",
					"target_state":  "l11-fielddiag",
					"poll_interval": "30s",
				}}},
		},
	}
	if err := Validate(tpl); err != nil {
		t.Fatalf("Validate: %v (want nil for probe_type + action-config step)", err)
	}
}

func TestValidate_ProbeStep_RejectsEmpty(t *testing.T) {
	tpl := Template{
		Schema:  "qac.template/v1",
		ID:      "firmware-release-canary",
		Version: 1,
		Title:   "x",
		Target:  Target{Scope: "rack"},
		Steps: []Step{
			{ID: "ghost", Kind: "probe", Title: "x"},
		},
	}
	err := Validate(tpl)
	if err == nil {
		t.Fatal("Validate: nil, want error for empty probe step (no action and no probe_type)")
	}
}

func TestValidate_ProbeStep_RejectsWhitespaceProbeType(t *testing.T) {
	tpl := Template{
		Schema:  "qac.template/v1",
		ID:      "firmware-release-canary",
		Version: 1,
		Title:   "x",
		Target:  Target{Scope: "rack"},
		Steps: []Step{
			{ID: "ghost", Kind: "probe", Title: "x", ProbeType: "   "},
		},
	}
	err := Validate(tpl)
	if err == nil {
		t.Fatal("Validate: nil, want error for whitespace-only probe_type")
	}
	if !strings.Contains(err.Error(), "action.tool or probe_type") {
		t.Errorf("err = %q, want mention of 'action.tool or probe_type'", err.Error())
	}
}

func TestValidate_ReviewStep_CanReferenceProbeTypeShapedProbe(t *testing.T) {
	tpl := Template{
		Schema:  "qac.template/v1",
		ID:      "firmware-release-canary",
		Version: 1,
		Title:   "x",
		Target:  Target{Scope: "rack"},
		Steps: []Step{
			{ID: "alert_baseline", Kind: "probe", Title: "x", ProbeType: "alert_probe"},
			{ID: "review_step", Kind: "review", Title: "Review", Sources: []string{"alert_baseline"}},
		},
	}
	if err := Validate(tpl); err != nil {
		t.Fatalf("Validate: %v (want nil — review step should accept probe_type-shaped probe as source)", err)
	}
}
