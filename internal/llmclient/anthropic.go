package llmclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// modelID is the assessment model. Opus 4.8 with adaptive thinking; effort is
// left at the SDK default (high). Do not set sampling params or budget_tokens
// — they 400 on Opus 4.8.
const modelID = anthropic.ModelClaudeOpus4_8

// AnthropicClient is the live LLM backend. anthropic.NewClient() reads
// ANTHROPIC_API_KEY from the environment.
type AnthropicClient struct {
	client anthropic.Client
}

// NewAnthropicClient returns a live client. The caller selects this over the
// fixture only when ANTHROPIC_API_KEY is present.
func NewAnthropicClient() *AnthropicClient {
	return &AnthropicClient{client: anthropic.NewClient()}
}

// Info reports the live backend.
func (*AnthropicClient) Info() Info { return Info{Model: string(modelID), Live: true} }

// Assess asks the model to reason over the evidence and call the strict
// record_assessment tool, then decodes that tool call into an Assessment.
func (c *AnthropicClient) Assess(ctx context.Context, req AssessRequest) (Assessment, error) {
	tool := anthropic.ToolParam{
		Name:        "record_assessment",
		Description: anthropic.String("Record the canary verdict, confidence, reasoning, ranked likely causes, and the likely owning team."),
		Strict:      anthropic.Bool(true),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"verdict":      map[string]any{"type": "string", "enum": req.ExpectedVerdict},
				"confidence":   map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
				"reasoning":    map[string]any{"type": "string"},
				"likely_owner": map[string]any{"type": "string"},
				"ranked_causes": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"summary":      map[string]any{"type": "string"},
							"likely_owner": map[string]any{"type": "string"},
							"confidence":   map[string]any{"type": "string"},
						},
						"required": []string{"summary"},
					},
				},
			},
			Required: []string{"verdict", "confidence", "reasoning"},
			ExtraFields: map[string]any{
				"additionalProperties": false,
			},
		},
	}

	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     modelID,
		MaxTokens: 16000,
		Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
		System: []anthropic.TextBlockParam{{
			Text:         req.SystemPrompt,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Tools: []anthropic.ToolUnionParam{{OfTool: &tool}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.EvidenceBundle)),
		},
	})
	if err != nil {
		return Assessment{}, fmt.Errorf("anthropic assess: %w", err)
	}

	for _, block := range resp.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok && tu.Name == "record_assessment" {
			var a Assessment
			if err := json.Unmarshal(tu.Input, &a); err != nil {
				return Assessment{}, fmt.Errorf("decode assessment: %w", err)
			}
			a.Model = string(resp.Model)
			return a, nil
		}
	}
	if resp.StopReason == anthropic.StopReasonMaxTokens {
		return Assessment{}, fmt.Errorf("assessment truncated at max_tokens before the record_assessment tool call; raise MaxTokens or reduce the evidence bundle")
	}
	return Assessment{}, fmt.Errorf("model did not call record_assessment (stop_reason %q)", resp.StopReason)
}
