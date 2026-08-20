package api

import (
	"net/http"

	"qac/internal/engine"
)

// listAgentsHandler serves GET /api/agents: every registered agent with its
// purpose and prompt_ref (from the registry) plus the shared LLM backend
// model and whether it is live (fixture when ANTHROPIC_API_KEY is unset).
func listAgentsHandler(eng *engine.Engine) http.HandlerFunc {
	type view struct {
		Type      string `json:"type"`
		Purpose   string `json:"purpose"`
		PromptRef string `json:"prompt_ref"`
		Model     string `json:"model"`
		Live      bool   `json:"live"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		llm := eng.AgentLLMInfo()
		infos := eng.Agents()
		out := make([]view, 0, len(infos))
		for _, a := range infos {
			out = append(out, view{
				Type:      a.Type,
				Purpose:   a.Purpose,
				PromptRef: a.PromptRef,
				Model:     llm.Model,
				Live:      llm.Live,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"agents": out})
	}
}
