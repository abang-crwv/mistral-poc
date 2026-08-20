import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import { adaptAlertEvidence } from './adapters';
import type { AlertEvidence } from './types';

interface EvidenceRow {
  deviceslot: string | null;
  payload: Record<string, unknown>;
}

export function useRunAlertEvidence(id: string) {
  return useQuery<AlertEvidence>({
    queryKey: ['canary', 'runs', id, 'alert-evidence'],
    queryFn: async () => {
      const rows =
        (await apiGet<{ evidence: EvidenceRow[] }>(`/runs/${id}/evidence?step=alert_baseline`))
          .evidence ?? [];
      const aggregate = rows.find((r) => r.deviceslot == null);
      return adaptAlertEvidence(aggregate?.payload);
    },
  });
}
