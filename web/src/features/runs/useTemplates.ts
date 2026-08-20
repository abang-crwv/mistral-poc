import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import type { Template, TemplateSummary } from './newRun.types';

/** GET /api/templates — list of template summaries for the wizard picker. */
export function useTemplates() {
  return useQuery({
    queryKey: ['templates'],
    queryFn: () => apiGet<{ templates: TemplateSummary[] }>('/templates').then((r) => r.templates),
    staleTime: 60_000,
  });
}

/** GET /api/templates/{id} — the full template, whose inputs[] drive the form. */
export function useTemplate(id: string) {
  return useQuery({
    queryKey: ['templates', id],
    queryFn: () => apiGet<{ template: Template }>(`/templates/${id}`).then((r) => r.template),
    enabled: !!id,
    staleTime: 60_000,
  });
}
