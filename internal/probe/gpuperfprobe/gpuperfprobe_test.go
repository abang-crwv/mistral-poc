package gpuperfprobe

import (
	"context"
	"encoding/json"
	"testing"

	"qac/internal/gpuperfclient"
	"qac/internal/probe"
)

func TestProbe_TypeAndCategory(t *testing.T) {
	p := New()
	if p.Type() != "gpu_performance_probe" {
		t.Errorf("Type = %q, want gpu_performance_probe", p.Type())
	}
	if p.Category() != probe.CategoryGatherer {
		t.Errorf("Category = %q, want gatherer", p.Category())
	}
}

type evidence struct {
	ProbedAt int64  `json:"probed_at"`
	Source   string `json:"source"`
	PerRack  map[string]struct {
		Queried bool `json:"queried"`
		Metrics map[string]struct {
			Title string                    `json:"title"`
			Dims  []string                  `json:"dims"`
			Rows  []gpuperfclient.MetricRow `json:"rows"`
		} `json:"metrics"`
	} `json:"per_rack"`
}

func TestProbe_Run_GathersEveryMetric(t *testing.T) {
	c := probe.Clients{GPUPerfClient: gpuperfclient.NewMapClient(gpuperfclient.SeedDemoGPUPerf())}
	sc := probe.StepContext{RunID: "x", StepID: "gpu_perf", Racks: []string{"dh3-r012-us-east-01a"}}

	out, err := New().Run(context.Background(), c, sc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var body evidence
	if err := json.Unmarshal(out.StructuredData, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Source != "hpc-verification-perf" {
		t.Errorf("source = %q, want hpc-verification-perf", body.Source)
	}
	rack := body.PerRack["dh3-r012-us-east-01a"]
	if !rack.Queried {
		t.Fatalf("rack queried = false, want true")
	}
	// Every registry metric must appear as a key (even sparse ones, with empty rows).
	if len(rack.Metrics) != len(gpuperfclient.Metrics()) {
		t.Errorf("metrics keys = %d, want %d (full registry)", len(rack.Metrics), len(gpuperfclient.Metrics()))
	}
	// Seeded metric has rows; a sparse one is present but empty.
	if len(rack.Metrics["gpu_blaze_gflops_avg"].Rows) != 3 {
		t.Errorf("gpu_blaze_gflops_avg rows = %d, want 3", len(rack.Metrics["gpu_blaze_gflops_avg"].Rows))
	}
	if rack.Metrics["gpu_blaze_gflops_avg"].Title == "" {
		t.Errorf("metric evidence missing title")
	}
	if rows := rack.Metrics["ib_bw_avg"].Rows; len(rows) != 0 {
		t.Errorf("sparse metric ib_bw_avg rows = %d, want 0", len(rows))
	}
	if out.ProbedAt == 0 {
		t.Errorf("ProbedAt = 0")
	}
}

func TestProbe_Run_SourceError_Bubbles(t *testing.T) {
	c := probe.Clients{GPUPerfClient: gpuperfclient.NewMapClient(nil)}
	sc := probe.StepContext{RunID: "x", StepID: "gpu_perf", Racks: []string{gpuperfclient.FailingSourceRack}}

	_, err := New().Run(context.Background(), c, sc)
	if err == nil {
		t.Fatal("Run returned nil error; want bubble-up of source failure")
	}
}

func TestProbe_Run_NilClient_Errors(t *testing.T) {
	_, err := New().Run(context.Background(), probe.Clients{}, probe.StepContext{Racks: []string{"r"}})
	if err == nil {
		t.Fatal("Run returned nil error; want nil-client error")
	}
}
