package alertcategoryclient

import (
	"context"
	"fmt"
)

// FailingSourceRack is the sentinel rack id MapClient surfaces
// ErrSourceUnavailable for, supporting engine error-path tests (mirrors
// alertclient's failing-source-rack).
const FailingSourceRack = "failing-source-rack"

// MapClient is a deterministic in-memory Client for tests and the offline
// (QAC_VM_BACKEND=map) backend. Rows are keyed by category id, then rack id.
type MapClient struct {
	rows map[string]map[string][]CategoryRow
}

// NewMapClient returns a MapClient backed by the given
// categoryID -> rack -> rows table. A nil table is normalized to empty (every
// gather returns no rows).
func NewMapClient(rows map[string]map[string][]CategoryRow) *MapClient {
	if rows == nil {
		rows = map[string]map[string][]CategoryRow{}
	}
	return &MapClient{rows: rows}
}

var _ Client = (*MapClient)(nil)

// Categories satisfies Client.
func (m *MapClient) Categories() []CategorySpec { return Categories() }

// Gather satisfies Client. Returns the seeded rows for (categoryID, rack), or
// an empty slice on miss. The FailingSourceRack sentinel surfaces
// ErrSourceUnavailable; an unknown category id is an error.
func (m *MapClient) Gather(_ context.Context, categoryID string, _ DomainScope, rack string, _ Lookback) ([]CategoryRow, error) {
	if rack == FailingSourceRack {
		return nil, ErrSourceUnavailable
	}
	if _, ok := specByID(categoryID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownCategory, categoryID)
	}
	byRack := m.rows[categoryID]
	if byRack == nil {
		return nil, nil
	}
	rows := byRack[rack]
	out := make([]CategoryRow, len(rows))
	copy(out, rows)
	return out, nil
}

// SeedDemoCategories returns a deterministic categoryID -> rack -> rows table
// for the demo racks (those in inventoryclient.SeedDemoBMNs). Only a couple of
// categories carry sample rows; the rest resolve empty, which is a valid
// "no alerts in this class" observation. Timestamps are fixed so demo output
// is reproducible.
func SeedDemoCategories() map[string]map[string][]CategoryRow {
	const (
		demoFirstSeen = 1_700_000_000
		demoLastSeen  = 1_700_086_400 // ~24h later
	)
	racks := []string{
		"dh3-r012-us-east-01a",
		"dh3-r012-us-east-02a",
		"dh4-r020-us-west-01b",
		"dh4-r020-us-west-02b",
	}
	out := map[string]map[string][]CategoryRow{
		"node_alert_history":   {},
		"nvlink_switch_alerts": {},
	}
	for _, rk := range racks {
		out["node_alert_history"][rk] = []CategoryRow{
			{
				Labels: map[string]string{
					"node": rk + "-node-01", "alertname": "NodeGPUNVLinkDown",
					"alertstate": "firing", "severity": "warning", "slot": "01",
				},
				FirstSeen: demoFirstSeen, LastSeen: demoLastSeen, SampleCount: 1440,
			},
		}
		out["nvlink_switch_alerts"][rk] = []CategoryRow{
			{
				Labels: map[string]string{
					"deviceslot": rk + "-nvl-mgmt-01", "alertname": "NVLinkSwitchNVOSUnreachable",
					"alertstate": "firing", "severity": "critical",
				},
				FirstSeen: demoFirstSeen, LastSeen: demoLastSeen, SampleCount: 1440,
			},
		}
	}
	return out
}
