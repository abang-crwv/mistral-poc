package probe

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"qac/internal/alertclient"
	"qac/internal/store"
)

func TestCategoryConstants(t *testing.T) {
	if CategoryGatherer != "gatherer" {
		t.Errorf("CategoryGatherer = %q, want gatherer", CategoryGatherer)
	}
	if CategoryAssertion != "assertion" {
		t.Errorf("CategoryAssertion = %q, want assertion", CategoryAssertion)
	}
	if CategoryAction != "action" {
		t.Errorf("CategoryAction = %q, want action", CategoryAction)
	}
}

type stubProbe struct{ kind string }

func (p *stubProbe) Type() string       { return p.kind }
func (p *stubProbe) Category() Category { return CategoryGatherer }
func (p *stubProbe) Run(ctx context.Context, c Clients, sc StepContext) (Output, error) {
	return Output{}, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	p := &stubProbe{kind: "alert_probe"}
	r.Register(p)

	got, ok := r.Get("alert_probe")
	if !ok {
		t.Fatal("Get: not found, want found")
	}
	if got.Type() != "alert_probe" {
		t.Errorf("Type = %q, want alert_probe", got.Type())
	}
}

func TestRegistry_Get_UnknownReturnsFalse(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("ghost_probe")
	if ok {
		t.Fatal("Get: found, want not-found")
	}
}

func TestRegistry_Register_LastWins(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubProbe{kind: "alert_probe"})
	replacement := &stubProbe{kind: "alert_probe"}
	r.Register(replacement)
	got, _ := r.Get("alert_probe")
	if got != replacement {
		t.Errorf("Register did not replace existing probe — got %p, want %p", got, replacement)
	}
}

func TestOutput_JSONRoundTrip(t *testing.T) {
	src := Output{
		StructuredData: json.RawMessage(`{"alerts_clear":true}`),
		Sources:        []string{"alertmanager-stub"},
		ProbedAt:       1700000000,
	}
	b, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Output
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(got.StructuredData) != `{"alerts_clear":true}` {
		t.Errorf("StructuredData = %s, want round-trip equality", got.StructuredData)
	}
	if got.ProbedAt != 1700000000 || got.Sources[0] != "alertmanager-stub" {
		t.Errorf("got = %+v", got)
	}
}

// Compile-time: Clients carries the alertclient.Client port.
var _ = alertclient.Client(nil) // ensures import is real (referenced in Clients)

func TestStoreEmitter_EmitBMNIgnored(t *testing.T) {
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "qac.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	runID := "01HZX0000000000000000RUNX"
	rc, _ := json.Marshal(map[string]any{
		"template_id": "firmware-release-canary",
		"created_by":  "wpena",
		"inputs":      map[string]any{"bundle_tag": "x", "canary_racks": []string{"rack-a"}},
	})
	if err := s.AppendEvent(t.Context(), runID, "RunCreated", rc); err != nil {
		t.Fatalf("RunCreated: %v", err)
	}

	em := NewStoreEmitter(s)
	if err := em.EmitBMNIgnored(t.Context(), runID, BMNIgnoredPayload{
		Rack: "rack-a", Deviceslot: "rack-a-node-07-z", Source: "rlcc", MarkedAt: 1700000000,
	}); err != nil {
		t.Fatalf("EmitBMNIgnored: %v", err)
	}

	evs, err := s.LoadEvents(t.Context(), runID)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	last := evs[len(evs)-1]
	if last.Kind != "BMNIgnored" {
		t.Fatalf("last event kind = %q, want BMNIgnored", last.Kind)
	}
	var p BMNIgnoredPayload
	if err := json.Unmarshal(last.Payload, &p); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if p.Source != "rlcc" || p.Deviceslot != "rack-a-node-07-z" {
		t.Errorf("payload = %+v, want source=rlcc deviceslot=rack-a-node-07-z", p)
	}
}
