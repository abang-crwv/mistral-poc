import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import { adaptRun } from './adapters';
import type { RawRun, CanaryRun } from './types';

export function useCanaryRuns() {
  return useQuery<CanaryRun[]>({
    queryKey: ['canary', 'runs'],
    queryFn: async () =>
      (await apiGet<{ runs: RawRun[] }>('/runs')).runs.map((r) => adaptRun(r, [])),
  });
}
