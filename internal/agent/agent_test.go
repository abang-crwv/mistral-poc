package agent

import (
	"context"
	"testing"

	"qac/internal/llmclient"
	"qac/internal/verdict"
)

func TestMapVerdict(t *testing.T) {
	cases := map[string]string{
		"pass":         verdict.Passed,
		"needs_review": verdict.Warning,
		"fail":         verdict.Failed,
		"weird":        verdict.Warning, // unknown → flag for a human
	}
	for raw, want := range cases {
		if got := MapVerdict(raw); got != want {
			t.Errorf("MapVerdict(%q) = %q, want %q", raw, got, want)
		}
	}
}

type stubAgent struct{ ref string }

func (s stubAgent) Type() string      { return "stub" }
func (s stubAgent) Purpose() string   { return "test agent" }
func (s stubAgent) PromptRef() string { return s.ref }
func (stubAgent) Run(context.Context, Clients, StepContext) (Output, error) {
	return Output{}, nil
}

func TestRegistry_RegisterGetList(t *testing.T) {
	r := NewRegistry()
	r.Register(stubAgent{ref: "x.v1"})
	if _, ok := r.Get("x.v1"); !ok {
		t.Fatal("Get(x.v1) not found after Register")
	}
	if _, ok := r.Get("missing"); ok {
		t.Fatal("Get(missing) should be false")
	}
	list := r.List()
	if len(list) != 1 || list[0].PromptRef != "x.v1" {
		t.Fatalf("List() = %+v, want one entry keyed x.v1", list)
	}
}

// Compile-time check that llmclient.Client is what Clients carries.
var _ = Clients{LLM: llmclient.NewFixtureClient()}
