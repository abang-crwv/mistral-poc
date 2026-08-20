import { createBrowserRouter, Navigate } from 'react-router-dom';
import AppShell from '@/app/AppShell';
import StubPage from '@/design/StubPage';
import { CanaryRoute, RunDetailRoute } from '@/features/canary/routes';
import { NodeTracker, RackTracker } from '@/features/tracking/Trackers';
import Jira from '@/features/jira/Jira';
import Knowledge from '@/features/knowledge/Knowledge';
import GrafanaLinks from '@/features/grafana/GrafanaLinks';
import Silencer from '@/features/silences/Silencer';
import WebPush from '@/features/notifications/WebPush';
import Probes from '@/features/probes/Probes';
import Agents from '@/features/agents/Agents';

export const router = createBrowserRouter([
  {
    path: '/',
    element: <AppShell />,
    children: [
      { index: true, element: <Navigate to="/canary" replace /> },
      { path: 'canary', element: <CanaryRoute /> },
      { path: 'runs/:id', element: <RunDetailRoute /> },
      { path: 'node-tracker', element: <NodeTracker /> },
      { path: 'rack-tracker', element: <RackTracker /> },
      { path: 'jira', element: <Jira onOpenRun={() => {}} /> },
      { path: 'knowledge', element: <Knowledge /> },
      { path: 'grafana', element: <GrafanaLinks /> },
      { path: 'silences', element: <Silencer /> },
      { path: 'notifications', element: <WebPush /> },
      { path: 'l11-fielddiag', element: <StubPage title="L11 FieldDiag" /> },
      { path: 'templates', element: <StubPage title="Templates" /> },
      { path: 'probes', element: <Probes /> },
      { path: 'agents', element: <Agents /> },
      { path: 'history', element: <StubPage title="History" /> },
      { path: 'reports', element: <StubPage title="Reports" /> },
      { path: 'integrations', element: <StubPage title="Integrations" /> },
      { path: 'settings', element: <StubPage title="Settings" /> },
      { path: '*', element: <StubPage title="Not found" /> },
    ],
  },
]);
