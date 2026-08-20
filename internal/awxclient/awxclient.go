// Package awxclient is a read-only source of AWX job history for a rack's
// compute nodes. awx_job_probe uses it to gather the firmware-zap jobs FLCC
// and RLCC launched against each canary node — fwmanager (node-zap),
// dpu-update (dpu-zap), and fielddiag-ist-gpu / metal-fielddiag (fielddiag /
// l11-fielddiag) — with their status and timing as evidence. A cross-run diff
// then shows which node's which job changed across a firmware release.
//
// Source: the `awxctl` CLI (github.com/coreweave/awxctl), which owns
// credentials and AWX region resolution. qac only builds the invocation,
// decodes awxctl's JSON output, and maps it onto Job.
//
// # Read-only envelope (§0.25, ported from fwqual/internal/awx)
//
// This package may shell out ONLY to verbs that read job status / metadata
// (today: `awxctl job info bmn ... -o json`). It must NEVER call
// `awxctl job launch`, `awxctl job cancel`, `awxctl job retry`, or any other
// mutating verb. qac's role on the AWX boundary is observation, not
// orchestration. The Client interface exposes a single method, JobsForBMNs;
// adding a second is a deliberate scope expansion that envelope_test.go and
// reviewer attention together guard.
package awxclient

import (
	"context"
	"errors"
	"time"
)

// Job is one AWX job as reported by `awxctl job info ... -o json`, narrowed to
// the fields a firmware-release canary cares about. BMNName is the short BMN
// the job targeted (awxctl's "name" field); Slot is the canonical deviceslot
// awxctl resolved; Stage is derived from Template via StageForTemplate.
// Read-only observation — qac never mutates a job.
//
// Note: `job info` carries NO failure detail (no error codes, even for failed
// jobs — validated against live awxctl). Failure signatures come only from
// AnalyzeFailures.
type Job struct {
	JobID    int       `json:"job_id"`
	BMNName  string    `json:"bmn_name"`
	SKU      string    `json:"sku,omitempty"`
	Slot     string    `json:"slot,omitempty"`
	Template string    `json:"template"`
	Stage    string    `json:"stage,omitempty"`
	Limit    string    `json:"limit,omitempty"`
	Status   string    `json:"status"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	Elapsed  float64   `json:"elapsed_seconds"`
	AWXLink  string    `json:"awx_link,omitempty"`
}

// Options scopes a JobsForBMNs query. LimitType is required by awxctl's bmn
// mode ("mgmt" resolves the node management IP, "bmc" the BMC IP); the
// firmware-zap jobs in the stage table run over mgmt. PerTarget is awxctl's
// -n (jobs per BMN). Template, when non-empty, narrows to a single awxctl -t
// template; empty captures every template (the probe categorises afterward).
type Options struct {
	LimitType string
	PerTarget int
	Template  string
}

// Client reads AWX job history. Implementations must be goroutine-safe and
// read-only — every method maps to a read-only awxctl verb (`job info ...`);
// none launch, cancel, or retry (see the §0.25 envelope note above).
type Client interface {
	// JobsForBMNs returns recent AWX jobs for the given short BMN names,
	// scoped by opts. Used for the per-node zap stages (node-zap/dpu-zap/
	// fielddiag). An empty BMN list returns an empty slice, not an error; a
	// source-side failure (binary missing, awxctl error) returns a wrapped
	// ErrSourceUnavailable. Verb: `awxctl job info bmn`.
	JobsForBMNs(ctx context.Context, bmns []string, opts Options) ([]Job, error)

	// JobByID returns one AWX job by id in the given region. Used for the
	// rack-wide l11-fielddiag job, whose id comes from Loki (it spans many
	// mgmt IPs and can't be found via JobsForBMNs). Verb: `awxctl job info id`.
	JobByID(ctx context.Context, region string, jobID int) (Job, error)

	// AnalyzeFailures groups the given failed jobs by failure signature
	// (template + failed task + MODS codes + error message). This is the only
	// source of failure detail — the job listing carries none. Verb:
	// `awxctl job info id ... --errsort`.
	AnalyzeFailures(ctx context.Context, region string, jobIDs []int) ([]FailureGroup, error)
}

// ErrSourceUnavailable signals a source-side failure (awxctl missing, exec
// error, undecodable output). Callers may map it onto a StepFailed event.
var ErrSourceUnavailable = errors.New("awxclient: source unavailable")

// canaryStages maps a firmware-zap AWX job template to the FLCC/RLCC workflow
// stage it drives. The fielddiag stage covers both fielddiag-ist-gpu and
// metal-fielddiag; it is launched per-node (FLCC "fielddiag") and rack-wide
// (RLCC "l11-fielddiag"), which the template alone can't distinguish — the
// job's limit width hints at l11.
var canaryStages = map[string]string{
	"fwmanager":         "node-zap",
	"dpu-update":        "dpu-zap",
	"fielddiag-ist-gpu": "fielddiag",
	"metal-fielddiag":   "fielddiag",
}

// StageForTemplate returns the firmware-zap workflow stage an AWX job template
// drives, or "" for a template outside the canary set.
func StageForTemplate(template string) string {
	return canaryStages[template]
}
