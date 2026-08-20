package engine

import (
	"encoding/json"
	"testing"

	"qac/internal/rlccclient"
)

func TestRunCreatedPayload_LegacyDecode(t *testing.T) {
	// Pre-iter-5d payloads decode to nil RLCCWorkflow.
	body := []byte(`{"template_id":"firmware-release-canary","rack":"dh3-r012-us-east-01a","created_by":"wpena"}`)
	var p RunCreatedPayload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if p.RLCCWorkflow != nil {
		t.Errorf("RLCCWorkflow = %v, want nil (legacy)", p.RLCCWorkflow)
	}
}

func TestRunCreatedPayload_Iter5dTypedDecode(t *testing.T) {
	// iter-5d payloads decode to a typed WorkflowSnapshot with actions
	// available via the rlccclient.Action shape.
	body := []byte(`{
		"template_id":"firmware-release-canary",
		"created_by":"wpena",
		"rlcc_workflow":{
			"name":"gb200-rack-bringup-v4",
			"source_sha":"abc123",
			"actions":[
				{"name":"hlcc-state action","handler":"hlcc-state","arguments":{"components":{"rack":{"state":"onboarded"}}}}
			]
		}
	}`)
	var p RunCreatedPayload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if p.RLCCWorkflow == nil {
		t.Fatal("RLCCWorkflow = nil, want non-nil")
	}
	if p.RLCCWorkflow.Name != "gb200-rack-bringup-v4" {
		t.Errorf("Name = %q, want gb200-rack-bringup-v4", p.RLCCWorkflow.Name)
	}
	if p.RLCCWorkflow.SourceSHA != "abc123" {
		t.Errorf("SourceSHA = %q, want abc123", p.RLCCWorkflow.SourceSHA)
	}
	if len(p.RLCCWorkflow.Actions) != 1 {
		t.Fatalf("len(Actions) = %d, want 1", len(p.RLCCWorkflow.Actions))
	}
	if got := p.RLCCWorkflow.Actions[0].Handler; got != rlccclient.HandlerHLCCState {
		t.Errorf("Actions[0].Handler = %q, want %q", got, rlccclient.HandlerHLCCState)
	}
}
