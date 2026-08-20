import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';

// The content type the ai_assess step's engine handler stamps on the
// evidence row it persists (internal/engine/aiassess.go:assessmentContentType).
// The frontend keys off this to distinguish an agent's advisory verdict from
// any other evidence a step might record.
const ASSESSMENT_CONTENT_TYPE = 'application/vnd.qac.assessment+json';

// Assessment is the agent.Output shape persisted as the evidence payload.
export interface Assessment {
  verdict: string;
  confidence: string;
  reasoning: string;
  ranked_causes?: Array<{ summary: string; likely_owner?: string; confidence?: string }>;
  likely_owner?: string;
  sources?: string[];
}

interface EvidenceRow {
  content_type: string;
  payload: Record<string, unknown>;
}

// useRunAssessment looks up the run's ai_assess advisory-verdict evidence, if
// one has been persisted. Evidence is fetched for the whole run (no ?step=
// filter) so this doesn't need to know which step id the template's
// ai_assess step uses — it finds the row by content_type instead.
export function useRunAssessment(id: string) {
  return useQuery<Assessment | null>({
    queryKey: ['canary', 'runs', id, 'assessment'],
    queryFn: async () => {
      const rows =
        (await apiGet<{ evidence: EvidenceRow[] }>(`/runs/${id}/evidence`)).evidence ?? [];
      const row = rows.find((r) => r.content_type === ASSESSMENT_CONTENT_TYPE);
      return (row?.payload as Assessment) ?? null;
    },
  });
}
