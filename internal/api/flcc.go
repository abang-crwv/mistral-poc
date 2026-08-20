package api

import (
	"errors"
	"net/http"

	"qac/internal/flccclient"
)

func getFLCCWorkflowHandler(c flccclient.Client, isLive bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		g, src, err := c.GetWorkflow(r.Context(), name)
		if err != nil {
			if errors.Is(err, flccclient.ErrWorkflowNotFound) {
				writeError(w, http.StatusNotFound, codeNotFound, "no flcc workflow named "+name)
				return
			}
			writeError(w, http.StatusInternalServerError, codeInternal, err.Error())
			return
		}
		if !isLive {
			w.Header().Set(degradedHeader, "true")
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"workflow": g,
			"source":   src,
		})
	}
}
