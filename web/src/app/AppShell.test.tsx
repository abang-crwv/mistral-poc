import { type ReactNode } from 'react';
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { RouterProvider, createMemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import AppShell from '@/app/AppShell';
import StubPage from '@/design/StubPage';
import RunsList from '@/features/canary/RunsList';

// Renders the shell (Header + Sidebar) with one child route element, inside a
// memory router so the design Sidebar's useNavigate has a router context.
function renderShell(childPath: string, element: ReactNode, initial: string) {
  const router = createMemoryRouter(
    [{ path: '/', element: <AppShell />, children: [{ path: childPath, element }] }],
    { initialEntries: [initial] },
  );
  const qc = new QueryClient();
  return render(
    <QueryClientProvider client={qc}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

describe('AppShell + nav', () => {
  it('renders the sidebar sections and the Canary FW Release item', () => {
    renderShell(
      'canary',
      <RunsList onOpenRun={() => {}} onNewRun={() => {}} density="compact" />,
      '/canary',
    );
    // Sidebar eyebrow + the one real feature.
    expect(screen.getByText('QA Workflows')).toBeInTheDocument();
    expect(screen.getByText('Canary FW Release')).toBeInTheDocument();
    // A few other nav items are present (visible-but-stub).
    expect(screen.getByText('Node Tracker')).toBeInTheDocument();
    expect(screen.getByText('Knowledge')).toBeInTheDocument();
    expect(screen.getByText('Settings')).toBeInTheDocument();
  });

  it('renders a stub page without crashing', () => {
    renderShell('templates', <StubPage title="Templates" />, '/templates');
    // 'Templates' also appears as a sidebar nav item, so scope to the page heading.
    expect(screen.getByRole('heading', { name: 'Templates' })).toBeInTheDocument();
    expect(screen.getByText(/not built yet/i)).toBeInTheDocument();
  });
});
