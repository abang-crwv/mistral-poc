import { type ReactNode } from 'react';
import { describe, it, expect } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { RouterProvider, createMemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { http, HttpResponse } from 'msw';
import { server } from '@/mocks/server';
import { CanaryRoute, RunDetailRoute } from '@/features/canary/routes';

// Render one route element inside a memory router (the canary routes use
// useNavigate/useParams) and a fresh QueryClient (retries off so error states
// surface immediately). MSW is active via setupTests.ts.
function renderRoute(path: string, element: ReactNode, initial: string) {
  const router = createMemoryRouter([{ path, element }], { initialEntries: [initial] });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

describe('CanaryRoute', () => {
  it('shows the loading state, then renders a mocked run bundle', async () => {
    renderRoute('/canary', <CanaryRoute />, '/canary');
    expect(screen.getByText(/loading runs/i)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText('gb200-fw-2026-05-canary-3')).toBeInTheDocument());
    // Adapter derived racks from inputs.canary_racks.
    expect(screen.getAllByText('dh3-r012-us-east-01a').length).toBeGreaterThan(0);
  });

  it('renders the ErrorPane when the runs request fails', async () => {
    server.use(
      http.get('/api/runs', () =>
        HttpResponse.json({ error: { code: 'internal', message: 'boom' } }, { status: 500 }),
      ),
    );
    renderRoute('/canary', <CanaryRoute />, '/canary');
    await waitFor(() => expect(screen.getByText(/could not load/i)).toBeInTheDocument());
    expect(screen.getByText('boom')).toBeInTheDocument();
  });
});

describe('RunDetailRoute', () => {
  it('renders the run id, steps, and discovered facts', async () => {
    renderRoute('/runs/:id', <RunDetailRoute />, '/runs/01HZX0000000000000000000A');
    await waitFor(() => expect(screen.getByText('01HZX0000000000000000000A')).toBeInTheDocument());
    // Step rail rendered from the real template steps.
    expect(screen.getByText('5 steps')).toBeInTheDocument();
    expect(screen.getAllByText('Alert baseline').length).toBeGreaterThan(0);
    // Discovered facts mapped from /facts.
    expect(screen.getByText('gb200-4x')).toBeInTheDocument();
  });

  it('renders the advisory assessment card for an ai_assess evidence row', async () => {
    // Template with an ai_assess step (the shared mock has none) so StepPanel
    // has an ai_assess step to select.
    server.use(
      http.get('/api/templates/:id', () =>
        HttpResponse.json({
          template: {
            schema: 'qac.template/v1',
            id: 'firmware-release-canary',
            version: 9,
            title: 'FW release canary',
            target: { scope: 'rack' },
            inputs: [],
            steps: [
              {
                id: 'alert_baseline',
                kind: 'probe',
                title: 'Alert baseline',
                probe_type: 'alert_probe',
              },
              { id: 'ai_summary', kind: 'ai_assess', title: 'AI assessment' },
              { id: 'signoff', kind: 'signoff', title: 'Sign off' },
            ],
          },
        }),
      ),
      // The ai_assess step persisted an assessment evidence row. useRunAssessment
      // queries the whole run (no ?step= filter) and keys off content_type;
      // useRunAlertEvidence queries ?step=alert_baseline (empty here).
      http.get('/api/runs/:id/evidence', ({ request }) => {
        const step = new URL(request.url).searchParams.get('step');
        if (step === 'alert_baseline') return HttpResponse.json({ evidence: [] });
        return HttpResponse.json({
          evidence: [
            {
              id: 'ev-assess-1',
              step_id: 'ai_summary',
              deviceslot: null,
              content_type: 'application/vnd.qac.assessment+json',
              created_at: 1748358245,
              payload: {
                verdict: 'fail',
                confidence: 'high',
                reasoning: 'Two racks did not converge.',
                ranked_causes: [{ summary: 'BMC firmware stuck', likely_owner: 'hardware' }],
                likely_owner: 'hardware',
                sources: ['claude-opus-4-8'],
              },
            },
          ],
        });
      }),
    );

    renderRoute('/runs/:id', <RunDetailRoute />, '/runs/01HZX0000000000000000000A');
    await waitFor(() => expect(screen.getByText('01HZX0000000000000000000A')).toBeInTheDocument());
    // Select the ai_assess step in the rail to open its panel.
    fireEvent.click(screen.getByText('AI assessment'));
    // The card rendered end-to-end: content_type → useRunAssessment → the
    // StepPanel ai_assess branch → AssessmentCard.
    expect(await screen.findByText(/advisory — operator decides/i)).toBeInTheDocument();
    expect(screen.getByText(/did not converge/i)).toBeInTheDocument();
  });
});

describe('Canary writes', () => {
  it('NewRunModal submit POSTs /api/runs with the mapped body', async () => {
    let captured: Record<string, unknown> | null = null;
    server.use(
      http.post('/api/runs', async ({ request }) => {
        captured = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ run: { id: '01HZX0000000000000000000Z' } }, { status: 201 });
      }),
    );
    renderRoute('/canary', <CanaryRoute />, '/canary');
    await waitFor(() => expect(screen.getByText('gb200-fw-2026-05-canary-3')).toBeInTheDocument());

    fireEvent.click(screen.getByText('New canary run'));
    // Fill the form.
    fireEvent.change(screen.getByPlaceholderText('dell-xe9712-default-2.0.0-rc'), {
      target: { value: 'gb200-fw-2026-06-canary-1' },
    });
    fireEvent.change(screen.getByPlaceholderText(/dh3-r012-us-east-01a, dh3-r014/), {
      target: { value: 'dh3-r012-us-east-01a' },
    });
    fireEvent.change(screen.getByPlaceholderText('k.osei'), { target: { value: 'k.osei' } });
    // Workflow <select> is populated from useRlccWorkflows, filtered to
    // workflows that contain the L11 field-diag step.
    await waitFor(() =>
      expect(screen.getByRole('option', { name: 'gb200-rack-bringup-v4' })).toBeInTheDocument(),
    );
    // The power-off workflow has no l11-fielddiag handler — it must be hidden.
    expect(screen.queryByRole('option', { name: 'gb200-rack-power-off' })).not.toBeInTheDocument();
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'gb200-rack-bringup-v4' } });

    // Step 2: advance to the per-rack tray review, then create.
    fireEvent.click(screen.getByText('Next: review trays'));
    await waitFor(() => expect(screen.getByText(/compute trays/i)).toBeInTheDocument());
    fireEvent.click(screen.getByText('Create run'));
    await waitFor(() => expect(captured).not.toBeNull());
    expect(captured).toEqual({
      template_id: 'firmware-prezap-baseline',
      inputs: {
        bundle_tag: 'gb200-fw-2026-06-canary-1',
        canary_racks: ['dh3-r012-us-east-01a'],
        requester: 'k.osei',
        rlcc_workflow: 'gb200-rack-bringup-v4',
      },
      ignored_bmns: [],
    });
  });

  it('ignoring a tray in the review step posts it in ignored_bmns', async () => {
    let captured: Record<string, unknown> | null = null;
    server.use(
      http.post('/api/runs', async ({ request }) => {
        captured = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ run: { id: '01HZX0000000000000000000Z' } }, { status: 201 });
      }),
    );
    renderRoute('/canary', <CanaryRoute />, '/canary');
    await waitFor(() => expect(screen.getByText('gb200-fw-2026-05-canary-3')).toBeInTheDocument());

    fireEvent.click(screen.getByText('New canary run'));
    fireEvent.change(screen.getByPlaceholderText('dell-xe9712-default-2.0.0-rc'), {
      target: { value: 'gb200-fw-2026-06-canary-1' },
    });
    fireEvent.change(screen.getByPlaceholderText(/dh3-r012-us-east-01a, dh3-r014/), {
      target: { value: 'dh3-r012-us-east-01a' },
    });
    fireEvent.change(screen.getByPlaceholderText('k.osei'), { target: { value: 'k.osei' } });
    await waitFor(() =>
      expect(screen.getByRole('option', { name: 'gb200-rack-bringup-v4' })).toBeInTheDocument(),
    );
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'gb200-rack-bringup-v4' } });

    fireEvent.click(screen.getByText('Next: review trays'));
    // The first tray's deviceslot is clickable; toggling it marks it ignored.
    await waitFor(() => expect(screen.getByText('ct-01')).toBeInTheDocument());
    fireEvent.click(screen.getByText('ct-01'));
    fireEvent.click(screen.getByText('Create run'));

    await waitFor(() => expect(captured).not.toBeNull());
    expect(captured!.ignored_bmns).toEqual([{ rack: 'dh3-r012-us-east-01a', deviceslot: 'ct-01' }]);
  });

  it('Sign off POSTs /api/runs/:id/actions with signer fields and verdict', async () => {
    let captured: Record<string, unknown> | null = null;
    server.use(
      http.post('/api/runs/:id/actions', async ({ request }) => {
        captured = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ run: {} }, { status: 200 });
      }),
    );
    // Run B is `passed` — the header Sign off button shows and the SignoffPanel
    // renders its inputs.
    renderRoute('/runs/:id', <RunDetailRoute />, '/runs/01HZX0000000000000000000B');
    await waitFor(() => expect(screen.getByText('01HZX0000000000000000000B')).toBeInTheDocument());

    // Header "Sign off" button selects the signoff step, revealing the panel
    // inputs. (The step rail also renders a "Sign off" step title, so target
    // the actual <button> elements.)
    const headerSignOff = screen
      .getAllByRole('button')
      .find((b) => b.textContent?.includes('Sign off'));
    expect(headerSignOff).toBeDefined();
    fireEvent.click(headerSignOff!);
    fireEvent.change(await screen.findByPlaceholderText('k.osei'), {
      target: { value: 'k.osei' },
    });
    fireEvent.change(screen.getByPlaceholderText('fleet release engineer'), {
      target: { value: 'release engineer' },
    });
    // The panel's own "Sign off" button (the last "Sign off" button — header is first).
    const signOffButtons = screen
      .getAllByRole('button')
      .filter((b) => b.textContent?.includes('Sign off'));
    fireEvent.click(signOffButtons[signOffButtons.length - 1]);

    await waitFor(() => expect(captured).not.toBeNull());
    expect(captured).toEqual({
      action: 'signoff',
      step_id: 'signoff',
      verdict: 'pass',
      signer_name: 'k.osei',
      signer_role: 'release engineer',
    });
  });
});
