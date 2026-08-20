package api

import (
	"errors"
	"net/http"

	"qac/internal/rlccclient"
)

// degradedHeader marks responses served by a MapClient backend rather than
// live Sourcegraph. The wizard can use it to flag "fixture data" to operators.
const degradedHeader = "X-Qac-Degraded"

func listRLCCWorkflowsHandler(c rlccclient.Client, isLive bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		summaries, src, err := c.ListWorkflows(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, err.Error())
			return
		}
		if !isLive {
			w.Header().Set(degradedHeader, "true")
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"workflows": summaries,
			"source":    src,
		})
	}
}

func getRLCCWorkflowHandler(c rlccclient.Client, isLive bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		g, src, err := c.GetWorkflow(r.Context(), name)
		if err != nil {
			if errors.Is(err, rlccclient.ErrWorkflowNotFound) {
				writeError(w, http.StatusNotFound, codeNotFound, "no rlcc workflow named "+name)
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
