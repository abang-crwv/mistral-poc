import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import type { Probe } from './types';

// useProbes fetches the registered probe types from GET /api/probes.
export function useProbes() {
  return useQuery<Probe[]>({
    queryKey: ['probes'],
    queryFn: async () => (await apiGet<{ probes: Probe[] }>('/probes')).probes,
  });
}
