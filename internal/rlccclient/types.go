// Package rlccclient exposes RLCC workflow definitions as typed Go data.
// Production reads come from Sourcegraph via internal/sourcegraph.Client;
// tests and offline dev use a code-generated Fixture.
package rlccclient

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"qac/internal/sourcegraph"
)

// HandlerKind is the action's handler type. The const block enumerates every
// kind observed in rack-lifecycle-controller v3.87.0; unknown values still
// parse (HandlerKind is a string under the hood) and surface in API
// responses verbatim.
type HandlerKind string

const (
	HandlerPowerOperation  HandlerKind = "power-operation"
	HandlerAwaitCheckpoint HandlerKind = "await-checkpoint"
	HandlerHLCCState       HandlerKind = "hlcc-state"
	HandlerProvision       HandlerKind = "provision"
	HandlerL11FieldDiag    HandlerKind = "l11-fielddiag"
	HandlerHPCVerification HandlerKind = "hpc-verification"
	HandlerFLCCWorkflow    HandlerKind = "flcc-workflow"
	HandlerEnableCKS       HandlerKind = "enable-cks"
)

// ComponentKind buckets the device classes RLCC actions fan out over.
type ComponentKind string

const (
	ComponentCDU         ComponentKind = "coolantdistributionunit"
	ComponentComputeTray ComponentKind = "computetray"
	ComponentNVLink      ComponentKind = "nvlink"
	ComponentPowerShelf  ComponentKind = "powershelf"
	ComponentRack        ComponentKind = "rack"
)

// WorkflowSummary is the picker-level view of one workflow. RackKind,
// ActionCount, and Handlers are derived in parse.go (not present in YAML).
//
// Handlers is the sorted, de-duplicated set of action handler kinds the
// workflow contains (e.g. "await-checkpoint", "l11-fielddiag"). The
// canary new-run picker uses it to show only workflows that can actually
// drive the firmware canary — i.e. those whose Handlers include
// "l11-fielddiag". Reporting the capability set here (rather than a
// canary-specific boolean) keeps this RLCC type free of canary semantics.
type WorkflowSummary struct {
	Name          string   `yaml:"name"           json:"name"`
	Description   string   `yaml:"description"    json:"description"`
	Lifecycle     string   `yaml:"lifecycle"      json:"lifecycle"`
	CurrentStates []string `yaml:"current-states" json:"current_states"`
	DesiredState  string   `yaml:"desired-state"  json:"desired_state"`
	RackKind      string   `yaml:"-"              json:"rack_kind"`
	ActionCount   int      `yaml:"-"              json:"action_count"`
	Handlers      []string `yaml:"-"              json:"handlers"`
}

// WorkflowGraph is the full view including the linear actions list. The
// embedded WorkflowSummary uses `yaml:",inline"` so its fields appear at the
// top level of each workflow entry in YAML.
type WorkflowGraph struct {
	WorkflowSummary `yaml:",inline"`
	Actions         []Action `yaml:"actions" json:"actions"`
}

// Action is one entry in actions[]. Handler determines which Arguments fields
// carry meaning (await-checkpoint reads Components[].Checks, power-operation
// reads Power, etc.). We model Arguments as one wide struct rather than a
// sum type — Go's type system is happier with that, and validation lives in
// code anyway.
type Action struct {
	Name          string      `yaml:"name"                    json:"name"`
	Handler       HandlerKind `yaml:"handler"                 json:"handler"`
	Description   string      `yaml:"description"             json:"description"`
	Force         bool        `yaml:"force,omitempty"         json:"force,omitempty"`
	SkipOnFailure bool        `yaml:"skipOnFailure,omitempty" json:"skip_on_failure,omitempty"`
	Arguments     ActionArgs  `yaml:"arguments"               json:"arguments"`
}

// ActionArgs carries every observed argument key.
type ActionArgs struct {
	Checkpoint         string     `yaml:"checkpoint,omitempty"            json:"checkpoint,omitempty"`
	CheckpointArgsStep string     `yaml:"checkpoint-args-step,omitempty"  json:"checkpoint_args_step,omitempty"`
	Power              string     `yaml:"power,omitempty"                 json:"power,omitempty"`
	Components         Components `yaml:"components,omitempty"            json:"components,omitempty"`
}

// Components is the per-device-class argument block. The live RLCC chart
// writes it two ways: a mapping (kind → spec) for provision/checkpoint
// actions, and a bare sequence of kind names (e.g. `components: [computetray]`)
// for power-operation actions. We accept both — the sequence form decodes to
// entries with an empty ComponentSpec (presence only).
type Components map[ComponentKind]ComponentSpec

// UnmarshalYAML accepts either the mapping or the bare-sequence form.
func (c *Components) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.MappingNode:
		m := map[ComponentKind]ComponentSpec{}
		if err := value.Decode(&m); err != nil {
			return err
		}
		*c = m
	case yaml.SequenceNode:
		var names []ComponentKind
		if err := value.Decode(&names); err != nil {
			return err
		}
		m := make(map[ComponentKind]ComponentSpec, len(names))
		for _, n := range names {
			m[n] = ComponentSpec{}
		}
		*c = m
	case 0: // empty/absent
		*c = nil
	default:
		if value.Tag == "!!null" {
			*c = nil
			return nil
		}
		return fmt.Errorf("components: unexpected YAML node kind %d", value.Kind)
	}
	return nil
}

// ComponentSpec is the per-component arg block.
type ComponentSpec struct {
	State        string   `yaml:"state,omitempty"           json:"state,omitempty"`
	Workflow     string   `yaml:"workflow,omitempty"        json:"workflow,omitempty"`
	FLCCEndState string   `yaml:"flcc-end-state,omitempty"  json:"flcc_end_state,omitempty"`
	Checks       []string `yaml:"checks,omitempty"          json:"checks,omitempty"`
}

// SourceRef is a re-export of sourcegraph.SourceRef so callers of this
// package don't have to import sourcegraph for the (chart_tag, sha) pair
// that appears in API responses.
type SourceRef = sourcegraph.SourceRef

// WorkflowSnapshot is the run-create-time pin of an RLCC workflow: enough
// to walk the workflow without consulting Sourcegraph again. JSON-serialized
// into RunCreated.payload.rlcc_workflow at run-create time, then read back
// out of the event log by iter-5e's rlcc_action_probe when it walks the
// per-CT actions.
//
// Name + SourceSHA together uniquely identify the workflow revision.
// SourceSHA is captured at GetWorkflow time and pinned for the life of
// the run — if the chart changes upstream, the run keeps walking what
// was snapshotted here.
//
// Actions is the linear sequence iter-5e walks; the slice header is
// 24 bytes (pointer, len, cap) and the backing array is shared with
// the source WorkflowGraph until either side mutates (we never mutate,
// so no copy cost).
type WorkflowSnapshot struct {
	Name      string   `json:"name"`
	SourceSHA string   `json:"source_sha"`
	Actions   []Action `json:"actions"`
}
