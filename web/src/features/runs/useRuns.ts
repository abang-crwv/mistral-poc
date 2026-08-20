import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import type { Run } from './runs.types';

export function useRuns() {
  return useQuery({
    queryKey: ['runs'],
    queryFn: () => apiGet<{ runs: Run[] }>('/runs').then((r) => r.runs),
    staleTime: 5_000,
  });
}
