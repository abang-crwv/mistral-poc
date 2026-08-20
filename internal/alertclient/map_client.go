package alertclient

import "context"

// MapAlertClient is a deterministic in-memory Client backed by a per-rack
// lookup table. iter-4b production and tests both use this — the
// production wiring (cmd/qac/serve.go) injects SeedDemoAlerts(); the
// real Alertmanager-backed client lands in iter-4c.
type MapAlertClient struct {
	table map[string][]Alert
	// fail lists rack ids whose ListByRack call returns ErrSourceUnavailable.
	// Selected at construction time via the "failing-source-rack" sentinel
	// in SeedDemoAlerts so engine error-path tests can exercise the path
	// without a custom mock type.
	fail map[string]bool
	// dsTable is the per-deviceslot lookup table backing ListByDeviceslot.
	dsTable map[string][]Alert
	// dsFail is the deviceslot analogue of fail: deviceslots whose
	// ListByDeviceslot call returns ErrSourceUnavailable, selected via the
	// failingDeviceslotID sentinel at construction time. (iter-5l)
	dsFail map[string]bool
}

// failingRackID is the sentinel id whose presence in the table triggers
// an ErrSourceUnavailable return. Kept package-private so callers can
// only opt in via SeedDemoAlerts or explicit fail-map construction.
const failingRackID = "failing-source-rack"

// failingDeviceslotID is the deviceslot-scoped analogue of failingRackID:
// its presence in the deviceslot table makes ListByDeviceslot surface
// ErrSourceUnavailable for that slot. (iter-5l)
const failingDeviceslotID = "failing-source-deviceslot"

// NewMapAlertClient returns a Client backed by the given lookup tables.
// rackTable backs ListByRack and deviceslotTable backs ListByDeviceslot.
// A nil or empty table is fine — every lookup misses and returns an
// empty slice. If rackTable contains the failingRackID sentinel, rack
// lookups for that id surface ErrSourceUnavailable; likewise if
// deviceslotTable contains the failingDeviceslotID sentinel.
func NewMapAlertClient(rackTable, deviceslotTable map[string][]Alert) *MapAlertClient {
	c := &MapAlertClient{
		table:   rackTable,
		fail:    map[string]bool{},
		dsTable: deviceslotTable,
		dsFail:  map[string]bool{},
	}
	if _, ok := rackTable[failingRackID]; ok {
		c.fail[failingRackID] = true
	}
	if _, ok := deviceslotTable[failingDeviceslotID]; ok {
		c.dsFail[failingDeviceslotID] = true
	}
	return c
}

// ListByRack returns the alerts for rackID, or an empty slice on miss.
// Returns ErrSourceUnavailable when the table marks the rack failing.
func (c *MapAlertClient) ListByRack(ctx context.Context, rackID string) ([]Alert, error) {
	if c.fail[rackID] {
		return nil, ErrSourceUnavailable
	}
	if alerts, ok := c.table[rackID]; ok {
		return alerts, nil
	}
	return nil, nil
}

// ListByDeviceslot returns the alerts for deviceslot, or an empty slice on
// miss. Returns ErrSourceUnavailable when the table marks the slot failing.
func (c *MapAlertClient) ListByDeviceslot(ctx context.Context, deviceslot string) ([]Alert, error) {
	if c.dsFail[deviceslot] {
		return nil, ErrSourceUnavailable
	}
	if alerts, ok := c.dsTable[deviceslot]; ok {
		return alerts, nil
	}
	return nil, nil
}

// SeedDemoAlerts returns the canonical fixture map covering the iter-4a
// SeedDemoFixtures rack ids plus a sentinel "failing-source-rack" entry
// that surfaces ErrSourceUnavailable. Coverage assertions live in
// fixture_coverage_test.go.
func SeedDemoAlerts() map[string][]Alert {
	return map[string][]Alert{
		// Clean rack (matches iter-4a happy-path rack).
		"dh3-r012-us-east-01a": {},

		// One warning alert.
		"dh3-r012-us-east-02a": {
			{
				ID:       "alertmanager-stub:GPUTempHigh:dh3-r012-us-east-02a",
				Name:     "GPUTempHigh",
				Severity: "warning",
				FiredAt:  1700000000,
				Summary:  "GPU temperature above warning threshold",
				Scope:    "dh3-r012-us-east-02a",
			},
		},

		// Clean rack.
		"dh4-r020-us-west-01b": {},

		// One critical alert.
		"dh4-r020-us-west-02b": {
			{
				ID:       "alertmanager-stub:NVLinkLinkDown:dh4-r020-us-west-02b",
				Name:     "NVLinkLinkDown",
				Severity: "critical",
				FiredAt:  1700000001,
				Summary:  "NVLink port reports link down",
				Scope:    "dh4-r020-us-west-02b",
			},
		},

		// Parallels iter-4a's ambiguous inventory fixture. Empty here so
		// AlertClient-only unit tests can use the same id without crossing
		// into inventory discovery.
		"dh9-r999-ambig-test-01a": {},

		// Sentinel: triggers ErrSourceUnavailable. Used by engine error
		// path tests and by the AlertClient unit test.
		failingRackID: {},
	}
}

// SeedDemoDeviceslotAlerts returns the canonical deviceslot-keyed fixture
// backing ListByDeviceslot. It mirrors SeedDemoAlerts but at node/CT
// granularity, so fail_cause_probe (iter-5l) can pin a failed tray to its
// firing alerts. Includes one critical alert on a representative deviceslot
// plus the failing-source-deviceslot sentinel that surfaces
// ErrSourceUnavailable.
func SeedDemoDeviceslotAlerts() map[string][]Alert {
	return map[string][]Alert{
		// One critical node-scoped alert (parallels the rack-level
		// NVLinkLinkDown fixture, but pinned to a deviceslot).
		"dh4-r020-node-02-us-west-02b": {
			{
				ID:         "alertmanager-stub:NVLinkLinkDown:dh4-r020-node-02-us-west-02b",
				Name:       "NVLinkLinkDown",
				Severity:   "critical",
				FiredAt:    1700000001,
				Summary:    "NVLink port reports link down",
				Scope:      "dh4-r020-us-west-02b",
				Deviceslot: "dh4-r020-node-02-us-west-02b",
			},
		},

		// Sentinel: triggers ErrSourceUnavailable for deviceslot lookups.
		failingDeviceslotID: {},
	}
}
