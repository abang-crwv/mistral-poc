package llmclient

import (
	"context"
	"testing"
)

func TestFixtureClient_PassWhenNoFailureMarker(t *testing.T) {
	a, err := NewFixtureClient().Assess(context.Background(), AssessRequest{
		EvidenceBundle:  `{"outcome":"ok","rack":"rack-a"}`,
		ExpectedVerdict: []string{"pass", "fail", "needs_review"},
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Verdict != "pass" {
		t.Errorf("verdict = %q, want pass", a.Verdict)
	}
	if a.Model != "fixture" {
		t.Errorf("model = %q, want fixture", a.Model)
	}
}

func TestFixtureClient_FailWhenFailureMarker(t *testing.T) {
	a, err := NewFixtureClient().Assess(context.Background(), AssessRequest{
		EvidenceBundle:  `{"outcome":"failed","rack":"rack-a"}`,
		ExpectedVerdict: []string{"pass", "fail", "needs_review"},
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Verdict != "fail" {
		t.Errorf("verdict = %q, want fail", a.Verdict)
	}
}

func TestFixtureClient_Info(t *testing.T) {
	got := NewFixtureClient().Info()
	if got.Live || got.Model != "fixture" {
		t.Errorf("Info() = %+v, want {fixture false}", got)
	}
}
