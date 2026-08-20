package rlccclient

import (
	"encoding/json"
	"testing"
)

func TestWorkflowSnapshot_RoundTrip(t *testing.T) {
	in := WorkflowSnapshot{
		Name:      "gb200-rack-bringup-v4",
		SourceSHA: "abc123def456",
		Actions: []Action{
			{
				Name:        "checked-in → onboarded",
				Handler:     HandlerHLCCState,
				Description: "state mover",
				Arguments: ActionArgs{
					Components: map[ComponentKind]ComponentSpec{
						ComponentRack: {State: "onboarded"},
					},
				},
			},
		},
	}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out WorkflowSnapshot
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Name != in.Name {
		t.Errorf("Name = %q, want %q", out.Name, in.Name)
	}
	if out.SourceSHA != in.SourceSHA {
		t.Errorf("SourceSHA = %q, want %q", out.SourceSHA, in.SourceSHA)
	}
	if len(out.Actions) != len(in.Actions) {
		t.Fatalf("len(Actions) = %d, want %d", len(out.Actions), len(in.Actions))
	}
	if out.Actions[0].Name != in.Actions[0].Name {
		t.Errorf("Actions[0].Name = %q, want %q", out.Actions[0].Name, in.Actions[0].Name)
	}
}

func TestWorkflowSnapshot_EmptyActionsRoundTrip(t *testing.T) {
	in := WorkflowSnapshot{Name: "x", SourceSHA: "y", Actions: []Action{}}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// JSON encoder emits `"actions":[]` for an empty (non-nil) slice. Verify.
	if want := `"actions":[]`; !contains(string(body), want) {
		t.Errorf("marshaled body = %s, want substring %s", body, want)
	}
	var out WorkflowSnapshot
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Actions == nil {
		t.Error("Actions decoded to nil; want empty slice")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
