import { useQuery } from '@tanstack/react-query';
import { apiGetWithMeta } from '@/lib/api';
import type { RlccWorkflowSummary } from './newRun.types';

export type RlccWorkflowsResult = {
  workflows: RlccWorkflowSummary[];
  source: string;
  /** From X-Qac-Degraded — true when served from fixtures, not live Sourcegraph. */
  degraded: boolean;
};

/**
 * GET /api/rlcc/workflows — RLCC bringup workflows, shown as context in the
 * wizard's inventory step (what runs at the L11/L12 rlcc steps).
 */
export function useRlccWorkflows() {
  return useQuery<RlccWorkflowsResult>({
    queryKey: ['rlcc', 'workflows'],
    queryFn: async () => {
      const { data, headers } = await apiGetWithMeta<{
        workflows: RlccWorkflowSummary[];
        source: string;
      }>('/rlcc/workflows');
      return { ...data, degraded: headers.get('X-Qac-Degraded') === 'true' };
    },
    staleTime: 5 * 60_000,
  });
}
