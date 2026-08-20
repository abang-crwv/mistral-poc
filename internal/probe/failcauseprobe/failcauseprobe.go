// Package failcauseprobe implements fail_cause_probe — a gatherer (iter-5l)
// that explains failed trays. It reads a prior step's per-CT evidence
// (via EvidenceReader), finds the deviceslots whose outcome was "failed",
// and correlates node/deviceslot-scoped alerts (via AlertClient) to each.
// It writes one per-failed-node cause evidence row and returns an aggregate
// Output with no verdict — interpretation is the AI agent's job (iter-5m).
package failcauseprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	"qac/internal/alertclient"
	"qac/internal/probe"
	"qac/internal/store"
)

// FailCauseProbe correlates firing alerts to the trays a prior step failed.
type FailCauseProbe struct{}

// New returns a zero-value FailCauseProbe.
func New() *FailCauseProbe { return &FailCauseProbe{} }

// Compile-time satisfaction check.
var _ probe.Probe = (*FailCauseProbe)(nil)

// Type returns the registry key for this probe.
func (*FailCauseProbe) Type() string { return "fail_cause_probe" }

// Category returns CategoryGatherer — fail_cause_probe produces evidence,
// not a verdict.
func (*FailCauseProbe) Category() probe.Category { return probe.CategoryGatherer }

// severityRank mirrors alert_probe's ranking (ported, not imported — the
// project's port-don't-import norm). Higher means worse; unknown severities
// rank below "info" so they cannot dominate max_severity by accident.
var severityRank = map[string]int{"": 0, "info": 1, "warning": 2, "critical": 3}

// Run satisfies probe.Probe. It reads the fail_step's per-CT evidence,
// correlates a deviceslot-scoped alert lookup to each failed tray, writes one
// cause evidence row per failed deviceslot, and returns an aggregate rollup.
// Runs unconditionally: a run with no failed trays writes zero cause rows and
// returns an aggregate with failed_count == 0.
func (*FailCauseProbe) Run(ctx context.Context, c probe.Clients, sc probe.StepContext) (probe.Output, error) {
	if c.AlertClient == nil || c.EvidenceReader == nil || c.EvidenceWriter == nil {
		return probe.Output{}, fmt.Errorf("fail_cause_probe: AlertClient, EvidenceReader, and EvidenceWriter are required")
	}
	failStep, _ := sc.Config["fail_step"].(string)
	if failStep == "" {
		return probe.Output{}, fmt.Errorf("fail_cause_probe: config.fail_step is required")
	}

	rows, err := c.EvidenceReader.ListEvidence(ctx, sc.RunID, failStep)
	if err != nil {
		return probe.Output{}, fmt.Errorf("fail_cause_probe: list evidence for %s: %w", failStep, err)
	}

	failedCount := 0
	withAlerts := 0
	globalMax := ""
	for _, row := range rows {
		if row.Deviceslot == nil { // skip the aggregate row
			continue
		}
		var p struct {
			Deviceslot string `json:"deviceslot"`
			Outcome    string `json:"outcome"`
		}
		if err := json.Unmarshal(row.Payload, &p); err != nil {
			continue // tolerant: malformed evidence is skipped
		}
		if p.Outcome != "failed" {
			continue
		}
		failedCount++
		ds := *row.Deviceslot

		alerts, err := c.AlertClient.ListByDeviceslot(ctx, ds)
		if err != nil {
			return probe.Output{}, fmt.Errorf("fail_cause_probe: list alerts for %s: %w", ds, err)
		}
		if alerts == nil {
			alerts = []alertclient.Alert{}
		}
		counts := map[string]int{"critical": 0, "warning": 0, "info": 0}
		maxSev := ""
		for _, a := range alerts {
			if _, ok := counts[a.Severity]; ok {
				counts[a.Severity]++
			}
			if severityRank[a.Severity] > severityRank[maxSev] {
				maxSev = a.Severity
			}
		}
		if len(alerts) > 0 {
			withAlerts++
		}
		if severityRank[maxSev] > severityRank[globalMax] {
			globalMax = maxSev
		}

		body, _ := json.Marshal(map[string]any{
			"deviceslot":        ds,
			"fail_step":         failStep,
			"alerts":            alerts,
			"max_severity":      maxSev,
			"count_by_severity": counts,
		})
		dsCopy := ds
		// Best-effort per-CT write (mirrors rlcc_action_probe): the aggregate
		// Output is canonical, so a write error is not fatal to the step.
		_ = c.EvidenceWriter.PutEvidence(ctx, store.Evidence{
			ID:          ulid.Make().String(),
			RunID:       sc.RunID,
			StepID:      sc.StepID,
			ContentType: "application/json",
			Payload:     body,
			Deviceslot:  &dsCopy,
		})
	}

	probedAt := time.Now().Unix()
	agg, err := json.Marshal(map[string]any{
		"probed_at":         probedAt,
		"fail_step":         failStep,
		"failed_count":      failedCount,
		"with_alerts_count": withAlerts,
		"max_severity":      globalMax,
		"source":            "alertmanager-stub",
	})
	if err != nil {
		return probe.Output{}, fmt.Errorf("fail_cause_probe: marshal aggregate: %w", err)
	}
	return probe.Output{
		StructuredData: agg,
		Sources:        []string{"alertmanager-stub"},
		ProbedAt:       probedAt,
		// Verdict intentionally empty — gatherer.
	}, nil
}
