package lokiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// L11JobRef is a rack-wide l11-fielddiag AWX job as RLCC recorded it at launch:
// the rack it ran against, the AWX job id + region (needed for
// `awxctl job info id <id> -r <region>`), and when RLCC launched it. A rack may
// have several (retries) — newest first.
type L11JobRef struct {
	Rack       string    `json:"rack"`
	JobID      int       `json:"job_id"`
	Region     string    `json:"region"`
	LaunchedAt time.Time `json:"launched_at"`
}

// l11LogQL is the deterministic selector for RLCC's l11-fielddiag job-launch
// event. Validated live: `RLCCAWXJobStarted` + `action-l11-controller` matches
// only the clean structured launch events, no reconcile-diff noise.
const l11LogQLBase = `{namespace="rack-lifecycle", app="rack-lifecycle-controller"} |= "RLCCAWXJobStarted" |= "action-l11-controller"`

// jobURLRe pulls the AWX job id and region out of the launch message, e.g.
// https://awx.us-central-03a.int.coreweave.com/#/jobs/playbook/810195/output
var jobURLRe = regexp.MustCompile(`awx\.([a-z0-9-]+)\.int\.coreweave\.com/#/jobs/playbook/(\d+)/`)

// rlccEvent is the RLCC structured log payload we care about (the promtail
// prefix `<ts> stdout F ` is stripped before unmarshalling).
type rlccEvent struct {
	Time         string `json:"time"`
	Msg          string `json:"msg"`
	Reason       string `json:"reason"`
	ReconcilerNm string `json:"reconciler_name"`
	Name         string `json:"name"` // the rack
}

// L11FieldDiagJobs queries RLCC's l11-fielddiag launch events for a rack over
// the lookback window and returns the parsed job refs (newest first). An empty
// rack returns every rack's refs in the window. A source failure bubbles up.
func L11FieldDiagJobs(ctx context.Context, c Client, region, rack string, lookback time.Duration) ([]L11JobRef, error) {
	logql := l11LogQLBase
	if rack != "" {
		logql += fmt.Sprintf(" |= %q", rack)
	}
	end := time.Now()
	start := end.Add(-lookback)

	entries, err := c.QueryRange(ctx, region, logql, start, end, 100)
	if err != nil {
		return nil, err
	}

	refs := make([]L11JobRef, 0, len(entries))
	for _, e := range entries {
		ref, ok := parseL11Launch(e.Line)
		if !ok {
			continue
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// parseL11Launch extracts an L11JobRef from a raw RLCC log line. Returns
// ok=false for lines that don't carry a launch event / job URL.
func parseL11Launch(line string) (L11JobRef, bool) {
	// Strip the promtail prefix: `<ts> stdout F {json...}`.
	brace := strings.IndexByte(line, '{')
	if brace < 0 {
		return L11JobRef{}, false
	}
	var ev rlccEvent
	if err := json.Unmarshal([]byte(line[brace:]), &ev); err != nil {
		return L11JobRef{}, false
	}
	if ev.Reason != "RLCCAWXJobStarted" || ev.Name == "" {
		return L11JobRef{}, false
	}
	m := jobURLRe.FindStringSubmatch(ev.Msg)
	if m == nil {
		return L11JobRef{}, false
	}
	jobID, err := strconv.Atoi(m[2])
	if err != nil {
		return L11JobRef{}, false
	}
	ref := L11JobRef{Rack: ev.Name, JobID: jobID, Region: m[1]}
	if t, err := time.Parse(time.RFC3339Nano, ev.Time); err == nil {
		ref.LaunchedAt = t
	}
	return ref, true
}
