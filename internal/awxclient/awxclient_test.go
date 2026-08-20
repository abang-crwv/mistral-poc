package awxclient

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestStageForTemplate(t *testing.T) {
	cases := map[string]string{
		"fwmanager":         "node-zap",
		"dpu-update":        "dpu-zap",
		"fielddiag-ist-gpu": "fielddiag",
		"metal-fielddiag":   "fielddiag",
		"something-else":    "",
	}
	for tmpl, want := range cases {
		if got := StageForTemplate(tmpl); got != want {
			t.Errorf("StageForTemplate(%q) = %q, want %q", tmpl, got, want)
		}
	}
}

func TestBuildArgs_ReadOnlyShape(t *testing.T) {
	got := buildArgs([]string{"s90txs51", "s90txs52"}, Options{LimitType: "mgmt", PerTarget: 5})
	want := []string{"job", "info", "bmn", "s90txs51", "s90txs52", "-l", "mgmt", "-n", "5", "-o", "json"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("buildArgs = %v, want %v", got, want)
	}
	// Verb must be the read-only `job info` path — never a mutating verb.
	if got[0] != "job" || got[1] != "info" {
		t.Errorf("envelope: verb = %v, want job info ...", got[:2])
	}
	// Template filter appends -t when set.
	withT := buildArgs([]string{"s1"}, Options{LimitType: "bmc", Template: "fwmanager"})
	if !contains(withT, "-t") || !contains(withT, "fwmanager") {
		t.Errorf("buildArgs with template = %v, want -t fwmanager", withT)
	}
}

func TestMapClient_LimitTypeAndTemplateFilter(t *testing.T) {
	m := NewMapClient(SeedDemoAWXJobs())

	// mgmt pass: fwmanager + fielddiag-ist-gpu for each of the two BMNs (4);
	// dpu-update is bmc-only and must NOT appear.
	mgmt, err := m.JobsForBMNs(context.Background(), []string{"s90txs51", "s90txs52"}, Options{LimitType: "mgmt", PerTarget: 5})
	if err != nil {
		t.Fatalf("JobsForBMNs(mgmt): %v", err)
	}
	if len(mgmt) != 4 {
		t.Fatalf("mgmt jobs = %d, want 4 (fwmanager+fielddiag x2)", len(mgmt))
	}
	for _, j := range mgmt {
		if j.Template == "dpu-update" {
			t.Errorf("dpu-update leaked into mgmt pass: %+v", j)
		}
		if j.Stage != StageForTemplate(j.Template) {
			t.Errorf("job %d stage = %q, want %q", j.JobID, j.Stage, StageForTemplate(j.Template))
		}
	}

	// bmc pass: dpu-update only (2).
	bmc, err := m.JobsForBMNs(context.Background(), []string{"s90txs51", "s90txs52"}, Options{LimitType: "bmc", PerTarget: 5})
	if err != nil {
		t.Fatalf("JobsForBMNs(bmc): %v", err)
	}
	if len(bmc) != 2 {
		t.Fatalf("bmc jobs = %d, want 2 (dpu-update x2)", len(bmc))
	}
	for _, j := range bmc {
		if j.Template != "dpu-update" || j.Stage != "dpu-zap" {
			t.Errorf("bmc job = %+v, want dpu-update/dpu-zap", j)
		}
	}

	// Template filter narrows further (and dpu-update needs the bmc pass).
	only, err := m.JobsForBMNs(context.Background(), []string{"s90txs51", "s90txs52"}, Options{LimitType: "mgmt", Template: "fwmanager"})
	if err != nil {
		t.Fatalf("JobsForBMNs(filter): %v", err)
	}
	if len(only) != 2 {
		t.Fatalf("fwmanager jobs = %d, want 2", len(only))
	}
	// Unseeded BMN -> no jobs, no error.
	none, err := m.JobsForBMNs(context.Background(), []string{"s90txs99"}, Options{LimitType: "mgmt"})
	if err != nil || len(none) != 0 {
		t.Errorf("unseeded BMN: jobs=%d err=%v, want 0/nil", len(none), err)
	}
	// Empty target list -> empty, no error.
	empty, err := m.JobsForBMNs(context.Background(), nil, Options{LimitType: "mgmt"})
	if err != nil || len(empty) != 0 {
		t.Errorf("empty targets: jobs=%d err=%v, want 0/nil", len(empty), err)
	}
}

func TestMapClient_FailingSourceBMN(t *testing.T) {
	m := NewMapClient(SeedDemoAWXJobs())
	_, err := m.JobsForBMNs(context.Background(), []string{"s90txs51", FailingSourceBMN}, Options{LimitType: "mgmt"})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err = %v, want ErrSourceUnavailable", err)
	}
}

func TestCLIClient_DecodesJobInfoJSON(t *testing.T) {
	var gotArgs []string
	c := NewCLIClient("awxctl")
	c.runner = func(_ context.Context, _ string, args ...string) ([]byte, []byte, error) {
		gotArgs = args
		body := `[
			{"name":"s90txs52","sku":"gb200-4x","slot":"02","job_id":884311,"template":"fielddiag-ist-gpu","limit":"s90txs52-mgmt","status":"failed","started":"2026-06-14T09:11:00Z","finished":"2026-06-14T09:18:17Z","elapsed":437,"awx_link":"https://awx.us-east-01a.int.coreweave.com/#/jobs/playbook/884311","error_codes":["GPU_XID_149"]},
			{"name":"s90txs51","sku":"gb200-4x","slot":"01","job_id":884201,"template":"fwmanager","limit":"s90txs51-mgmt","status":"successful","started":"2026-06-14T09:00:00Z","finished":"2026-06-14T09:10:12Z","elapsed":612,"awx_link":"https://awx.us-east-01a.int.coreweave.com/#/jobs/playbook/884201"}
		]`
		return []byte(body), nil, nil
	}
	jobs, err := c.JobsForBMNs(context.Background(), []string{"s90txs51", "s90txs52"}, Options{LimitType: "mgmt", PerTarget: 5})
	if err != nil {
		t.Fatalf("JobsForBMNs: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}
	// Sorted by BMN name: s90txs51 first.
	if jobs[0].BMNName != "s90txs51" || jobs[0].Template != "fwmanager" || jobs[0].Stage != "node-zap" {
		t.Errorf("jobs[0] = %+v, want s90txs51/fwmanager/node-zap", jobs[0])
	}
	// error_codes in the source JSON is ignored (job info carries no reliable
	// failure detail; that comes from AnalyzeFailures).
	if jobs[1].Status != "failed" || jobs[1].Stage != "fielddiag" {
		t.Errorf("jobs[1] = %+v, want failed fielddiag", jobs[1])
	}
	if !jobs[1].Started.Equal(time.Date(2026, 6, 14, 9, 11, 0, 0, time.UTC)) {
		t.Errorf("jobs[1].Started = %v, want 2026-06-14T09:11:00Z", jobs[1].Started)
	}
	if !contains(gotArgs, "json") {
		t.Errorf("args missing -o json: %v", gotArgs)
	}
}

func TestCLIClient_ExitErrorWrapsSourceUnavailable(t *testing.T) {
	c := NewCLIClient("awxctl")
	c.runner = func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
		return nil, []byte("auth failed for https://awx.example/secret?token=abc"), errors.New("exit status 1")
	}
	_, err := c.JobsForBMNs(context.Background(), []string{"s90txs51"}, Options{LimitType: "mgmt"})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err = %v, want ErrSourceUnavailable", err)
	}
}

func TestCLIClient_EmptyTargets(t *testing.T) {
	called := false
	c := NewCLIClient("awxctl")
	c.runner = func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
		called = true
		return nil, nil, nil
	}
	jobs, err := c.JobsForBMNs(context.Background(), nil, Options{LimitType: "mgmt"})
	if err != nil || len(jobs) != 0 {
		t.Errorf("empty targets: jobs=%d err=%v, want 0/nil", len(jobs), err)
	}
	if called {
		t.Error("runner invoked for empty target list; want short-circuit")
	}
}

// jsonRoundTrip guards the evidence-facing JSON tags used by the probe.
func TestJob_JSONTags(t *testing.T) {
	b, _ := json.Marshal(Job{JobID: 1, BMNName: "s1", Template: "fwmanager", Elapsed: 10})
	s := string(b)
	for _, want := range []string{`"job_id"`, `"bmn_name"`, `"elapsed_seconds"`} {
		if !strings.Contains(s, want) {
			t.Errorf("Job JSON missing %s: %s", want, s)
		}
	}
}

func TestCLIClient_JobByID(t *testing.T) {
	var gotArgs []string
	c := NewCLIClient("awxctl")
	c.runner = func(_ context.Context, _ string, args ...string) ([]byte, []byte, error) {
		gotArgs = args
		// job info id returns a one-element array (l11 job: multi-IP limit).
		return []byte(`[{"name":"10.0.0.1,10.0.0.3","job_id":810195,"template":"fielddiag-ist-gpu","limit":"10.0.0.1,10.0.0.3","status":"running","awx_link":"https://awx.us-central-03a.int.coreweave.com/#/jobs/playbook/810195"}]`), nil, nil
	}
	job, err := c.JobByID(context.Background(), "us-central-03a", 810195)
	if err != nil {
		t.Fatalf("JobByID: %v", err)
	}
	if job.JobID != 810195 || job.Template != "fielddiag-ist-gpu" || job.Stage != "fielddiag" {
		t.Errorf("job = %+v, want 810195/fielddiag-ist-gpu/fielddiag", job)
	}
	for _, want := range []string{"id", "810195", "-r", "us-central-03a", "json"} {
		if !contains(gotArgs, want) {
			t.Errorf("args missing %q: %v", want, gotArgs)
		}
	}
}

func TestCLIClient_AnalyzeFailures(t *testing.T) {
	var gotArgs []string
	c := NewCLIClient("awxctl")
	c.runner = func(_ context.Context, _ string, args ...string) ([]byte, []byte, error) {
		gotArgs = args
		return []byte(`[
			{"template":"fielddiag-ist-gpu","failed_task":"Add additional artifacts to tarball","error_message":"non-zero return code","mods_codes":["288"],"mods_notes":["Power is below specified limit"],"host_failures":[{"host":"10.49.210.1","code":"MODS-020000291288","component_id":"0008:06:00.0","notes":"Power is below specified limit","count":1}],"nv_triages":[{"section":"GPU FD","section_id":"4.1","code_last3":"288","message":"Power is below specified limit","actions":["PROD_FIT","REPORT_NV_BUG"]}],"run_spec":"spec.json","job_count":1,"jobs":[{"job_id":131156}]}
		]`), nil, nil
	}
	groups, err := c.AnalyzeFailures(context.Background(), "us-central-08b", []int{131156})
	if err != nil {
		t.Fatalf("AnalyzeFailures: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	g := groups[0]
	if g.FailedTask == "" || len(g.ModsCodes) != 1 || len(g.HostFailures) != 1 || len(g.NVTriages) != 1 {
		t.Errorf("group missing signature detail: %+v", g)
	}
	if g.HostFailures[0].ComponentID != "0008:06:00.0" {
		t.Errorf("host_failure component_id = %q", g.HostFailures[0].ComponentID)
	}
	if len(g.JobIDs) != 1 || g.JobIDs[0] != 131156 {
		t.Errorf("job ids = %v, want [131156]", g.JobIDs)
	}
	if !contains(gotArgs, "--errsort") {
		t.Errorf("args missing --errsort: %v", gotArgs)
	}
	// Empty job list short-circuits (no runner call needed).
	if gs, err := c.AnalyzeFailures(context.Background(), "r", nil); err != nil || gs != nil {
		t.Errorf("empty ids: groups=%v err=%v, want nil/nil", gs, err)
	}
}

func TestMapClient_JobByIDAndAnalyzeFailures(t *testing.T) {
	m := NewMapClient(SeedDemoAWXJobs())
	// Rack-wide l11 job resolvable by id (seeded under the rack key).
	job, err := m.JobByID(context.Background(), "us-east-01a", 884321)
	if err != nil {
		t.Fatalf("JobByID(884321): %v", err)
	}
	if job.Template != "fielddiag-ist-gpu" || job.Stage != "fielddiag" {
		t.Errorf("l11 job = %+v", job)
	}
	// Failing sentinel + unseeded id both error.
	if _, err := m.JobByID(context.Background(), "r", FailingSourceJobID); !errors.Is(err, ErrSourceUnavailable) {
		t.Errorf("FailingSourceJobID err = %v, want ErrSourceUnavailable", err)
	}
	if _, err := m.JobByID(context.Background(), "r", 999999); !errors.Is(err, ErrSourceUnavailable) {
		t.Errorf("unseeded id err = %v, want ErrSourceUnavailable", err)
	}
	// AnalyzeFailures: 884311 (failed fielddiag) yields a group; 884201
	// (successful) yields nothing.
	groups, err := m.AnalyzeFailures(context.Background(), "us-east-01a", []int{884311, 884201})
	if err != nil {
		t.Fatalf("AnalyzeFailures: %v", err)
	}
	if len(groups) != 1 || groups[0].Template != "fielddiag-ist-gpu" || groups[0].JobCount != 1 {
		t.Errorf("groups = %+v, want one fielddiag group with 1 job", groups)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
