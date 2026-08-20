package api

import (
	"net/http"

	"qac/internal/engine"
	"qac/internal/flccclient"
	"qac/internal/inventoryclient"
	"qac/internal/lifecycleclient"
	"qac/internal/rlccclient"
	"qac/internal/store"
)

// Router builds the HTTP handler tree for the /api/* surface.
//
// rlccC + flccC drive the new /api/rlcc/* and /api/flcc/* routes.
// purgeSourcegraph is invoked by POST /api/admin/sourcegraph/refresh —
// callers wire it to the underlying sourcegraph.Client cache's Purge()
// method, or to a no-op when running with MapClient.
// liveBackend == true means SourcegraphClient is in use (suppresses the
// X-Qac-Degraded header).
// lifecycleC backs the per-CT RLCC ignore detection in createRunHandler
// step 4e; iter-5b wires MapClient, Task 14 adds env-var-based selection.
func Router(
	s *store.Store,
	dbPath string,
	resolver inventoryclient.Resolver,
	eng *engine.Engine,
	rlccC rlccclient.Client,
	flccC flccclient.Client,
	liveBackend bool,
	purgeSourcegraph func(),
	lifecycleC lifecycleclient.Client,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", healthHandler(dbPath))

	mux.HandleFunc("GET /api/runs", listRunsHandler(s))
	mux.HandleFunc("POST /api/runs", createRunHandler(s, resolver, eng, lifecycleC, rlccC))
	mux.HandleFunc("GET /api/runs/{id}", getRunHandler(s))
	mux.HandleFunc("POST /api/runs/{id}/actions", operatorActionHandler(s, eng))
	mux.HandleFunc("POST /api/runs/{id}/cancel", cancelRunHandler(s, eng))
	mux.HandleFunc("GET /api/runs/{id}/facts", factsHandler(s))
	mux.HandleFunc("GET /api/runs/{id}/evidence", evidenceListHandler(s))
	mux.HandleFunc("GET /api/runs/{id}/evidence/{eid}", evidenceHandler(s))

	mux.HandleFunc("GET /api/templates", listTemplatesHandler(s))
	mux.HandleFunc("GET /api/templates/{id}", getTemplateHandler(s))

	mux.HandleFunc("GET /api/probes", listProbesHandler(eng))
	mux.HandleFunc("GET /api/agents", listAgentsHandler(eng))

	mux.HandleFunc("GET /api/rlcc/workflows", listRLCCWorkflowsHandler(rlccC, liveBackend))
	mux.HandleFunc("GET /api/rlcc/workflows/{name}", getRLCCWorkflowHandler(rlccC, liveBackend))
	mux.HandleFunc("GET /api/flcc/workflows/{name}", getFLCCWorkflowHandler(flccC, liveBackend))

	mux.HandleFunc("POST /api/admin/sourcegraph/refresh", refreshSourcegraphHandler(purgeSourcegraph))

	mux.HandleFunc("GET /api/inventory/preview", inventoryPreviewHandler(resolver, lifecycleC))

	return mux
}
