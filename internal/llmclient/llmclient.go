// Package llmclient is the LLM port an agent uses to turn an evidence bundle
// into a structured assessment. It has a fixture default (offline, hermetic)
// and a live Anthropic implementation (see anthropic.go), selected by whether
// ANTHROPIC_API_KEY is set at server boot.
package llmclient

import (
	"context"
	"strings"
)

// Cause is one ranked likely cause of a canary problem.
type Cause struct {
	Summary     string `json:"summary"`
	LikelyOwner string `json:"likely_owner,omitempty"`
	Confidence  string `json:"confidence,omitempty"`
}

// Assessment is the structured verdict the LLM produces. Verdict is the
// template vocabulary ("pass" | "needs_review" | "fail"); Model is provenance
// ("claude-opus-4-8" for the live client, "fixture" for the fixture).
type Assessment struct {
	Verdict      string  `json:"verdict"`
	Confidence   string  `json:"confidence"`
	Reasoning    string  `json:"reasoning"`
	RankedCauses []Cause `json:"ranked_causes,omitempty"`
	LikelyOwner  string  `json:"likely_owner,omitempty"`
	Model        string  `json:"model"`
}

// AssessRequest is the input to Assess. ExpectedVerdict is the allowed verdict
// set from the step (the structured-output enum for the live client).
type AssessRequest struct {
	SystemPrompt    string
	EvidenceBundle  string
	ExpectedVerdict []string
}

// Info describes the active LLM backend, for the /api/agents surface.
type Info struct {
	Model string `json:"model"`
	Live  bool   `json:"live"`
}

// Client turns an evidence bundle into an assessment.
type Client interface {
	Assess(ctx context.Context, req AssessRequest) (Assessment, error)
	Info() Info
}

// FixtureClient is the offline default: a deterministic assessment derived
// from the evidence bundle. It exists so the flow works and tests run without
// ANTHROPIC_API_KEY — it is not a real reasoner.
type FixtureClient struct{}

// NewFixtureClient returns the offline fixture LLM client.
func NewFixtureClient() *FixtureClient { return &FixtureClient{} }

// Assess returns "fail" when the evidence bundle contains a failure marker,
// else "pass", always at low confidence. This is a stand-in heuristic, not
// reasoning.
func (FixtureClient) Assess(_ context.Context, req AssessRequest) (Assessment, error) {
	verdict := "pass"
	reasoning := "Fixture assessment: no failure markers found in the gathered evidence."
	if strings.Contains(req.EvidenceBundle, "failed") {
		verdict = "fail"
		reasoning = "Fixture assessment: the gathered evidence contains a failure marker."
	}
	return Assessment{
		Verdict:    verdict,
		Confidence: "low",
		Reasoning:  reasoning,
		Model:      "fixture",
	}, nil
}

// Info reports the fixture backend.
func (FixtureClient) Info() Info { return Info{Model: "fixture", Live: false} }
