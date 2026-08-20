package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"qac/internal/engine"
	"qac/internal/facts"
	"qac/internal/inventoryclient"
	"qac/internal/lifecycleclient"
	"qac/internal/rlccclient"
	"qac/internal/store"
	"qac/internal/template"
)

func listRunsHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runs, err := s.ListRuns(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to list runs")
			return
		}
		if runs == nil {
			runs = []store.Run{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
	}
}

func getRunHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "Run id is required")
			return
		}
		events, err := s.LoadEvents(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to load events")
			return
		}
		if len(events) == 0 {
			writeError(w, http.StatusNotFound, codeNotFound, "Run not found")
			return
		}
		runs, err := s.ListRuns(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to load run")
			return
		}
		var run *store.Run
		for i := range runs {
			if runs[i].ID == id {
				run = &runs[i]
				break
			}
		}
		if run == nil {
			writeError(w, http.StatusNotFound, codeNotFound, "Run not found")
			return
		}
		racks, err := getRacksRollup(r.Context(), s, id)
		if err != nil {
			// Don't fail the whole GET on rollup error — log and return empty.
			slog.Warn("racks rollup failed", "run_id", id, "err", err)
			racks = []rackRollup{}
		}
		if racks == nil {
			racks = []rackRollup{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"run": run, "events": events, "racks": racks})
	}
}

// createRunRequest accepts both shapes and normalizes inside the handler.
type createRunRequest struct {
	// Legacy iter-2 shape:
	Bundle string `json:"bundle"`
	Rack   string `json:"rack"`
	// New iter-3a shape:
	TemplateID string         `json:"template_id"`
	Inputs     map[string]any `json:"inputs"`
	// iter-5b: operator-supplied ignore list (validated in Task 11 handler wiring).
	IgnoredBMNs []ignoredBMNReq `json:"ignored_bmns,omitempty"`
}

// ignoredBMNReq is one operator-supplied ignore entry. Rack must match
// one of the canary_racks; Deviceslot must exist in the BMN enumeration
// for that rack (validated after ResolveBMNs runs in step 4a).
type ignoredBMNReq struct {
	Rack       string `json:"rack"`
	Deviceslot string `json:"deviceslot"`
	Reason     string `json:"reason,omitempty"`
}

func createRunHandler(s *store.Store, resolver inventoryclient.Resolver, eng *engine.Engine, lifeC lifecycleclient.Client, rlccC rlccclient.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "Invalid JSON body")
			return
		}

		legacyPresent := req.Bundle != "" || req.Rack != ""
		newPresent := req.TemplateID != "" || req.Inputs != nil
		if legacyPresent && newPresent {
			writeError(w, http.StatusBadRequest, codeInvalidRequest,
				"Provide either {bundle, rack} or {template_id, inputs}, not both")
			return
		}

		templateID, inputs, errMsg := normalize(req)
		if errMsg != "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, errMsg)
			return
		}

		row, ok, err := s.GetTemplate(r.Context(), templateID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to load template")
			return
		}
		if !ok {
			writeError(w, http.StatusUnprocessableEntity, codeTemplateNotFound,
				"unknown template_id: "+templateID)
			return
		}
		var tpl template.Template
		if err := json.Unmarshal(row.ParsedJSON, &tpl); err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Stored template is corrupt")
			return
		}

		if msg := ValidateInputs(tpl, inputs); msg != "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, msg)
			return
		}

		// === iter-5d: resolve workflow snapshot before any events emit. ===
		//
		// All-or-nothing posture (mirrors iter-4a inventory + iter-5b BMN
		// resolution): if the workflow can't be resolved, return 4xx with
		// zero event-log pollution. ValidateInputs already enforces presence
		// after the iter-5d template flip; we only reach here with a non-empty
		// name. (Pre-flip, the field is optional — see the `if wfName != ""`
		// guard below.)
		var snapshot *rlccclient.WorkflowSnapshot
		if wfNameRaw, ok := inputs["rlcc_workflow"]; ok {
			wfName, _ := wfNameRaw.(string)
			if wfName != "" {
				graph, src, err := rlccC.GetWorkflow(r.Context(), wfName)
				switch {
				case errors.Is(err, rlccclient.ErrWorkflowNotFound):
					writeError(w, http.StatusBadRequest, codeRLCCWorkflowUnknown,
						"RLCC workflow not found: "+wfName)
					return
				case err != nil:
					slog.Error("rlcc workflow resolve failed", "name", wfName, "err", err)
					writeError(w, http.StatusInternalServerError, codeInternal,
						"Failed to resolve RLCC workflow")
					return
				case len(graph.Actions) == 0:
					writeError(w, http.StatusBadRequest, codeRLCCWorkflowUnknown,
						"RLCC workflow has no actions to walk: "+wfName)
					return
				}
				snapshot = &rlccclient.WorkflowSnapshot{
					Name:      graph.Name,
					SourceSHA: src.SHA,
					Actions:   graph.Actions,
				}
			}
		}

		// === iter-4a discovery pipeline ===

		runID := ulid.Make().String()
		inputsJSON, _ := json.Marshal(inputs)
		payloadMap := map[string]any{
			"template_id":      templateID,
			"template_version": tpl.Version,
			"inputs":           json.RawMessage(inputsJSON),
			"created_by":       "wpena",
		}
		if snapshot != nil {
			payloadMap["rlcc_workflow"] = snapshot
		}
		payload, _ := json.Marshal(payloadMap)
		if err := s.AppendEvent(r.Context(), runID, "RunCreated", payload); err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to create run")
			return
		}

		// 1. Promote operator inputs to facts (run scope, source=operator).
		opBatch := operatorFactsFromInputs(inputs)
		if err := facts.Emit(r.Context(), s, runID, "run", "operator", opBatch); err != nil {
			recordDiscoveryFailure(r.Context(), s, runID, "Failed to emit operator facts", nil)
			writeError(w, http.StatusInternalServerError, codeInternal, "Failed to emit operator facts")
			return
		}

		// 2. Discover per-rack inventory facts. Collect-then-emit so the
		//    semantics are all-or-nothing.
		racks := extractRacks(inputs)
		type rackResult struct {
			rack  string
			facts inventoryclient.RackFacts
		}
		var resolved []rackResult
		var unresolved []string
		for _, rack := range racks {
			rf, err := inventoryclient.ResolveRack(r.Context(), resolver, rack)
			if err != nil {
				if errors.Is(err, inventoryclient.ErrNotFound) {
					unresolved = append(unresolved, rack)
					continue
				}
				var amb *inventoryclient.AmbiguousError
				if errors.As(err, &amb) {
					unresolved = append(unresolved, rack)
					continue
				}
				writeError(w, http.StatusInternalServerError, codeInternal, "Inventory lookup failed")
				return
			}
			resolved = append(resolved, rackResult{rack: rack, facts: rf})
		}

		// 3. If any unresolved → emit FactsDiscoveryFailed (status='failed') + 422.
		if len(unresolved) > 0 {
			failPayload, _ := json.Marshal(map[string]any{
				"scope":      "run",
				"source":     "inventory",
				"error":      "Racks could not be resolved",
				"unresolved": unresolved,
			})
			if err := s.AppendEvent(r.Context(), runID, "FactsDiscoveryFailed", failPayload); err != nil {
				writeError(w, http.StatusInternalServerError, codeInternal, "Failed to record discovery failure")
				return
			}
			writeErrorWithDetails(w, http.StatusUnprocessableEntity, codeInventoryUnresolved,
				"Racks could not be resolved",
				map[string]any{"unresolved": unresolved})
			return
		}

		// 4. All resolved → emit per-rack FactsDiscovered.
		for _, rr := range resolved {
			scope := "rack:" + rr.rack
			batch := rr.facts.ToFactBatch()
			if err := facts.Emit(r.Context(), s, runID, scope, "inventory", batch); err != nil {
				recordDiscoveryFailure(r.Context(), s, runID, "Failed to emit inventory facts", nil)
				writeError(w, http.StatusInternalServerError, codeInternal, "Failed to emit inventory facts")
				return
			}
		}

		// 4b. Enumerate BMNs per rack. All-or-nothing: gather first, fail
		//     fast on any unresolved rack, then validate operator ignores
		//     against the gathered set before any BMN events are written.
		type bmnFanout struct {
			rack string
			zone string
			bmns []inventoryclient.BMN
		}
		var bmnFanouts []bmnFanout
		for _, rr := range resolved {
			bmns, err := resolver.ResolveBMNs(r.Context(), rr.rack)
			if err != nil {
				failPayload, _ := json.Marshal(map[string]any{
					"rack":    rr.rack,
					"source":  "inventory",
					"message": err.Error(),
				})
				_ = s.AppendEvent(r.Context(), runID, "BMNDiscoveryFailed", failPayload)
				writeError(w, http.StatusUnprocessableEntity, codeBMNsUnresolved,
					"BMN enumeration failed for rack "+rr.rack)
				return
			}
			zone := ""
			if len(bmns) > 0 {
				zone = bmns[0].Zone
			}
			bmnFanouts = append(bmnFanouts, bmnFanout{rack: rr.rack, zone: zone, bmns: bmns})
		}

		// 4c. Validate operator-supplied ignored_bmns against enumerated set.
		//     This must complete before any BMNDiscovered events are written
		//     so a validation failure produces zero event-log pollution.
		canaryRackSet := map[string]bool{}
		bmnIndex := map[string]map[string]bool{} // rack -> deviceslot set
		for _, fo := range bmnFanouts {
			canaryRackSet[fo.rack] = true
			bmnIndex[fo.rack] = map[string]bool{}
			for _, b := range fo.bmns {
				bmnIndex[fo.rack][b.Deviceslot] = true
			}
		}
		seen := map[string]bool{} // rack|deviceslot composite key
		for _, ig := range req.IgnoredBMNs {
			if !canaryRackSet[ig.Rack] {
				writeError(w, http.StatusBadRequest, codeInvalidRequest,
					"ignored_bmns: rack "+ig.Rack+" is not in canary_racks")
				return
			}
			if !bmnIndex[ig.Rack][ig.Deviceslot] {
				writeError(w, http.StatusBadRequest, codeInvalidRequest,
					"ignored_bmns: deviceslot "+ig.Deviceslot+" not found in rack "+ig.Rack)
				return
			}
			key := ig.Rack + "|" + ig.Deviceslot
			if seen[key] {
				writeError(w, http.StatusBadRequest, codeInvalidRequest,
					"ignored_bmns: duplicate entry for deviceslot "+ig.Deviceslot)
				return
			}
			seen[key] = true
		}

		// 4d. Emit BMNDiscovered per rack.
		now := time.Now().Unix()
		for _, fo := range bmnFanouts {
			entries := make([]map[string]any, 0, len(fo.bmns))
			for _, b := range fo.bmns {
				entries = append(entries, map[string]any{
					"deviceslot":  b.Deviceslot,
					"bmn_name":    b.BMNName,
					"ct_position": b.CTPosition,
				})
			}
			discPayload, _ := json.Marshal(map[string]any{
				"rack":          fo.rack,
				"zone":          fo.zone,
				"source":        "inventory",
				"discovered_at": now,
				"bmns":          entries,
			})
			if err := s.AppendEvent(r.Context(), runID, "BMNDiscovered", discPayload); err != nil {
				writeError(w, http.StatusInternalServerError, codeInternal, "Failed to emit BMNDiscovered")
				return
			}
		}

		// 4e. Best-effort: query RLCC ignores per rack; emit BMNIgnored(source=rlcc).
		//     Log and continue on failure — the iter-5e probe re-detects mid-run.
		for _, fo := range bmnFanouts {
			ignored, qerr := lifeC.QueryRLCCIgnored(r.Context(),
				lifecycleclient.RackKey{Rack: fo.rack, Zone: fo.zone})
			if qerr != nil {
				slog.Warn("rlcc ignore detect failed", "rack", fo.rack, "err", qerr)
				continue
			}
			for _, ib := range ignored {
				ignPayload, _ := json.Marshal(map[string]any{
					"rack":       fo.rack,
					"deviceslot": ib.Deviceslot,
					"source":     "rlcc",
					"marked_at":  now,
				})
				if err := s.AppendEvent(r.Context(), runID, "BMNIgnored", ignPayload); err != nil {
					slog.Warn("failed to emit BMNIgnored(rlcc)", "rack", fo.rack, "err", err)
				}
			}
		}

		// 4f. Emit BMNIgnored(source=operator) for each validated entry.
		for _, ig := range req.IgnoredBMNs {
			ignPayload, _ := json.Marshal(map[string]any{
				"rack":       ig.Rack,
				"deviceslot": ig.Deviceslot,
				"source":     "operator",
				"reason":     ig.Reason,
				"marked_at":  now,
			})
			if err := s.AppendEvent(r.Context(), runID, "BMNIgnored", ignPayload); err != nil {
				writeError(w, http.StatusInternalServerError, codeInternal, "Failed to emit BMNIgnored")
				return
			}
		}

		// 5. Engine drives the probe loop forward asynchronously. Kick is
		//    fire-and-forget and idempotent; the goroutine reads from the
		//    same store the handler just committed to.
		if eng != nil {
			eng.Kick(r.Context(), runID)
		}

		runs, err := s.ListRuns(r.Context())
		if err != nil {
			writeJSON(w, http.StatusCreated, map[string]any{
				"run":     map[string]any{"id": runID, "status": "pending"},
				"warning": "projection_unavailable",
			})
			return
		}
		for i := range runs {
			if runs[i].ID == runID {
				writeJSON(w, http.StatusCreated, map[string]any{"run": runs[i]})
				return
			}
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"run":     map[string]any{"id": runID, "status": "pending"},
			"warning": "projection_unavailable",
		})
	}
}

// operatorFactsFromInputs builds the run-scope, source="operator" fact
// batch from the normalized inputs. It includes only keys that are in
// the registered fact-key registry; unregistered keys stay in
// runs.inputs JSON only.
func operatorFactsFromInputs(inputs map[string]any) map[facts.Key]any {
	out := map[facts.Key]any{}
	for k, v := range inputs {
		key := facts.Key(k)
		if !facts.IsRegistered(key) {
			continue
		}
		out[key] = v
	}
	return out
}

// extractRacks reads inputs["canary_racks"] as a []string. The
// ValidateInputs check has already guaranteed the value is a list of
// strings matching the rack regex, so we don't repeat that here.
func extractRacks(inputs map[string]any) []string {
	raw, ok := inputs["canary_racks"]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// recordDiscoveryFailure appends a FactsDiscoveryFailed event so the
// projection flips runs.status to 'failed' instead of leaving the run
// stranded in 'pending'. Best-effort: a failure here is logged via the
// returned error but does not change the outer response (the outer
// caller is already in an error path).
func recordDiscoveryFailure(ctx context.Context, s *store.Store, runID, reason string, unresolved []string) {
	payload, _ := json.Marshal(map[string]any{
		"scope":      "run",
		"source":     "inventory",
		"error":      reason,
		"unresolved": unresolved,
	})
	_ = s.AppendEvent(ctx, runID, "FactsDiscoveryFailed", payload)
}

// rackRollup is the per-rack summary returned by GET /api/runs/{id}.
type rackRollup struct {
	Rack    string             `json:"rack"`
	CTCount int                `json:"ct_count"`
	Ignored []rackRollupIgnore `json:"ignored"`
}

// rackRollupIgnore is one ignored BMN entry within a rackRollup.
type rackRollupIgnore struct {
	Deviceslot string  `json:"deviceslot"`
	Source     string  `json:"source"`
	Reason     *string `json:"reason,omitempty"`
}

// getRacksRollup returns one rackRollup per rack in the run, joining
// rack_deviceslots (for CT count) with bmn_ignores (for the ignore list).
// Empty result is valid — a pre-iter5b run has no rack_deviceslots rows.
func getRacksRollup(ctx context.Context, s *store.Store, runID string) ([]rackRollup, error) {
	db := s.DB()

	// Counts per rack.
	countRows, err := db.QueryContext(ctx,
		`SELECT rack, COUNT(*) FROM rack_deviceslots WHERE run_id = ? GROUP BY rack ORDER BY rack`,
		runID)
	if err != nil {
		return nil, fmt.Errorf("count rack_deviceslots: %w", err)
	}
	defer countRows.Close()

	type rackCount struct {
		rack string
		n    int
	}
	var crs []rackCount
	for countRows.Next() {
		var c rackCount
		if err := countRows.Scan(&c.rack, &c.n); err != nil {
			return nil, fmt.Errorf("scan rack count: %w", err)
		}
		crs = append(crs, c)
	}
	if err := countRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rack counts: %w", err)
	}

	// Ignores per rack.
	igRows, err := db.QueryContext(ctx,
		`SELECT rack, deviceslot, source, reason FROM bmn_ignores WHERE run_id = ? ORDER BY rack, deviceslot, source`,
		runID)
	if err != nil {
		return nil, fmt.Errorf("query bmn_ignores: %w", err)
	}
	defer igRows.Close()

	igByRack := map[string][]rackRollupIgnore{}
	for igRows.Next() {
		var rack, slot, src string
		var reason sql.NullString
		if err := igRows.Scan(&rack, &slot, &src, &reason); err != nil {
			return nil, fmt.Errorf("scan ignore row: %w", err)
		}
		entry := rackRollupIgnore{Deviceslot: slot, Source: src}
		if reason.Valid {
			r := reason.String
			entry.Reason = &r
		}
		igByRack[rack] = append(igByRack[rack], entry)
	}
	if err := igRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ignore rows: %w", err)
	}

	out := make([]rackRollup, 0, len(crs))
	for _, c := range crs {
		ignores := igByRack[c.rack]
		if ignores == nil {
			ignores = []rackRollupIgnore{}
		}
		out = append(out, rackRollup{
			Rack:    c.rack,
			CTCount: c.n,
			Ignored: ignores,
		})
	}
	return out, nil
}

// normalize converts a legacy {bundle, rack} request into the {template_id,
// inputs} shape, validating rack format in the process. For new-shape
// requests it returns them unchanged. Returns a non-empty errMsg on
// validation failure (bad rack, empty bundle, etc.).
func normalize(req createRunRequest) (templateID string, inputs map[string]any, errMsg string) {
	if req.TemplateID != "" || req.Inputs != nil {
		// New shape — pass through. Template checks happen later via
		// ValidateInputs against the looked-up template.
		if req.Inputs == nil {
			req.Inputs = map[string]any{}
		}
		return req.TemplateID, req.Inputs, ""
	}

	// Legacy shape — translate to canonical inputs.
	bundle := strings.TrimSpace(req.Bundle)
	if bundle == "" {
		return "", nil, "Bundle is required"
	}
	if len(bundle) > 200 {
		return "", nil, "Bundle is too long (max 200 characters)"
	}
	canonical, racks, msg := ValidateRacks(req.Rack)
	if msg != "" {
		return "", nil, msg
	}
	_ = canonical // canary_racks below is the canonical-list form
	racksAny := make([]any, len(racks))
	for i, r := range racks {
		racksAny[i] = r
	}
	return "firmware-release-canary", map[string]any{
		"bundle_tag":   bundle,
		"canary_racks": racksAny,
	}, ""
}
