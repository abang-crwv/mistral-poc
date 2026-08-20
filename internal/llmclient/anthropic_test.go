package llmclient

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestAnthropicClient_Assess_Live(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY unset; skipping live LLM test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	a, err := NewAnthropicClient().Assess(ctx, AssessRequest{
		SystemPrompt:    "You assess a canary. Call record_assessment with your verdict.",
		EvidenceBundle:  `{"rack":"rack-a","outcome":"ok","firmware":"converged"}`,
		ExpectedVerdict: []string{"pass", "fail", "needs_review"},
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	switch a.Verdict {
	case "pass", "fail", "needs_review":
	default:
		t.Errorf("verdict %q not in expected set", a.Verdict)
	}
	if a.Model == "" {
		t.Error("model provenance not set")
	}
}

func TestAnthropicClient_Info(t *testing.T) {
	got := NewAnthropicClient().Info()
	if !got.Live || got.Model != "claude-opus-4-8" {
		t.Errorf("Info() = %+v, want {claude-opus-4-8 true}", got)
	}
}
