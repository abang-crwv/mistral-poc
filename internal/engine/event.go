// Package engine projects an event log into a RunState. The Project
// function is pure: no I/O, no DB, no clock.
package engine

import (
	"encoding/json"
	"time"

	"qac/internal/rlccclient"
	"qac/internal/verdict"
)

// Event kind constants — string values must match what's persisted by
// internal/store and what the frontend sees on the wire.
const (
	KindRunCreated           = "RunCreated"
	KindStepStarted          = "StepStarted"
	KindStepCompleted        = "StepCompleted"
	KindStepFailed           = "StepFailed"
	KindOperatorAction       = "OperatorAction"
	KindFactsDiscoveryFailed = "FactsDiscoveryFailed" // iter-4a: hard inventory fail
	KindBMNDiscovered        = "BMNDiscovered"        // iter-5b
	KindBMNDiscoveryFailed   = "BMNDiscoveryFailed"   // iter-5b
	KindBMNIgnored           = "BMNIgnored"           // iter-5b
	KindRunCompleted         = "RunCompleted"         // walk reached the end with no operator gate
	KindRunCancelled         = "RunCancelled"         // operator cancelled the run
)

// Run status values — must match what internal/store writes to runs.status.
const (
	StatusPending   = verdict.StatusPending
	StatusRunning   = verdict.StatusRunning
	StatusPassed    = verdict.StatusPassed
	StatusWarning   = verdict.StatusWarning
	StatusFailed    = verdict.StatusFailed
	StatusSignedOff = verdict.StatusSignedOff
	StatusCancelled = verdict.StatusCancelled
)

// Verdict values for StepCompleted.
const (
	VerdictPassed  = verdict.Passed
	VerdictWarning = verdict.Warning
	VerdictFailed  = verdict.Failed
)

// Event is one entry from the event log. Payload is keyed by Kind.
type Event struct {
	Seq        int64
	Kind       string
	OccurredAt time.Time
	Payload    json.RawMessage
}

// Payload structs — one per kind. Used by both the producer (caller of
// store.AppendEvent) and the projection (decoded inside Project).

// RunCreatedPayload is engine's projection-side view of the RunCreated
// event. Mirror of store.runCreatedPayload — same JSON tags, different
// field set tailored to engine's needs. iter-5d adds RLCCWorkflow as a
// typed *rlccclient.WorkflowSnapshot so iter-5e's rlcc_action_probe can
// read .Actions directly; engine's projector ignores the field.
type RunCreatedPayload struct {
	TemplateID   string                       `json:"template_id"`
	Rack         string                       `json:"rack"`
	CreatedBy    string                       `json:"created_by"`
	RLCCWorkflow *rlccclient.WorkflowSnapshot `json:"rlcc_workflow,omitempty"`
}

// StepStartedPayload describes the beginning of a probe step run. StepType
// records the probe type name so a future query layer can group by probe.
type StepStartedPayload struct {
	StepID    string   `json:"step_id"`
	StepType  string   `json:"step_type,omitempty"`
	Racks     []string `json:"racks,omitempty"`
	StartedAt int64    `json:"started_at,omitempty"`
}

// StepCompletedPayload describes a successful probe completion. For
// gatherer probes (iter-4b's alert_probe), Verdict is empty — the
// projection leaves the run in StatusRunning. Assertion probes (iter-4c+)
// will set Verdict.
type StepCompletedPayload struct {
	StepID      string   `json:"step_id"`
	EvidenceID  string   `json:"evidence_id,omitempty"`
	DurationMS  int64    `json:"duration_ms,omitempty"`
	CompletedAt int64    `json:"completed_at,omitempty"`
	Sources     []string `json:"sources,omitempty"`
	Verdict     string   `json:"verdict,omitempty"` // passed | warning | failed (iter-4c+)
}

// StepFailedPayload describes a probe step that errored. Error carries
// the human-readable message; the projection flips run status to failed.
type StepFailedPayload struct {
	StepID     string `json:"step_id"`
	Error      string `json:"error"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	FailedAt   int64  `json:"failed_at,omitempty"`
}

// RunCompletedPayload records that the engine walk reached the end of the
// template without hitting an operator gate. FinalVerdict is the run's
// accumulated worst verdict at completion ("" when every step was a
// gatherer with no verdict); the projections finalize run status from it
// (empty => passed).
type RunCompletedPayload struct {
	CompletedAt  int64  `json:"completed_at"`
	FinalVerdict string `json:"final_verdict,omitempty"`
}

// RunCancelledPayload records an operator cancellation. The run is moved to
// the terminal 'cancelled' status and the in-flight walk (if any) is stopped.
type RunCancelledPayload struct {
	CancelledAt int64  `json:"cancelled_at"`
	By          string `json:"by,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type OperatorActionPayload struct {
	StepID     string `json:"step_id"`
	Action     string `json:"action"` // "advance" | "signoff"
	Verdict    string `json:"verdict,omitempty"`
	SignerName string `json:"signer_name,omitempty"`
	SignerRole string `json:"signer_role,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

// BMNDiscoveredPayload is one event per rack at run-create. The BMNs
// slice carries the per-CT enumeration as a single event (not one event
// per BMN) to keep the event log compact.
type BMNDiscoveredPayload struct {
	Rack         string     `json:"rack"`
	Zone         string     `json:"zone"`
	Source       string     `json:"source"`        // "inventory"
	DiscoveredAt int64      `json:"discovered_at"` // unix seconds
	BMNs         []BMNEntry `json:"bmns"`
}

// BMNEntry mirrors inventoryclient.BMN minus the parent Rack field
// (carried at the payload level). The duplication exists so engine
// doesn't import inventoryclient.
type BMNEntry struct {
	Deviceslot string `json:"deviceslot"`
	BMNName    string `json:"bmn_name"`
	CTPosition int    `json:"ct_position,omitempty"`
}

// BMNDiscoveryFailedPayload records that inventory enumeration failed for
// a rack. The projection flips runs.status to 'failed' so the run does
// not stay 'pending' forever.
type BMNDiscoveryFailedPayload struct {
	Rack    string `json:"rack"`
	Source  string `json:"source"` // "inventory"
	Message string `json:"message"`
}

// BMNIgnoredPayload describes one ignored BMN. Source is one of
// "operator" or "rlcc". Reason is operator-supplied free-text; RLCC-
// source ignores leave it empty.
type BMNIgnoredPayload struct {
	Rack       string `json:"rack"`
	Deviceslot string `json:"deviceslot"`
	Source     string `json:"source"` // "operator" | "rlcc"
	Reason     string `json:"reason,omitempty"`
	MarkedAt   int64  `json:"marked_at"` // unix seconds
}
