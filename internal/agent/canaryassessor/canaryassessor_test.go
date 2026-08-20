package canaryassessor

import (
	"context"
	"testing"

	"qac/internal/agent"
	"qac/internal/llmclient"
)

// stubLLM returns a canned assessment and records the request it received.
type stubLLM struct {
	got llmclient.AssessRequest
	ret llmclient.Assessment
}

func (s *stubLLM) Assess(_ context.Context, req llmclient.AssessRequest) (llmclient.Assessment, error) {
	s.got = req
	return s.ret, nil
}
func (*stubLLM) Info() llmclient.Info { return llmclient.Info{Model: "stub", Live: false} }

func TestCanaryAssessor_Run_PassesRubricAndTranslatesAssessment(t *testing.T) {
	llm := &stubLLM{ret: llmclient.Assessment{
		Verdict: "fail", Confidence: "high", Reasoning: "bad", Model: "claude-opus-4-8",
		RankedCauses: []llmclient.Cause{{Summary: "bmc", LikelyOwner: "hw"}},
	}}
	out, err := New().Run(context.Background(), agent.Clients{LLM: llm}, agent.StepContext{
		EvidenceBundle:  "some evidence",
		ExpectedVerdict: []string{"pass", "fail", "needs_review"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The rubric was fetched and passed as the system prompt.
	if llm.got.SystemPrompt == "" {
		t.Error("system prompt (rubric) was not passed to the LLM")
	}
	if llm.got.EvidenceBundle != "some evidence" {
		t.Errorf("evidence bundle = %q", llm.got.EvidenceBundle)
	}
	// The assessment is translated into the agent Output verbatim.
	if out.Verdict != "fail" || out.Confidence != "high" || len(out.RankedCauses) != 1 {
		t.Errorf("Output not translated from Assessment: %+v", out)
	}
	// Provenance records the model.
	if len(out.Sources) != 1 || out.Sources[0] != "claude-opus-4-8" {
		t.Errorf("Sources = %v, want [claude-opus-4-8]", out.Sources)
	}
}

func TestCanaryAssessor_Identity(t *testing.T) {
	a := New()
	if a.Type() != "canary_assessor" || a.PromptRef() != "fw-release-canary.summary.v1" {
		t.Errorf("identity = %q / %q", a.Type(), a.PromptRef())
	}
	var _ agent.Agent = a // satisfies the interface
}
