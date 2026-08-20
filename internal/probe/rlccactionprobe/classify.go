package rlccactionprobe

import "qac/internal/lifecycleclient"

// ctOutcome is the per-CT classification of an FLCC observation.
type ctOutcome string

const (
	outcomeSuccess    ctOutcome = "success"
	outcomeFailed     ctOutcome = "failed"
	outcomeIgnorable  ctOutcome = "ignorable"
	outcomeInProgress ctOutcome = "in_progress"
)

// flccIgnorableStates are FLCC states that take a node out of the normal
// production lifecycle (pulled / RMA / under debug). A CT in one of these
// is dropped from the pool and excluded from any verdict. Source: wpena
// 2026-06-02 (see memory flcc-ignorable-states).
var flccIgnorableStates = map[string]bool{
	"rma":    true,
	"broken": true,
	"debug":  true,
	"dev":    true,
}

// flccIgnorableWorkflow is the FLCC workflow whose nodes are ignored.
const flccIgnorableWorkflow = "broken-collect"

// classifyFLCC buckets one FLCC observation against the diagnostic's
// in-progress state (the step's target_state, e.g. "l11-fielddiag").
// fail takes precedence (captures a failure before an operator moves the
// node to broken). pass = the node moved PAST the diagnostic state without
// failing (prev_state was the diagnostic). A node still at the diagnostic
// state, or one that hasn't reached it yet, is in-progress.
func classifyFLCC(obs lifecycleclient.FLCCObservation, diagnosticState string) ctOutcome {
	switch {
	case obs.State == "fail":
		return outcomeFailed
	case flccIgnorableStates[obs.State] || obs.Workflow == flccIgnorableWorkflow:
		return outcomeIgnorable
	case obs.State == diagnosticState:
		return outcomeInProgress
	case obs.PrevState == diagnosticState:
		return outcomeSuccess
	default:
		return outcomeInProgress
	}
}
