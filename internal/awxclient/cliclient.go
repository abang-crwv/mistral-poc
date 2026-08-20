package awxclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// defaultBinary is the awxctl binary name, resolved via $PATH.
const defaultBinary = "awxctl"

// Runner executes a subprocess and returns stdout, stderr, and the exit error.
// Injectable so tests can fake awxctl without a binary on $PATH.
type Runner func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)

func defaultRunner(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return out.Bytes(), errb.Bytes(), err
}

// CLIClient shells out to awxctl. Zero-value is unusable; construct via
// NewCLIClient.
type CLIClient struct {
	binary string
	runner Runner
}

// NewCLIClient returns a CLIClient that resolves awxctl via $PATH. An empty
// binary falls back to "awxctl".
func NewCLIClient(binary string) *CLIClient {
	if binary == "" {
		binary = defaultBinary
	}
	return &CLIClient{binary: binary, runner: defaultRunner}
}

// Compile-time satisfaction check.
var _ Client = (*CLIClient)(nil)

// Available reports whether the awxctl binary is resolvable on $PATH. serve.go
// uses it to fall back to the map backend when awxctl isn't installed (CI,
// dev machines) without importing os/exec itself.
func Available() bool {
	_, err := exec.LookPath(defaultBinary)
	return err == nil
}

// displayableJob mirrors awxctl's JSON output (internal/utils/display.go
// DisplayableJob). It is the decode shape; toJob converts it to Job. There is
// no error-codes field: `job info` doesn't emit one (verified live).
type displayableJob struct {
	Name     string    `json:"name"`
	SKU      string    `json:"sku"`
	Slot     string    `json:"slot"`
	JobID    int       `json:"job_id"`
	Template string    `json:"template"`
	Limit    string    `json:"limit"`
	Status   string    `json:"status"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	Elapsed  float64   `json:"elapsed"`
	AWXLink  string    `json:"awx_link"`
}

// toJob maps a decoded awxctl job to the exported Job, deriving the stage.
func toJob(r displayableJob) Job {
	return Job{
		JobID:    r.JobID,
		BMNName:  r.Name,
		SKU:      r.SKU,
		Slot:     r.Slot,
		Template: r.Template,
		Stage:    StageForTemplate(r.Template),
		Limit:    r.Limit,
		Status:   r.Status,
		Started:  r.Started,
		Finished: r.Finished,
		Elapsed:  r.Elapsed,
		AWXLink:  r.AWXLink,
	}
}

// run executes awxctl with args, returning trimmed stdout. It prefers
// ctx.Err() on cancel, and wraps a non-zero exit / any error as
// ErrSourceUnavailable with only a label + redacted stderr snippet — never the
// raw args (they can carry URLs / partial creds).
func (c *CLIClient) run(ctx context.Context, label string, args ...string) ([]byte, error) {
	stdout, stderr, err := c.runner(ctx, c.binary, args...)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		if msg := strings.TrimSpace(string(stderr)); msg != "" {
			return nil, fmt.Errorf("%w: %s: %v: %s", ErrSourceUnavailable, label, err, redactSnippet(msg))
		}
		return nil, fmt.Errorf("%w: %s: %v", ErrSourceUnavailable, label, err)
	}
	return bytes.TrimSpace(stdout), nil
}

// JobsForBMNs runs `awxctl job info bmn ... -o json` and maps the result onto
// []Job. An empty BMN list short-circuits to an empty slice.
func (c *CLIClient) JobsForBMNs(ctx context.Context, bmns []string, opts Options) ([]Job, error) {
	if len(bmns) == 0 {
		return nil, nil
	}
	out, err := c.run(ctx, fmt.Sprintf("awxctl job info bmn (targets=%d)", len(bmns)), buildArgs(bmns, opts)...)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	var raw []displayableJob
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("%w: decode awxctl output (targets=%d): %v", ErrSourceUnavailable, len(bmns), err)
	}
	jobs := make([]Job, 0, len(raw))
	for _, r := range raw {
		jobs = append(jobs, toJob(r))
	}
	sortJobs(jobs)
	return jobs, nil
}

// sortJobs orders by BMN, then most-recent-first within a BMN (newest job
// id first), for deterministic evidence.
func sortJobs(jobs []Job) {
	sort.SliceStable(jobs, func(i, j int) bool {
		if jobs[i].BMNName != jobs[j].BMNName {
			return jobs[i].BMNName < jobs[j].BMNName
		}
		return jobs[i].JobID > jobs[j].JobID
	})
}

// redactSnippet trims an awxctl stderr line to a short, credential-safe
// snippet for error context (mirrors slyalert's F20 stance — no raw URLs or
// partial creds in logs).
func redactSnippet(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	const max = 160
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
