// Package awxjobprobe implements awx_job_probe — a gatherer that snapshots the
// AWX firmware-zap jobs FLCC and RLCC ran against each canary rack, why they
// failed, and how often. For every rack it:
//
//   - enumerates the node short BMNs (inventory) and reads each node's FLCC
//     state (lifecycle client);
//   - pulls the per-node zap-stage jobs — node-zap (fwmanager, mgmt), dpu-zap
//     (dpu-update, bmc), fielddiag (fielddiag-ist-gpu/metal-fielddiag, mgmt) —
//     via `awxctl job info bmn` over both mgmt and bmc;
//   - pulls the rack-wide l11-fielddiag job separately: RLCC logs its AWX job
//     link (the job spans many mgmt IPs, so `job info bmn` can't find it), so
//     the probe reads the link from Loki and fetches it via `job info id`;
//   - for the failed jobs, groups them by failure signature via
//     `awxctl ... --errsort` (template + failed task + MODS codes) — the only
//     source of failure detail;
//   - derives per-node/per-stage chronology: attempts, failures, and an
//     outcome class (clean / retry-clean / persistent-fail).
//
// No verdict (gatherer). A resolver/awxctl source failure bubbles to
// StepFailed; the supplementary signals (FLCC state, l11 Loki lookup, failure
// analysis) are recorded per-rack and never fail the step on their own.
package awxjobprobe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"qac/internal/awxclient"
	"qac/internal/inventoryclient"
	"qac/internal/lokiclient"
	"qac/internal/probe"
)

// Probe snapshots per-rack AWX zap-job history + failure analysis.
type Probe struct{}

// New returns a zero-value Probe.
func New() *Probe { return &Probe{} }

var _ probe.Probe = (*Probe)(nil)

func (*Probe) Type() string             { return "awx_job_probe" }
func (*Probe) Category() probe.Category { return probe.CategoryGatherer }

const source = "awxctl-job-info"

const defaultPerTarget = 5

// defaultLimitTypes: node-zap/fielddiag run over mgmt, dpu-zap over bmc.
var defaultLimitTypes = []string{"mgmt", "bmc"}

// defaultL11Lookback bounds the Loki search for the rack's l11-fielddiag job.
const defaultL11Lookback = 48 * time.Hour

// jobView is one per-node AWX job in evidence.
type jobView struct {
	JobID     int       `json:"job_id"`
	BMN       string    `json:"bmn"`
	Template  string    `json:"template"`
	Stage     string    `json:"stage,omitempty"`
	Status    string    `json:"status"`
	LimitType string    `json:"limit_type"`
	Limit     string    `json:"limit,omitempty"`
	Started   time.Time `json:"started"`
	Finished  time.Time `json:"finished"`
	Elapsed   float64   `json:"elapsed_seconds"`
}

// stageChrono is one node's history for one zap stage — the Q2/Q3 answers.
// Outcome: clean (passed, never failed) / retry-clean (passed after ≥1 fail) /
// persistent-fail (latest attempt failed) / running / none.
type stageChrono struct {
	Attempts     int    `json:"attempts"`
	Failures     int    `json:"failures"`
	Successes    int    `json:"successes"`
	LatestStatus string `json:"latest_status"`
	Outcome      string `json:"outcome"`
}

// nodeView is one compute node: its FLCC state, its zap jobs, and per-stage
// chronology.
type nodeView struct {
	BMN          string                 `json:"bmn"`
	Deviceslot   string                 `json:"deviceslot"`
	FLCCState    string                 `json:"flcc_state,omitempty"`
	FLCCWorkflow string                 `json:"flcc_workflow,omitempty"`
	Jobs         []jobView              `json:"jobs"`
	Stages       map[string]stageChrono `json:"stages"`
}

// l11View is the rack-wide l11-fielddiag job (RLCC-launched, from Loki+job id).
type l11View struct {
	JobID      int       `json:"job_id"`
	Region     string    `json:"region"`
	Status     string    `json:"status"`
	Limit      string    `json:"limit,omitempty"`
	Started    time.Time `json:"started"`
	Finished   time.Time `json:"finished"`
	LaunchedAt time.Time `json:"launched_at"`
}

// rackJobs is the per-rack body.
type rackJobs struct {
	Queried       bool                      `json:"queried"`
	Region        string                    `json:"region,omitempty"`
	BMNCount      int                       `json:"bmn_count"`
	Nodes         []nodeView                `json:"nodes"`
	L11           []l11View                 `json:"l11"`
	L11Error      string                    `json:"l11_error,omitempty"`
	ByStageStatus map[string]map[string]int `json:"by_stage_status"`
	Failures      []awxclient.FailureGroup  `json:"failures"`
	FailuresError string                    `json:"failures_error,omitempty"`
	Error         string                    `json:"error,omitempty"`
}

type awxEvidence struct {
	ProbedAt    int64               `json:"probed_at"`
	Source      string              `json:"source"`
	LimitTypes  []string            `json:"limit_types"`
	PerTarget   int                 `json:"per_target"`
	L11Lookback string              `json:"l11_lookback"`
	PerRack     map[string]rackJobs `json:"per_rack"`
}

// Run satisfies probe.Probe.
func (*Probe) Run(ctx context.Context, clients probe.Clients, sc probe.StepContext) (probe.Output, error) {
	if clients.AWXJobClient == nil {
		return probe.Output{}, fmt.Errorf("awx_job_probe: clients.AWXJobClient is nil")
	}
	if clients.InventoryResolver == nil {
		return probe.Output{}, fmt.Errorf("awx_job_probe: clients.InventoryResolver is nil")
	}

	limitTypes, perTarget, l11Lookback := parseConfig(sc.Config)
	perRack := make(map[string]rackJobs, len(sc.Racks))

	for _, rack := range sc.Racks {
		rj, err := runRack(ctx, clients, rack, limitTypes, perTarget, l11Lookback)
		if err != nil {
			return probe.Output{}, err
		}
		perRack[rack] = rj
	}

	probedAt := time.Now().Unix()
	ev := awxEvidence{
		ProbedAt:    probedAt,
		Source:      source,
		LimitTypes:  limitTypes,
		PerTarget:   perTarget,
		L11Lookback: l11Lookback.String(),
		PerRack:     perRack,
	}
	structured, err := json.Marshal(ev)
	if err != nil {
		return probe.Output{}, fmt.Errorf("awx_job_probe: marshal output: %w", err)
	}
	return probe.Output{StructuredData: structured, Sources: []string{source}, ProbedAt: probedAt}, nil
}

// runRack gathers one rack. A resolver miss or awxctl per-node source failure
// returns (partial, err) only when it must fail the step; supplementary signals
// are recorded in the returned rackJobs.
func runRack(ctx context.Context, clients probe.Clients, rack string, limitTypes []string, perTarget int, l11Lookback time.Duration) (rackJobs, error) {
	bmns, err := clients.InventoryResolver.ResolveBMNs(ctx, rack)
	if err != nil {
		if errors.Is(err, inventoryclient.ErrNotFound) || errors.Is(err, inventoryclient.ErrEmptyRack) {
			return rackJobs{Error: err.Error()}, nil
		}
		return rackJobs{}, fmt.Errorf("awx_job_probe: resolve BMNs %s: %w", rack, err)
	}

	// Query targets + deviceslot/region context.
	names := make([]string, 0, len(bmns))
	slotByBMN := make(map[string]string, len(bmns))
	deviceslots := make([]string, 0, len(bmns))
	region := ""
	for _, b := range bmns {
		if region == "" && b.Zone != "" {
			region = b.Zone
		}
		deviceslots = append(deviceslots, b.Deviceslot)
		if b.BMNName == "" {
			continue // tray with no BMN — a data gap, not a query target
		}
		names = append(names, b.BMNName)
		slotByBMN[b.BMNName] = b.Deviceslot
	}

	// FLCC state per node (supplementary; best-effort).
	flccBySlot := map[string]flccState{}
	if clients.LifecycleClient != nil {
		if obs, err := clients.LifecycleClient.QueryFLCCState(ctx, deviceslots); err == nil {
			for slot, o := range obs {
				flccBySlot[slot] = flccState{State: o.State, Workflow: o.Workflow}
			}
		}
	}

	// Per-node zap jobs: union of both limit-types, deduped by job id.
	nodesBySlot := map[string]*nodeView{}
	rollup := map[string]map[string]int{}
	seen := map[int]bool{}
	var failedIDs []int
	for _, lt := range limitTypes {
		jobs, err := clients.AWXJobClient.JobsForBMNs(ctx, names, awxclient.Options{LimitType: lt, PerTarget: perTarget})
		if err != nil {
			return rackJobs{}, fmt.Errorf("awx_job_probe: gather %s jobs for %s: %w", lt, rack, err)
		}
		for _, j := range jobs {
			if seen[j.JobID] {
				continue
			}
			seen[j.JobID] = true
			slot := j.Slot
			if slot == "" {
				slot = slotByBMN[j.BMNName]
			}
			nv := nodesBySlot[slot]
			if nv == nil {
				nv = &nodeView{BMN: j.BMNName, Deviceslot: slot}
				if fs, ok := flccBySlot[slot]; ok {
					nv.FLCCState, nv.FLCCWorkflow = fs.State, fs.Workflow
				}
				nodesBySlot[slot] = nv
			}
			nv.Jobs = append(nv.Jobs, jobView{
				JobID: j.JobID, BMN: j.BMNName, Template: j.Template, Stage: j.Stage,
				Status: j.Status, LimitType: lt, Limit: j.Limit,
				Started: j.Started, Finished: j.Finished, Elapsed: j.Elapsed,
			})
			addRollup(rollup, j.Stage, j.Status)
			if j.Status == "failed" {
				failedIDs = append(failedIDs, j.JobID)
			}
		}
	}

	rj := rackJobs{Queried: true, Region: region, BMNCount: len(names), ByStageStatus: rollup}

	// l11-fielddiag: rack-wide job link from Loki → job info id (supplementary).
	if clients.LokiClient != nil {
		l11, ferr, terr := gatherL11(ctx, clients, region, rack, l11Lookback)
		if terr != nil {
			return rackJobs{}, terr
		}
		rj.L11 = l11
		if ferr != "" {
			rj.L11Error = ferr
		}
		for _, v := range l11 {
			if v.Status == "failed" {
				failedIDs = append(failedIDs, v.JobID)
			}
		}
	}

	// Failure signatures for the failed jobs (supplementary).
	if len(failedIDs) > 0 && region != "" {
		groups, err := clients.AWXJobClient.AnalyzeFailures(ctx, region, failedIDs)
		if err != nil {
			rj.FailuresError = err.Error()
		} else {
			rj.Failures = groups
		}
	}

	// Finalize per-node chronology, sorted by deviceslot.
	rj.Nodes = finalizeNodes(nodesBySlot)
	return rj, nil
}

// gatherL11 reads the rack's l11-fielddiag job link(s) from Loki and fetches
// each job by id. Returns (views, recordedErr, terminalErr): a Loki/awx source
// failure is recorded (recordedErr) rather than failing the step, since l11 is
// supplementary; terminalErr is reserved for future hard failures.
func gatherL11(ctx context.Context, clients probe.Clients, region, rack string, lookback time.Duration) ([]l11View, string, error) {
	if region == "" {
		return nil, "no region resolved for rack", nil
	}
	refs, err := lokiclient.L11FieldDiagJobs(ctx, clients.LokiClient, region, rack, lookback)
	if err != nil {
		return nil, fmt.Sprintf("loki l11 lookup: %v", err), nil
	}
	var out []l11View
	seen := map[int]bool{}
	for _, r := range refs {
		if seen[r.JobID] {
			continue
		}
		seen[r.JobID] = true
		v := l11View{JobID: r.JobID, Region: r.Region, LaunchedAt: r.LaunchedAt, Status: "unknown"}
		if job, err := clients.AWXJobClient.JobByID(ctx, r.Region, r.JobID); err == nil {
			v.Status, v.Limit, v.Started, v.Finished = job.Status, job.Limit, job.Started, job.Finished
		}
		out = append(out, v)
	}
	return out, "", nil
}

// flccState is the narrow FLCC snapshot the probe records per node.
type flccState struct{ State, Workflow string }

func addRollup(rollup map[string]map[string]int, stage, status string) {
	bucket := stage
	if bucket == "" {
		bucket = "other"
	}
	if rollup[bucket] == nil {
		rollup[bucket] = map[string]int{}
	}
	rollup[bucket][status]++
}

// finalizeNodes computes each node's per-stage chronology and returns the nodes
// sorted by deviceslot for deterministic evidence.
func finalizeNodes(nodesBySlot map[string]*nodeView) []nodeView {
	out := make([]nodeView, 0, len(nodesBySlot))
	for _, nv := range nodesBySlot {
		// Order jobs oldest→newest so "latest" is well defined.
		sort.SliceStable(nv.Jobs, func(i, j int) bool { return nv.Jobs[i].Started.Before(nv.Jobs[j].Started) })
		nv.Stages = chronologyByStage(nv.Jobs)
		out = append(out, *nv)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Deviceslot < out[j].Deviceslot })
	return out
}

// chronologyByStage groups a node's jobs (already oldest→newest) by stage and
// classifies the outcome.
func chronologyByStage(jobs []jobView) map[string]stageChrono {
	stages := map[string]stageChrono{}
	for _, j := range jobs {
		stage := j.Stage
		if stage == "" {
			stage = "other"
		}
		c := stages[stage]
		c.Attempts++
		switch j.Status {
		case "failed":
			c.Failures++
		case "successful":
			c.Successes++
		}
		c.LatestStatus = j.Status // jobs are oldest→newest, so last wins
		stages[stage] = c
	}
	for stage, c := range stages {
		c.Outcome = classifyOutcome(c)
		stages[stage] = c
	}
	return stages
}

func classifyOutcome(c stageChrono) string {
	switch c.LatestStatus {
	case "running", "pending", "waiting":
		return "running"
	case "successful":
		if c.Failures > 0 {
			return "retry-clean"
		}
		return "clean"
	case "failed":
		return "persistent-fail"
	default:
		return "mixed"
	}
}

// parseConfig reads optional limit_types, per_target, and l11_lookback from the
// step config (action.args).
func parseConfig(cfg map[string]any) (limitTypes []string, perTarget int, l11Lookback time.Duration) {
	perTarget = defaultPerTarget
	l11Lookback = defaultL11Lookback
	if cfg == nil {
		return defaultLimitTypes, perTarget, l11Lookback
	}

	var raw []string
	if v, ok := cfg["limit_type"].(string); ok {
		raw = append(raw, v)
	}
	if vs, ok := cfg["limit_types"].([]any); ok {
		for _, v := range vs {
			if s, ok := v.(string); ok {
				raw = append(raw, s)
			}
		}
	}
	seen := map[string]bool{}
	for _, s := range raw {
		if (s == "mgmt" || s == "bmc") && !seen[s] {
			seen[s] = true
			limitTypes = append(limitTypes, s)
		}
	}
	if len(limitTypes) == 0 {
		limitTypes = defaultLimitTypes
	}

	switch n := cfg["per_target"].(type) {
	case float64:
		if n > 0 {
			perTarget = int(n)
		}
	case int:
		if n > 0 {
			perTarget = n
		}
	}
	if v, ok := cfg["l11_lookback"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			l11Lookback = d
		}
	}
	return limitTypes, perTarget, l11Lookback
}
