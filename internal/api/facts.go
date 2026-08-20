package api

import (
	"net/http"

	"qac/internal/store"
)

// factsHandler serves GET /api/runs/{id}/facts.
//
// Query params:
//
//	scope  — filter to a specific scope; "rack:*" wildcards any rack
//	source — filter to a specific source (operator | inventory | ...)
//
// Returns 404 not_found if the run id is unknown.
func factsHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "Run id is required")
			return
		}

		// Confirm the run exists. ListRuns is cheap (few runs in iter-4a)
		// and uses the existing iter-3a pattern.
		runs, err := s.ListRuns(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to load runs")
			return
		}
		found := false
		for _, run := range runs {
			if run.ID == id {
				found = true
				break
			}
		}
		if !found {
			writeError(w, http.StatusNotFound, codeNotFound, "Run not found")
			return
		}

		scope := r.URL.Query().Get("scope")
		source := r.URL.Query().Get("source")

		facts, err := s.ListFacts(r.Context(), id, scope, source)
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to load facts")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"facts": facts})
	}
}
