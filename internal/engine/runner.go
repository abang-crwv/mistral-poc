package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"qac/internal/probe"
	"qac/internal/rlccclient"
	"qac/internal/store"
	"qac/internal/template"
	"qac/internal/verdict"
)

// runOnce drives runID forward by walking every runnable probe_type
// step in template order. Stops when there are no more runnable steps,
// when it encounters an operator-gated step kind (review/ai_assess/
// signoff), on DB read/write failure, or on panic.
//
// Walk semantics:
//   - Each iteration re-reads the event log and rebuilds the seenSteps
//     set from StepStarted/StepCompleted/StepFailed events. This keeps
//     persistence as the source of truth.
//   - skippedThisRun is loop-local: action.tool-shaped steps (no
//     probe_type) are added here so the loop doesn't re-consider them
//     on subsequent iterations. The set is lost when the goroutine
//     exits; a re-Kick re-evaluates from scratch (idempotently).
//   - StepFailed events do NOT halt the walk — they're recorded data,
//     not a stop signal. Same for unknown probe types (StepFailed
//     emitted, walk continues to the next step).
//   - DB read/write failures DO halt the walk — we can't safely
//     continue without a working event log.
func (e *Engine) runOnce(ctx context.Context, runID string) {
	var stepIDForPanic string
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("panic: %v", r)
			e.emitStepFailed(ctx, runID, stepIDForPanic, msg, 0)
		}
	}()

	// One-time setup: template + inputs are immutable for the run's
	// lifetime, so we read them once outside the loop. Events are
	// re-read inside the loop.
	evs, err := e.store.LoadEvents(ctx, runID)
	if err != nil {
		e.emitStepFailed(ctx, runID, "", fmt.Sprintf("load events: %v", err), 0)
		return
	}
	tpl, ok, err := e.loadTemplateFromEvents(ctx, evs)
	if err != nil {
		e.emitStepFailed(ctx, runID, "", fmt.Sprintf("load template: %v", err), 0)
		return
	}
	if !ok {
		e.emitStepFailed(ctx, runID, "", "load template: template not found", 0)
		return
	}
	inputs, err := loadRunInputsFromEvents(evs)
	if err != nil {
		e.emitStepFailed(ctx, runID, "", fmt.Sprintf("load inputs: %v", err), 0)
		return
	}

	// iter-5e: workflow snapshot + operator-ignored CTs are pinned at
	// run-create, so we read them once from the setup event slice.
	rlccWorkflow := loadRLCCWorkflowFromEvents(evs)
	operatorIgnored := operatorIgnoredFromEvents(evs)

	skippedThisRun := map[string]bool{}

	for {
		// Operator cancellation (Cancel(runID)) cancels ctx — halt the walk
		// without emitting further events. The terminal RunCancelled event
		// is recorded by the cancel path, not here.
		if ctx.Err() != nil {
			slog.Info("engine: walk cancelled", "run_id", runID)
			return
		}

		evs, err := e.store.LoadEvents(ctx, runID)
		if err != nil {
			e.emitStepFailed(ctx, runID, "", fmt.Sprintf("load events: %v", err), 0)
			return
		}
		seen := stepIDsFromEvents(evs)

		step, ok := findNextRunnableStep(tpl, seen, skippedThisRun)
		if !ok {
			// Walk reached the end with no operator gate. Finalize the run
			// so it leaves 'running' — a baseline-only template is all
			// gatherers (empty verdict), so without this it would sit at
			// 'running' forever. Idempotent: only the first completion emits.
			if !runCompletedEmitted(evs) {
				e.emitRunCompleted(ctx, runID, worstVerdictFromEvents(evs))
			}
			slog.Info("engine: walk complete", "run_id", runID, "template_id", tpl.ID)
			return
		}

		// ai_assess runs an agent automatically: it reasons over the
		// context_steps' evidence and emits an advisory verdict that rolls
		// up via the existing StepCompleted plumbing. The walk then
		// continues to the operator gate.
		if step.Kind == "ai_assess" {
			e.runAIAssess(ctx, runID, step, inputs)
			continue
		}
		// Halt at the remaining operator-gated step kinds. The
		// operator-advance endpoint that unblocks these is iter-4d.
		if step.Kind == "review" || step.Kind == "signoff" {
			slog.Info("engine: halted at operator-gated step",
				"run_id", runID, "step_id", step.ID, "kind", step.Kind)
			return
		}

		// Skip action.tool-shaped probe steps — the engine has no tool
		// dispatcher yet. Adding to skippedThisRun prevents the next
		// iteration from re-finding the same step (no event is emitted
		// for skips, so the seen-from-events set wouldn't pick them up).
		if step.ProbeType == "" {
			skippedThisRun[step.ID] = true
			continue
		}

		stepIDForPanic = step.ID
		racks := resolveRacks(step.Racks, inputs)

		p, ok := e.probes.Get(step.ProbeType)
		if !ok {
			e.emitStepFailed(ctx, runID, step.ID,
				fmt.Sprintf("probe type %q not registered", step.ProbeType), 0)
			continue
		}

		startedAt := time.Now()
		if err := e.emitStepStarted(ctx, runID, step.ID, step.ProbeType, racks, startedAt.Unix()); err != nil {
			// DB write failed mid-walk. We try a best-effort StepFailed
			// then exit — every subsequent step would hit the same wall.
			slog.Error("engine: emit StepStarted failed", "run_id", runID, "step_id", step.ID, "err", err)
			e.emitStepFailed(ctx, runID, step.ID, fmt.Sprintf("emit StepStarted: %v", err), 0)
			return
		}

		var stepConfig map[string]any
		if step.Action != nil {
			if m, ok := step.Action.Args.(map[string]any); ok {
				stepConfig = m
			}
		}

		sc := probe.StepContext{
			RunID:              runID,
			StepID:             step.ID,
			Racks:              racks,
			Inputs:             inputs,
			Config:             stepConfig,
			RLCCWorkflow:       rlccWorkflow,
			IgnoredDeviceslots: operatorIgnored,
		}

		runCtx := ctx
		var cancel context.CancelFunc
		if step.Timeout != "" {
			if d, perr := time.ParseDuration(step.Timeout); perr == nil && d > 0 {
				runCtx, cancel = context.WithTimeout(ctx, d)
			}
		}
		output, runErr := p.Run(runCtx, e.clients, sc)
		if cancel != nil {
			cancel()
		}
		// If the run was cancelled while the probe was in flight, the probe
		// returns promptly on its ctx.Done; halt now without emitting a
		// StepCompleted/StepFailed (which would roll the run status off
		// 'cancelled'). The RunCancelled event is the run's terminal record.
		if ctx.Err() != nil {
			slog.Info("engine: walk cancelled mid-step", "run_id", runID, "step_id", step.ID)
			return
		}
		durationMS := time.Since(startedAt).Milliseconds()
		if runErr != nil {
			e.emitStepFailed(ctx, runID, step.ID, runErr.Error(), durationMS)
			continue
		}

		evidenceID := ulid.Make().String()
		if err := e.store.PutEvidence(ctx, store.Evidence{
			ID:          evidenceID,
			RunID:       runID,
			StepID:      step.ID,
			ContentType: "application/json",
			Payload:     output.StructuredData,
		}); err != nil {
			e.emitStepFailed(ctx, runID, step.ID, fmt.Sprintf("persist evidence: %v", err), durationMS)
			continue
		}

		if err := e.emitStepCompleted(ctx, runID, step.ID, evidenceID, durationMS, output.Sources, output.Verdict); err != nil {
			// Evidence row already landed; the StepCompleted event
			// didn't. Operator can reconstruct via the orphaned
			// evidence id. Continue the walk.
			slog.Error("engine: emit StepCompleted failed (evidence orphaned)",
				"run_id", runID, "step_id", step.ID, "evidence_id", evidenceID, "err", err)
		}
	}
}

// stepIDsFromEvents collects every step ID referenced by a
// StepStarted/StepCompleted/StepFailed event. This is the "seen" set the
// walk uses to skip steps it has already attempted (whether they
// succeeded or failed — both count as "looked at").
func stepIDsFromEvents(evs []store.Event) map[string]bool {
	seen := map[string]bool{}
	for _, ev := range evs {
		if ev.Kind != KindStepStarted && ev.Kind != KindStepCompleted && ev.Kind != KindStepFailed && ev.Kind != KindOperatorAction {
			continue
		}
		var p struct {
			StepID string `json:"step_id"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			continue // malformed events were already rejected at AppendEvent time; tolerant decode here is defensive
		}
		if p.StepID != "" {
			seen[p.StepID] = true
		}
	}
	return seen
}

// loadTemplateFromEvents reads the RunCreated event from a pre-loaded
// event slice and looks up the referenced template in the store.
// Separating the LoadEvents call from this helper keeps runOnce's data
// dependency on the event log explicit and avoids a duplicate query.
func (e *Engine) loadTemplateFromEvents(ctx context.Context, evs []store.Event) (template.Template, bool, error) {
	if len(evs) == 0 {
		return template.Template{}, false, errors.New("run has no events")
	}
	if evs[0].Kind != "RunCreated" {
		return template.Template{}, false, fmt.Errorf("first event is %q, not RunCreated", evs[0].Kind)
	}
	var rc struct {
		TemplateID string `json:"template_id"`
	}
	if err := json.Unmarshal(evs[0].Payload, &rc); err != nil {
		return template.Template{}, false, fmt.Errorf("decode RunCreated: %w", err)
	}
	row, ok, err := e.store.GetTemplate(ctx, rc.TemplateID)
	if err != nil {
		return template.Template{}, false, fmt.Errorf("GetTemplate: %w", err)
	}
	if !ok {
		return template.Template{}, false, nil
	}
	var tpl template.Template
	if err := json.Unmarshal(row.ParsedJSON, &tpl); err != nil {
		return template.Template{}, false, fmt.Errorf("decode template: %w", err)
	}
	return tpl, true, nil
}

// loadRunInputsFromEvents reads the inputs map from a pre-loaded event slice.
func loadRunInputsFromEvents(evs []store.Event) (map[string]any, error) {
	if len(evs) == 0 || evs[0].Kind != "RunCreated" {
		return nil, errors.New("no RunCreated event")
	}
	var rc struct {
		Inputs map[string]any `json:"inputs"`
	}
	if err := json.Unmarshal(evs[0].Payload, &rc); err != nil {
		return nil, err
	}
	if rc.Inputs == nil {
		rc.Inputs = map[string]any{}
	}
	return rc.Inputs, nil
}

// loadRLCCWorkflowFromEvents extracts the pinned workflow snapshot from the
// RunCreated event. Returns nil for pre-iter-5d runs (no rlcc_workflow key)
// or a malformed payload — the probe surfaces the nil as a StepFailed.
func loadRLCCWorkflowFromEvents(evs []store.Event) *rlccclient.WorkflowSnapshot {
	if len(evs) == 0 || evs[0].Kind != KindRunCreated {
		return nil
	}
	var rc struct {
		RLCCWorkflow *rlccclient.WorkflowSnapshot `json:"rlcc_workflow"`
	}
	if err := json.Unmarshal(evs[0].Payload, &rc); err != nil {
		return nil
	}
	return rc.RLCCWorkflow
}

// operatorIgnoredFromEvents collects the deviceslots ignored by the operator
// (BMNIgnored with source=="operator"). RLCC-source ignores are excluded —
// those are re-detected live by rlcc_action_probe each tick.
func operatorIgnoredFromEvents(evs []store.Event) []string {
	var out []string
	for _, ev := range evs {
		if ev.Kind != KindBMNIgnored {
			continue
		}
		var p BMNIgnoredPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			continue
		}
		if p.Source == "operator" && p.Deviceslot != "" {
			out = append(out, p.Deviceslot)
		}
	}
	return out
}

// findNextRunnableStep returns the next step the loop hasn't dealt with
// yet. It does NOT filter by kind — the caller (runOnce) inspects the
// returned step's Kind to decide whether to halt (operator-gated steps)
// or skip (action.tool-shaped probe steps).
//
// `seen` is the set of step IDs that appear in any StepStarted /
// StepCompleted / StepFailed event in the run's history. `skipped`
// is the loop-local set of step IDs the current goroutine has already
// looked at and decided to skip without persisting any event.
//
// Both `seen` and `skipped` may be nil — Go reads from a nil map as
// the zero value, so a nil map behaves identically to an empty one here.
func findNextRunnableStep(tpl template.Template, seen map[string]bool, skipped map[string]bool) (template.Step, bool) {
	for _, st := range tpl.Steps {
		if seen[st.ID] || skipped[st.ID] {
			continue
		}
		return st, true
	}
	return template.Step{}, false
}

// resolveRacks inflates the iter-4b "{{ inputs.canary_racks }}" literal
// — the only template expression iter-4b supports. Anything else is
// returned verbatim as a single-element slice (best-effort).
func resolveRacks(expr string, inputs map[string]any) []string {
	if strings.TrimSpace(expr) == "" || strings.TrimSpace(expr) == "{{ inputs.canary_racks }}" {
		raw, ok := inputs["canary_racks"]
		if !ok {
			return nil
		}
		list, ok := raw.([]any)
		if !ok {
			return nil
		}
		out := make([]string, 0, len(list))
		for _, v := range list {
			if s, ok := v.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return []string{expr}
}

func (e *Engine) emitStepStarted(ctx context.Context, runID, stepID, stepType string, racks []string, startedAt int64) error {
	payload, _ := json.Marshal(StepStartedPayload{
		StepID:    stepID,
		StepType:  stepType,
		Racks:     racks,
		StartedAt: startedAt,
	})
	return e.store.AppendEvent(ctx, runID, KindStepStarted, payload)
}

func (e *Engine) emitStepCompleted(ctx context.Context, runID, stepID, evidenceID string, durationMS int64, sources []string, verdict string) error {
	payload, _ := json.Marshal(StepCompletedPayload{
		StepID:      stepID,
		EvidenceID:  evidenceID,
		DurationMS:  durationMS,
		CompletedAt: time.Now().Unix(),
		Sources:     sources,
		Verdict:     verdict,
	})
	return e.store.AppendEvent(ctx, runID, KindStepCompleted, payload)
}

// emitRunCompleted best-efforts a RunCompleted event finalizing the run.
// A failed append just leaves the run at its current status (the next walk
// trigger would retry) — so we log rather than hard-fail.
func (e *Engine) emitRunCompleted(ctx context.Context, runID, finalVerdict string) {
	payload, _ := json.Marshal(RunCompletedPayload{
		CompletedAt:  time.Now().Unix(),
		FinalVerdict: finalVerdict,
	})
	if err := e.store.AppendEvent(ctx, runID, KindRunCompleted, payload); err != nil {
		slog.Error("engine: emit RunCompleted failed", "run_id", runID, "err", err)
	}
}

// runCompletedEmitted reports whether a RunCompleted event is already in the
// run's history, so a re-triggered walk doesn't append a duplicate.
func runCompletedEmitted(evs []store.Event) bool {
	for _, ev := range evs {
		if ev.Kind == KindRunCompleted {
			return true
		}
	}
	return false
}

// worstVerdictFromEvents folds the run's StepCompleted verdicts (and any
// StepFailed) into the worst-wins verdict, matching the store/projection
// rollup. Used only to stamp RunCompleted.FinalVerdict; the projections
// finalize status from their own authoritative worst_verdict.
func worstVerdictFromEvents(evs []store.Event) string {
	worst := ""
	for _, ev := range evs {
		switch ev.Kind {
		case KindStepCompleted:
			var p StepCompletedPayload
			if json.Unmarshal(ev.Payload, &p) == nil {
				worst = verdict.Worse(worst, p.Verdict)
			}
		case KindStepFailed, KindFactsDiscoveryFailed, KindBMNDiscoveryFailed:
			worst = verdict.Worse(worst, VerdictFailed)
		}
	}
	return worst
}

// emitStepFailed best-efforts an emission of a StepFailed event. If even
// that append fails (database down, etc.), we log and return — the runner
// has no further fallback.
func (e *Engine) emitStepFailed(ctx context.Context, runID, stepID, errMsg string, durationMS int64) {
	payload, _ := json.Marshal(StepFailedPayload{
		StepID:     stepID,
		Error:      errMsg,
		DurationMS: durationMS,
		FailedAt:   time.Now().Unix(),
	})
	if err := e.store.AppendEvent(ctx, runID, KindStepFailed, payload); err != nil {
		slog.Error("engine: emit StepFailed failed (stuck run)",
			"run_id", runID, "step_id", stepID, "original_err", errMsg, "append_err", err)
	}
}
