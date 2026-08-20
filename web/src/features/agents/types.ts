// Agent is one registered agent as served by GET /api/agents. The backend
// registry is the source of truth for type/purpose/prompt_ref; model + live
// describe the shared LLM backend (fixture when no ANTHROPIC_API_KEY).
export interface Agent {
  type: string;
  purpose: string;
  prompt_ref: string;
  model: string;
  live: boolean;
}
