// fwqual — Canary Runs list (dashboard + table).
import { useState } from 'react';
import { Icon, icons, Button, Input, Card, Tag, StatusPill, fmtRel, shortId } from '@/design/components';

const _cwUseState = useState;

const fwStatCard = ({ label, value, unit, tint }) => {
  const tints = {
    blue:  { bg: 'var(--cw-blue-200)',  fg: 'var(--cw-blue-950)' },
    green: { bg: 'var(--cw-green-100)',  fg: 'var(--status-success-fg)' },
    yellow:{ bg: 'var(--cw-yellow-100)', fg: 'var(--status-warning-fg)' },
    red:   { bg: 'var(--cw-red-100)',    fg: 'var(--status-danger-fg)' },
    gray:  { bg: 'var(--cw-gray-100)',   fg: 'var(--cw-gray-900)' },
  };
  const t = tints[tint];
  return (
    <Card style={{ padding: '14px 16px', flex: 1, display: 'flex', flexDirection: 'column', justifyContent: 'space-between', height: 84, boxSizing: 'border-box' }}>
      <span style={{ fontFamily: 'var(--font-ui)', fontSize: 10, fontWeight: 700, letterSpacing: '0.06em', textTransform: 'uppercase', padding: '2px 8px', borderRadius: 4, alignSelf: 'flex-start', background: t.bg, color: t.fg }}>{label}</span>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 8 }}>
        <span style={{ fontFamily: 'var(--font-display)', fontWeight: 700, fontSize: 30, color: 'var(--cw-gray-950)', lineHeight: 1 }}>{value}</span>
        <span style={{ fontFamily: 'var(--font-sans)', fontSize: 14, color: 'var(--cw-gray-900)' }}>{unit}</span>
      </div>
    </Card>
  );
};
const StatCard = fwStatCard;

const fwTh = { textAlign: 'left', padding: '10px 16px', fontFamily: 'var(--font-ui)', fontSize: 12, fontWeight: 600, color: 'var(--cw-gray-950)', borderBottom: '1px solid var(--cw-gray-400)', whiteSpace: 'nowrap', userSelect: 'none' };
const fwTd = { padding: '13px 16px', fontFamily: 'var(--font-sans)', fontSize: 13.5, color: 'var(--cw-gray-950)', borderBottom: '1px solid var(--cw-gray-200)', verticalAlign: 'middle' };

function RunsList({ runs = [], flatSteps = [], stepIndex = {}, onOpenRun, onNewRun, density = 'regular' }) {
  const [query, setQuery] = _cwUseState('');
  const [statusFilter, setStatusFilter] = _cwUseState('all');
  const rowTd = { ...fwTd, padding: density === 'compact' ? '7px 16px' : '13px 16px' };
  const stepTitle = (id) => (flatSteps.find((s) => s.id === id) || {}).title || '—';

  const counts = {
    running: runs.filter((r) => r.status === 'running').length,
    awaiting: runs.filter((r) => r.status === 'warning' || r.status === 'passed').length,
    signed: runs.filter((r) => r.status === 'signed_off').length,
    failed: runs.filter((r) => r.status === 'failed').length,
  };

  const filtered = runs.filter((r) => {
    if (statusFilter !== 'all' && r.status !== statusFilter) return false;
    if (!query) return true;
    const q = query.toLowerCase();
    return r.bundle.toLowerCase().includes(q) || r.racks.some((rk) => rk.includes(q)) || r.id.toLowerCase().includes(q) || r.requester.includes(q);
  });

  const filterChips = [
    ['all', 'All'], ['running', 'Running'], ['warning', 'Awaiting review'],
    ['signed_off', 'Signed off'], ['failed', 'Failed'],
  ];

  return (
    <div style={{ padding: '28px 32px 64px', background: 'var(--cw-bg-canvas, #FCFCFE)', flex: 1, overflow: 'auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 6 }}>
        <div>
          <h1 style={{ margin: 0, fontFamily: 'var(--font-display)', fontWeight: 700, fontSize: 28, color: 'var(--cw-gray-950)', letterSpacing: '-0.01em' }}>Canary Runs</h1>
          <p style={{ margin: '6px 0 0', fontFamily: 'var(--font-sans)', fontSize: 14, color: 'var(--cw-gray-900)', maxWidth: 640 }}>
            Firmware-release canary verification against the <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13 }}>fw-release-canary</span> template. Each run drives the rack through the L10/L11/L12 ladder and accumulates auditable per-rack evidence.
          </p>
        </div>
        <Button variant="primary" leftIcon={icons.plus} onClick={onNewRun}>New canary run</Button>
      </div>

      <div style={{ display: 'flex', gap: 16, margin: '24px 0' }}>
        <StatCard label="Running" value={counts.running} unit="in flight" tint="blue" />
        <StatCard label="Awaiting review" value={counts.awaiting} unit="need an operator" tint="yellow" />
        <StatCard label="Signed off" value={counts.signed} unit="this week" tint="green" />
        <StatCard label="Failed" value={counts.failed} unit="this week" tint="red" />
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16 }}>
        <div style={{ width: 320 }}>
          <Input icon={icons.search} placeholder="Search by bundle, rack, run id, or requester" value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>
        <div style={{ display: 'flex', gap: 6, marginLeft: 'auto' }}>
          {filterChips.map(([val, label]) => {
            const active = statusFilter === val;
            return (
              <button key={val} onClick={() => setStatusFilter(val)} style={{
                fontFamily: 'var(--font-ui)', fontSize: 12.5, fontWeight: 600, height: 30, padding: '0 12px',
                borderRadius: 4, cursor: 'pointer', border: '1px solid ' + (active ? 'var(--cw-blue)' : 'var(--cw-gray-400)'),
                background: active ? 'var(--cw-blue-200)' : 'var(--fw-surface)', color: active ? 'var(--cw-blue-800)' : 'var(--cw-gray-900)',
              }}>{label}</button>
            );
          })}
        </div>
      </div>

      <Card style={{ overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ background: 'var(--cw-gray-100)' }}>
              <th style={fwTh}>Run</th>
              <th style={fwTh}>Firmware bundle</th>
              <th style={fwTh}>Racks</th>
              <th style={fwTh}>Status</th>
              <th style={fwTh}>Current step</th>
              <th style={fwTh}>Requester</th>
              <th style={fwTh}>Updated</th>
              <th style={{ ...fwTh, width: 36 }}></th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((r) => (
              <tr key={r.id} onClick={() => onOpenRun(r.id)} style={{ cursor: 'pointer' }}
                onMouseEnter={(e) => e.currentTarget.style.background = 'var(--cw-gray-50)'}
                onMouseLeave={(e) => e.currentTarget.style.background = 'transparent'}>
                <td style={rowTd}>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13, color: 'var(--cw-blue-800)', fontWeight: 500 }}>{shortId(r.id)}</span>
                </td>
                <td style={rowTd}>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5 }}>{r.bundle}</span>
                </td>
                <td style={rowTd}>
                  <div style={{ display: 'flex', gap: 5, flexWrap: 'wrap', maxWidth: 230 }}>
                    {r.racks.map((rk) => <Tag key={rk} mono>{rk}</Tag>)}
                  </div>
                </td>
                <td style={rowTd}><StatusPill status={r.status} /></td>
                <td style={{ ...rowTd, color: 'var(--cw-gray-900)' }}>
                  {r.currentStep ? (
                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 7 }}>
                      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--cw-gray-700)' }}>
                        {stepIndex[r.currentStep] + 1}/{flatSteps.length}
                      </span>
                      {stepTitle(r.currentStep)}
                    </span>
                  ) : <span style={{ color: 'var(--cw-gray-700)' }}>Not started</span>}
                </td>
                <td style={rowTd}><span style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5 }}>{r.requester}</span></td>
                <td style={{ ...rowTd, color: 'var(--cw-gray-900)', whiteSpace: 'nowrap' }}>{fmtRel(r.updated_at)}</td>
                <td style={{ ...rowTd, color: 'var(--cw-gray-700)' }}><Icon path={icons.chevRight} size={16} /></td>
              </tr>
            ))}
          </tbody>
        </table>
        {filtered.length === 0 && (
          <div style={{ padding: '48px 16px', textAlign: 'center', fontFamily: 'var(--font-sans)', fontSize: 14, color: 'var(--cw-gray-700)' }}>
            No runs match this filter.
          </div>
        )}
      </Card>
    </div>
  );
}


export default RunsList;
