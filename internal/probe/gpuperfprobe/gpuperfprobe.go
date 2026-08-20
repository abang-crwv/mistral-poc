// Package gpuperfprobe implements gpu_performance_probe — a gatherer that
// snapshots the full HPC-verification performance pack for each canary rack:
// GPU Blaze GFLOPS, thermal/throttle, NVBandwidth, NCCL allreduce, Megatron
// iteration timings, IB bandwidth/latency, and CUDA sync latency. A cross-run
// diff shows how a rack's performance moved across a firmware zap.
//
// Source: the HPC-verification recording rules via gpuperfclient, scoped to the
// rack through the node_context join. The query is scoped purely by rack_name,
// so no zone or domain-scope resolution is needed. These rules are run-driven:
// a rack with no recent verification run yields empty rows per metric, which is
// recorded as data (queried=true), not an error. Only a source/transport
// failure bubbles up to StepFailed.
package gpuperfprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"qac/internal/gpuperfclient"
	"qac/internal/probe"
)

// Probe snapshots per-rack HPC-verification performance metrics.
type Probe struct{}

// New returns a zero-value Probe.
func New() *Probe { return &Probe{} }

// Compile-time satisfaction check.
var _ probe.Probe = (*Probe)(nil)

func (*Probe) Type() string             { return "gpu_performance_probe" }
func (*Probe) Category() probe.Category { return probe.CategoryGatherer }

// metricEvidence is one metric's gathered rows plus its descriptors.
type metricEvidence struct {
	Title string                    `json:"title"`
	Dims  []string                  `json:"dims"`
	Rows  []gpuperfclient.MetricRow `json:"rows"`
}

// rackPerf is the per-rack body: every registry metric keyed by id. Queried=true
// means the source was read; per-metric Rows may be empty when a rule is sparse
// (no recent run).
type rackPerf struct {
	Queried bool                      `json:"queried"`
	Metrics map[string]metricEvidence `json:"metrics"`
}

// perfEvidence is the structured output.
type perfEvidence struct {
	ProbedAt int64               `json:"probed_at"`
	Source   string              `json:"source"`
	PerRack  map[string]rackPerf `json:"per_rack"`
}

const source = "hpc-verification-perf"

// Run satisfies probe.Probe. For each rack it gathers every registered perf
// metric and records the rows under that metric's id. A sparse rule (empty
// rows) is normal data; a source/transport failure bubbles up to StepFailed.
func (*Probe) Run(ctx context.Context, clients probe.Clients, sc probe.StepContext) (probe.Output, error) {
	if clients.GPUPerfClient == nil {
		return probe.Output{}, fmt.Errorf("gpu_performance_probe: clients.GPUPerfClient is nil")
	}

	specs := clients.GPUPerfClient.Metrics()
	perRack := make(map[string]rackPerf, len(sc.Racks))

	for _, rack := range sc.Racks {
		metrics := make(map[string]metricEvidence, len(specs))
		for _, spec := range specs {
			rows, err := clients.GPUPerfClient.Gather(ctx, spec.ID, rack)
			if err != nil {
				// Source/transport failure — bubble up so the engine emits
				// StepFailed for this step. The walk continues to the next step.
				return probe.Output{}, fmt.Errorf("gpu_performance_probe: gather %s on %s: %w", spec.ID, rack, err)
			}
			metrics[spec.ID] = metricEvidence{
				Title: spec.Title,
				Dims:  spec.Dims,
				Rows:  rows,
			}
		}
		perRack[rack] = rackPerf{Queried: true, Metrics: metrics}
	}

	probedAt := time.Now().Unix()
	ev := perfEvidence{
		ProbedAt: probedAt,
		Source:   source,
		PerRack:  perRack,
	}
	structured, err := json.Marshal(ev)
	if err != nil {
		return probe.Output{}, fmt.Errorf("gpu_performance_probe: marshal output: %w", err)
	}
	return probe.Output{
		StructuredData: structured,
		Sources:        []string{source},
		ProbedAt:       probedAt,
	}, nil
}
