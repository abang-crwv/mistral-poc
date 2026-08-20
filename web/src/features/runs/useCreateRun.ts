import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiPost } from '@/lib/api';
import type { CreateRunInput } from './newRun.types';
import type { Run } from './runs.types';

/**
 * POST /api/runs with the canonical {template_id, inputs, ignored_bmns} shape.
 *
 * NOTE: this replaces the legacy version that posted {bundle, rack}. The
 * backend still accepts the legacy shape, but the wizard always sends the
 * canonical one. Errors surface as ApiException (see lib/api.ts) — callers
 * branch on `code` (e.g. 'inventory_unresolved', 'template_not_found').
 */
export function useCreateRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateRunInput) => apiPost<{ run: Run }, CreateRunInput>('/runs', input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['runs'] });
    },
  });
}
