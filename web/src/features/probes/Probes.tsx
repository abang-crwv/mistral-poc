import { useProbes } from './useProbes';

// categoryStyle colors the category chip. Unknown categories fall back to gray.
function categoryStyle(category: string): { bg: string; fg: string } {
  switch (category) {
    case 'gatherer':
      return { bg: 'var(--cw-blue-100, #e6efff)', fg: 'var(--cw-blue-800, #1b4fa0)' };
    case 'assertion':
      return { bg: 'var(--cw-amber-100, #fdf0d5)', fg: 'var(--cw-amber-800, #8a5a00)' };
    case 'action':
      return { bg: 'var(--cw-green-100, #e3f6e8)', fg: 'var(--cw-green-800, #1f7a3d)' };
    default:
      return { bg: 'var(--cw-gray-200, #eee)', fg: 'var(--cw-gray-800, #444)' };
  }
}

const muted = 'var(--cw-gray-700)';

export default function Probes() {
  const { data, isLoading, isError, error } = useProbes();

  return (
    <main
      style={{ flex: 1, overflow: 'auto', padding: '32px 40px', fontFamily: 'var(--font-sans)' }}
    >
      <div
        style={{ fontSize: 12, letterSpacing: '.08em', textTransform: 'uppercase', color: muted }}
      >
        QAgenticCow
      </div>
      <h1
        style={{
          fontFamily: 'var(--font-display)',
          fontSize: 24,
          fontWeight: 700,
          color: 'var(--cw-gray-950)',
          margin: '6px 0 4px',
        }}
      >
        Probes
      </h1>
      <p style={{ color: muted, fontSize: 14, maxWidth: 640 }}>
        The probe types the engine can run as canary steps. Each probe gathers evidence or performs
        an action; gatherers produce no verdict, so pre and post comparison happens across runs.
      </p>

      {isLoading && <p style={{ marginTop: 20, color: muted, fontSize: 13.5 }}>Loading probes…</p>}
      {isError && (
        <p style={{ marginTop: 20, color: 'var(--cw-red-700, #b42318)', fontSize: 13.5 }}>
          Could not load probes. {(error as Error)?.message ?? ''}
        </p>
      )}

      {data && (
        <div style={{ marginTop: 20, display: 'grid', gap: 12, maxWidth: 820 }}>
          {data.map((p) => {
            const c = categoryStyle(p.category);
            return (
              <div
                key={p.type}
                style={{
                  border: '1px solid var(--cw-gray-400)',
                  borderRadius: 6,
                  padding: '14px 16px',
                  background: 'var(--cw-white, #fff)',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
                  <span style={{ fontWeight: 600, fontSize: 15, color: 'var(--cw-gray-950)' }}>
                    {p.title}
                  </span>
                  <span
                    style={{
                      fontSize: 11.5,
                      fontWeight: 600,
                      textTransform: 'uppercase',
                      letterSpacing: '.04em',
                      padding: '2px 8px',
                      borderRadius: 999,
                      background: c.bg,
                      color: c.fg,
                    }}
                  >
                    {p.category}
                  </span>
                </div>
                <code
                  style={{
                    fontSize: 12.5,
                    color: muted,
                    fontFamily: 'var(--font-mono, monospace)',
                  }}
                >
                  {p.type}
                </code>
                {p.description && (
                  <p style={{ margin: '6px 0 0', fontSize: 13.5, color: 'var(--cw-gray-800)' }}>
                    {p.description}
                  </p>
                )}
              </div>
            );
          })}
          {data.length === 0 && (
            <p style={{ color: muted, fontSize: 13.5 }}>No probes are registered.</p>
          )}
        </div>
      )}
    </main>
  );
}
