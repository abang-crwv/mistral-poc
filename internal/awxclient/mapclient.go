package awxclient

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// FailingSourceBMN is a sentinel BMN name. A JobsForBMNs call whose target
// list contains it returns ErrSourceUnavailable, so probe tests can exercise
// the transport-failure path without a real awxctl.
const FailingSourceBMN = "failing-source-bmn"

// FailingSourceJobID is a sentinel job id. JobByID / AnalyzeFailures against it
// return ErrSourceUnavailable, for the l11 transport-failure path.
const FailingSourceJobID = -1

// MapClient is an in-memory Client backed by a BMN→jobs table. It is the
// QAC_AWX_BACKEND=map / no-awxctl backend, and the fixture for tests. It also
// indexes every seeded job by id (for JobByID) and synthesizes failure groups
// from failed jobs (for AnalyzeFailures).
type MapClient struct {
	byBMN map[string][]Job
	byID  map[int]Job
}

// NewMapClient returns a MapClient over the given BMN→jobs table (a nil table
// is empty). Every seeded job is also indexed by id so JobByID can resolve the
// rack-wide l11 job (seeded under the rack key, not a real BMN).
func NewMapClient(byBMN map[string][]Job) *MapClient {
	if byBMN == nil {
		byBMN = map[string][]Job{}
	}
	byID := map[int]Job{}
	for _, jobs := range byBMN {
		for _, j := range jobs {
			byID[j.JobID] = j
		}
	}
	return &MapClient{byBMN: byBMN, byID: byID}
}

// Compile-time satisfaction check.
var _ Client = (*MapClient)(nil)

// JobsForBMNs returns the seeded jobs for the given BMNs, filtered by
// opts.LimitType and opts.Template. To model awxctl's IP resolution — `-l
// mgmt|bmc` matches only jobs whose limit ran against that IP — a seeded job
// is returned for a limit-type only when its Limit ends with "-<limit_type>"
// (e.g. "s90txs51-bmc"); a job with no suffixed Limit matches any limit-type.
// The FailingSourceBMN sentinel triggers ErrSourceUnavailable. Stage is
// (re)derived so fixtures need not set it.
func (m *MapClient) JobsForBMNs(_ context.Context, bmns []string, opts Options) ([]Job, error) {
	var jobs []Job
	for _, bmn := range bmns {
		if bmn == FailingSourceBMN {
			return nil, fmt.Errorf("%w: seeded failing source for %s", ErrSourceUnavailable, bmn)
		}
		for _, j := range m.byBMN[bmn] {
			if opts.Template != "" && j.Template != opts.Template {
				continue
			}
			if !limitMatches(j.Limit, opts.LimitType) {
				continue
			}
			j.BMNName = bmn
			j.Stage = StageForTemplate(j.Template)
			jobs = append(jobs, j)
		}
	}
	sortJobs(jobs)
	return jobs, nil
}

// JobByID returns the seeded job with the given id. The FailingSourceJobID
// sentinel triggers ErrSourceUnavailable; an unseeded id is not-found.
func (m *MapClient) JobByID(_ context.Context, _ string, jobID int) (Job, error) {
	if jobID == FailingSourceJobID {
		return Job{}, fmt.Errorf("%w: seeded failing job id", ErrSourceUnavailable)
	}
	j, ok := m.byID[jobID]
	if !ok {
		return Job{}, fmt.Errorf("%w: no seeded job %d", ErrSourceUnavailable, jobID)
	}
	j.Stage = StageForTemplate(j.Template)
	return j, nil
}

// AnalyzeFailures synthesizes one failure group per distinct template among the
// requested job ids that are seeded as failed (mirroring errsort, which only
// groups failed jobs). The FailingSourceJobID sentinel triggers
// ErrSourceUnavailable. The signature detail is demo-shaped.
func (m *MapClient) AnalyzeFailures(_ context.Context, _ string, jobIDs []int) ([]FailureGroup, error) {
	byTemplate := map[string]*FailureGroup{}
	var order []string
	for _, id := range jobIDs {
		if id == FailingSourceJobID {
			return nil, fmt.Errorf("%w: seeded failing job id", ErrSourceUnavailable)
		}
		j, ok := m.byID[id]
		if !ok || j.Status != "failed" {
			continue
		}
		g := byTemplate[j.Template]
		if g == nil {
			g = &FailureGroup{
				Template:     j.Template,
				FailedTask:   "demo failed task",
				ErrorMessage: "demo failure",
				ModsCodes:    []string{"288"},
				ModsNotes:    []string{"Power is below specified limit"},
			}
			byTemplate[j.Template] = g
			order = append(order, j.Template)
		}
		g.JobIDs = append(g.JobIDs, id)
		g.JobCount++
	}
	groups := make([]FailureGroup, 0, len(order))
	for _, tmpl := range order {
		groups = append(groups, *byTemplate[tmpl])
	}
	return groups, nil
}

// limitMatches reports whether a seeded job's Limit belongs to the requested
// limit-type. An empty limitType (no filter) or a Limit without a "-mgmt" /
// "-bmc" suffix matches anything.
func limitMatches(limit, limitType string) bool {
	if limitType == "" {
		return true
	}
	if !strings.HasSuffix(limit, "-mgmt") && !strings.HasSuffix(limit, "-bmc") {
		return true
	}
	return strings.HasSuffix(limit, "-"+limitType)
}

// SeedDemoAWXJobs returns a deterministic BMN→jobs table for the demo rack
// dh3-r012-us-east-01a, keyed by the same short BMN names SeedDemoBMNs emits
// (s90txs51…). Two nodes carry a full zap trio (fwmanager + dpu-update +
// fielddiag-ist-gpu); one fielddiag is failed so the demo rollup shows a
// non-successful stage. The rack key holds the two rack-wide l11-fielddiag
// jobs (ids matching lokiclient.SeedDemoL11Logs) so JobByID resolves them;
// those never appear in JobsForBMNs (never queried by the rack key).
func SeedDemoAWXJobs() map[string][]Job {
	base := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	// ipType selects the limit suffix per template, mirroring the stage→IP
	// table: dpu-update runs over bmc, the rest over mgmt.
	mk := func(bmn, template, status string, jobID int, elapsed float64) Job {
		started := base.Add(time.Duration(jobID%100) * time.Minute)
		ipType := "mgmt"
		if template == "dpu-update" {
			ipType = "bmc"
		}
		return Job{
			JobID:    jobID,
			BMNName:  bmn,
			SKU:      "gb200-4x",
			Template: template,
			Status:   status,
			Limit:    bmn + "-" + ipType,
			Started:  started,
			Finished: started.Add(time.Duration(elapsed) * time.Second),
			Elapsed:  elapsed,
			AWXLink:  fmt.Sprintf("https://awx.us-east-01a.int.coreweave.com/#/jobs/playbook/%d", jobID),
		}
	}
	// l11 makes a rack-wide job (id from lokiclient.SeedDemoL11Logs) with a
	// multi-IP limit and no per-node BMN. 883990 is a failed prior attempt.
	l11 := func(status string, jobID int) Job {
		j := mk("dh3-r012-us-east-01a", "fielddiag-ist-gpu", status, jobID, 1600)
		j.Limit = "10.0.0.1,10.0.0.3,10.0.0.5"
		j.SKU, j.Slot = "N/A", "N/A"
		return j
	}
	return map[string][]Job{
		"s90txs51": {
			mk("s90txs51", "fwmanager", "successful", 884201, 612),
			mk("s90txs51", "dpu-update", "successful", 884255, 188),
			mk("s90txs51", "fielddiag-ist-gpu", "successful", 884310, 1455),
		},
		"s90txs52": {
			mk("s90txs52", "fwmanager", "successful", 884202, 633),
			mk("s90txs52", "dpu-update", "successful", 884256, 201),
			mk("s90txs52", "fielddiag-ist-gpu", "failed", 884311, 437),
		},
		"dh3-r012-us-east-01a": {
			l11("running", 884321),
			l11("failed", 883990),
		},
	}
}
