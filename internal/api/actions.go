package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"qac/internal/engine"
	"qac/internal/store"
	"qac/internal/template"
	"qac/internal/verdict"
)

// operatorActionRequest is the wire shape for POST /api/runs/{id}/actions.
// An operator either advances a review/ai_assess gate or signs off the final
// signoff gate. The handler validates against the run's template and the
// current (first un-actioned) gated step before appending an OperatorAction
// event and re-kicking the engine walk.
type operatorActionRequest struct {
	StepID     string `json:"step_id"`
	Action     string `json:"action"`
	Verdict    string `json:"verdict"`
	SignerName string `json:"signer_name"`
	SignerRole string `json:"signer_role"`
	Notes      string `json:"notes"`
}

func operatorActionHandler(s *store.Store, eng *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := r.PathValue("id")
		var req operatorActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "Invalid JSON body")
			return
		}
		evs, err := s.LoadEvents(r.Context(), runID)
		if err != nil || len(evs) == 0 {
			writeError(w, http.StatusNotFound, codeNotFound, "Run not found")
			return
		}
		tpl, ok, err := loadTemplateForRun(r.Context(), s, evs)
		if err != nil || !ok {
			writeError(w, http.StatusNotFound, codeNotFound, "Template not found for run")
			return
		}
		gated := gatedSteps(tpl)
		actioned := operatorActionedStepIDs(evs)
		current, ok := firstUnactionedGated(gated, actioned)
		if !ok || current.ID != req.StepID {
			writeError(w, http.StatusUnprocessableEntity, codeInvalidRequest, "step_id is not the run's current gated step")
			return
		}
		switch current.Kind {
		case "review", "ai_assess":
			if req.Action != "advance" {
				writeError(w, http.StatusUnprocessableEntity, codeInvalidRequest, "this step accepts action 'advance'")
				return
			}
		case "signoff":
			if req.Action != "signoff" {
				writeError(w, http.StatusUnprocessableEntity, codeInvalidRequest, "this step accepts action 'signoff'")
				return
			}
			if req.SignerName == "" || req.SignerRole == "" {
				writeError(w, http.StatusUnprocessableEntity, codeInvalidRequest, "signoff requires signer_name and signer_role")
				return
			}
			if !verdictAllowed(current, req.Verdict) {
				writeError(w, http.StatusUnprocessableEntity, codeInvalidRequest, "verdict not in final_verdict_options")
				return
			}
		}
		payload, _ := json.Marshal(engine.OperatorActionPayload{
			StepID: req.StepID, Action: req.Action, Verdict: req.Verdict,
			SignerName: req.SignerName, SignerRole: req.SignerRole, Notes: req.Notes,
		})
		if err := s.AppendEvent(r.Context(), runID, engine.KindOperatorAction, payload); err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to record action")
			return
		}
		eng.Kick(r.Context(), runID)
		runs, _ := s.ListRuns(r.Context())
		for i := range runs {
			if runs[i].ID == runID {
				writeJSON(w, http.StatusOK, map[string]any{"run": runs[i]})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"run": map[string]any{"id": runID}})
	}
}

// cancelRunHandler handles POST /api/runs/{id}/cancel. It cancels a
// pending/running run: appends a terminal RunCancelled event (flipping the
// run to 'cancelled') and stops the in-flight engine walk. A run that is
// already terminal (passed/failed/warning/signed_off/cancelled) returns 422.
func cancelRunHandler(s *store.Store, eng *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := r.PathValue("id")
		cur, ok, err := findRun(r.Context(), s, runID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to load run")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, codeNotFound, "Run not found")
			return
		}
		if cur.Status != verdict.StatusPending && cur.Status != verdict.StatusRunning {
			writeError(w, http.StatusUnprocessableEntity, codeInvalidRequest,
				"run is not cancellable (status "+cur.Status+")")
			return
		}
		payload, _ := json.Marshal(engine.RunCancelledPayload{CancelledAt: time.Now().Unix()})
		if err := s.AppendEvent(r.Context(), runID, engine.KindRunCancelled, payload); err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to record cancellation")
			return
		}
		// Stop the in-flight walk, if any. No-op when the run has no active
		// runner (e.g. after a process restart) — the event already flipped
		// the status, which is what the operator sees.
		eng.Cancel(runID)
		if updated, ok, _ := findRun(r.Context(), s, runID); ok {
			writeJSON(w, http.StatusOK, map[string]any{"run": updated})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"run": map[string]any{"id": runID, "status": verdict.StatusCancelled}})
	}
}

// findRun returns the projected run row by id.
func findRun(ctx context.Context, s *store.Store, runID string) (store.Run, bool, error) {
	runs, err := s.ListRuns(ctx)
	if err != nil {
		return store.Run{}, false, err
	}
	for i := range runs {
		if runs[i].ID == runID {
			return runs[i], true, nil
		}
	}
	return store.Run{}, false, nil
}

// loadTemplateForRun mirrors (*engine.Engine).loadTemplateFromEvents: it
// requires the first event to be RunCreated, decodes the referenced
// template_id, and unmarshals the stored canonical ParsedJSON into a
// template.Template. The stored ParsedJSON IS the canonical template, so we
// json.Unmarshal it directly rather than re-parsing the YAML.
func loadTemplateForRun(ctx context.Context, s *store.Store, evs []store.Event) (template.Template, bool, error) {
	if len(evs) == 0 || evs[0].Kind != "RunCreated" {
		return template.Template{}, false, nil
	}
	var rc struct {
		TemplateID string `json:"template_id"`
	}
	if err := json.Unmarshal(evs[0].Payload, &rc); err != nil {
		return template.Template{}, false, err
	}
	row, ok, err := s.GetTemplate(ctx, rc.TemplateID)
	if err != nil {
		return template.Template{}, false, err
	}
	if !ok {
		return template.Template{}, false, nil
	}
	var tpl template.Template
	if err := json.Unmarshal(row.ParsedJSON, &tpl); err != nil {
		return template.Template{}, false, err
	}
	return tpl, true, nil
}

// gatedSteps returns the template's operator-gated steps (kinds review,
// ai_assess, signoff) in template order.
func gatedSteps(tpl template.Template) []template.Step {
	var out []template.Step
	for _, st := range tpl.Steps {
		switch st.Kind {
		case "review", "ai_assess", "signoff":
			out = append(out, st)
		}
	}
	return out
}

// operatorActionedStepIDs returns the set of step ids that already carry an
// OperatorAction event.
func operatorActionedStepIDs(evs []store.Event) map[string]bool {
	set := map[string]bool{}
	for _, ev := range evs {
		if ev.Kind != engine.KindOperatorAction {
			continue
		}
		var p struct {
			StepID string `json:"step_id"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			continue
		}
		if p.StepID != "" {
			set[p.StepID] = true
		}
	}
	return set
}

// firstUnactionedGated returns the first gated step (template order) that has
// not yet been actioned.
func firstUnactionedGated(gated []template.Step, actioned map[string]bool) (template.Step, bool) {
	for _, st := range gated {
		if !actioned[st.ID] {
			return st, true
		}
	}
	return template.Step{}, false
}

// verdictAllowed reports whether v is one of the step's final_verdict_options.
func verdictAllowed(step template.Step, v string) bool {
	for _, opt := range step.FinalVerdictOptions {
		if opt == v {
			return true
		}
	}
	return false
}
