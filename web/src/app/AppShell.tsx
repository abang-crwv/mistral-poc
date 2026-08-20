import { useEffect, useState } from 'react';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { Header, Sidebar } from '@/design/Shell';
import { RUNS } from '@/design/seed';
import { navKeyToPath, pathToNavKey } from '@/app/nav';
import Login from '@/auth/Login';

export default function AppShell() {
  const navigate = useNavigate();
  const location = useLocation();
  const [showGate, setShowGate] = useState(false);

  // Dark is the only theme (toggle removed). Token overrides hang off
  // <html data-theme="dark"> — see styles/theme.css.
  useEffect(() => {
    document.documentElement.dataset.theme = 'dark';
  }, []);

  // Accent: fixed at the design default (the Tweaks panel that varied it is dropped).
  useEffect(() => {
    const root = document.documentElement;
    for (const v of ['--cw-blue', '--cw-blue-700', '--cw-blue-800']) {
      root.style.setProperty(v, '#4264ff');
    }
  }, []);

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        height: '100vh',
        background: 'var(--cw-gray-400)',
      }}
    >
      <Header health="ok · :8080" onLock={() => setShowGate(true)} />
      <div
        style={{
          display: 'flex',
          flex: 1,
          overflow: 'hidden',
          gap: 1,
          background: 'var(--cw-gray-400)',
        }}
      >
        <Sidebar
          current={pathToNavKey(location.pathname)}
          onNav={(key: string) => navigate(navKeyToPath[key] ?? '/canary')}
          runCount={RUNS.length}
        />
        <Outlet />
      </div>
      {showGate && <Login onAcknowledge={() => setShowGate(false)} />}
    </div>
  );
}
