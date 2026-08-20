import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import type { Run, Event } from './runs.types';

export function useRun(id: string) {
  return useQuery({
    queryKey: ['runs', id],
    queryFn: () => apiGet<{ run: Run; events: Event[] }>(`/runs/${id}`),
    enabled: !!id,
    staleTime: 5_000,
  });
}
