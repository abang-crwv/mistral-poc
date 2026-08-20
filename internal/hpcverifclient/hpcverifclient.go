// Package hpcverifclient is a self-contained source of HPC-verification
// failures for a rack. hpc_verification_failure_probe uses it to gather which
// nodes failed which verification testcase — the signal that a rack is not
// fit for workloads after a firmware zap.
//
// Source: the kube_node_annotations metric, whose
// annotation_hpc_verification_coreweave_cloud_message label carries the
// per-node verification message (non-empty and != "OK" means a failure). A
// testcase id is extracted from the message, and the node set is narrowed by
// k8s state, FLCC lifecycle (ignoring rma/broken/etc.), and the target rack via
// physical topology. One row per failing node — counts are derived by the
// probe, so a cross-run diff shows which node's which testcase changed.
//
// Like the sibling ports, the bare `qac` module means we carry the shape we
// need and run through the shared super-region fan-out client (internal/vm).
package hpcverifclient

import (
	"context"
	"errors"
)

// NodeFailure is one failing node's HPC-verification reading: the extracted
// testcase plus the raw message and last-heartbeat for evidence.
type NodeFailure struct {
	Node          string `json:"node"`
	Deviceslot    string `json:"deviceslot"`
	Testcase      string `json:"testcase"`
	Message       string `json:"message"`
	LastHeartbeat string `json:"last_heartbeat,omitempty"`
}

// NodeStatus is one tested node on a rack: its identity plus the unix-seconds
// timestamp of its last verification run. The heartbeat gauge is emitted for
// every node that has run verification (pass OR fail), so the roster is the
// coverage denominator and freshness source the failures path lacks.
type NodeStatus struct {
	Node              string `json:"node"`
	Deviceslot        string `json:"deviceslot"`
	LastHeartbeatUnix int64  `json:"last_heartbeat_unix"`
}

// Client reads the HPC-verification state for a rack. Implementations must
// be goroutine-safe.
type Client interface {
	// VerificationFailures returns one row per node on the rack whose
	// verification message is non-OK, after FLCC/state filtering. A healthy
	// rack (no failing nodes) returns an empty slice, not an error.
	VerificationFailures(ctx context.Context, rack string) ([]NodeFailure, error)

	// VerificationRoster returns one row per tested node on the rack (any node
	// with a verification heartbeat), after the same FLCC/state filtering. This
	// is the coverage denominator: a rack with no recent verification run
	// returns an empty slice (not an error), which is how the probe tells
	// "not verified" apart from "passed".
	VerificationRoster(ctx context.Context, rack string) ([]NodeStatus, error)
}

// ErrSourceUnavailable signals a source-side failure (network, auth). Callers
// may map it onto a StepFailed event.
var ErrSourceUnavailable = errors.New("hpcverifclient: source unavailable")
