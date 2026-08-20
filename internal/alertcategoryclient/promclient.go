package alertcategoryclient

import (
	"context"
	"fmt"
	"sort"
	"time"

	"qac/internal/vm"
)

// defaultLookbackWindow is the alert-history window when Lookback.Window is
// zero — 24h per the canary design.
const defaultLookbackWindow = 24 * time.Hour

// PromClient is the production backend. It renders a category's embedded
// PromQL, runs it as a range query through the shared super-region fan-out
// client (internal/vm), and collapses each matrix series to one CategoryRow.
type PromClient struct {
	vm *vm.Client
}

// NewPromClient returns a PromClient backed by the shared vm.Client.
func NewPromClient(c *vm.Client) *PromClient { return &PromClient{vm: c} }

var _ Client = (*PromClient)(nil)

// Categories satisfies Client.
func (c *PromClient) Categories() []CategorySpec { return Categories() }

// Gather satisfies Client. It renders the category query for the rack's domain
// scope, runs it over [now-window, now] at the configured step, and returns
// one CategoryRow per result series (sorted for determinism). A clean miss
// (no firing alerts) returns an empty slice, not an error.
func (c *PromClient) Gather(ctx context.Context, categoryID string, scope DomainScope, rack string, lb Lookback) ([]CategoryRow, error) {
	spec, ok := specByID(categoryID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownCategory, categoryID)
	}
	tmpl := loadQuery(spec.queryFile)
	if tmpl == "" {
		return nil, fmt.Errorf("alertcategoryclient: empty query template for %q", categoryID)
	}
	q := renderQuery(tmpl, scope, rack)

	window := lb.Window
	if window <= 0 {
		window = defaultLookbackWindow
	}
	end := time.Now()
	start := end.Add(-window)
	series, err := c.vm.QueryRange(ctx, q, vm.RangeParams{Start: start, End: end, Step: lb.Step})
	if err != nil {
		return nil, err
	}
	out := make([]CategoryRow, 0, len(series))
	for _, s := range series {
		if len(s.Values) == 0 {
			continue
		}
		out = append(out, CategoryRow{
			Labels:      s.Metric,
			FirstSeen:   s.Values[0].TS,
			LastSeen:    s.Values[len(s.Values)-1].TS,
			SampleCount: len(s.Values),
		})
	}
	sortRows(out)
	return out, nil
}

// sortRows orders rows deterministically by node, then deviceslot, then
// alertname — the labels common across categories (absent labels sort first).
func sortRows(rows []CategoryRow) {
	sort.Slice(rows, func(i, j int) bool {
		li, lj := rows[i].Labels, rows[j].Labels
		if li["node"] != lj["node"] {
			return li["node"] < lj["node"]
		}
		if li["deviceslot"] != lj["deviceslot"] {
			return li["deviceslot"] < lj["deviceslot"]
		}
		if li["alertname"] != lj["alertname"] {
			return li["alertname"] < lj["alertname"]
		}
		return li["alertstate"] < lj["alertstate"]
	})
}
