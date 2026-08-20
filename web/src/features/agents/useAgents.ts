import { useQuery } from '@tanstack/react-query';
import { apiGet } from '@/lib/api';
import type { Agent } from './types';

// useAgents fetches the registered agents from GET /api/agents.
export function useAgents() {
  return useQuery<Agent[]>({
    queryKey: ['agents'],
    queryFn: async () => (await apiGet<{ agents: Agent[] }>('/agents')).agents,
  });
}
