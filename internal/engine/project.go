package engine

import (
	"encoding/json"
	"time"

	"qac/internal/rlccclient"
	"qac/internal/verdict"
)

// StepState carries per-step verdict + signal data. Empty in the walking
// skeleton; populated once probes are wired in.
type StepState struct {
	StepID  string
	Verdict string
	// Signals []Signal — added when probes ship
}

// RunState is the projected view of a run derived from its event log.
type RunState struct {
	ID         string
	TemplateID string
	Rack       string
	CreatedBy  string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	StepStates map[string]StepState
	// iter-5e: workflow snapshot projected from RunCreated.payload.rlcc_workflow.
	// Nil for pre-iter-5d runs. iter-5e probes read this via
	// StepContext.RLCCWorkflow (populated by the runner, not by Project).
	RLCCWorkflow *rlccclient.WorkflowSnapshot
}

// Project folds a sequence of events into a RunState. Pure function:
// no I/O, no clock, no shared state. Caller passes events in seq order.
func Project(events []Event) RunState {
	st := RunState{
		StepStates: map[string]StepState{},
	}
	// worstVerdict tracks the worst step verdict seen so far.
	// passed < warning < failed; empty means no completed steps yet.
	worstVerdict := ""

	for _, e := range events {
		st.UpdatedAt = e.OccurredAt
		switch e.Kind {

		case KindRunCreated:
			var p RunCreatedPayload
			_ = json.Unmarshal(e.Payload, &p)
			st.TemplateID = p.TemplateID
			st.Rack = p.Rack
			st.CreatedBy = p.CreatedBy
			st.CreatedAt = e.OccurredAt
			st.Status = StatusPending
			st.RLCCWorkflow = p.RLCCWorkflow // iter-5e

		case KindStepStarted:
			var p StepStartedPayload
			_ = json.Unmarshal(e.Payload, &p)
			// Don't downgrade from a terminal status.
			if !verdict.IsTerminal(st.Status) {
				st.Status = StatusRunning
			}

		case KindStepCompleted:
			var p StepCompletedPayload
			_ = json.Unmarshal(e.Payload, &p)
			st.StepStates[p.StepID] = StepState{StepID: p.StepID, Verdict: p.Verdict}
			worstVerdict = worse(worstVerdict, p.Verdict)
			if !verdict.IsTerminal(st.Status) {
				st.Status = verdictToStatus(worstVerdict)
			}

		case KindStepFailed:
			var p StepFailedPayload
			_ = json.Unmarshal(e.Payload, &p)
			worstVerdict = worse(worstVerdict, VerdictFailed)
			if !verdict.IsTerminal(st.Status) {
				st.Status = StatusFailed
			}

		case KindOperatorAction:
			var p OperatorActionPayload
			_ = json.Unmarshal(e.Payload, &p)
			if p.Action == "signoff" {
				st.Status = StatusSignedOff
			}

		case KindRunCancelled:
			// Operator cancelled the run — terminal, overrides any prior
			// status (including a just-rolled-up verdict). Matches the
			// store's RunCancelled projection.
			st.Status = StatusCancelled

		case KindRunCompleted:
			// Walk finished with no operator gate. Finalize from the
			// accumulated worst verdict: empty (all gatherers, no failure)
			// => passed; otherwise the verdict's status. Mirrors the
			// store's RunCompleted projection. signed_off stays terminal.
			if !verdict.IsTerminal(st.Status) {
				if worstVerdict == "" {
					st.Status = StatusPassed
				} else {
					st.Status = verdictToStatus(worstVerdict)
				}
			}

		case KindFactsDiscoveryFailed:
			// iter-4a: inventory hard-fail at run scope → run is failed.
			// Mirrors store's FactsDiscoveryFailed UPDATE runs SET status='failed'.
			worstVerdict = worse(worstVerdict, VerdictFailed)
			if !verdict.IsTerminal(st.Status) {
				st.Status = StatusFailed
			}

		case KindBMNDiscoveryFailed:
			// iter-5b: BMN enumeration failed for a rack → run is failed.
			// Mirrors store's BMNDiscoveryFailed UPDATE runs SET status='failed'.
			worstVerdict = worse(worstVerdict, VerdictFailed)
			if !verdict.IsTerminal(st.Status) {
				st.Status = StatusFailed
			}

		case KindBMNDiscovered, KindBMNIgnored:
			// iter-5b: these events project into rack_deviceslots and
			// bmn_ignores at the store layer (internal/store/events.go).
			// They do not affect RunState.Status here; the case branch
			// exists for explicitness so a future reader doesn't wonder
			// if they were forgotten.
			continue

		default:
			// Unknown kinds are ignored. Reviewers: when adding a new kind,
			// add a case here and update the tests in project_test.go.
		}
	}
	return st
}

// worse returns the worse of two verdicts. Empty string is best (no verdict yet).
func worse(a, b string) string { return verdict.Worse(a, b) }

func verdictToStatus(v string) string { return verdict.ToStatus(v) }
