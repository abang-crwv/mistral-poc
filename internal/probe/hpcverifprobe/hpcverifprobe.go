// Package hpcverifprobe implements hpc_verification_failure_probe — a gatherer
// that snapshots each canary rack's HPC-verification state. A node fails
// verification when its message annotation is non-OK; the probe records which
// node failed which testcase, plus the raw message.
//
// It also gathers the tested-node roster (the heartbeat gauge, one series per
// node that has run verification) so an empty failure list is no longer read as
// "healthy" on its own. Each rack gets a coverage-aware Status:
//
//   - failed        : at least one node's message is non-OK
//   - stale         : a tested node's last run predates the staleness threshold
//   - not_verified  : no verification data at all (never/not-recently tested)
//   - passed        : every tested node is fresh and non-failing
//
// This closes the false-green where a freshly-zapped rack that has not
// re-verified would otherwise report healthy. The probe never treats missing
// data as an error — only a source/transport failure bubbles up to StepFailed.
//
// Source: kube_node_annotations (failures) and
// kube_node_hpc_verification_last_heartbeat_time (roster) via hpcverifclient.
// Both queries are scoped purely by rack_name, so no zone or domain-scope
// resolution is needed.
package hpcverifprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"qac/internal/probe"
)

// Probe snapshots per-node HPC-verification failures per rack.
type Probe struct{}

// New returns a zero-value Probe.
func New() *Probe { return &Probe{} }

// Compile-time satisfaction check.
var _ probe.Probe = (*Probe)(nil)

func (*Probe) Type() string             { return "hpc_verification_failure_probe" }
func (*Probe) Category() probe.Category { return probe.CategoryGatherer }

// nodeFailure is one failing node in the evidence.
type nodeFailure struct {
	Node          string `json:"node"`
	Deviceslot    string `json:"deviceslot"`
	Testcase      string `json:"testcase"`
	Message       string `json:"message"`
	LastHeartbeat string `json:"last_heartbeat,omitempty"`
}

// staleNode is one tested node whose last verification run is older than the
// staleness threshold: its identity plus the heartbeat age in seconds.
type staleNode struct {
	Node            string `json:"node"`
	Deviceslot      string `json:"deviceslot"`
	HeartbeatAgeSec int64  `json:"heartbeat_age_sec"`
}

// rackVerification is the per-rack body. Queried=true means the source was read
// successfully. Status is the coverage-aware verdict: an empty FailingNodes no
// longer means "healthy" on its own — the roster distinguishes passed from
// not_verified (never/not-recently tested) from stale (tested, but the last run
// predates the staleness threshold).
type rackVerification struct {
	Queried               bool           `json:"queried"`
	Status                string         `json:"status"`
	FailureCount          int            `json:"failure_count"`
	TestedCount           int            `json:"tested_count"`
	PassedCount           int            `json:"passed_count"`
	StaleCount            int            `json:"stale_count"`
	OldestHeartbeatAgeSec int64          `json:"oldest_heartbeat_age_sec,omitempty"`
	ByTestcase            map[string]int `json:"by_testcase,omitempty"`
	FailingNodes          []nodeFailure  `json:"failing_nodes,omitempty"`
	StaleNodes            []staleNode    `json:"stale_nodes,omitempty"`
	Error                 string         `json:"error,omitempty"`
}

// verificationEvidence is the structured output.
type verificationEvidence struct {
	ProbedAt       int64                       `json:"probed_at"`
	Source         string                      `json:"source"`
	PerRack        map[string]rackVerification `json:"per_rack"`
	AnyFailures    bool                        `json:"any_failures"`
	AnyStale       bool                        `json:"any_stale"`
	AnyNotVerified bool                        `json:"any_not_verified"`
}

const source = "hpc-verification"

// defaultStalenessHours is how old a node's last verification run may be before
// it is classed as stale. Overridable per step via Config["staleness_hours"].
// 48h matches hpc-debug's fleetops threshold; production canary racks re-verify
// well inside that window.
const defaultStalenessHours = 48

// Rack-level status values (mutually exclusive, in precedence order).
const (
	statusFailed      = "failed"
	statusStale       = "stale"
	statusNotVerified = "not_verified"
	statusPassed      = "passed"
)

// stalenessSeconds reads the per-step staleness threshold override (in hours)
// from Config, falling back to the default. JSON numbers decode to float64, but
// an int (from a Go-constructed StepContext) is honored too.
func stalenessSeconds(cfg map[string]any) int64 {
	hours := float64(defaultStalenessHours)
	switch v := cfg["staleness_hours"].(type) {
	case float64:
		if v > 0 {
			hours = v
		}
	case int:
		if v > 0 {
			hours = float64(v)
		}
	}
	return int64(hours * 3600)
}

// Run satisfies probe.Probe. For each rack it queries the HPC-verification
// failures, derives the per-testcase histogram, and records the failing nodes.
// A healthy rack (no failures) is normal data, not an error; a source/transport
// failure bubbles up to StepFailed.
func (*Probe) Run(ctx context.Context, clients probe.Clients, sc probe.StepContext) (probe.Output, error) {
	if clients.HPCVerifClient == nil {
		return probe.Output{}, fmt.Errorf("hpc_verification_failure_probe: clients.HPCVerifClient is nil")
	}

	perRack := make(map[string]rackVerification, len(sc.Racks))
	anyFailures := false
	anyStale := false
	anyNotVerified := false

	now := time.Now().Unix()
	staleAfter := stalenessSeconds(sc.Config)

	for _, rack := range sc.Racks {
		failures, err := clients.HPCVerifClient.VerificationFailures(ctx, rack)
		if err != nil {
			// Source/transport failure — bubble up so the engine emits
			// StepFailed for this step. The walk continues to the next step.
			return probe.Output{}, fmt.Errorf("hpc_verification_failure_probe: verification failures %s: %w", rack, err)
		}
		roster, err := clients.HPCVerifClient.VerificationRoster(ctx, rack)
		if err != nil {
			return probe.Output{}, fmt.Errorf("hpc_verification_failure_probe: verification roster %s: %w", rack, err)
		}

		byTestcase := map[string]int{}
		nodes := make([]nodeFailure, 0, len(failures))
		failing := make(map[string]bool, len(failures))
		for _, f := range failures {
			byTestcase[f.Testcase]++
			failing[f.Node] = true
			nodes = append(nodes, nodeFailure{
				Node:          f.Node,
				Deviceslot:    f.Deviceslot,
				Testcase:      f.Testcase,
				Message:       f.Message,
				LastHeartbeat: f.LastHeartbeat,
			})
		}

		// Classify the roster (every tested node): a failing node is counted as
		// failed; a non-failing node is passed if fresh, stale otherwise.
		passed := 0
		stale := make([]staleNode, 0)
		var oldestAge int64
		for _, n := range roster {
			age := now - n.LastHeartbeatUnix
			if age > oldestAge {
				oldestAge = age
			}
			if failing[n.Node] {
				continue // already accounted for in FailingNodes
			}
			if age > staleAfter {
				stale = append(stale, staleNode{Node: n.Node, Deviceslot: n.Deviceslot, HeartbeatAgeSec: age})
			} else {
				passed++
			}
		}

		// Status precedence: any failure dominates, then stale, then a rack
		// with no verification data at all is not_verified, else passed.
		status := statusPassed
		switch {
		case len(nodes) > 0:
			status = statusFailed
			anyFailures = true
		case len(stale) > 0:
			status = statusStale
		case len(roster) == 0:
			status = statusNotVerified
		}
		if len(stale) > 0 {
			anyStale = true
		}
		if status == statusNotVerified {
			anyNotVerified = true
		}

		perRack[rack] = rackVerification{
			Queried:               true,
			Status:                status,
			FailureCount:          len(nodes),
			TestedCount:           len(roster),
			PassedCount:           passed,
			StaleCount:            len(stale),
			OldestHeartbeatAgeSec: oldestAge,
			ByTestcase:            byTestcase,
			FailingNodes:          nodes,
			StaleNodes:            stale,
		}
	}

	probedAt := time.Now().Unix()
	ev := verificationEvidence{
		ProbedAt:       probedAt,
		Source:         source,
		PerRack:        perRack,
		AnyFailures:    anyFailures,
		AnyStale:       anyStale,
		AnyNotVerified: anyNotVerified,
	}
	structured, err := json.Marshal(ev)
	if err != nil {
		return probe.Output{}, fmt.Errorf("hpc_verification_failure_probe: marshal output: %w", err)
	}
	return probe.Output{
		StructuredData: structured,
		Sources:        []string{source},
		ProbedAt:       probedAt,
	}, nil
}
