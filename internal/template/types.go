// Package template owns the qac.template/v1 YAML schema:
// parsing, validation, and the in-memory types the engine consumes.
// No I/O here — file loading and DB persistence live in callers.
package template

// Template is the top-level parsed document.
type Template struct {
	Schema  string  `yaml:"schema"  json:"schema"`
	ID      string  `yaml:"id"      json:"id"`
	Version int     `yaml:"version" json:"version"`
	Title   string  `yaml:"title"   json:"title"`
	Target  Target  `yaml:"target"  json:"target"`
	Inputs  []Input `yaml:"inputs"  json:"inputs"`
	Steps   []Step  `yaml:"steps"   json:"steps"`
}

// Target scopes a template to a unit of fleet (rack / node / cluster)
// and an instance-type set.
type Target struct {
	Scope         string   `yaml:"scope"          json:"scope"`
	InstanceTypes []string `yaml:"instance_types" json:"instance_types"`
}

// Input declares one user-supplied form field. Type drives both the form
// renderer (iter-3b) and the runtime validator (ValidateInputs in iter-3a).
type Input struct {
	ID       string   `yaml:"id"                 json:"id"`
	Label    string   `yaml:"label"              json:"label"`
	Type     string   `yaml:"type"               json:"type"` // text|multi_text|enum|url|multi_url|textarea
	Required bool     `yaml:"required"           json:"required"`
	Options  []string `yaml:"options,omitempty"  json:"options,omitempty"`
	Validate string   `yaml:"validate,omitempty" json:"validate,omitempty"`
	Example  string   `yaml:"example,omitempty"  json:"example,omitempty"`
}

// Step is one ordered step in the template's rail. Kind-specific fields
// are optional pointers (probe) or slices/strings with omitempty (review,
// ai_assess, signoff).
type Step struct {
	ID    string `yaml:"id"    json:"id"`
	Kind  string `yaml:"kind"  json:"kind"` // probe|review|ai_assess|signoff
	Title string `yaml:"title" json:"title"`

	// probe-only
	Action     *Action     `yaml:"action,omitempty"     json:"action,omitempty"`
	Checkpoint *Checkpoint `yaml:"checkpoint,omitempty" json:"checkpoint,omitempty"`

	// probe-only — probe_type-shaped (iter-4b+) alternative to action.tool.
	// A kind:probe step requires action.tool or probe_type. A probe_type step may
	// also carry an action whose args (map form) become the probe's config (iter-5f).
	ProbeType string `yaml:"probe_type,omitempty" json:"probe_type,omitempty"`
	Racks     string `yaml:"racks,omitempty"      json:"racks,omitempty"`   // e.g., "{{ inputs.canary_racks }}"
	Timeout   string `yaml:"timeout,omitempty"    json:"timeout,omitempty"` // read but not enforced in iter-4b

	// review-only
	Sources        []string        `yaml:"sources,omitempty"         json:"sources,omitempty"`
	Enrichment     map[string]bool `yaml:"enrichment,omitempty"      json:"enrichment,omitempty"`
	OptionalInputs []Input         `yaml:"optional_inputs,omitempty" json:"optional_inputs,omitempty"`

	// ai_assess-only
	PromptRef           string   `yaml:"prompt_ref,omitempty"           json:"prompt_ref,omitempty"`
	ContextSteps        []string `yaml:"context_steps,omitempty"        json:"context_steps,omitempty"`
	RespectSuppressions bool     `yaml:"respect_suppressions,omitempty" json:"respect_suppressions,omitempty"`
	CitePatterns        bool     `yaml:"cite_patterns,omitempty"        json:"cite_patterns,omitempty"`
	CiteGlean           bool     `yaml:"cite_glean,omitempty"           json:"cite_glean,omitempty"`
	ExpectedVerdict     []string `yaml:"expected_verdict,omitempty"     json:"expected_verdict,omitempty"`

	// signoff-only
	FinalVerdictOptions []string `yaml:"final_verdict_options,omitempty" json:"final_verdict_options,omitempty"`
	Fields              []Input  `yaml:"fields,omitempty"                json:"fields,omitempty"`
	SlackReply          bool     `yaml:"slack_reply,omitempty"           json:"slack_reply,omitempty"`
}

// Action is the "initiate work" half of an RLCC-faithful probe step.
type Action struct {
	Tool        string `yaml:"tool"                  json:"tool"`
	MultiRack   bool   `yaml:"multi_rack,omitempty"  json:"multi_rack,omitempty"`
	Parallelism string `yaml:"parallelism,omitempty" json:"parallelism,omitempty"`
	Args        any    `yaml:"args,omitempty"        json:"args,omitempty"`
	Timeout     string `yaml:"timeout,omitempty"     json:"timeout,omitempty"`
}

// Checkpoint is the "gate to advance" half of an RLCC-faithful probe step.
type Checkpoint struct {
	Targets     string              `yaml:"targets"                 json:"targets"` // per_rack|rollup
	SuccessWhen map[string]any      `yaml:"success_when,omitempty"  json:"success_when,omitempty"`
	FailWhen    map[string]any      `yaml:"fail_when,omitempty"     json:"fail_when,omitempty"`
	Blockers    []CheckpointBlocker `yaml:"blockers,omitempty"      json:"blockers,omitempty"`
}

// CheckpointBlocker is a single Informational/ActionRequired predicate.
type CheckpointBlocker struct {
	Kind    string         `yaml:"kind"    json:"kind"` // Informational|ActionRequired
	When    map[string]any `yaml:"when"    json:"when"`
	Message string         `yaml:"message" json:"message"`
}
