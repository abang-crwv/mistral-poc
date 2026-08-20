// Package server wires the API router, embedded SPA, and middleware
// into an http.Server.
package server

import (
	"embed"
	"net/http"

	"qac/internal/api"
	"qac/internal/engine"
	"qac/internal/flccclient"
	"qac/internal/inventoryclient"
	"qac/internal/lifecycleclient"
	"qac/internal/rlccclient"
	"qac/internal/store"
)

// New returns a configured *http.Server. addr is "host:port".
// resolver backs the inventory-fact discovery in POST /api/runs.
// eng drives runs forward after facts are discovered.
// rlccC + flccC back the /api/rlcc/* and /api/flcc/* routes.
// liveBackend indicates whether a Sourcegraph-backed client is in use.
// purgeSourcegraph is called by POST /api/admin/sourcegraph/refresh.
// lifecycleC backs the per-CT RLCC ignore detection in createRunHandler;
// iter-5b passes MapClient, Task 14 adds env-var-based backend selection.
func New(
	s *store.Store,
	addr, dbPath string,
	distFS embed.FS,
	resolver inventoryclient.Resolver,
	eng *engine.Engine,
	rlccC rlccclient.Client,
	flccC flccclient.Client,
	liveBackend bool,
	purgeSourcegraph func(),
	lifecycleC lifecycleclient.Client,
) (*http.Server, error) {
	apiHandler := api.Router(s, dbPath, resolver, eng, rlccC, flccC, liveBackend, purgeSourcegraph, lifecycleC)
	spa, err := SPAHandler(distFS)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	// API takes /api/* — everything else falls through to the SPA.
	mux.Handle("/api/", apiHandler)
	mux.Handle("/", spa)

	return &http.Server{
		Addr:    addr,
		Handler: requestLogger(mux),
	}, nil
}
