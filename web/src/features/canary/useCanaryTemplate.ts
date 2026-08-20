import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import { adaptTemplateSteps } from './adapters';
import type { RawTemplate, CanaryStep } from './types';

export interface CanaryTemplate {
  version: number;
  flatSteps: CanaryStep[];
  stepIndex: Record<string, number>;
}

export function useCanaryTemplate(id: string) {
  return useQuery<CanaryTemplate>({
    queryKey: ['canary', 'template', id],
    queryFn: async () => {
      const tpl = await apiGet<{ template: RawTemplate }>(`/templates/${id}`).then(
        (r) => r.template,
      );
      const { flatSteps, stepIndex } = adaptTemplateSteps(tpl);
      return { version: tpl.version, flatSteps, stepIndex };
    },
  });
}
