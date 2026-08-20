import { describe, it, expect } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { http, HttpResponse } from 'msw';
import { server } from '@/mocks/server';
import Probes from './Probes';

function renderProbes() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Probes />
    </QueryClientProvider>,
  );
}

describe('Probes', () => {
  it('shows loading, then lists probes from /api/probes', async () => {
    renderProbes();
    expect(screen.getByText(/loading probes/i)).toBeInTheDocument();
    // Titles + type identifiers from the mocked registry response.
    await waitFor(() => expect(screen.getByText('AWX zap jobs')).toBeInTheDocument());
    expect(screen.getByText('awx_job_probe')).toBeInTheDocument();
    expect(screen.getByText('firmware_inventory_probe')).toBeInTheDocument();
    // The retired fact_probe name must be gone.
    expect(screen.queryByText('fact_probe')).not.toBeInTheDocument();
  });

  it('renders an error state when /api/probes fails', async () => {
    server.use(
      http.get('/api/probes', () =>
        HttpResponse.json({ error: { code: 'internal', message: 'boom' } }, { status: 500 }),
      ),
    );
    renderProbes();
    await waitFor(() => expect(screen.getByText(/could not load probes/i)).toBeInTheDocument());
  });

  it('renders the empty state when no probes are registered', async () => {
    server.use(http.get('/api/probes', () => HttpResponse.json({ probes: [] })));
    renderProbes();
    await waitFor(() => expect(screen.getByText(/no probes are registered/i)).toBeInTheDocument());
  });
});
