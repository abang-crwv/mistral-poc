import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import { adaptFacts } from './adapters';
import type { Facts } from './types';

export function useRunFacts(id: string, racks: string[]) {
  return useQuery<Facts>({
    queryKey: ['canary', 'runs', id, 'facts'],
    queryFn: async () =>
      adaptFacts(
        (await apiGet<{ facts: Record<string, unknown> }>(`/runs/${id}/facts`)).facts,
        racks,
      ),
  });
}
