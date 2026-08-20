import { Icon, icons } from '@/design/components';

export function Loading({ label = 'Loading…' }) {
  return (
    <main style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--cw-gray-700)', fontFamily: 'var(--font-sans)', gap: 10 }}>
      <span style={{ display: 'inline-flex', animation: 'fwspin 0.8s linear infinite' }}><Icon path={icons.spinner} size={18} /></span>{label}
    </main>
  );
}

export function ErrorPane({ message }) {
  return (
    <main style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', flexDirection: 'column', gap: 8, color: 'var(--status-danger-fg)', fontFamily: 'var(--font-sans)' }}>
      <Icon path={icons.alert} size={22} />
      <div style={{ fontWeight: 600 }}>Could not load</div>
      <div style={{ color: 'var(--cw-gray-700)', fontSize: 13 }}>{message}</div>
    </main>
  );
}
