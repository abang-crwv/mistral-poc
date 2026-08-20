package api

import (
	"encoding/json"
	"net/http"

	"qac/internal/store"
)

func listTemplatesHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := s.ListTemplates(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to list templates")
			return
		}
		if out == nil {
			out = []store.TemplateSummary{} // encode [] not null
		}
		writeJSON(w, http.StatusOK, map[string]any{"templates": out})
	}
}

func getTemplateHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "Template id is required")
			return
		}
		row, ok, err := s.GetTemplate(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to load template")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, codeNotFound, "Template not found")
			return
		}
		// Re-decode parsed_json so the wire shape is the canonical template
		// object, not a {id, version, parsed} wrapper.
		var parsed json.RawMessage = row.ParsedJSON
		writeJSON(w, http.StatusOK, map[string]any{"template": parsed})
	}
}
