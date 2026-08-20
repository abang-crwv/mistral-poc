package engine

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"qac/internal/store"
)

// toEngineEvents converts store.Event slices to engine.Event slices so
// they can be folded through the pure Project function. The two types are
// structurally identical; the conversion is a field-for-field copy.
func toEngineEvents(evs []store.Event) []Event {
	out := make([]Event, len(evs))
	for i, e := range evs {
		out[i] = Event{
			Seq:        e.Seq,
			Kind:       e.Kind,
			Payload:    e.Payload,
			OccurredAt: e.OccurredAt,
		}
	}
	return out
}

func TestProjectionParity_StoreMatchesEngineProject(t *testing.T) {
	scripts := map[string][]struct {
		kind    string
		payload map[string]any
	}{
		"gatherer-only":     {{"RunCreated", map[string]any{"template_id": "t", "created_by": "x"}}, {"StepStarted", map[string]any{"step_id": "s1"}}, {"StepCompleted", map[string]any{"step_id": "s1"}}},
		"one-fail":          {{"RunCreated", map[string]any{"template_id": "t", "created_by": "x"}}, {"StepCompleted", map[string]any{"step_id": "s1", "verdict": "passed"}}, {"StepCompleted", map[string]any{"step_id": "s2", "verdict": "failed"}}},
		"all-pass":          {{"RunCreated", map[string]any{"template_id": "t", "created_by": "x"}}, {"StepCompleted", map[string]any{"step_id": "s1", "verdict": "passed"}}, {"StepCompleted", map[string]any{"step_id": "s2", "verdict": "passed"}}},
		"warning-then-fail": {{"RunCreated", map[string]any{"template_id": "t", "created_by": "x"}}, {"StepCompleted", map[string]any{"step_id": "s1", "verdict": "warning"}}, {"StepCompleted", map[string]any{"step_id": "s2", "verdict": "failed"}}},
		"step-failed":       {{"RunCreated", map[string]any{"template_id": "t", "created_by": "x"}}, {"StepStarted", map[string]any{"step_id": "s1"}}, {"StepFailed", map[string]any{"step_id": "s1", "error": "x"}}},
		"signoff-terminal":  {{"RunCreated", map[string]any{"template_id": "t", "created_by": "x"}}, {"StepCompleted", map[string]any{"step_id": "s1", "verdict": "passed"}}, {"OperatorAction", map[string]any{"action": "signoff"}}, {"StepStarted", map[string]any{"step_id": "s2"}}},
		"facts-failed":      {{"RunCreated", map[string]any{"template_id": "t", "created_by": "x"}}, {"FactsDiscoveryFailed", map[string]any{"error": "boom"}}},
		"bmn-failed":        {{"RunCreated", map[string]any{"template_id": "t", "created_by": "x"}}, {"BMNDiscoveryFailed", map[string]any{"rack": "r1", "source": "inventory", "message": "boom"}}},
		// Non-terminal: a passed StepCompleted FOLLOWS the discovery failure.
		// Without worst_verdict pinning, the store's StepCompleted worst-wins
		// rollup (Worse('', 'passed') = 'passed') would downgrade status to
		// passed while engine.Project stays failed. Both must end failed.
		"facts-failed-then-pass": {{"RunCreated", map[string]any{"template_id": "t", "created_by": "x"}}, {"FactsDiscoveryFailed", map[string]any{"error": "boom"}}, {"StepCompleted", map[string]any{"step_id": "s1", "verdict": "passed"}}},
		"bmn-failed-then-pass":   {{"RunCreated", map[string]any{"template_id": "t", "created_by": "x"}}, {"BMNDiscoveryFailed", map[string]any{"rack": "r1", "source": "inventory", "message": "boom"}}, {"StepCompleted", map[string]any{"step_id": "s1", "verdict": "passed"}}},
		// iter-5k: an advance OperatorAction must NOT change status — the
		// projection only branches signoff→signed_off. Both end passed.
		"advance-no-status-change": {{"RunCreated", map[string]any{"template_id": "t", "created_by": "x"}}, {"StepCompleted", map[string]any{"step_id": "s1", "verdict": "passed"}}, {"OperatorAction", map[string]any{"step_id": "r1", "action": "advance"}}},
		// iter-5k: the full operator sequence advance→advance→signoff ends
		// signed_off in both projections.
		"full-operator-sequence": {{"RunCreated", map[string]any{"template_id": "t", "created_by": "x"}}, {"StepCompleted", map[string]any{"step_id": "s1", "verdict": "passed"}}, {"OperatorAction", map[string]any{"step_id": "r1", "action": "advance"}}, {"OperatorAction", map[string]any{"step_id": "a1", "action": "advance"}}, {"OperatorAction", map[string]any{"step_id": "sign", "action": "signoff"}}},
	}
	for name, script := range scripts {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s, err := store.Open(ctx, filepath.Join(t.TempDir(), "p.db"))
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			t.Cleanup(func() { _ = s.Close() })
			runID := "01HZX0000000000000PARIT0"
			for _, e := range script {
				b, _ := json.Marshal(e.payload)
				if err := s.AppendEvent(ctx, runID, e.kind, b); err != nil {
					t.Fatalf("%s: append %s: %v", name, e.kind, err)
				}
			}
			runs, _ := s.ListRuns(ctx)
			var storeStatus string
			for _, r := range runs {
				if r.ID == runID {
					storeStatus = r.Status
				}
			}
			evs, _ := s.LoadEvents(ctx, runID)
			engineStatus := Project(toEngineEvents(evs)).Status
			if storeStatus != engineStatus {
				t.Errorf("%s: store=%q engine.Project=%q (diverged)", name, storeStatus, engineStatus)
			}
		})
	}
}
