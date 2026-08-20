// Package gpuperfclient is a self-contained source of HPC-verification
// performance metrics for a rack. gpu_performance_probe uses it to gather the
// full perf pack — GPU Blaze GFLOPS, thermal/throttle, NVBandwidth, NCCL
// allreduce, Megatron iteration timings, IB bandwidth/latency, and CUDA sync
// latency — so a cross-run diff shows how a rack's performance moved across a
// firmware zap.
//
// Source: the HPC-verification recording rules (gpu:/node:/devpair: families),
// scoped to a rack by joining each metric onto node:node_context:limit_1 on
// (node, cluster, cluster_org, zone). These rules are populated while a
// verification run emits data and are sparse between runs — an empty result is
// "no recent run", not an error.
//
// Metrics are modeled as DATA: a registry of MetricSpec, each rendered through
// one templated join query. Adding a signal is a one-line registry entry.
//
// Like the sibling ports, the bare `qac` module means we carry the shape we
// need and run through the shared super-region fan-out client (internal/vm).
package gpuperfclient

import (
	"context"
	"errors"
)

// MetricRow is one gathered series: its label set (the spec's Dims, after
// aggregation) plus the numeric value.
type MetricRow struct {
	Labels map[string]string `json:"labels"`
	Value  float64           `json:"value"`
}

// Metric kinds. The default (empty) kind aggregates a single recording rule;
// histogram_mean takes the _sum/_count mean of an OTel histogram base metric
// (used for NCCL, which has no recording rule — only the gamble histogram).
const (
	kindRule          = "" // aggregate one recording rule (the common case)
	kindHistogramMean = "histogram_mean"
)

// MetricSpec describes one perf signal: which source metric, how to aggregate
// it, and the dimensions to group by. The query is generated from these.
type MetricSpec struct {
	ID    string   // stable id, e.g. "gpu_blaze_gflops_avg"
	Title string   // human label for evidence
	Agg   string   // "avg" | "min" | "max"
	Rule  string   // recording-rule name (kindRule) or histogram base (kindHistogramMean)
	Dims  []string // group-by labels, e.g. {"node","gpu","precision"}
	Kind  string   // "" (kindRule) or kindHistogramMean
}

// Client gathers one perf metric for a rack. Implementations must be
// goroutine-safe.
type Client interface {
	// Gather runs the metric identified by metricID, scoped to rack, and
	// returns one row per (dims) series. A sparse/absent rule (no recent
	// verification run) returns an empty slice, not an error.
	Gather(ctx context.Context, metricID, rack string) ([]MetricRow, error)
	// Metrics returns the registry in stable order so the probe can iterate
	// every signal without knowing the ids ahead of time.
	Metrics() []MetricSpec
}

// ErrSourceUnavailable signals a source-side failure (network, auth). Callers
// may map it onto a StepFailed event.
var ErrSourceUnavailable = errors.New("gpuperfclient: source unavailable")

// ErrUnknownMetric is returned by Gather for an id not in the registry.
var ErrUnknownMetric = errors.New("gpuperfclient: unknown metric id")
