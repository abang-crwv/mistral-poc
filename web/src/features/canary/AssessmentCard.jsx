// AssessmentCard renders an agent's advisory verdict for an ai_assess step.
// `assessment` is the parsed payload of an evidence row whose content_type is
// application/vnd.qac.assessment+json.
const verdictColor = {
  pass: { bg: 'var(--cw-green-100, #e3f6e8)', fg: 'var(--cw-green-800, #1f7a3d)' },
  needs_review: { bg: 'var(--cw-amber-100, #fdf0d5)', fg: 'var(--cw-amber-800, #8a5a00)' },
  fail: { bg: 'var(--cw-red-100, #fde8e6)', fg: 'var(--cw-red-800, #b42318)' },
};

export default function AssessmentCard({ assessment }) {
  const a = assessment || {};
  const c = verdictColor[a.verdict] || {
    bg: 'var(--cw-gray-200, #eee)',
    fg: 'var(--cw-gray-800, #444)',
  };
  return (
    <div
      style={{
        border: '1px solid var(--cw-gray-400)',
        borderRadius: 6,
        padding: '14px 16px',
        background: 'var(--cw-white, #fff)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
        <span
          style={{
            fontSize: 12,
            fontWeight: 700,
            textTransform: 'uppercase',
            letterSpacing: '.04em',
            padding: '3px 10px',
            borderRadius: 999,
            background: c.bg,
            color: c.fg,
          }}
        >
          {a.verdict || 'unknown'}
        </span>
        <span style={{ fontSize: 12.5, color: 'var(--cw-gray-700)' }}>
          confidence: {a.confidence || 'unknown'}
        </span>
        <span
          style={{
            fontSize: 11.5,
            color: 'var(--cw-gray-700)',
            border: '1px solid var(--cw-gray-400)',
            borderRadius: 4,
            padding: '2px 8px',
          }}
        >
          advisory — operator decides
        </span>
      </div>
      {a.reasoning && (
        <p style={{ margin: '10px 0 0', fontSize: 13.5, color: 'var(--cw-gray-900)' }}>
          {a.reasoning}
        </p>
      )}
      {a.likely_owner && (
        <p style={{ margin: '6px 0 0', fontSize: 13, color: 'var(--cw-gray-700)' }}>
          Likely owner: {a.likely_owner}
        </p>
      )}
      {Array.isArray(a.ranked_causes) && a.ranked_causes.length > 0 && (
        <ul style={{ margin: '8px 0 0', paddingLeft: 18, fontSize: 13, color: 'var(--cw-gray-800)' }}>
          {a.ranked_causes.map((cause, i) => (
            <li key={i}>
              {cause.summary}
              {cause.likely_owner ? ` — ${cause.likely_owner}` : ''}
            </li>
          ))}
        </ul>
      )}
      {Array.isArray(a.sources) && a.sources.length > 0 && (
        <p style={{ margin: '8px 0 0', fontSize: 12, color: 'var(--cw-gray-600)' }}>
          Source: {a.sources.join(', ')}
        </p>
      )}
    </div>
  );
}
