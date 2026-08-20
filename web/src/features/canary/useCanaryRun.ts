import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import { adaptRun, adaptEvents } from './adapters';
import type { RawRun, RawEvent, CanaryRun, RunEvent } from './types';

export interface CanaryRunDetail {
  run: CanaryRun;
  events: RunEvent[];
  racks: string[];
}

export function useCanaryRun(id: string) {
  return useQuery<CanaryRunDetail>({
    queryKey: ['canary', 'runs', id],
    queryFn: async () => {
      const d = await apiGet<{ run: RawRun; events: RawEvent[]; racks: string[] }>(`/runs/${id}`);
      return {
        run: adaptRun(d.run, d.events ?? []),
        events: adaptEvents(d.events ?? []),
        racks: d.racks ?? [],
      };
    },
  });
}
