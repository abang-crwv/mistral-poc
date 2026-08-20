// Package rlccactionprobe implements rlcc_action_probe — a gatherer that
// walks one RLCC workflow action per compute tray (CT). It reads the pinned
// workflow snapshot from StepContext, picks the action matching a configured
// handler, polls per-CT FLCC state on a ticker, classifies each observation
// into success/failed/ignorable/in_progress, drops terminal CTs from the pool,
// re-detects RLCC ignores each tick, writes one evidence row per terminal CT,
// and returns one aggregate Output with per-bucket counts and a computed verdict.
//
// Config keys (from the template step's action.args):
//   - match_handler    (required): handler kind to match in workflow.actions[]
//   - target_state     (required): the diagnostic in-progress FLCC state (e.g. "l11-fielddiag")
//   - poll_interval    (optional, default 60s): time.ParseDuration string
//   - evaluation_unit  (optional, default "tray"): "tray" | "nvlink-domain" | "backend-fabric"
//
// iter-5g rewires per-tick poll from QueryActionState to QueryFLCCState +
// classifyFLCC, records raw FLCC observation per CT as evidence, drops terminal
// CTs (failed/ignorable/success) from the pool, and reports per-bucket counts.
// iter-5h computes an all-must-pass verdict (passed/warning/failed) and sets
// Output.Verdict.
package rlccactionprobe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/oklog/ulid/v2"

	"qac/internal/inventoryclient"
	"qac/internal/lifecycleclient"
	"qac/internal/probe"
	"qac/internal/rlccclient"
	"qac/internal/store"
	"qac/internal/verdict"
)

// Probe satisfies probe.Probe.
type Probe struct{}

// New returns a zero-value Probe.
func New() *Probe { return &Probe{} }

var _ probe.Probe = (*Probe)(nil)

func (*Probe) Type() string             { return "rlcc_action_probe" }
func (*Probe) Category() probe.Category { return probe.CategoryGatherer }

type config struct {
	matchHandler   string
	targetState    string
	pollInterval   time.Duration
	evaluationUnit string // "tray" (default) | "nvlink-domain" | "backend-fabric"
}

func parseConfig(c map[string]any) (config, error) {
	cfg := config{pollInterval: 60 * time.Second}
	mh, _ := c["match_handler"].(string)
	if mh == "" {
		return config{}, fmt.Errorf("rlcc_action_probe: missing required config: match_handler")
	}
	ts, _ := c["target_state"].(string)
	if ts == "" {
		return config{}, fmt.Errorf("rlcc_action_probe: missing required config: target_state")
	}
	cfg.matchHandler = mh
	cfg.targetState = ts
	if pi, ok := c["poll_interval"].(string); ok && pi != "" {
		d, err := time.ParseDuration(pi)
		if err != nil {
			return config{}, fmt.Errorf("rlcc_action_probe: invalid poll_interval %q: %w", pi, err)
		}
		if d <= 0 {
			return config{}, fmt.Errorf("rlcc_action_probe: poll_interval must be positive, got %q", pi)
		}
		cfg.pollInterval = d
	}
	cfg.evaluationUnit = "tray"
	if eu, ok := c["evaluation_unit"].(string); ok && eu != "" {
		cfg.evaluationUnit = eu
	}
	return cfg, nil
}

func findAction(wf *rlccclient.WorkflowSnapshot, handler string) (rlccclient.Action, bool) {
	for _, a := range wf.Actions {
		if string(a.Handler) == handler {
			return a, true
		}
	}
	return rlccclient.Action{}, false
}

// Run walks the matched RLCC action across the CT pool. It validates
// prerequisites (workflow snapshot, config, wired clients), picks the action
// by handler, builds the active CT pool (resolved BMNs minus operator- and
// RLCC-ignored), then polls per-CT FLCC state on a ticker: each tick it
// classifies each observation into success/failed/ignorable/in_progress,
// writes evidence for terminal CTs and drops them from the pool,
// re-detects RLCC ignores (emitting BMNIgnored(source=rlcc) and dropping
// them), and stops when the pool empties or the step context is cancelled.
// It returns one aggregate Output with per-bucket counts; per-CT outcomes
// are the per-CT evidence rows. A nil workflow snapshot, missing config, or
// unmatched handler returns an error (StepFailed); ctx timeout returns a
// partial Output with timed_out=true, not an error.
func (p *Probe) Run(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
	if sc.RLCCWorkflow == nil {
		return probe.Output{}, fmt.Errorf("rlcc_action_probe: no workflow snapshot on run (legacy or non-iter-5d run)")
	}
	if c.LifecycleClient == nil || c.EvidenceWriter == nil || c.EventEmitter == nil || c.InventoryResolver == nil {
		return probe.Output{}, fmt.Errorf("rlcc_action_probe: required clients not wired (lifecycle/evidence/emitter/inventory)")
	}
	cfg, err := parseConfig(sc.Config)
	if err != nil {
		return probe.Output{}, err
	}
	action, ok := findAction(sc.RLCCWorkflow, cfg.matchHandler)
	if !ok {
		return probe.Output{}, fmt.Errorf("rlcc_action_probe: no matching action: handler=%q", cfg.matchHandler)
	}

	source := lifecycleSource(c.LifecycleClient)

	pool, rackKeys, err := buildPool(ctx, c, sc)
	if err != nil {
		return probe.Output{}, err
	}

	active := make(map[string]ctRef, len(pool))
	for _, ct := range pool {
		active[ct.deviceslot] = ct
	}
	initialActive := len(active)
	counts := map[ctOutcome]int{}      // outcome -> count
	ignoredMidRun := map[string]bool{} // deviceslot -> dropped mid-run
	tick := 0

	if initialActive == 0 {
		// The remaining active set is the in-progress set at every exit.
		counts[outcomeInProgress] = len(active)
		return summarize(action, cfg, initialActive, counts, ignoredMidRun, tick, false, source), nil
	}

	ticker := time.NewTicker(cfg.pollInterval)
	defer ticker.Stop()
	consecutiveFailures := 0

	for len(active) > 0 {
		select {
		case <-ctx.Done():
			counts[outcomeInProgress] = len(active)
			return summarize(action, cfg, initialActive, counts, ignoredMidRun, tick, true, source), nil
		case <-ticker.C:
			tick++

			// (a) Poll per-CT FLCC state.
			obs, qerr := c.LifecycleClient.QueryFLCCState(ctx, deviceslotsOf(active))
			if qerr != nil {
				consecutiveFailures++
				if consecutiveFailures >= 3 {
					return probe.Output{}, fmt.Errorf("rlcc_action_probe: QueryFLCCState failed 3x: %w", qerr)
				}
				continue
			}
			consecutiveFailures = 0

			// (b) Classify; terminal CTs get evidence + leave the pool.
			for ds, ob := range obs {
				ct, ok := active[ds]
				if !ok {
					continue
				}
				outcome := classifyFLCC(ob, cfg.targetState)
				if outcome == outcomeInProgress {
					continue
				}
				writeCTEvidence(ctx, c, sc, cfg, ct, ob, outcome, tick)
				counts[outcome]++
				delete(active, ds)
			}

			// (c) Re-detect RLCC ignores; emit + drop newly-ignored CTs.
			for _, rk := range rackKeys {
				ig, rerr := c.LifecycleClient.QueryRLCCIgnored(ctx, rk)
				if rerr != nil {
					continue // best-effort
				}
				for _, b := range ig {
					ct, ok := active[b.Deviceslot]
					if !ok {
						continue // already completed, already dropped, or not in pool
					}
					if eerr := c.EventEmitter.EmitBMNIgnored(ctx, sc.RunID, probe.BMNIgnoredPayload{
						Rack:       ct.rack,
						Deviceslot: ct.deviceslot,
						Source:     "rlcc",
						MarkedAt:   time.Now().Unix(),
					}); eerr != nil {
						continue // keep in active; retry next tick
					}
					ignoredMidRun[b.Deviceslot] = true
					delete(active, b.Deviceslot)
				}
			}
		}
	}
	counts[outcomeInProgress] = len(active) // 0 here (loop exits on empty pool), but kept uniform
	return summarize(action, cfg, initialActive, counts, ignoredMidRun, tick, false, source), nil
}

func deviceslotsOf(active map[string]ctRef) []string {
	out := make([]string, 0, len(active))
	for ds := range active {
		out = append(out, ds)
	}
	sort.Strings(out) // deterministic query order
	return out
}

func writeCTEvidence(ctx context.Context, c probe.Clients, sc probe.StepContext, cfg config, ct ctRef, ob lifecycleclient.FLCCObservation, outcome ctOutcome, tick int) {
	body, _ := json.Marshal(map[string]any{
		"deviceslot":       ct.deviceslot,
		"rack":             ct.rack,
		"ct_position":      ct.ctPosition,
		"evaluation_unit":  cfg.evaluationUnit,
		"outcome":          string(outcome),
		"flcc_state":       ob.State,
		"flcc_step":        ob.Step,
		"flcc_prev_state":  ob.PrevState,
		"flcc_prev_step":   ob.PrevStep,
		"flcc_workflow":    ob.Workflow,
		"observed_at_tick": tick,
	})
	ds := ct.deviceslot
	// Best-effort: per-CT evidence is supplementary; the aggregate Output is
	// canonical, so a write error here is not fatal to the step.
	_ = c.EvidenceWriter.PutEvidence(ctx, store.Evidence{
		ID:          ulid.Make().String(),
		RunID:       sc.RunID,
		StepID:      sc.StepID,
		ContentType: "application/json",
		Payload:     body,
		Deviceslot:  &ds,
	})
}

func summarize(action rlccclient.Action, cfg config, initialActive int, counts map[ctOutcome]int, ignoredMidRun map[string]bool, ticks int, timedOut bool, source string) probe.Output {
	v := verdict.Passed
	switch {
	case counts[outcomeFailed] > 0:
		v = verdict.Failed
	case counts[outcomeInProgress] > 0 || timedOut:
		v = verdict.Warning
	}

	probedAt := time.Now().Unix()
	body, _ := json.Marshal(map[string]any{
		"action_name":       action.Name,
		"handler":           string(action.Handler),
		"evaluation_unit":   cfg.evaluationUnit,
		"count_active":      initialActive,
		"count_success":     counts[outcomeSuccess],
		"count_failed":      counts[outcomeFailed],
		"count_ignorable":   counts[outcomeIgnorable] + len(ignoredMidRun),
		"count_in_progress": counts[outcomeInProgress],
		"duration_ticks":    ticks,
		"timed_out":         timedOut,
		"probed_at":         probedAt,
	})
	return probe.Output{
		StructuredData: body,
		Sources:        []string{source},
		ProbedAt:       probedAt,
		Verdict:        v,
	}
}

// lifecycleSource labels evidence provenance. Best-effort backend detection:
// the in-memory MapClient is the fixture backend; anything else (PromClient
// in production, test fakes) reports "lifecycleclient".
func lifecycleSource(lc probe.LifecycleClient) string {
	if _, ok := lc.(*lifecycleclient.MapClient); ok {
		return "lifecycleclient-fixture"
	}
	return "lifecycleclient"
}

// ctRef is one compute tray in the active pool.
type ctRef struct {
	deviceslot string
	rack       string
	zone       string
	ctPosition int
}

// buildPool resolves every rack's CTs, subtracts operator-ignored deviceslots
// (from StepContext.IgnoredDeviceslots) and initially RLCC-ignored deviceslots
// (an initial QueryRLCCIgnored per rack), and returns the active pool plus the
// per-rack keys used for per-tick RLCC re-detection. A rack that resolves to
// zero CTs (or ErrNotFound) contributes nothing.
func buildPool(ctx context.Context, c probe.Clients, sc probe.StepContext) ([]ctRef, []lifecycleclient.RackKey, error) {
	operatorIgnored := map[string]bool{}
	for _, ds := range sc.IgnoredDeviceslots {
		operatorIgnored[ds] = true
	}

	var pool []ctRef
	var rackKeys []lifecycleclient.RackKey
	seenRK := map[lifecycleclient.RackKey]bool{}

	for _, rack := range sc.Racks {
		resolved, err := c.InventoryResolver.ResolveBMNs(ctx, rack)
		if err != nil {
			if errors.Is(err, inventoryclient.ErrNotFound) {
				continue
			}
			return nil, nil, fmt.Errorf("rlcc_action_probe: resolve bmns %s: %w", rack, err)
		}
		if len(resolved) == 0 {
			continue
		}

		rk := lifecycleclient.RackKey{Rack: rack, Zone: resolved[0].Zone}
		if !seenRK[rk] {
			seenRK[rk] = true
			rackKeys = append(rackKeys, rk)
		}

		// Initial RLCC ignores for this rack are excluded from the pool
		// (they were never going to be walked). Best-effort: a query error
		// here means we start with no known RLCC ignores and re-detect live.
		rlccIgnored := map[string]bool{}
		if ig, qerr := c.LifecycleClient.QueryRLCCIgnored(ctx, rk); qerr == nil {
			for _, b := range ig {
				rlccIgnored[b.Deviceslot] = true
			}
		}

		for _, b := range resolved {
			if operatorIgnored[b.Deviceslot] || rlccIgnored[b.Deviceslot] {
				continue
			}
			pool = append(pool, ctRef{
				deviceslot: b.Deviceslot,
				rack:       b.Rack,
				zone:       b.Zone,
				ctPosition: b.CTPosition,
			})
		}
	}
	return pool, rackKeys, nil
}
