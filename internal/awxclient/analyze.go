package awxclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// HostFailure is one host's MODS-level failure within a FailureGroup: the
// component (GPU PCI address) and CoreWeave MODS code that failed.
type HostFailure struct {
	Host        string `json:"host"`
	Code        string `json:"code"`
	ComponentID string `json:"component_id"`
	Notes       string `json:"notes"`
	Count       int    `json:"count"`
}

// NVTriage is the NVIDIA triage guidance awxctl attaches to a MODS failure
// (section, code, and recommended actions like PROD_FIT / REPORT_NV_BUG).
type NVTriage struct {
	Section   string   `json:"section"`
	SectionID string   `json:"section_id"`
	CodeLast3 string   `json:"code_last3"`
	Message   string   `json:"message"`
	Actions   []string `json:"actions"`
}

// FailureGroup is one failure signature from `awxctl ... --errsort`: jobs that
// failed the same way, grouped by template + failed task + MODS codes + error
// message. This is the only source of failure detail (job listings carry none)
// and answers "did they fail on the same task and errors?".
type FailureGroup struct {
	Template     string        `json:"template"`
	FailedTask   string        `json:"failed_task"`
	ErrorMessage string        `json:"error_message"`
	ModsCodes    []string      `json:"mods_codes"`
	ModsNotes    []string      `json:"mods_notes"`
	HostFailures []HostFailure `json:"host_failures"`
	NVTriages    []NVTriage    `json:"nv_triages"`
	RunSpec      string        `json:"run_spec"`
	ModsWikiURL  string        `json:"mods_wiki_url"`
	JobCount     int           `json:"job_count"`
	JobIDs       []int         `json:"job_ids"`
}

// rawFailureGroup decodes the errsort JSON, whose jobs are nested objects; the
// exported FailureGroup flattens them to job ids.
type rawFailureGroup struct {
	FailureGroup
	Jobs []struct {
		JobID int `json:"job_id"`
	} `json:"jobs"`
}

// JobByID runs `awxctl job info id <id> -r <region> -o json` and returns the
// single job. Used for the rack-wide l11-fielddiag job (id from Loki).
func (c *CLIClient) JobByID(ctx context.Context, region string, jobID int) (Job, error) {
	args := []string{"job", "info", "id", strconv.Itoa(jobID), "-r", region, "-o", "json"}
	out, err := c.run(ctx, fmt.Sprintf("awxctl job info id %d", jobID), args...)
	if err != nil {
		return Job{}, err
	}
	var raw []displayableJob
	if err := json.Unmarshal(out, &raw); err != nil {
		return Job{}, fmt.Errorf("%w: decode job info id %d: %v", ErrSourceUnavailable, jobID, err)
	}
	if len(raw) == 0 {
		return Job{}, fmt.Errorf("%w: job info id %d returned no job", ErrSourceUnavailable, jobID)
	}
	return toJob(raw[0]), nil
}

// AnalyzeFailures runs `awxctl job info id <ids> -r <region> --errsort -o json`
// and returns the failure-signature groups. An empty job list short-circuits.
// errsort only groups failed jobs, so a set with no failures yields no groups.
func (c *CLIClient) AnalyzeFailures(ctx context.Context, region string, jobIDs []int) ([]FailureGroup, error) {
	if len(jobIDs) == 0 {
		return nil, nil
	}
	args := make([]string, 0, len(jobIDs)+6)
	args = append(args, "job", "info", "id")
	for _, id := range jobIDs {
		args = append(args, strconv.Itoa(id))
	}
	args = append(args, "-r", region, "--errsort", "-o", "json")

	out, err := c.run(ctx, fmt.Sprintf("awxctl job info id --errsort (jobs=%d)", len(jobIDs)), args...)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	var raw []rawFailureGroup
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("%w: decode errsort output (jobs=%d): %v", ErrSourceUnavailable, len(jobIDs), err)
	}
	groups := make([]FailureGroup, 0, len(raw))
	for _, r := range raw {
		g := r.FailureGroup
		g.JobIDs = make([]int, 0, len(r.Jobs))
		for _, j := range r.Jobs {
			g.JobIDs = append(g.JobIDs, j.JobID)
		}
		groups = append(groups, g)
	}
	return groups, nil
}
