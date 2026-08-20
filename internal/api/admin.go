package api

import "net/http"

// refreshSourcegraphHandler invalidates the shared sourcegraph cache so the
// next read of either chart hits the live API. The purge callback is wired
// from cmd/qac/serve.go — Router doesn't need to know how the underlying
// client is built.
func refreshSourcegraphHandler(purge func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		purge()
		writeJSON(w, http.StatusOK, map[string]any{
			"invalidated": []string{"rlcc", "flcc"},
		})
	}
}
