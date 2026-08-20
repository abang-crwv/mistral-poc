import { useAgents } from './useAgents';

const muted = 'var(--cw-gray-700)';

export default function Agents() {
  const { data, isLoading, isError, error } = useAgents();

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
        Agents
      </h1>
      <p style={{ color: muted, fontSize: 14, maxWidth: 640 }}>
        The agents that reason over gathered evidence to produce advisory verdicts. Agents narrow
        and rank; the operator decides.
      </p>

      {isLoading && <p style={{ marginTop: 20, color: muted, fontSize: 13.5 }}>Loading agents…</p>}
      {isError && (
        <p style={{ marginTop: 20, color: 'var(--cw-red-700, #b42318)', fontSize: 13.5 }}>
          Could not load agents. {(error as Error)?.message ?? ''}
        </p>
      )}

      {data && (
        <div style={{ marginTop: 20, display: 'grid', gap: 12, maxWidth: 820 }}>
          {data.map((a) => (
            <div
              key={a.type}
              style={{
                border: '1px solid var(--cw-gray-400)',
                borderRadius: 6,
                padding: '14px 16px',
                background: 'var(--cw-white, #fff)',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
                <span style={{ fontWeight: 600, fontSize: 15, color: 'var(--cw-gray-950)' }}>
                  {a.type}
                </span>
                <span
                  style={{
                    fontSize: 11.5,
                    fontWeight: 600,
                    textTransform: 'uppercase',
                    letterSpacing: '.04em',
                    padding: '2px 8px',
                    borderRadius: 999,
                    background: a.live
                      ? 'var(--cw-green-100, #e3f6e8)'
                      : 'var(--cw-gray-200, #eee)',
                    color: a.live ? 'var(--cw-green-800, #1f7a3d)' : 'var(--cw-gray-800, #444)',
                  }}
                >
                  {a.live ? 'live' : 'fixture'}: {a.model}
                </span>
              </div>
              <code
                style={{ fontSize: 12.5, color: muted, fontFamily: 'var(--font-mono, monospace)' }}
              >
                {a.prompt_ref}
              </code>
              <p style={{ margin: '6px 0 0', fontSize: 13.5, color: 'var(--cw-gray-800)' }}>
                {a.purpose}
              </p>
            </div>
          ))}
          {data.length === 0 && (
            <p style={{ color: muted, fontSize: 13.5 }}>No agents are registered.</p>
          )}
        </div>
      )}
    </main>
  );
}
