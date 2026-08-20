package template

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	templateIDPattern = regexp.MustCompile(`^[a-z0-9-]+$`)
	stepIDPattern     = regexp.MustCompile(`^[a-z0-9_]+$`)

	validInputTypes = map[string]bool{
		"text": true, "multi_text": true, "enum": true,
		"url": true, "multi_url": true, "textarea": true,
	}
	validStepKinds = map[string]bool{
		"probe": true, "review": true, "ai_assess": true, "signoff": true,
	}
	validScopes             = map[string]bool{"rack": true, "node": true, "cluster": true}
	validCheckpointTargets  = map[string]bool{"per_rack": true, "rollup": true}
)

// Validate enforces semantic rules on a parsed Template. Returns nil if
// the template is well-formed; otherwise a single error describing the
// first violation found. Errors mention the template id when known so
// callers can wrap them with file context.
func Validate(t Template) error {
	if t.Schema != "qac.template/v1" {
		return fmt.Errorf("schema must be qac.template/v1, got %q", t.Schema)
	}
	if t.ID == "" {
		return fmt.Errorf("id is required")
	}
	if !templateIDPattern.MatchString(t.ID) {
		return fmt.Errorf("id %q does not match %s", t.ID, templateIDPattern.String())
	}
	if t.Version < 1 {
		return fmt.Errorf("version must be >= 1, got %d", t.Version)
	}
	if t.Title == "" {
		return fmt.Errorf("title is required")
	}
	if !validScopes[t.Target.Scope] {
		return fmt.Errorf("target.scope %q is not one of rack|node|cluster", t.Target.Scope)
	}

	// Inputs
	seenInputs := map[string]bool{}
	for i, in := range t.Inputs {
		if in.ID == "" {
			return fmt.Errorf("input %d: id is required", i)
		}
		if seenInputs[in.ID] {
			return fmt.Errorf("duplicate input id %q", in.ID)
		}
		seenInputs[in.ID] = true
		if !validInputTypes[in.Type] {
			return fmt.Errorf("input %q: type %q is not valid", in.ID, in.Type)
		}
		if in.Type == "enum" && len(in.Options) == 0 {
			return fmt.Errorf("input %q: enum requires options", in.ID)
		}
		if in.Validate != "" {
			if _, err := regexp.Compile(in.Validate); err != nil {
				return fmt.Errorf("input %q: validate regex %q failed to compile: %v", in.ID, in.Validate, err)
			}
		}
	}

	// Steps
	if len(t.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}
	seenSteps := map[string]bool{}
	probeIDs := map[string]bool{}
	for i, st := range t.Steps {
		if st.ID == "" {
			return fmt.Errorf("step %d: id is required", i)
		}
		if !stepIDPattern.MatchString(st.ID) {
			return fmt.Errorf("step %q: id does not match %s", st.ID, stepIDPattern.String())
		}
		if seenSteps[st.ID] {
			return fmt.Errorf("duplicate step id %q", st.ID)
		}
		seenSteps[st.ID] = true
		if !validStepKinds[st.Kind] {
			return fmt.Errorf("step %q: kind %q is not valid", st.ID, st.Kind)
		}

		switch st.Kind {
		case "probe":
			hasAction := st.Action != nil
			hasProbeType := strings.TrimSpace(st.ProbeType) != ""
			if !hasAction && !hasProbeType {
				return fmt.Errorf("step %q: probe requires action.tool or probe_type", st.ID)
			}
			// iter-5f: a probe_type step may carry an optional action as a
			// config carrier — the runner maps action.args (map form) into
			// StepContext.Config. No tool or checkpoint is required there;
			// gatherers produce evidence, not verdicts. The classic
			// action-only probe (no probe_type) still requires tool +
			// checkpoint with a valid target.
			if hasAction && !hasProbeType {
				if st.Action.Tool == "" {
					return fmt.Errorf("step %q: probe action requires tool", st.ID)
				}
				if st.Checkpoint == nil {
					return fmt.Errorf("step %q: probe with action requires checkpoint", st.ID)
				}
				if !validCheckpointTargets[st.Checkpoint.Targets] {
					return fmt.Errorf("step %q: checkpoint targets %q is not per_rack|rollup", st.ID, st.Checkpoint.Targets)
				}
			}
			probeIDs[st.ID] = true
		case "review":
			if len(st.Sources) == 0 {
				return fmt.Errorf("step %q: review requires sources", st.ID)
			}
			for _, src := range st.Sources {
				if !probeIDs[src] {
					return fmt.Errorf("step %q: sources reference %q which is not a probe step earlier in the template", st.ID, src)
				}
			}
		case "ai_assess":
			if st.PromptRef == "" {
				return fmt.Errorf("step %q: ai_assess requires prompt_ref", st.ID)
			}
			for _, src := range st.ContextSteps {
				if !probeIDs[src] {
					return fmt.Errorf("step %q: context_steps reference %q which is not a probe step earlier in the template", st.ID, src)
				}
			}
		case "signoff":
			if len(st.FinalVerdictOptions) == 0 {
				return fmt.Errorf("step %q: signoff requires final_verdict_options", st.ID)
			}
			seenFields := map[string]bool{}
			for _, f := range st.Fields {
				if seenFields[f.ID] {
					return fmt.Errorf("step %q: duplicate field id %q", st.ID, f.ID)
				}
				seenFields[f.ID] = true
			}
		}
	}

	return nil
}
