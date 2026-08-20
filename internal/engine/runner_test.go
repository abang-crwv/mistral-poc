package engine

import (
	"testing"

	"qac/internal/store"
	"qac/internal/template"
)

// stepTpl is a test-local convenience: builds a Template with N steps.
func stepTpl(steps ...template.Step) template.Template {
	return template.Template{
		Schema:  "qac.template/v1",
		ID:      "test-tpl",
		Version: 1,
		Title:   "test",
		Target:  template.Target{Scope: "rack"},
		Steps:   steps,
	}
}

func TestFindNextRunnableStep_Empty_NoMatch(t *testing.T) {
	tpl := stepTpl()
	step, ok := findNextRunnableStep(tpl, nil, nil)
	if ok {
		t.Fatalf("findNextRunnableStep returned (%+v, true) for empty template; want (_, false)", step)
	}
}

func TestFindNextRunnableStep_ReturnsFirstStep_WhenNoneSeenOrSkipped(t *testing.T) {
	tpl := stepTpl(
		template.Step{ID: "alert_baseline", Kind: "probe", ProbeType: "alert_probe"},
		template.Step{ID: "firmware_baseline", Kind: "probe", ProbeType: "firmware_inventory_probe"},
	)
	step, ok := findNextRunnableStep(tpl, nil, nil)
	if !ok {
		t.Fatal("findNextRunnableStep: not found, want first step (alert_baseline)")
	}
	if step.ID != "alert_baseline" {
		t.Errorf("step.ID = %q, want alert_baseline", step.ID)
	}
}

func TestFindNextRunnableStep_SkipsSeen(t *testing.T) {
	tpl := stepTpl(
		template.Step{ID: "alert_baseline", Kind: "probe", ProbeType: "alert_probe"},
		template.Step{ID: "firmware_baseline", Kind: "probe", ProbeType: "firmware_inventory_probe"},
	)
	seen := map[string]bool{"alert_baseline": true}
	step, ok := findNextRunnableStep(tpl, seen, nil)
	if !ok {
		t.Fatal("findNextRunnableStep: not found, want firmware_baseline")
	}
	if step.ID != "firmware_baseline" {
		t.Errorf("step.ID = %q, want firmware_baseline", step.ID)
	}
}

func TestFindNextRunnableStep_SkipsSkipped(t *testing.T) {
	// A step in `skipped` (action.tool-shaped or otherwise filtered this
	// iteration) is treated identically to a `seen` step for the purposes
	// of "find the next one to look at." This is what lets the walk loop
	// iterate past skipped steps without re-considering them.
	tpl := stepTpl(
		template.Step{ID: "alert_baseline", Kind: "probe", ProbeType: "alert_probe"},
		template.Step{ID: "firmware_baseline", Kind: "probe", ProbeType: "firmware_inventory_probe"},
	)
	skipped := map[string]bool{"alert_baseline": true}
	step, ok := findNextRunnableStep(tpl, nil, skipped)
	if !ok {
		t.Fatal("findNextRunnableStep: not found, want firmware_baseline")
	}
	if step.ID != "firmware_baseline" {
		t.Errorf("step.ID = %q, want firmware_baseline", step.ID)
	}
}

func TestFindNextRunnableStep_ReturnsNonProbeKindForHaltSignaling(t *testing.T) {
	// The helper does NOT filter by kind — it returns the next un-seen +
	// un-skipped step regardless of kind. The caller (runOnce) inspects
	// step.Kind to decide whether to halt (operator-gated) or skip
	// (action.tool-shaped). This keeps the helper a single-responsibility
	// "find the next step the loop hasn't dealt with yet."
	tpl := stepTpl(
		template.Step{ID: "review_step", Kind: "review", Sources: []string{"some_probe"}},
	)
	step, ok := findNextRunnableStep(tpl, nil, nil)
	if !ok {
		t.Fatal("findNextRunnableStep: not found, want review_step (caller halts)")
	}
	if step.ID != "review_step" || step.Kind != "review" {
		t.Errorf("step = %+v, want review_step (kind=review)", step)
	}
}

func TestStepIDsFromEvents_OperatorActionMarksSeen(t *testing.T) {
	evs := []store.Event{
		{Kind: "StepStarted", Payload: []byte(`{"step_id":"s1"}`)},
		{Kind: "OperatorAction", Payload: []byte(`{"step_id":"evidence_review","action":"advance"}`)},
	}
	seen := stepIDsFromEvents(evs)
	if !seen["s1"] {
		t.Error("s1 not marked seen")
	}
	if !seen["evidence_review"] {
		t.Error("OperatorAction step not marked seen")
	}
}
