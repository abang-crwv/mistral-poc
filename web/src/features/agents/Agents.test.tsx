import { describe, it, expect } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { http, HttpResponse } from 'msw';
import { server } from '@/mocks/server';
import Agents from './Agents';

function renderAgents() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Agents />
    </QueryClientProvider>,
  );
}

describe('Agents', () => {
  it('lists agents from /api/agents with a backend badge', async () => {
    renderAgents();
    expect(screen.getByText(/loading agents/i)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText('canary_assessor')).toBeInTheDocument());
    expect(screen.getByText('fw-release-canary.summary.v1')).toBeInTheDocument();
    expect(screen.getByText(/fixture/i)).toBeInTheDocument();
  });

  it('renders an error state when /api/agents fails', async () => {
    server.use(
      http.get('/api/agents', () =>
        HttpResponse.json({ error: { code: 'internal', message: 'boom' } }, { status: 500 }),
      ),
    );
    renderAgents();
    await waitFor(() => expect(screen.getByText(/could not load agents/i)).toBeInTheDocument());
  });
});
