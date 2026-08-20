import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiPost } from '@/lib/api';

/** One operator-supplied tray to exclude from the run. */
export interface IgnoredBMNInput {
  rack: string;
  deviceslot: string;
  reason?: string;
}

export interface CreateCanaryInput {
  bundle: string;
  racks: string[];
  requester: string;
  rlcc_workflow: string;
  /** Trays the operator chose to ignore in the tray-review step. */
  ignored_bmns?: IgnoredBMNInput[];
}

export function useCreateCanaryRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateCanaryInput) =>
      apiPost<{ run: { id: string } }, unknown>('/runs', {
        // Pre-zap baseline phase only (alert baseline + inventory facts +
        // baseline_pre). The full firmware-release-canary pipeline is paused
        // while we iterate on the baseline; runs against this template stop
        // after the pre-zap health snapshot instead of walking into L11.
        template_id: 'firmware-prezap-baseline',
        inputs: {
          bundle_tag: input.bundle,
          canary_racks: input.racks,
          requester: input.requester,
          rlcc_workflow: input.rlcc_workflow,
        },
        ignored_bmns: input.ignored_bmns ?? [],
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['canary', 'runs'] });
    },
  });
}
