package hpcverifclient

import (
	"context"
	"fmt"
	"strings"
)

// FailingSourceRack is the sentinel rack id MapClient surfaces
// ErrSourceUnavailable for, supporting probe error-path tests. (This models a
// source/transport failure — distinct from a healthy rack, which returns an
// empty slice.)
const FailingSourceRack = "failing-source-rack"

// MapClient is a deterministic in-memory Client for tests and the offline
// (QAC_VM_BACKEND=map) backend. Failures are keyed by rack id; a rack absent
// from the table is healthy (no failures). The roster table is keyed the same
// way; a rack absent from it has no verification data ("not verified").
type MapClient struct {
	byRack map[string][]NodeFailure
	roster map[string][]NodeStatus
}

// NewMapClient returns a MapClient backed by the given rack -> failures table.
// A nil table is normalized to empty (every rack is healthy). The roster table
// starts empty; attach one with WithRoster.
func NewMapClient(byRack map[string][]NodeFailure) *MapClient {
	if byRack == nil {
		byRack = map[string][]NodeFailure{}
	}
	return &MapClient{byRack: byRack, roster: map[string][]NodeStatus{}}
}

// WithRoster attaches a rack -> tested-node roster table and returns the client
// for chaining. A nil table is normalized to empty.
func (m *MapClient) WithRoster(roster map[string][]NodeStatus) *MapClient {
	if roster == nil {
		roster = map[string][]NodeStatus{}
	}
	m.roster = roster
	return m
}

var _ Client = (*MapClient)(nil)

// VerificationFailures satisfies Client. Returns the seeded failures for the
// rack, or an empty slice when the rack is healthy/absent. The FailingSourceRack
// sentinel surfaces ErrSourceUnavailable.
func (m *MapClient) VerificationFailures(_ context.Context, rack string) ([]NodeFailure, error) {
	if rack == FailingSourceRack {
		return nil, ErrSourceUnavailable
	}
	rows := m.byRack[rack]
	out := make([]NodeFailure, len(rows))
	copy(out, rows)
	return out, nil
}

// VerificationRoster satisfies Client. Returns the seeded roster for the rack,
// or an empty slice when the rack has no verification data. The FailingSourceRack
// sentinel surfaces ErrSourceUnavailable.
func (m *MapClient) VerificationRoster(_ context.Context, rack string) ([]NodeStatus, error) {
	if rack == FailingSourceRack {
		return nil, ErrSourceUnavailable
	}
	rows := m.roster[rack]
	out := make([]NodeStatus, len(rows))
	copy(out, rows)
	return out, nil
}

// SeedDemoHPCRoster returns a deterministic rack -> roster table for the demo
// racks. The demo rack dh3-r012-us-east-01a has an 8-node roster: the two nodes
// that fail (positions 2 and 7) plus six passing nodes, with one node (position
// 5) deliberately stale so the demo exercises the passed/failed/stale split.
// Heartbeats are relative to now so fresh nodes stay fresh as the demo runs.
func SeedDemoHPCRoster(now int64) map[string][]NodeStatus {
	rack := "dh3-r012-us-east-01a"
	const day = 24 * 60 * 60
	out := map[string][]NodeStatus{}
	for _, pos := range []int{1, 2, 3, 4, 5, 6, 7, 8} {
		hb := now - 3600 // fresh: last ran an hour ago
		if pos == 5 {
			hb = now - 5*day // stale: no run in 5 days
		}
		out[rack] = append(out[rack], NodeStatus{
			Node:              fmt.Sprintf("s90txs%02d", pos+50),
			Deviceslot:        demoDeviceslot(rack, pos),
			LastHeartbeatUnix: hb,
		})
	}
	return out
}

// SeedDemoHPCFailures returns a deterministic rack -> failures table for the
// demo racks. One demo rack (dh3-r012-us-east-01a) has two nodes failing the
// nvlink_bandwidth testcase; the other demo racks are healthy (absent), so the
// probe exercises both the failing and clean paths.
func SeedDemoHPCFailures() map[string][]NodeFailure {
	rack := "dh3-r012-us-east-01a"
	out := map[string][]NodeFailure{}
	for _, pos := range []int{2, 7} {
		ds := demoDeviceslot(rack, pos)
		out[rack] = append(out[rack], NodeFailure{
			Node:          fmt.Sprintf("s90txs%02d", pos+50),
			Deviceslot:    ds,
			Testcase:      "nvlink_bandwidth",
			Message:       fmt.Sprintf("[6B0B] [nvlink_bandwidth] NVLink bandwidth check failed - 8 link(s) below 50.0 GB/s threshold on %s", ds),
			LastHeartbeat: "2026-06-07T12:00:00Z",
		})
	}
	return out
}

// demoDeviceslot derives the canonical deviceslot for a node position on a demo
// rack: "dh3-r012-us-east-01a" + pos 2 -> "dh3-r012-node-02-us-east-01a".
// Mirrors firmwareclient.demoDeviceslot so demo evidence lines up across probes.
func demoDeviceslot(rack string, pos int) string {
	parts := strings.SplitN(rack, "-", 3)
	if len(parts) < 3 {
		return fmt.Sprintf("%s-node-%02d", rack, pos)
	}
	return fmt.Sprintf("%s-%s-node-%02d-%s", parts[0], parts[1], pos, parts[2])
}
