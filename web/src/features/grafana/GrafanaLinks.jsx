// fwqual — Grafana Links.
// A curated catalog of the Grafana dashboards operators open while watching a
// canary run. Links are templated with $rack / $region / $run_id.
import { useState } from 'react';
import { Icon, icons, Tag } from '@/design/components';
import { GRAFANA_BASE, GRAFANA_LINKS } from '@/design/seed';

const _glUseState = useState;

const SCOPE_TINT = {
  rack:   { bg: 'var(--cw-blue-200)',   fg: 'var(--cw-blue-950)' },
  node:   { bg: 'var(--cw-green-100)',  fg: 'var(--status-success-fg)' },
  fabric: { bg: 'var(--cw-yellow-100)', fg: 'var(--status-warning-fg)' },
  run:    { bg: 'var(--cw-gray-100)',   fg: 'var(--cw-gray-900)' },
  fleet:  { bg: 'var(--cw-gray-100)',   fg: 'var(--cw-gray-900)' },
};

function LinkCard({ d }) {
  const href = GRAFANA_BASE + '/' + d.uid;
  const t = SCOPE_TINT[d.scope] || SCOPE_TINT.fleet;
  return (
    <a href={href} target="_blank" rel="noopener noreferrer" style={{
      display: 'flex', flexDirection: 'column', textDecoration: 'none',
      background: 'var(--fw-surface)', border: '1px solid var(--cw-gray-400)', borderRadius: 'var(--card-radius)',
      padding: '15px 16px', transition: 'background 120ms, border-color 120ms',
    }}
      onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--cw-gray-50)'; e.currentTarget.style.borderColor = 'var(--cw-blue)'; }}
      onMouseLeave={(e) => { e.currentTarget.style.background = 'var(--fw-surface)'; e.currentTarget.style.borderColor = 'var(--cw-gray-400)'; }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 10 }}>
        <span style={{ color: 'var(--cw-gray-700)', display: 'flex', marginTop: 1, flexShrink: 0 }}><Icon path={icons.lineChart} size={18} /></span>
        <span style={{ fontFamily: 'var(--font-sans)', fontSize: 14.5, fontWeight: 600, color: 'var(--cw-gray-950)', flex: 1 }}>{d.title}</span>
        <span style={{ color: 'var(--cw-gray-600)', display: 'flex', flexShrink: 0 }}><Icon path={icons.externalLink} size={15} /></span>
      </div>
      <p style={{ margin: '9px 0 0', fontFamily: 'var(--font-sans)', fontSize: 13, color: 'var(--cw-gray-700)', lineHeight: 1.5, flex: 1 }}>{d.desc}</p>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 13, paddingTop: 11, borderTop: '1px solid var(--cw-gray-200)', flexWrap: 'wrap' }}>
        <Tag style={{ fontSize: 10.5, padding: '1px 7px', background: t.bg, color: t.fg }}>{d.scope}</Tag>
        {d.vars.map((v) => (
          <span key={v} style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--cw-gray-700)', background: 'var(--cw-gray-100)', borderRadius: 3, padding: '1px 6px' }}>{v}</span>
        ))}
        <span style={{ marginLeft: 'auto', fontFamily: 'var(--font-mono)', fontSize: 10.5, color: 'var(--cw-gray-600)' }}>{d.uid}</span>
      </div>
    </a>
  );
}

function GrafanaLinks() {
  const [cat, setCat] = _glUseState('all');
  const cats = ['all', ...Array.from(new Set(GRAFANA_LINKS.map((d) => d.category)))];
  const visible = GRAFANA_LINKS.filter((d) => cat === 'all' || d.category === cat);

  return (
    <div style={{ padding: '28px 32px 64px', background: 'var(--cw-bg-canvas, #FCFCFE)', flex: 1, overflow: 'auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 6 }}>
        <div>
          <h1 style={{ margin: 0, fontFamily: 'var(--font-display)', fontWeight: 700, fontSize: 28, color: 'var(--cw-gray-950)', letterSpacing: '-0.01em' }}>Grafana Links</h1>
          <p style={{ margin: '6px 0 0', fontFamily: 'var(--font-sans)', fontSize: 14, color: 'var(--cw-gray-900)', maxWidth: 660 }}>
            Dashboards operators open while watching a canary run. Each link is templated with the run's <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13 }}>$rack</span> / <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13 }}>$region</span> / <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13 }}>$run_id</span> and opens in Grafana.
          </p>
        </div>
      </div>

      <div style={{ display: 'flex', gap: 6, margin: '20px 0 20px', flexWrap: 'wrap' }}>
        {cats.map((c) => {
          const active = cat === c;
          return (
            <button key={c} onClick={() => setCat(c)} style={{
              fontFamily: 'var(--font-ui)', fontSize: 12.5, fontWeight: 600, height: 30, padding: '0 12px',
              borderRadius: 4, cursor: 'pointer', border: '1px solid ' + (active ? 'var(--cw-blue)' : 'var(--cw-gray-400)'),
              background: active ? 'var(--cw-blue-200)' : 'var(--fw-surface)', color: active ? 'var(--cw-blue-800)' : 'var(--cw-gray-900)',
            }}>{c === 'all' ? 'All dashboards' : c}</button>
          );
        })}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(310px, 1fr))', gap: 14 }}>
        {visible.map((d) => <LinkCard key={d.id} d={d} />)}
      </div>
    </div>
  );
}


export default GrafanaLinks;
