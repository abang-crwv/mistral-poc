// Package canaryassessor is the first concrete agent: it reasons over a
// firmware-release canary's gathered evidence and emits an advisory verdict.
// Iteration 1 is a single-shot LLM call; the tool-using loop (iteration 2)
// replaces the body of Run behind the same agent.Agent interface.
package canaryassessor

import (
	"context"
	"fmt"

	"qac/internal/agent"
	"qac/internal/llmclient"
	"qac/internal/prompt"
)

// Agent is the canary-assessor agent.
type Agent struct{}

// New returns the canary-assessor agent.
func New() *Agent { return &Agent{} }

// Type is the stable identifier for the /api/agents surface.
func (*Agent) Type() string { return "canary_assessor" }

// Purpose is one-line copy for the Agents page.
func (*Agent) Purpose() string {
	return "Reasons over per-rack canary evidence and emits an advisory verdict with confidence and ranked likely causes."
}

// PromptRef is the template prompt_ref this agent services.
func (*Agent) PromptRef() string { return "fw-release-canary.summary.v1" }

// Run fetches the rubric, asks the LLM to assess the evidence bundle, and
// translates the assessment into an agent Output.
func (a *Agent) Run(ctx context.Context, clients agent.Clients, sc agent.StepContext) (agent.Output, error) {
	if clients.LLM == nil {
		return agent.Output{}, fmt.Errorf("canaryassessor: no LLM client configured")
	}
	system, err := prompt.Get(a.PromptRef())
	if err != nil {
		return agent.Output{}, err
	}
	assessment, err := clients.LLM.Assess(ctx, llmclient.AssessRequest{
		SystemPrompt:    system,
		EvidenceBundle:  sc.EvidenceBundle,
		ExpectedVerdict: sc.ExpectedVerdict,
	})
	if err != nil {
		return agent.Output{}, err
	}
	return agent.Output{
		Verdict:      assessment.Verdict,
		Confidence:   assessment.Confidence,
		Reasoning:    assessment.Reasoning,
		RankedCauses: assessment.RankedCauses,
		LikelyOwner:  assessment.LikelyOwner,
		Sources:      []string{assessment.Model},
	}, nil
}
