// Maps the design Sidebar's nav keys to router paths and back.
export const navKeyToPath: Record<string, string> = {
  runs: '/canary',
  nodetracker: '/node-tracker',
  racktracker: '/rack-tracker',
  l11dance: '/l11-fielddiag',
  jira: '/jira',
  knowledge: '/knowledge',
  templates: '/templates',
  probes: '/probes',
  agents: '/agents',
  grafana: '/grafana',
  evidence: '/history',
  reports: '/reports',
  silencer: '/silences',
  webpush: '/notifications',
  integrations: '/integrations',
  settings: '/settings',
};

// Which nav key should appear active for a given pathname. /canary and
// /runs/:id both light up Canary FW Release ('runs').
export function pathToNavKey(pathname: string): string {
  if (pathname === '/' || pathname.startsWith('/canary') || pathname.startsWith('/runs/')) {
    return 'runs';
  }
  const hit = Object.entries(navKeyToPath).find(
    ([, p]) => p !== '/canary' && pathname.startsWith(p),
  );
  return hit ? hit[0] : 'runs';
}
