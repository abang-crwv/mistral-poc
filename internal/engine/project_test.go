package engine

import (
	"encoding/json"
	"testing"
	"time"

	"qac/internal/rlccclient"
)

func mustPayload(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestProject_RunCreated_Pending(t *testing.T) {
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	events := []Event{
		{Seq: 1, Kind: KindRunCreated, OccurredAt: now, Payload: mustPayload(t, RunCreatedPayload{
			TemplateID: "firmware-release-canary",
			Rack:       "as1-r07",
			CreatedBy:  "wpena",
		})},
	}
	st := Project(events)
	if st.Status != StatusPending {
		t.Fatalf("status = %q, want %q", st.Status, StatusPending)
	}
	if st.Rack != "as1-r07" {
		t.Fatalf("rack = %q, want as1-r07", st.Rack)
	}
	if !st.CreatedAt.Equal(now) {
		t.Fatalf("created_at = %v, want %v", st.CreatedAt, now)
	}
}

func TestProject_StepStarted_Running(t *testing.T) {
	events := []Event{
		{Kind: KindRunCreated, Payload: mustPayload(t, RunCreatedPayload{TemplateID: "t", Rack: "r"})},
		{Kind: KindStepStarted, Payload: mustPayload(t, StepStartedPayload{StepID: "capacity_check"})},
	}
	st := Project(events)
	if st.Status != StatusRunning {
		t.Fatalf("status = %q, want %q", st.Status, StatusRunning)
	}
}

func TestProject_WorstWinsAcrossSteps(t *testing.T) {
	cases := []struct {
		name     string
		verdicts []string
		want     string
	}{
		{"all passed", []string{"passed", "passed"}, StatusPassed},
		{"warning beats passed", []string{"passed", "warning"}, StatusWarning},
		{"failed beats warning", []string{"warning", "failed"}, StatusFailed},
		{"failed sticks even if later passed", []string{"failed", "passed"}, StatusFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events := []Event{
				{Kind: KindRunCreated, Payload: mustPayload(t, RunCreatedPayload{TemplateID: "t", Rack: "r"})},
			}
			for _, v := range tc.verdicts {
				events = append(events,
					Event{Kind: KindStepStarted, Payload: mustPayload(t, StepStartedPayload{StepID: "s"})},
					Event{Kind: KindStepCompleted, Payload: mustPayload(t, StepCompletedPayload{StepID: "s", Verdict: v})},
				)
			}
			st := Project(events)
			if st.Status != tc.want {
				t.Fatalf("status = %q, want %q", st.Status, tc.want)
			}
		})
	}
}

func TestProject_Signoff_OverridesAll(t *testing.T) {
	events := []Event{
		{Kind: KindRunCreated, Payload: mustPayload(t, RunCreatedPayload{TemplateID: "t", Rack: "r"})},
		{Kind: KindStepCompleted, Payload: mustPayload(t, StepCompletedPayload{StepID: "s", Verdict: "failed"})},
		{Kind: KindOperatorAction, Payload: mustPayload(t, OperatorActionPayload{Action: "signoff", Notes: "expected failure"})},
	}
	st := Project(events)
	if st.Status != StatusSignedOff {
		t.Fatalf("status = %q, want signed_off", st.Status)
	}
}

func TestProject_StepFailed_SetsStatusFailed(t *testing.T) {
	events := []Event{
		{Kind: KindRunCreated, Payload: mustPayload(t, RunCreatedPayload{TemplateID: "t", Rack: "r"})},
		{Kind: KindStepStarted, Payload: mustPayload(t, StepStartedPayload{StepID: "alert_baseline", StepType: "alert_probe"})},
		{Kind: KindStepFailed, Payload: mustPayload(t, StepFailedPayload{StepID: "alert_baseline", Error: "alertclient: source unavailable"})},
	}
	st := Project(events)
	if st.Status != StatusFailed {
		t.Fatalf("status = %q, want %q", st.Status, StatusFailed)
	}
}

func TestProject_GathererStepCompleted_StaysRunning(t *testing.T) {
	// A gatherer probe emits StepCompleted with no Verdict. The projection
	// must leave the run in "running" (not flip to passed/warning/failed).
	events := []Event{
		{Kind: KindRunCreated, Payload: mustPayload(t, RunCreatedPayload{TemplateID: "t", Rack: "r"})},
		{Kind: KindStepStarted, Payload: mustPayload(t, StepStartedPayload{StepID: "alert_baseline", StepType: "alert_probe"})},
		{Kind: KindStepCompleted, Payload: mustPayload(t, StepCompletedPayload{StepID: "alert_baseline", EvidenceID: "01HEVIDENCEID"})},
	}
	st := Project(events)
	if st.Status != StatusRunning {
		t.Fatalf("status = %q, want %q (gatherer leaves run running)", st.Status, StatusRunning)
	}
}

func TestProject_StepFailed_OverridesPriorPassed(t *testing.T) {
	// Defensive: even if a prior step passed, a later StepFailed wins.
	events := []Event{
		{Kind: KindRunCreated, Payload: mustPayload(t, RunCreatedPayload{TemplateID: "t", Rack: "r"})},
		{Kind: KindStepCompleted, Payload: mustPayload(t, StepCompletedPayload{StepID: "s1", Verdict: VerdictPassed})},
		{Kind: KindStepFailed, Payload: mustPayload(t, StepFailedPayload{StepID: "s2", Error: "boom"})},
	}
	st := Project(events)
	if st.Status != StatusFailed {
		t.Fatalf("status = %q, want %q", st.Status, StatusFailed)
	}
}

func TestProject_StepFailed_AfterSignoff_StaysSignedOff(t *testing.T) {
	events := []Event{
		{Kind: KindRunCreated, Payload: mustPayload(t, RunCreatedPayload{TemplateID: "t", Rack: "r"})},
		{Kind: KindOperatorAction, Payload: mustPayload(t, OperatorActionPayload{Action: "signoff", Notes: "ok"})},
		{Kind: KindStepFailed, Payload: mustPayload(t, StepFailedPayload{StepID: "s", Error: "boom"})},
	}
	st := Project(events)
	if st.Status != StatusSignedOff {
		t.Fatalf("status = %q, want signed_off (StepFailed cannot override operator signoff)", st.Status)
	}
}

func TestProject_GathererAfterStepFailed_StaysFailed(t *testing.T) {
	// Regression: a StepFailed event must pin worstVerdict so a later
	// gatherer StepCompleted (verdict="") cannot downgrade status from
	// failed back to running.
	events := []Event{
		{Kind: KindRunCreated, Payload: mustPayload(t, RunCreatedPayload{TemplateID: "t", Rack: "r"})},
		{Kind: KindStepFailed, Payload: mustPayload(t, StepFailedPayload{StepID: "s1", Error: "boom"})},
		{Kind: KindStepCompleted, Payload: mustPayload(t, StepCompletedPayload{StepID: "s2", EvidenceID: "01HEVIDENCE"})},
	}
	st := Project(events)
	if st.Status != StatusFailed {
		t.Fatalf("status = %q, want failed (gatherer after failure must not downgrade)", st.Status)
	}
}

func TestProject_IgnoresBMNEvents(t *testing.T) {
	// BMNDiscovered/Failed/Ignored should not panic or change run.Status.
	now := time.Now()
	events := []Event{
		{Seq: 1, Kind: KindRunCreated, OccurredAt: now, Payload: mustPayload(t, RunCreatedPayload{TemplateID: "t", Rack: "r"})},
		{Seq: 2, Kind: KindBMNDiscovered, OccurredAt: now, Payload: mustPayload(t, BMNDiscoveredPayload{
			Rack: "r", Zone: "z", Source: "inventory", DiscoveredAt: now.Unix(),
			BMNs: []BMNEntry{{Deviceslot: "r-node-01-z", BMNName: "s1"}},
		})},
		{Seq: 3, Kind: KindBMNIgnored, OccurredAt: now, Payload: mustPayload(t, BMNIgnoredPayload{
			Rack: "r", Deviceslot: "r-node-01-z", Source: "operator", MarkedAt: now.Unix(),
		})},
	}
	state := Project(events)
	if state.Status != StatusPending {
		t.Errorf("Status = %q, want pending (BMN events don't change status)", state.Status)
	}
}

func TestProject_RunCreated_PopulatesRLCCWorkflow(t *testing.T) {
	payload, _ := json.Marshal(RunCreatedPayload{
		TemplateID: "firmware-release-canary",
		Rack:       "rack-a",
		CreatedBy:  "wpena",
		RLCCWorkflow: &rlccclient.WorkflowSnapshot{
			Name:      "gb200-canary",
			SourceSHA: "abc123",
			Actions: []rlccclient.Action{
				{Name: "field diag", Handler: rlccclient.HandlerL11FieldDiag},
			},
		},
	})
	st := Project([]Event{{Seq: 1, Kind: KindRunCreated, OccurredAt: time.Unix(1700000000, 0), Payload: payload}})

	if st.RLCCWorkflow == nil {
		t.Fatal("RunState.RLCCWorkflow is nil, want populated")
	}
	if st.RLCCWorkflow.Name != "gb200-canary" || st.RLCCWorkflow.SourceSHA != "abc123" {
		t.Errorf("snapshot = %+v, want name=gb200-canary sha=abc123", st.RLCCWorkflow)
	}
	if len(st.RLCCWorkflow.Actions) != 1 || st.RLCCWorkflow.Actions[0].Handler != rlccclient.HandlerL11FieldDiag {
		t.Errorf("actions = %+v, want one l11-fielddiag action", st.RLCCWorkflow.Actions)
	}
}

func TestProject_RunCreated_NoWorkflow_NilSnapshot(t *testing.T) {
	// Pre-iter-5d run: payload has no rlcc_workflow key.
	payload, _ := json.Marshal(map[string]any{
		"template_id": "firmware-release-canary",
		"rack":        "rack-a",
		"created_by":  "wpena",
	})
	st := Project([]Event{{Seq: 1, Kind: KindRunCreated, OccurredAt: time.Unix(1700000000, 0), Payload: payload}})
	if st.RLCCWorkflow != nil {
		t.Errorf("RLCCWorkflow = %+v, want nil for pre-iter-5d run", st.RLCCWorkflow)
	}
}
