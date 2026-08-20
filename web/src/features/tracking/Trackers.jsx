// fwqual — Node Tracker & Rack Tracker (Debugging).
// My personal worklist of hardware pulled OUT of the canary QA flow for repair.
// Each row is a node or rack I'm actively fixing: the failure, where it sits in
// the repair lifecycle, the next action, and a linked ticket where one exists.
// Status facets on the left, dense worklist table on the right.
import { useState } from 'react';
import { Icon, icons, Input, Card, Eyebrow, fmtRel } from '@/design/components';
import { FIX_STATUSES, NODE_FIXES, RACK_FIXES } from '@/design/seed';

const _ftUseState = useState;

const FT_TINT = {
  gray:   { bg: 'var(--cw-gray-100)',   fg: 'var(--cw-gray-900)',       dot: 'var(--cw-gray-700)' },
  blue:   { bg: 'var(--cw-blue-200)',   fg: 'var(--cw-blue-950)',       dot: 'var(--cw-blue)' },
  yellow: { bg: 'var(--cw-yellow-100)', fg: 'var(--status-warning-fg)', dot: 'var(--cw-yellow)' },
  red:    { bg: 'var(--cw-red-100)',    fg: 'var(--status-danger-fg)',  dot: 'var(--cw-red)' },
  green:  { bg: 'var(--cw-green-100)',  fg: 'var(--status-success-fg)', dot: 'var(--cw-green)' },
};

const FT_PRIORITY = {
  high:   { icon: icons.arrowUp,   color: 'var(--status-danger-fg)', label: 'High' },
  medium: { icon: icons.equal,     color: 'var(--cw-yellow)',        label: 'Medium' },
  low:    { icon: icons.arrowDown, color: 'var(--cw-gray-700)',      label: 'Low' },
};

const FtStatusPill = ({ status }) => {
  const def = FIX_STATUSES.find((s) => s.id === status) || FIX_STATUSES[0];
  const t = FT_TINT[def.tint];
  return (
    <span title={def.desc} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontFamily: 'var(--font-ui)', fontWeight: 600, fontSize: 11.5, padding: '2px 9px', borderRadius: 4, background: t.bg, color: t.fg, whiteSpace: 'nowrap' }}>
      <span style={{ width: 6, height: 6, borderRadius: '50%', background: t.dot }} />{def.label}
    </span>
  );
};

const FtFacet = ({ active, label, count, dot, onClick }) => (
  <div onClick={onClick} style={{
    display: 'flex', alignItems: 'center', gap: 9, padding: '6px 10px', borderRadius: 4, cursor: 'pointer',
    background: active ? 'var(--cw-blue-200)' : 'transparent',
    border: '1px solid ' + (active ? 'var(--cw-blue)' : 'transparent'),
  }}
    onMouseEnter={(e) => { if (!active) e.currentTarget.style.background = 'var(--cw-gray-50)'; }}
    onMouseLeave={(e) => { if (!active) e.currentTarget.style.background = 'transparent'; }}>
    {dot ? <span style={{ width: 7, height: 7, borderRadius: '50%', background: dot, flexShrink: 0 }} /> : <span style={{ width: 7 }} />}
    <span style={{ flex: 1, fontFamily: 'var(--font-ui)', fontSize: 13, fontWeight: active ? 600 : 500, color: active ? 'var(--cw-blue-800)' : 'var(--cw-gray-950)' }}>{label}</span>
    <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11.5, color: active ? 'var(--cw-blue-800)' : 'var(--cw-gray-700)' }}>{count}</span>
  </div>
);

function FixTracker({ scope, data, title, blurb, idLabel }) {
  const [statusFilter, setStatusFilter] = _ftUseState('open'); // open = not resolved
  const [query, setQuery] = _ftUseState('');

  const counts = {};
  data.forEach((r) => { counts[r.status] = (counts[r.status] || 0) + 1; });
  const openCount = data.filter((r) => r.status !== 'resolved').length;

  const q = query.trim().toLowerCase();
  const rows = data.filter((r) => {
    if (statusFilter === 'open' && r.status === 'resolved') return false;
    if (statusFilter !== 'open' && statusFilter !== 'all' && r.status !== statusFilter) return false;
    if (q && !(`${r.id} ${r.issue} ${r.region} ${r.instance} ${r.jira || ''} ${r.next}`.toLowerCase().includes(q))) return false;
    return true;
  });

  const th = { textAlign: 'left', fontFamily: 'var(--font-ui)', fontWeight: 600, fontSize: 11.5, letterSpacing: '0.03em', textTransform: 'uppercase', color: 'var(--cw-gray-700)', padding: '0 16px', height: 38, whiteSpace: 'nowrap', borderBottom: '1px solid var(--cw-gray-400)', background: 'var(--cw-gray-100)' };
  const td = { fontFamily: 'var(--font-sans)', fontSize: 13.5, color: 'var(--cw-gray-950)', padding: '12px 16px', borderBottom: '1px solid var(--cw-gray-200)', verticalAlign: 'top' };

  return (
    <div style={{ display: 'flex', flex: 1, minHeight: 0, background: 'var(--cw-bg-canvas, #FCFCFE)' }}>
      {/* status facet rail */}
      <aside style={{ width: 230, flexShrink: 0, borderRight: '1px solid var(--cw-gray-400)', padding: '24px 12px', overflow: 'auto' }}>
        <Eyebrow style={{ padding: '0 10px 8px' }}>Repair Status</Eyebrow>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
          <FtFacet label="Open" count={openCount} active={statusFilter === 'open'} onClick={() => setStatusFilter('open')} />
          <FtFacet label="All" count={data.length} active={statusFilter === 'all'} onClick={() => setStatusFilter('all')} />
          <div style={{ height: 1, background: 'var(--cw-gray-200)', margin: '8px 10px' }} />
          {FIX_STATUSES.filter((s) => counts[s.id]).map((s) => (
            <FtFacet key={s.id} label={s.label} dot={FT_TINT[s.tint].dot} count={counts[s.id]} active={statusFilter === s.id} onClick={() => setStatusFilter(statusFilter === s.id ? 'all' : s.id)} />
          ))}
        </div>
      </aside>

      {/* worklist */}
      <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        <div style={{ padding: '28px 32px 0' }}>
          <h1 style={{ margin: 0, fontFamily: 'var(--font-display)', fontWeight: 700, fontSize: 28, color: 'var(--cw-gray-950)', letterSpacing: '-0.01em' }}>{title}</h1>
          <p style={{ margin: '6px 0 0', fontFamily: 'var(--font-sans)', fontSize: 14, color: 'var(--cw-gray-900)', maxWidth: 720 }}>{blurb}</p>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12, margin: '20px 0 14px' }}>
            <div style={{ flex: 1, maxWidth: 420 }}>
              <Input icon={icons.search} placeholder={`Search ${scope}s, issues, regions…`} value={query} onChange={(e) => setQuery(e.target.value)} />
            </div>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5, color: 'var(--cw-gray-700)' }}>
              {rows.length} {scope}{rows.length === 1 ? '' : 's'}
            </span>
          </div>
        </div>

        <div style={{ flex: 1, overflow: 'auto', padding: '0 32px 48px' }}>
          <Card style={{ overflow: 'hidden' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', tableLayout: 'fixed' }}>
              <colgroup>
                <col style={{ width: 220 }} />
                <col />
                <col style={{ width: 128 }} />
                <col style={{ width: 104 }} />
                <col style={{ width: 96 }} />
              </colgroup>
              <thead>
                <tr>
                  <th style={th}>{idLabel}</th>
                  <th style={th}>Issue &amp; next action</th>
                  <th style={th}>Status</th>
                  <th style={th}>Priority</th>
                  <th style={th}>Updated</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => {
                  const pr = FT_PRIORITY[r.priority] || FT_PRIORITY.medium;
                  return (
                    <tr key={r.id}>
                      <td style={{ ...td, overflow: 'hidden' }}>
                        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 600, color: 'var(--cw-gray-950)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{r.id}</div>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 7, marginTop: 5, flexWrap: 'wrap' }}>
                          <span style={{ fontFamily: 'var(--font-ui)', fontSize: 10.5, fontWeight: 600, color: 'var(--cw-gray-700)' }}>{r.region}</span>
                          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10.5, color: 'var(--cw-gray-700)', background: 'var(--cw-gray-100)', borderRadius: 3, padding: '1px 6px' }}>{r.instance}</span>
                          {scope === 'rack' && <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10.5, color: 'var(--cw-gray-700)' }}>{r.nodes} nodes</span>}
                          {scope === 'node' && <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10.5, color: 'var(--cw-gray-700)' }}>{r.rack.replace(/-[a-z]+-[a-z]+-\d+\w$/, '')}</span>}
                        </div>
                      </td>
                      <td style={{ ...td, whiteSpace: 'normal' }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                          <span style={{ fontFamily: 'var(--font-sans)', fontSize: 13.5, fontWeight: 600, color: 'var(--cw-gray-950)' }}>{r.issue}</span>
                          {r.xid != null && <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10.5, fontWeight: 600, color: 'var(--status-danger-fg)', background: 'var(--cw-red-100)', borderRadius: 3, padding: '1px 6px' }}>XID {r.xid}</span>}
                        </div>
                        <div style={{ display: 'flex', alignItems: 'flex-start', gap: 6, marginTop: 5, fontFamily: 'var(--font-sans)', fontSize: 12.5, color: 'var(--cw-gray-900)', lineHeight: 1.45 }}>
                          <span style={{ fontFamily: 'var(--font-ui)', fontSize: 10.5, fontWeight: 700, letterSpacing: '0.04em', color: 'var(--cw-gray-700)', flexShrink: 0, paddingTop: 1 }}>NEXT</span>
                          <span>{r.next}</span>
                        </div>
                        {r.jira && (
                          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, marginTop: 7, fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--cw-blue-800)' }}>
                            <Icon path={icons.link2} size={12} />{r.jira}
                          </span>
                        )}
                      </td>
                      <td style={td}><FtStatusPill status={r.status} /></td>
                      <td style={td}>
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, color: pr.color, fontFamily: 'var(--font-ui)', fontSize: 12.5, fontWeight: 600 }}>
                          <Icon path={pr.icon} size={15} strokeWidth={2.4} />{pr.label}
                        </span>
                      </td>
                      <td style={{ ...td, fontFamily: 'var(--font-ui)', fontSize: 12, color: 'var(--cw-gray-700)' }}>{fmtRel(r.updated_at)}</td>
                    </tr>
                  );
                })}
                {rows.length === 0 && (
                  <tr>
                    <td colSpan={5} style={{ ...td, height: 120, textAlign: 'center', color: 'var(--cw-gray-700)', fontFamily: 'var(--font-ui)', fontSize: 13 }}>
                      No {scope}s match the current filters.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </Card>
        </div>
      </div>
    </div>
  );
}

export const NodeTracker = () => (
  <FixTracker
    scope="node"
    data={NODE_FIXES}
    title="Node Tracker"
    idLabel="Node"
    blurb="GPU nodes I've pulled out of the fleet for repair, outside the canary QA flow. Each is a node I'm working to fix — tracked through its repair lifecycle to RMA or return."
  />
);

export const RackTracker = () => (
  <FixTracker
    scope="rack"
    data={RACK_FIXES}
    title="Rack Tracker"
    idLabel="Rack"
    blurb="Whole racks I'm working to fix outside the canary QA flow — fabric, power, cooling, and L11/L12 failures. Tracked through repair to verification and return to the fleet."
  />
);

