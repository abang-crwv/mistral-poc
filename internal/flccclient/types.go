// Package flccclient exposes FLCC workflow definitions as typed Go data.
// Production reads come from Sourcegraph via internal/sourcegraph.Client;
// tests and offline dev use a code-generated Fixture. Mirrors
// internal/rlccclient/.
package flccclient

import "qac/internal/sourcegraph"

// PhaseClass buckets FLCC workflows by which controller drives them.
type PhaseClass string

const (
	PhaseClassRackProvision PhaseClass = "rack-provision"
	PhaseClassNodeProvision PhaseClass = "node-provision"
)

// FLCCWorkflowSummary is the picker-level view of one FLCC workflow.
// PhaseCount and TerminalPhase are derived in parse.go.
type FLCCWorkflowSummary struct {
	Name          string     `yaml:"name"        json:"name"`
	Class         PhaseClass `yaml:"class"       json:"class"`
	Description   string     `yaml:"description" json:"description"`
	PhaseCount    int        `yaml:"-"           json:"phase_count"`
	TerminalPhase string     `yaml:"-"           json:"terminal_phase"`
}

// FLCCWorkflowGraph is the full ordered view including phases[].
type FLCCWorkflowGraph struct {
	FLCCWorkflowSummary `yaml:",inline"`
	Phases              []FLCCPhase `yaml:"phases" json:"phases"`
}

// FLCCPhase pins a node state, with watchdog timeout, failState transition,
// optional poll interval, and a free-form parameters map.
type FLCCPhase struct {
	Name        string            `yaml:"name"                  json:"name"`
	State       string            `yaml:"state"                 json:"state"`
	Timeout     string            `yaml:"timeout,omitempty"     json:"timeout,omitempty"`
	FailState   string            `yaml:"failState,omitempty"   json:"fail_state,omitempty"`
	RequeueTime string            `yaml:"requeueTime,omitempty" json:"requeue_time,omitempty"`
	Parameters  map[string]string `yaml:"parameters,omitempty"  json:"parameters,omitempty"`
}

// SourceRef re-exports sourcegraph.SourceRef so callers don't have to
// import sourcegraph for the (chart_tag, sha) pair.
type SourceRef = sourcegraph.SourceRef
