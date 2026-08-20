package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"qac/internal/store"
)

// evidenceListItem is the list response shape. store.Evidence.Payload is
// json:"-" (served raw by evidenceHandler); the list inlines it as raw JSON.
type evidenceListItem struct {
	ID          string          `json:"id"`
	StepID      string          `json:"step_id"`
	Deviceslot  *string         `json:"deviceslot,omitempty"`
	ContentType string          `json:"content_type"`
	CreatedAt   int64           `json:"created_at"`
	Payload     json.RawMessage `json:"payload"`
}

// evidenceListHandler serves GET /api/runs/{id}/evidence (optional ?step=).
func evidenceListHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := r.PathValue("id")
		if runID == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "Run id is required")
			return
		}
		rows, err := s.ListEvidence(r.Context(), runID, r.URL.Query().Get("step"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to list evidence")
			return
		}
		items := make([]evidenceListItem, 0, len(rows))
		for _, e := range rows {
			items = append(items, evidenceListItem{
				ID:          e.ID,
				StepID:      e.StepID,
				Deviceslot:  e.Deviceslot,
				ContentType: e.ContentType,
				CreatedAt:   e.CreatedAt,
				Payload:     json.RawMessage(e.Payload),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"evidence": items})
	}
}

// evidenceHandler serves GET /api/runs/{id}/evidence/{eid}.
//
// 200 on a match between path id and evidence.RunID; 404 if eid unknown;
// 403 if eid exists but belongs to a different run. Content-Type is
// taken from the evidence row.
func evidenceHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := r.PathValue("id")
		evID := r.PathValue("eid")
		if runID == "" || evID == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "Run id and evidence id are required")
			return
		}
		ev, err := s.GetEvidence(r.Context(), evID)
		if err != nil {
			if errors.Is(err, store.ErrEvidenceNotFound) {
				writeError(w, http.StatusNotFound, codeNotFound, "Evidence not found")
				return
			}
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to load evidence")
			return
		}
		if ev.RunID != runID {
			writeError(w, http.StatusForbidden, codeForbidden, "Evidence does not belong to this run")
			return
		}
		w.Header().Set("Content-Type", ev.ContentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(ev.Payload)
	}
}
