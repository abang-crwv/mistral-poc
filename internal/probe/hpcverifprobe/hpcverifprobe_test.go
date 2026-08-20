package hpcverifprobe

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"qac/internal/hpcverifclient"
	"qac/internal/probe"
)

// coverageEvidence decodes the coverage/freshness fields added on top of the
// original failure evidence.
type coverageEvidence struct {
	AnyStale       bool `json:"any_stale"`
	AnyNotVerified bool `json:"any_not_verified"`
	PerRack        map[string]struct {
		Status                string `json:"status"`
		FailureCount          int    `json:"failure_count"`
		TestedCount           int    `json:"tested_count"`
		PassedCount           int    `json:"passed_count"`
		StaleCount            int    `json:"stale_count"`
		OldestHeartbeatAgeSec int64  `json:"oldest_heartbeat_age_sec"`
		StaleNodes            []struct {
			Node            string `json:"node"`
			Deviceslot      string `json:"deviceslot"`
			HeartbeatAgeSec int64  `json:"heartbeat_age_sec"`
		} `json:"stale_nodes"`
	} `json:"per_rack"`
}

const daySec = 24 * 60 * 60

func TestProbe_Run_ClassifiesCoverageAndFreshness(t *testing.T) {
	now := time.Now().Unix()
	fresh := now - 3600     // 1h ago
	stale := now - 3*daySec // 3d ago (default threshold is 48h)

	c := probe.Clients{HPCVerifClient: hpcverifclient.NewMapClient(map[string][]hpcverifclient.NodeFailure{
		"rack-fail": {{Node: "f1", Deviceslot: "rack-fail-node-01", Testcase: "nvlink_bandwidth", Message: "m"}},
	}).WithRoster(map[string][]hpcverifclient.NodeStatus{
		"rack-fail":  {{Node: "f1", Deviceslot: "rack-fail-node-01", LastHeartbeatUnix: fresh}},
		"rack-stale": {{Node: "s1", Deviceslot: "rack-stale-node-01", LastHeartbeatUnix: fresh}, {Node: "s2", Deviceslot: "rack-stale-node-02", LastHeartbeatUnix: stale}},
		"rack-pass":  {{Node: "p1", Deviceslot: "rack-pass-node-01", LastHeartbeatUnix: fresh}, {Node: "p2", Deviceslot: "rack-pass-node-02", LastHeartbeatUnix: fresh}},
		// rack-none: no failures, no roster
	})}
	sc := probe.StepContext{RunID: "x", StepID: "hpc_verification", Racks: []string{"rack-fail", "rack-stale", "rack-pass", "rack-none"}}

	out, err := New().Run(context.Background(), c, sc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var body coverageEvidence
	if err := json.Unmarshal(out.StructuredData, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got := body.PerRack["rack-fail"].Status; got != "failed" {
		t.Errorf("rack-fail status = %q, want failed", got)
	}
	stale2 := body.PerRack["rack-stale"]
	if stale2.Status != "stale" {
		t.Errorf("rack-stale status = %q, want stale", stale2.Status)
	}
	if stale2.TestedCount != 2 || stale2.PassedCount != 1 || stale2.StaleCount != 1 {
		t.Errorf("rack-stale coverage = tested %d passed %d stale %d, want 2/1/1", stale2.TestedCount, stale2.PassedCount, stale2.StaleCount)
	}
	if len(stale2.StaleNodes) != 1 || stale2.StaleNodes[0].Node != "s2" {
		t.Errorf("rack-stale stale_nodes = %+v, want [s2]", stale2.StaleNodes)
	}
	if got := body.PerRack["rack-pass"].Status; got != "passed" {
		t.Errorf("rack-pass status = %q, want passed", got)
	}
	if got := body.PerRack["rack-pass"].PassedCount; got != 2 {
		t.Errorf("rack-pass passed = %d, want 2", got)
	}
	if got := body.PerRack["rack-none"].Status; got != "not_verified" {
		t.Errorf("rack-none status = %q, want not_verified", got)
	}
	if !body.AnyStale {
		t.Errorf("any_stale = false, want true")
	}
	if !body.AnyNotVerified {
		t.Errorf("any_not_verified = false, want true")
	}
}

func TestProbe_Run_StalenessThresholdOverride(t *testing.T) {
	now := time.Now().Unix()
	c := probe.Clients{HPCVerifClient: hpcverifclient.NewMapClient(nil).WithRoster(map[string][]hpcverifclient.NodeStatus{
		"rack-x": {{Node: "n1", Deviceslot: "rack-x-node-01", LastHeartbeatUnix: now - 2*3600}}, // 2h ago
	})}

	// Default 48h threshold: a 2h-old heartbeat is fresh -> passed.
	out, err := New().Run(context.Background(), c, probe.StepContext{Racks: []string{"rack-x"}})
	if err != nil {
		t.Fatalf("Run (default): %v", err)
	}
	var def coverageEvidence
	_ = json.Unmarshal(out.StructuredData, &def)
	if got := def.PerRack["rack-x"].Status; got != "passed" {
		t.Errorf("default threshold: status = %q, want passed", got)
	}

	// Override to 1h: the same 2h-old heartbeat is now stale.
	out, err = New().Run(context.Background(), c, probe.StepContext{Racks: []string{"rack-x"}, Config: map[string]any{"staleness_hours": float64(1)}})
	if err != nil {
		t.Fatalf("Run (override): %v", err)
	}
	var ov coverageEvidence
	_ = json.Unmarshal(out.StructuredData, &ov)
	if got := ov.PerRack["rack-x"].Status; got != "stale" {
		t.Errorf("override threshold: status = %q, want stale", got)
	}
}

func TestProbe_TypeAndCategory(t *testing.T) {
	p := New()
	if p.Type() != "hpc_verification_failure_probe" {
		t.Errorf("Type = %q, want hpc_verification_failure_probe", p.Type())
	}
	if p.Category() != probe.CategoryGatherer {
		t.Errorf("Category = %q, want gatherer", p.Category())
	}
}

type evidence struct {
	ProbedAt int64  `json:"probed_at"`
	Source   string `json:"source"`
	PerRack  map[string]struct {
		Queried      bool           `json:"queried"`
		FailureCount int            `json:"failure_count"`
		ByTestcase   map[string]int `json:"by_testcase"`
		FailingNodes []struct {
			Node       string `json:"node"`
			Deviceslot string `json:"deviceslot"`
			Testcase   string `json:"testcase"`
			Message    string `json:"message"`
		} `json:"failing_nodes"`
		Error string `json:"error"`
	} `json:"per_rack"`
	AnyFailures bool `json:"any_failures"`
}

func TestProbe_Run_HistogramAndFailingNodes(t *testing.T) {
	c := probe.Clients{HPCVerifClient: hpcverifclient.NewMapClient(map[string][]hpcverifclient.NodeFailure{
		"rack-a": {
			{Node: "n1", Deviceslot: "rack-a-node-01", Testcase: "nvlink_bandwidth", Message: "m1"},
			{Node: "n2", Deviceslot: "rack-a-node-02", Testcase: "nvlink_bandwidth", Message: "m2"},
			{Node: "n3", Deviceslot: "rack-a-node-03", Testcase: "gpu_check", Message: "m3"},
		},
	})}
	sc := probe.StepContext{RunID: "x", StepID: "hpc_verification", Racks: []string{"rack-a"}}

	out, err := New().Run(context.Background(), c, sc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var body evidence
	if err := json.Unmarshal(out.StructuredData, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.AnyFailures {
		t.Errorf("any_failures = false, want true")
	}
	if body.Source != "hpc-verification" {
		t.Errorf("source = %q, want hpc-verification", body.Source)
	}
	ra := body.PerRack["rack-a"]
	if !ra.Queried {
		t.Errorf("rack-a queried = false, want true")
	}
	if ra.FailureCount != 3 {
		t.Errorf("failure_count = %d, want 3", ra.FailureCount)
	}
	if ra.ByTestcase["nvlink_bandwidth"] != 2 || ra.ByTestcase["gpu_check"] != 1 {
		t.Errorf("by_testcase = %v, want nvlink_bandwidth:2 gpu_check:1", ra.ByTestcase)
	}
	if len(ra.FailingNodes) != 3 {
		t.Errorf("failing_nodes = %d, want 3", len(ra.FailingNodes))
	}
}

func TestProbe_Run_HealthyRack_NoFailuresNoError(t *testing.T) {
	c := probe.Clients{HPCVerifClient: hpcverifclient.NewMapClient(nil)} // every rack healthy
	sc := probe.StepContext{RunID: "x", StepID: "hpc_verification", Racks: []string{"rack-clean"}}

	out, err := New().Run(context.Background(), c, sc)
	if err != nil {
		t.Fatalf("Run: %v (healthy rack must not error)", err)
	}
	var body evidence
	if err := json.Unmarshal(out.StructuredData, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.AnyFailures {
		t.Errorf("any_failures = true, want false (healthy rack)")
	}
	rc := body.PerRack["rack-clean"]
	if !rc.Queried {
		t.Errorf("rack-clean queried = false, want true")
	}
	if rc.FailureCount != 0 || len(rc.FailingNodes) != 0 {
		t.Errorf("healthy rack should have 0 failures, got count=%d nodes=%d", rc.FailureCount, len(rc.FailingNodes))
	}
}

func TestProbe_Run_SourceError_Bubbles(t *testing.T) {
	c := probe.Clients{HPCVerifClient: hpcverifclient.NewMapClient(nil)}
	sc := probe.StepContext{RunID: "x", StepID: "hpc_verification", Racks: []string{hpcverifclient.FailingSourceRack}}

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
