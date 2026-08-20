package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"qac/internal/agent"
	"qac/internal/store"
	"qac/internal/template"
)

// assessmentContentType marks an evidence row as an agent assessment so the
// UI can render it distinctly.
const assessmentContentType = "application/vnd.qac.assessment+json"

// aiAssessDefaultTimeout bounds the inline LLM call when the step has no
// explicit timeout, so a hung/slow call can't hold an engine concurrency
// slot for the SDK's much longer default.
const aiAssessDefaultTimeout = 5 * time.Minute

// runAIAssess dispatches an ai_assess step to the registered agent for the
// step's prompt_ref: it assembles the context_steps' evidence into a bundle,
// runs the agent, persists the agent output as an evidence row, and emits
// StepCompleted with the mapped verdict (which the existing rollup picks up).
// Any failure emits StepFailed and returns so the walk moves on.
func (e *Engine) runAIAssess(ctx context.Context, runID string, step template.Step, inputs map[string]any) {
	startedAt := time.Now()
	if err := e.emitStepStarted(ctx, runID, step.ID, "ai_assess", nil, startedAt.Unix()); err != nil {
		e.emitStepFailed(ctx, runID, step.ID, fmt.Sprintf("emit StepStarted: %v", err), 0)
		return
	}
	if e.agents == nil {
		e.emitStepFailed(ctx, runID, step.ID, "no agent registry configured", 0)
		return
	}
	ag, ok := e.agents.Get(step.PromptRef)
	if !ok {
		e.emitStepFailed(ctx, runID, step.ID,
			fmt.Sprintf("no agent registered for prompt_ref %q", step.PromptRef), 0)
		return
	}
	bundle, err := e.assembleEvidenceBundle(ctx, runID, step.ContextSteps)
	if err != nil {
		e.emitStepFailed(ctx, runID, step.ID, err.Error(), time.Since(startedAt).Milliseconds())
		return
	}
	timeout := aiAssessDefaultTimeout
	if step.Timeout != "" {
		if d, perr := time.ParseDuration(step.Timeout); perr == nil && d > 0 {
			timeout = d
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	out, err := ag.Run(runCtx, e.agentClients, agent.StepContext{
		RunID:               runID,
		StepID:              step.ID,
		EvidenceBundle:      bundle,
		ExpectedVerdict:     step.ExpectedVerdict,
		RespectSuppressions: step.RespectSuppressions,
		CitePatterns:        step.CitePatterns,
		CiteGlean:           step.CiteGlean,
		Inputs:              inputs,
	})
	cancel()
	// If the run was cancelled (as opposed to the per-call timeout expiring),
	// halt now without emitting StepFailed — mirrors the probe path in
	// runOnce, which checks the parent ctx (not runCtx) after the call
	// returns. A timeout only cancels runCtx, so it still falls through to
	// the error branch below and emits StepFailed with the deadline error.
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		e.emitStepFailed(ctx, runID, step.ID, err.Error(), time.Since(startedAt).Milliseconds())
		return
	}
	payload, err := json.Marshal(out)
	if err != nil {
		e.emitStepFailed(ctx, runID, step.ID, fmt.Sprintf("marshal assessment: %v", err), time.Since(startedAt).Milliseconds())
		return
	}
	evidenceID := ulid.Make().String()
	if err := e.store.PutEvidence(ctx, store.Evidence{
		ID:          evidenceID,
		RunID:       runID,
		StepID:      step.ID,
		ContentType: assessmentContentType,
		Payload:     payload,
	}); err != nil {
		e.emitStepFailed(ctx, runID, step.ID, fmt.Sprintf("persist assessment: %v", err), time.Since(startedAt).Milliseconds())
		return
	}
	if err := e.emitStepCompleted(ctx, runID, step.ID, evidenceID,
		time.Since(startedAt).Milliseconds(), out.Sources, agent.MapVerdict(out.Verdict)); err != nil {
		slog.Error("engine: emit StepCompleted failed (assessment evidence orphaned)",
			"run_id", runID, "step_id", step.ID, "evidence_id", evidenceID, "err", err)
	}
}

// assembleEvidenceBundle concatenates the evidence rows of contextSteps into a
// text bundle the agent reasons over. Suppression filtering is a documented
// no-op stand-in in iteration 1 (there is no per-row suppression concept yet).
func (e *Engine) assembleEvidenceBundle(ctx context.Context, runID string, contextSteps []string) (string, error) {
	var b strings.Builder
	for _, sid := range contextSteps {
		rows, err := e.store.ListEvidence(ctx, runID, sid)
		if err != nil {
			return "", fmt.Errorf("list evidence for step %s: %w", sid, err)
		}
		for _, row := range rows {
			fmt.Fprintf(&b, "## step %s", sid)
			if row.Deviceslot != nil {
				fmt.Fprintf(&b, " (deviceslot %s)", *row.Deviceslot)
			}
			b.WriteString("\n")
			b.Write(row.Payload)
			b.WriteString("\n\n")
		}
	}
	return b.String(), nil
}
