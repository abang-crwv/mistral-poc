// Package alertprobe implements alert_probe — a gatherer that snapshots
// firing/pending alerts for a run's NVLink domain over a lookback window.
// It is category-driven: it iterates the alertcategoryclient registry and
// groups the gathered rows under each category id in its evidence. No verdict
// — pre/post comparison happens across runs, outside the probe.
package alertprobe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"qac/internal/alertcategoryclient"
	"qac/internal/inventoryclient"
	"qac/internal/probe"
)

// AlertProbe gathers alert categories per rack. For each rack it resolves the
// NVLink-domain scope, then runs every registered category's range query and
// records the rows. No verdict (gatherer).
type AlertProbe struct{}

// New returns a zero-value AlertProbe.
func New() *AlertProbe { return &AlertProbe{} }

// Compile-time satisfaction check.
var _ probe.Probe = (*AlertProbe)(nil)

// Type returns the registry key for this probe.
func (*AlertProbe) Type() string { return "alert_probe" }

// Category returns CategoryGatherer — alert_probe produces evidence, not
// verdicts.
func (*AlertProbe) Category() probe.Category { return probe.CategoryGatherer }

// defaultLookbackWindow is the alert-history window when the step config omits
// `lookback`.
const defaultLookbackWindow = 24 * time.Hour

// scopeView is the domain scope echoed into evidence for each resolved rack.
type scopeView struct {
	Region       string `json:"region"`
	Zone         string `json:"zone"`
	Cluster      string `json:"cluster"`
	NVLinkDomain string `json:"nvlink_domain"`
}

// rackCategory is one rack's rows within a category. Error is set (and Rows
// nil) when the rack's domain scope could not be resolved.
type rackCategory struct {
	Scope    scopeView                         `json:"scope"`
	Rows     []alertcategoryclient.CategoryRow `json:"rows"`
	RowCount int                               `json:"row_count"`
	Error    string                            `json:"error,omitempty"`
}

// categoryView is one category across all racks.
type categoryView struct {
	Title   string                  `json:"title"`
	PerRack map[string]rackCategory `json:"per_rack"`
}

// alertEvidence is the structured output: categories → category → per-rack rows.
type alertEvidence struct {
	ProbedAt   int64                   `json:"probed_at"`
	Lookback   string                  `json:"lookback"`
	Categories map[string]categoryView `json:"categories"`
}

// Run satisfies probe.Probe. It resolves each rack's NVLink-domain scope once,
// then gathers every registered category for each resolved rack. A scope
// miss (ErrNotFound / AmbiguousError) is recorded per-rack; transport errors
// and gather errors bubble up to StepFailed.
func (*AlertProbe) Run(ctx context.Context, clients probe.Clients, sc probe.StepContext) (probe.Output, error) {
	if clients.AlertCategoryClient == nil {
		return probe.Output{}, fmt.Errorf("alert_probe: clients.AlertCategoryClient is nil")
	}
	if clients.InventoryResolver == nil {
		return probe.Output{}, fmt.Errorf("alert_probe: clients.InventoryResolver is nil")
	}

	lb, lookbackStr := parseLookback(sc.Config)

	// Resolve each rack's domain scope once. Misses are recorded; transport
	// errors abort the step.
	scopes := make(map[string]inventoryclient.DomainScope, len(sc.Racks))
	scopeErr := make(map[string]string, len(sc.Racks))
	for _, rack := range sc.Racks {
		ds, err := clients.InventoryResolver.ResolveDomainScope(ctx, rack)
		if err != nil {
			if errors.Is(err, inventoryclient.ErrNotFound) {
				scopeErr[rack] = err.Error()
				continue
			}
			var amb *inventoryclient.AmbiguousError
			if errors.As(err, &amb) {
				scopeErr[rack] = amb.Error()
				continue
			}
			return probe.Output{}, fmt.Errorf("alert_probe: resolve domain scope %s: %w", rack, err)
		}
		scopes[rack] = ds
	}

	cats := clients.AlertCategoryClient.Categories()
	evCats := make(map[string]categoryView, len(cats))
	for _, c := range cats {
		cv := categoryView{Title: c.Title, PerRack: make(map[string]rackCategory, len(sc.Racks))}
		for _, rack := range sc.Racks {
			if msg, bad := scopeErr[rack]; bad {
				cv.PerRack[rack] = rackCategory{Error: msg}
				continue
			}
			ds := scopes[rack]
			rows, err := clients.AlertCategoryClient.Gather(ctx, c.ID, toCategoryScope(ds), rack, lb)
			if err != nil {
				return probe.Output{}, fmt.Errorf("alert_probe: gather %s for %s: %w", c.ID, rack, err)
			}
			cv.PerRack[rack] = rackCategory{
				Scope:    scopeView{Region: ds.Region, Zone: ds.Zone, Cluster: ds.Cluster, NVLinkDomain: ds.NVLinkDomain},
				Rows:     rows,
				RowCount: len(rows),
			}
		}
		evCats[c.ID] = cv
	}

	probedAt := time.Now().Unix()
	ev := alertEvidence{ProbedAt: probedAt, Lookback: lookbackStr, Categories: evCats}
	structured, err := json.Marshal(ev)
	if err != nil {
		return probe.Output{}, fmt.Errorf("alert_probe: marshal output: %w", err)
	}
	return probe.Output{
		StructuredData: structured,
		Sources:        []string{"victoriametrics-alerts"},
		ProbedAt:       probedAt,
	}, nil
}

// parseLookback reads the optional `lookback` and `step` durations from the
// step config (action.args). Both are duration strings (e.g. "24h", "60s").
// Defaults: 24h window, zero step (the client applies its own 60s default).
// Returns the lookback and a display string for the evidence.
func parseLookback(cfg map[string]any) (alertcategoryclient.Lookback, string) {
	window := defaultLookbackWindow
	display := "24h"
	var step time.Duration
	if cfg != nil {
		if v, ok := cfg["lookback"].(string); ok {
			if d, err := time.ParseDuration(v); err == nil && d > 0 {
				window = d
				display = v
			}
		}
		if v, ok := cfg["step"].(string); ok {
			if d, err := time.ParseDuration(v); err == nil && d > 0 {
				step = d
			}
		}
	}
	return alertcategoryclient.Lookback{Window: window, Step: step}, display
}

// toCategoryScope adapts the inventory DomainScope to the alertcategoryclient
// shape (the ports stay decoupled; this is the one translation point).
func toCategoryScope(ds inventoryclient.DomainScope) alertcategoryclient.DomainScope {
	return alertcategoryclient.DomainScope{
		Region:       ds.Region,
		Zone:         ds.Zone,
		Cluster:      ds.Cluster,
		NVLinkDomain: ds.NVLinkDomain,
	}
}
