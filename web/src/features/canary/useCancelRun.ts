import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiPost } from '@/lib/api';

// POST /api/runs/:id/cancel — cancels a pending/running run. The backend
// appends a terminal RunCancelled event and stops the in-flight walk.
export function useCancelRun(runId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiPost<{ run: unknown }, Record<string, never>>(`/runs/${runId}/cancel`, {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['canary', 'runs', runId] });
      qc.invalidateQueries({ queryKey: ['canary', 'runs'] });
    },
  });
}
