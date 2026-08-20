import { useQuery } from '@tanstack/react-query';
import { apiGetWithMeta } from '@/lib/api';
import type { InventoryPreview } from './newRun.types';

/** InventoryPreview plus the client-derived `degraded` flag (from the
 *  X-Qac-Degraded response header — fixture data rather than live inventory). */
export type InventoryPreviewResult = InventoryPreview & { degraded: boolean };

/**
 * GET /api/inventory/preview?racks=a,b,c
 *
 * Resolves per-rack facts (instance_type / sku / zone), the BMN/CT
 * enumeration, and any CTs already ignored in RLCC. Unresolved racks come
 * back under `errors[]` rather than failing the whole call.
 *
 * staleTime matches the server's 60s preview cache TTL so iterating the
 * wizard doesn't hammer inventory.
 */
export function useInventoryPreview(racks: string[]) {
  const joined = racks.join(',');
  return useQuery<InventoryPreviewResult>({
    queryKey: ['inventory', 'preview', joined],
    queryFn: async () => {
      const { data, headers } = await apiGetWithMeta<InventoryPreview>(
        `/inventory/preview?racks=${encodeURIComponent(joined)}`,
      );
      // Normalize: Go marshals nil slices as `null`. The TS contract says
      // these are arrays, so coalesce here once rather than guarding every
      // call site. Without this, a request where every rack is unresolved
      // returns `racks: null` and crashes `data.racks.map(...)` downstream.
      return {
        ...data,
        racks: data.racks ?? [],
        errors: data.errors ?? [],
        degraded: headers.get('X-Qac-Degraded') === 'true',
      };
    },
    enabled: racks.length > 0,
    staleTime: 60_000,
  });
}
