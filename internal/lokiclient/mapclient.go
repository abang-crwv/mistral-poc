package lokiclient

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FailingSourceRegion is a sentinel region; a QueryRange against it returns
// ErrSourceUnavailable so probe tests can exercise the transport-failure path.
const FailingSourceRegion = "failing-source-region"

// MapClient is an in-memory Client backed by a region→entries table. It mimics
// Loki's line filters: a query keeps only entries whose Line contains every
// `|= "..."` substring in the LogQL. It is the offline/test backend.
type MapClient struct {
	byRegion map[string][]LogEntry
}

// NewMapClient returns a MapClient over the given region→entries table. A nil
// table is treated as empty.
func NewMapClient(byRegion map[string][]LogEntry) *MapClient {
	if byRegion == nil {
		byRegion = map[string][]LogEntry{}
	}
	return &MapClient{byRegion: byRegion}
}

// Compile-time satisfaction check.
var _ Client = (*MapClient)(nil)

var lineFilterRe = regexp.MustCompile(`\|=\s*"([^"]*)"`)

// QueryRange returns seeded entries for the region whose Line contains every
// `|= "..."` substring in logql, capped at limit. The FailingSourceRegion
// sentinel triggers ErrSourceUnavailable.
func (m *MapClient) QueryRange(_ context.Context, region, logql string, _, _ time.Time, limit int) ([]LogEntry, error) {
	if region == FailingSourceRegion {
		return nil, fmt.Errorf("%w: seeded failing region %s", ErrSourceUnavailable, region)
	}
	var filters []string
	for _, m := range lineFilterRe.FindAllStringSubmatch(logql, -1) {
		filters = append(filters, m[1])
	}
	var out []LogEntry
	for _, e := range m.byRegion[region] {
		if lineMatchesAll(e.Line, filters) {
			out = append(out, e)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func lineMatchesAll(line string, filters []string) bool {
	for _, f := range filters {
		if !strings.Contains(line, f) {
			return false
		}
	}
	return true
}

// SeedDemoL11Logs returns a demo region→entries table with two RLCC
// l11-fielddiag launch events for the demo rack dh3-r012-us-east-01a (matching
// SeedDemoBMNs), in the real promtail `<ts> stdout F {json}` line format.
func SeedDemoL11Logs() map[string][]LogEntry {
	mk := func(ts, rack, region string, jobID int) LogEntry {
		line := fmt.Sprintf(
			`%s stdout F {"time":%q,"level":"INFO","msg":"AWX job launched, see details for jobID[%d]: https://awx.%s.int.coreweave.com/#/jobs/playbook/%d/output","reconciler_type":"action","reconciler_name":"action-l11-controller","event_type":"Normal","reason":"RLCCAWXJobStarted","name":%q}`,
			ts, ts, jobID, region, jobID, rack,
		)
		t, _ := time.Parse(time.RFC3339Nano, ts)
		return LogEntry{
			Timestamp: t,
			Line:      line,
			Labels: map[string]string{
				"app":       "rack-lifecycle-controller",
				"namespace": "rack-lifecycle",
				"cluster":   "us-east-01a-mgmt",
			},
		}
	}
	return map[string][]LogEntry{
		"us-east-01a": {
			mk("2026-07-03T05:28:48.109163101Z", "dh3-r012-us-east-01a", "us-east-01a", 884321),
			mk("2026-07-02T14:14:48.960985380Z", "dh3-r012-us-east-01a", "us-east-01a", 883990),
		},
	}
}
