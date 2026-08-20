package awxjobprobe

import (
	"context"
	"encoding/json"
	"testing"

	"qac/internal/awxclient"
	"qac/internal/inventoryclient"
	"qac/internal/lifecycleclient"
	"qac/internal/lokiclient"
	"qac/internal/probe"
)

func demoClients() probe.Clients {
	return probe.Clients{
		AWXJobClient:      awxclient.NewMapClient(awxclient.SeedDemoAWXJobs()),
		LokiClient:        lokiclient.NewMapClient(lokiclient.SeedDemoL11Logs()),
		LifecycleClient:   lifecycleclient.NewMapClient(nil),
		InventoryResolver: inventoryclient.NewMapResolverWithBMNs(inventoryclient.SeedDemoFixtures(), inventoryclient.SeedDemoBMNs()),
	}
}

type evidence struct {
	Source      string   `json:"source"`
	LimitTypes  []string `json:"limit_types"`
	PerTarget   int      `json:"per_target"`
	L11Lookback string   `json:"l11_lookback"`
	PerRack     map[string]struct {
		Queried  bool   `json:"queried"`
		Region   string `json:"region"`
		BMNCount int    `json:"bmn_count"`
		Nodes    []struct {
			BMN        string                 `json:"bmn"`
			Deviceslot string                 `json:"deviceslot"`
			Jobs       []jobView              `json:"jobs"`
			Stages     map[string]stageChrono `json:"stages"`
		} `json:"nodes"`
		L11 []struct {
			JobID  int    `json:"job_id"`
			Status string `json:"status"`
		} `json:"l11"`
		ByStageStatus map[string]map[string]int `json:"by_stage_status"`
		Failures      []struct {
			Template string `json:"template"`
			JobCount int    `json:"job_count"`
			JobIDs   []int  `json:"job_ids"`
		} `json:"failures"`
		Error string `json:"error"`
	} `json:"per_rack"`
}

func TestProbe_TypeAndCategory(t *testing.T) {
	p := New()
	if p.Type() != "awx_job_probe" {
		t.Errorf("Type = %q, want awx_job_probe", p.Type())
	}
	if p.Category() != probe.CategoryGatherer {
		t.Errorf("Category = %q, want gatherer", p.Category())
	}
}

func TestProbe_Run_FullRack(t *testing.T) {
	sc := probe.StepContext{RunID: "x", StepID: "awx_jobs", Racks: []string{"dh3-r012-us-east-01a"}}
	out, err := New().Run(context.Background(), demoClients(), sc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var body evidence
	if err := json.Unmarshal(out.StructuredData, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Source != "awxctl-job-info" {
		t.Errorf("source = %q", body.Source)
	}
	if len(body.LimitTypes) != 2 || body.PerTarget != 5 {
		t.Errorf("limit_types=%v per_target=%d, want [mgmt bmc]/5", body.LimitTypes, body.PerTarget)
	}
	rack := body.PerRack["dh3-r012-us-east-01a"]
	if !rack.Queried || rack.BMNCount != 18 || rack.Region != "us-east-01a" {
		t.Fatalf("rack queried=%v bmn=%d region=%q", rack.Queried, rack.BMNCount, rack.Region)
	}
	// Two nodes carry jobs (s90txs51 clean trio, s90txs52 with a failed fielddiag).
	if len(rack.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(rack.Nodes))
	}
	// Rollup across nodes: node-zap x2 successful, dpu-zap x2 successful,
	// fielddiag 1 successful + 1 failed.
	if rack.ByStageStatus["node-zap"]["successful"] != 2 {
		t.Errorf("node-zap successful = %d, want 2", rack.ByStageStatus["node-zap"]["successful"])
	}
	if rack.ByStageStatus["dpu-zap"]["successful"] != 2 {
		t.Errorf("dpu-zap successful = %d, want 2", rack.ByStageStatus["dpu-zap"]["successful"])
	}
	if rack.ByStageStatus["fielddiag"]["failed"] != 1 {
		t.Errorf("fielddiag failed = %d, want 1", rack.ByStageStatus["fielddiag"]["failed"])
	}
	// The failing node's fielddiag chronology: one attempt, one failure,
	// outcome persistent-fail.
	var sawPersistentFail bool
	for _, n := range rack.Nodes {
		if c, ok := n.Stages["fielddiag"]; ok && c.Failures == 1 && c.Outcome == "persistent-fail" {
			sawPersistentFail = true
		}
		// deviceslot is populated for every node with jobs.
		if n.Deviceslot == "" {
			t.Errorf("node %s missing deviceslot", n.BMN)
		}
	}
	if !sawPersistentFail {
		t.Errorf("expected a node with a persistent-fail fielddiag chronology")
	}
	// l11: two rack-wide jobs (running + failed) via Loki→job-id.
	if len(rack.L11) != 2 {
		t.Fatalf("l11 = %d, want 2", len(rack.L11))
	}
	// Failure analysis: the failed fielddiag jobs (per-node 884311 + l11 883990)
	// group under fielddiag-ist-gpu.
	if len(rack.Failures) != 1 || rack.Failures[0].Template != "fielddiag-ist-gpu" || rack.Failures[0].JobCount != 2 {
		t.Errorf("failures = %+v, want one fielddiag-ist-gpu group with 2 jobs", rack.Failures)
	}
	if out.Verdict != "" {
		t.Errorf("gatherer must not set a verdict, got %q", out.Verdict)
	}
}

func TestProbe_Run_ResolverMissRecorded(t *testing.T) {
	sc := probe.StepContext{Racks: []string{"no-such-rack-us-east-09z"}}
	out, err := New().Run(context.Background(), demoClients(), sc)
	if err != nil {
		t.Fatalf("Run should not bubble a resolver miss: %v", err)
	}
	var body evidence
	_ = json.Unmarshal(out.StructuredData, &body)
	rack := body.PerRack["no-such-rack-us-east-09z"]
	if rack.Queried || rack.Error == "" {
		t.Errorf("resolver miss: queried=%v error=%q", rack.Queried, rack.Error)
	}
}

type fakeResolver struct{ inventoryclient.Resolver }

func (fakeResolver) ResolveBMNs(_ context.Context, _ string) ([]inventoryclient.BMN, error) {
	return []inventoryclient.BMN{{Deviceslot: "dh3-r012-node-01-us-east-01a", BMNName: awxclient.FailingSourceBMN, Zone: "us-east-01a"}}, nil
}

func TestProbe_Run_SourceErrorBubbles(t *testing.T) {
	c := demoClients()
	c.InventoryResolver = fakeResolver{}
	sc := probe.StepContext{Racks: []string{"dh3-r012-us-east-01a"}}
	if _, err := New().Run(context.Background(), c, sc); err == nil {
		t.Fatal("Run returned nil; want bubble-up of per-node source failure")
	}
}

func TestProbe_Run_NilDepsError(t *testing.T) {
	sc := probe.StepContext{Racks: []string{"r"}}
	if _, err := New().Run(context.Background(), probe.Clients{}, sc); err == nil {
		t.Fatal("nil AWXJobClient: want error")
	}
	if _, err := New().Run(context.Background(), probe.Clients{AWXJobClient: awxclient.NewMapClient(nil)}, sc); err == nil {
		t.Fatal("nil InventoryResolver: want error")
	}
}

func TestProbe_Run_LokiAndLifecycleOptional(t *testing.T) {
	// With no LokiClient/LifecycleClient the probe still gathers per-node jobs.
	c := probe.Clients{
		AWXJobClient:      awxclient.NewMapClient(awxclient.SeedDemoAWXJobs()),
		InventoryResolver: inventoryclient.NewMapResolverWithBMNs(inventoryclient.SeedDemoFixtures(), inventoryclient.SeedDemoBMNs()),
	}
	out, err := New().Run(context.Background(), c, probe.StepContext{Racks: []string{"dh3-r012-us-east-01a"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var body evidence
	_ = json.Unmarshal(out.StructuredData, &body)
	rack := body.PerRack["dh3-r012-us-east-01a"]
	if len(rack.Nodes) != 2 || len(rack.L11) != 0 {
		t.Errorf("nodes=%d l11=%d, want 2/0 without loki", len(rack.Nodes), len(rack.L11))
	}
}

func TestParseConfig(t *testing.T) {
	lts, pt, lb := parseConfig(nil)
	if len(lts) != 2 || pt != 5 || lb != defaultL11Lookback {
		t.Errorf("nil config = %v/%d/%v", lts, pt, lb)
	}
	lts, pt, lb = parseConfig(map[string]any{"limit_types": []any{"bmc"}, "per_target": float64(10), "l11_lookback": "12h"})
	if len(lts) != 1 || lts[0] != "bmc" || pt != 10 || lb.Hours() != 12 {
		t.Errorf("override = %v/%d/%v", lts, pt, lb)
	}
	lts, _, _ = parseConfig(map[string]any{"limit_type": "mgmt"})
	if len(lts) != 1 || lts[0] != "mgmt" {
		t.Errorf("singular limit_type = %v", lts)
	}
}
