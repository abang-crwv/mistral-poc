import { Icon, icons, Eyebrow } from '@/design/components';

export default function StubPage({ title }) {
  return (
    <main style={{ flex: 1, overflow: 'auto', padding: '32px 40px', fontFamily: 'var(--font-sans)' }}>
      <Eyebrow>QAgenticCow</Eyebrow>
      <h1 style={{ fontFamily: 'var(--font-display)', fontSize: 24, fontWeight: 700, color: 'var(--cw-gray-950)', margin: '6px 0 4px' }}>{title}</h1>
      <p style={{ color: 'var(--cw-gray-700)', fontSize: 14, maxWidth: 520 }}>
        This screen is not built yet. It is a placeholder in the navigation so the full console layout is visible.
      </p>
      <div style={{ marginTop: 20, display: 'inline-flex', alignItems: 'center', gap: 8, padding: '8px 12px', border: '1px solid var(--cw-gray-400)', borderRadius: 4, color: 'var(--cw-gray-700)', fontSize: 12.5 }}>
        <Icon path={icons.clock} size={16} /> Planned
      </div>
    </main>
  );
}
