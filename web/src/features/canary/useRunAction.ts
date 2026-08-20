import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiPost } from '@/lib/api';

export interface RunActionInput {
  step_id: string;
  action: 'advance' | 'signoff';
  verdict?: string;
  signer_name?: string;
  signer_role?: string;
  notes?: string;
}

export function useRunAction(runId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: RunActionInput) =>
      apiPost<{ run: unknown }, RunActionInput>(`/runs/${runId}/actions`, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['canary', 'runs', runId] });
      qc.invalidateQueries({ queryKey: ['canary', 'runs'] });
    },
  });
}
