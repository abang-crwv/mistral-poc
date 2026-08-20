package failcauseprobe

import (
	"context"
	"encoding/json"
	"testing"

	"qac/internal/alertclient"
	"qac/internal/probe"
	"qac/internal/store"
)

// fakeReader is an in-memory EvidenceReader returning a fixed row set.
type fakeReader struct {
	rows []store.Evidence
	err  error
}

func (f *fakeReader) ListEvidence(ctx context.Context, runID, stepID string) ([]store.Evidence, error) {
	return f.rows, f.err
}

// fakeAlerts is an in-memory alertclient.Client keyed by deviceslot.
type fakeAlerts struct {
	byDS map[string][]alertclient.Alert
}

func (f *fakeAlerts) ListByRack(ctx context.Context, rackID string) ([]alertclient.Alert, error) {
	return nil, nil
}

func (f *fakeAlerts) ListByDeviceslot(ctx context.Context, deviceslot string) ([]alertclient.Alert, error) {
	return f.byDS[deviceslot], nil
}

// capWriter captures the per-CT evidence rows the probe writes.
type capWriter struct {
	rows []store.Evidence
}

func (w *capWriter) PutEvidence(ctx context.Context, e store.Evidence) error {
	w.rows = append(w.rows, e)
	return nil
}

// evRow builds a per-CT evidence row with the given deviceslot + outcome,
// matching the shape rlcc_action_probe writes.
func evRow(ds, outcome string) store.Evidence {
	payload, _ := json.Marshal(map[string]any{"deviceslot": ds, "outcome": outcome})
	dsCopy := ds
	return store.Evidence{
		ID:          "ev-" + ds,
		StepID:      "l11_fielddiag",
		ContentType: "application/json",
		Payload:     payload,
		Deviceslot:  &dsCopy,
	}
}

// run is a small harness: wires the fakes into probe.Clients and runs the
// probe with a fail_step config.
func run(t *testing.T, reader probe.EvidenceReader, alerts alertclient.Client, writer *capWriter, cfg map[string]any) (probe.Output, error) {
	t.Helper()
	clients := probe.Clients{
		AlertClient:    alerts,
		EvidenceReader: reader,
		EvidenceWriter: writer,
	}
	sc := probe.StepContext{RunID: "run1", StepID: "fail_cause", Config: cfg}
	return New().Run(context.Background(), clients, sc)
}

func decodeAgg(t *testing.T, out probe.Output) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(out.StructuredData, &m); err != nil {
		t.Fatalf("decode aggregate: %v", err)
	}
	return m
}

func TestFailCauseProbe_TypeAndCategory(t *testing.T) {
	p := New()
	if p.Type() != "fail_cause_probe" {
		t.Errorf("Type() = %q, want fail_cause_probe", p.Type())
	}
	if p.Category() != probe.CategoryGatherer {
		t.Errorf("Category() = %q, want gatherer", p.Category())
	}
}

func TestFailCauseProbe_FailedWithCriticalAlert(t *testing.T) {
	reader := &fakeReader{rows: []store.Evidence{
		evRow("ds1", "failed"),
		evRow("ds2", "success"),
	}}
	alerts := &fakeAlerts{byDS: map[string][]alertclient.Alert{
		"ds1": {{ID: "a1", Name: "NVLinkLinkDown", Severity: "critical", Deviceslot: "ds1"}},
	}}
	w := &capWriter{}
	out, err := run(t, reader, alerts, w, map[string]any{"fail_step": "l11_fielddiag"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Verdict != "" {
		t.Errorf("Verdict = %q, want empty (gatherer)", out.Verdict)
	}
	if len(w.rows) != 1 {
		t.Fatalf("cause rows = %d, want 1", len(w.rows))
	}
	if w.rows[0].Deviceslot == nil || *w.rows[0].Deviceslot != "ds1" {
		t.Errorf("cause row deviceslot = %v, want ds1", w.rows[0].Deviceslot)
	}
	var row struct {
		Alerts []alertclient.Alert `json:"alerts"`
	}
	if err := json.Unmarshal(w.rows[0].Payload, &row); err != nil {
		t.Fatalf("decode cause row: %v", err)
	}
	if len(row.Alerts) != 1 || row.Alerts[0].Severity != "critical" {
		t.Errorf("cause row alerts = %+v, want 1 critical", row.Alerts)
	}
	agg := decodeAgg(t, out)
	if agg["max_severity"] != "critical" {
		t.Errorf("aggregate max_severity = %v, want critical", agg["max_severity"])
	}
	if agg["failed_count"] != float64(1) {
		t.Errorf("aggregate failed_count = %v, want 1", agg["failed_count"])
	}
	if agg["with_alerts_count"] != float64(1) {
		t.Errorf("aggregate with_alerts_count = %v, want 1", agg["with_alerts_count"])
	}
}

func TestFailCauseProbe_FailedWithNoAlerts(t *testing.T) {
	reader := &fakeReader{rows: []store.Evidence{evRow("ds1", "failed")}}
	alerts := &fakeAlerts{byDS: map[string][]alertclient.Alert{}} // no alerts anywhere
	w := &capWriter{}
	out, err := run(t, reader, alerts, w, map[string]any{"fail_step": "l11_fielddiag"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(w.rows) != 1 {
		t.Fatalf("cause rows = %d, want 1 (failed tray still gets a row)", len(w.rows))
	}
	var row struct {
		Alerts []alertclient.Alert `json:"alerts"`
	}
	if err := json.Unmarshal(w.rows[0].Payload, &row); err != nil {
		t.Fatalf("decode cause row: %v", err)
	}
	if len(row.Alerts) != 0 {
		t.Errorf("cause row alerts = %+v, want empty", row.Alerts)
	}
	agg := decodeAgg(t, out)
	if agg["with_alerts_count"] != float64(0) {
		t.Errorf("aggregate with_alerts_count = %v, want 0", agg["with_alerts_count"])
	}
}

func TestFailCauseProbe_NoFailures(t *testing.T) {
	reader := &fakeReader{rows: []store.Evidence{
		evRow("ds1", "success"),
		evRow("ds2", "ignorable"),
	}}
	w := &capWriter{}
	out, err := run(t, reader, &fakeAlerts{}, w, map[string]any{"fail_step": "l11_fielddiag"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(w.rows) != 0 {
		t.Errorf("cause rows = %d, want 0 (no failures)", len(w.rows))
	}
	agg := decodeAgg(t, out)
	if agg["failed_count"] != float64(0) {
		t.Errorf("aggregate failed_count = %v, want 0", agg["failed_count"])
	}
}

func TestFailCauseProbe_MissingFailStep(t *testing.T) {
	w := &capWriter{}
	_, err := run(t, &fakeReader{}, &fakeAlerts{}, w, map[string]any{})
	if err == nil {
		t.Fatal("Run with no fail_step config = nil error, want error")
	}
}

func TestFailCauseProbe_NilPorts(t *testing.T) {
	p := New()
	sc := probe.StepContext{RunID: "run1", StepID: "fail_cause", Config: map[string]any{"fail_step": "l11_fielddiag"}}

	// nil AlertClient
	_, err := p.Run(context.Background(), probe.Clients{EvidenceReader: &fakeReader{}, EvidenceWriter: &capWriter{}}, sc)
	if err == nil {
		t.Error("Run with nil AlertClient = nil error, want error")
	}
	// nil EvidenceReader
	_, err = p.Run(context.Background(), probe.Clients{AlertClient: &fakeAlerts{}, EvidenceWriter: &capWriter{}}, sc)
	if err == nil {
		t.Error("Run with nil EvidenceReader = nil error, want error")
	}
}
