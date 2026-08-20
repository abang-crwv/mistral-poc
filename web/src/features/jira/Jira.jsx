// fwqual — Jira (Debugging).
// FWQA project board: bugs, incidents, and tasks filed from canary failures,
// regressions, and tooling work. Many issues link back to the canary run that
// surfaced them — click through to the run. Mirrors a Jira issue list.
import { useState } from 'react';
import { Icon, icons, Input, Card, Eyebrow, fmtRel, shortId } from '@/design/components';
import { JIRA } from '@/design/seed';

const _jrUseState = useState;

const JR_STATUS_TINT = {
  gray:   { bg: 'var(--cw-gray-100)',   fg: 'var(--cw-gray-900)',       dot: 'var(--cw-gray-700)' },
  blue:   { bg: 'var(--cw-blue-200)',   fg: 'var(--cw-blue-950)',       dot: 'var(--cw-blue)' },
  yellow: { bg: 'var(--cw-yellow-100)', fg: 'var(--status-warning-fg)', dot: 'var(--cw-yellow)' },
  green:  { bg: 'var(--cw-green-100)',  fg: 'var(--status-success-fg)', dot: 'var(--cw-green)' },
};

// issue type → icon + color (Jira-style square glyphs)
const JR_TYPE = {
  bug:      { icon: icons.bug,   color: 'var(--cw-red)',    label: 'Bug' },
  incident: { icon: icons.alert, color: 'var(--cw-red)',    label: 'Incident' },
  task:     { icon: icons.check, color: 'var(--cw-blue)',   label: 'Task' },
};
// priority → arrow + color
const JR_PRIORITY = {
  highest: { icon: icons.arrowUp,   color: 'var(--cw-red)',    label: 'Highest' },
  high:    { icon: icons.arrowUp,   color: 'var(--status-danger-fg)', label: 'High' },
  medium:  { icon: icons.equal,     color: 'var(--cw-yellow)', label: 'Medium' },
  low:     { icon: icons.arrowDown, color: 'var(--cw-gray-700)', label: 'Low' },
};

const JrStatusPill = ({ status }) => {
  const def = JIRA.statuses.find((s) => s.id === status) || JIRA.statuses[0];
  const t = JR_STATUS_TINT[def.tint];
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontFamily: 'var(--font-ui)', fontWeight: 600, fontSize: 11.5, padding: '2px 9px', borderRadius: 4, background: t.bg, color: t.fg, whiteSpace: 'nowrap' }}>
      <span style={{ width: 6, height: 6, borderRadius: '50%', background: t.dot }} />{def.label}
    </span>
  );
};

const JrTypeGlyph = ({ type }) => {
  const def = JR_TYPE[type] || JR_TYPE.task;
  return (
    <span title={def.label} style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: 22, height: 22, borderRadius: 4, background: 'var(--cw-gray-100)', color: def.color, flexShrink: 0 }}>
      <Icon path={def.icon} size={14} />
    </span>
  );
};

const JrAvatar = ({ name }) => {
  if (!name) return <span style={{ fontFamily: 'var(--font-ui)', fontSize: 12, color: 'var(--cw-gray-700)' }}>Unassigned</span>;
  const initials = name.split(/[.\s]/).map((p) => p[0]).join('').slice(0, 2).toUpperCase();
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
      <span style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: 24, height: 24, borderRadius: '50%', background: 'var(--cw-blue-300)', color: 'var(--cw-blue-950)', fontFamily: 'var(--font-ui)', fontSize: 10.5, fontWeight: 700, flexShrink: 0 }}>{initials}</span>
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5, color: 'var(--cw-gray-900)' }}>{name}</span>
    </span>
  );
};

function Jira({ onOpenRun }) {
  const [statusFilter, setStatusFilter] = _jrUseState('all');
  const [typeFilter, setTypeFilter] = _jrUseState('all');
  const [query, setQuery] = _jrUseState('');

  const q = query.trim().toLowerCase();
  const issues = JIRA.issues.filter((it) => {
    if (statusFilter !== 'all' && it.status !== statusFilter) return false;
    if (typeFilter !== 'all' && it.type !== typeFilter) return false;
    if (q && !(`${it.key} ${it.summary} ${it.assignee || ''} ${it.labels.join(' ')}`.toLowerCase().includes(q))) return false;
    return true;
  });

  // open = anything not done; quick counts for the summary chips
  const openCount = JIRA.issues.filter((i) => i.status !== 'done').length;
  const incidentCount = JIRA.issues.filter((i) => i.type === 'incident' && i.status !== 'done').length;

  const th = { textAlign: 'left', fontFamily: 'var(--font-ui)', fontWeight: 600, fontSize: 11.5, letterSpacing: '0.03em', textTransform: 'uppercase', color: 'var(--cw-gray-700)', padding: '0 16px', height: 38, whiteSpace: 'nowrap', borderBottom: '1px solid var(--cw-gray-400)', background: 'var(--cw-gray-100)' };
  const td = { fontFamily: 'var(--font-sans)', fontSize: 13.5, color: 'var(--cw-gray-950)', padding: '0 16px', height: 52, borderBottom: '1px solid var(--cw-gray-200)', verticalAlign: 'middle' };

  const chip = (active) => ({
    fontFamily: 'var(--font-ui)', fontSize: 12.5, fontWeight: 600, height: 30, padding: '0 12px',
    borderRadius: 4, cursor: 'pointer', border: '1px solid ' + (active ? 'var(--cw-blue)' : 'var(--cw-gray-400)'),
    background: active ? 'var(--cw-blue-200)' : 'var(--fw-surface)', color: active ? 'var(--cw-blue-800)' : 'var(--cw-gray-900)',
  });

  return (
    <div style={{ padding: '28px 32px 64px', background: 'var(--cw-bg-canvas, #FCFCFE)', flex: 1, overflow: 'auto' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16, flexWrap: 'wrap' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 9, marginBottom: 6 }}>
            <Eyebrow>{JIRA.project.board}</Eyebrow>
          </div>
          <h1 style={{ margin: 0, fontFamily: 'var(--font-display)', fontWeight: 700, fontSize: 28, color: 'var(--cw-gray-950)', letterSpacing: '-0.01em' }}>Jira</h1>
          <p style={{ margin: '6px 0 0', fontFamily: 'var(--font-sans)', fontSize: 14, color: 'var(--cw-gray-900)', maxWidth: 680 }}>
            Bugs, incidents, and tasks in the <span style={{ fontFamily: 'var(--font-mono)' }}>{JIRA.project.key}</span> project. Issues filed from canary failures link the run that surfaced them.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 10 }}>
          <Card style={{ padding: '10px 16px', minWidth: 88 }}>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 24, fontWeight: 600, color: 'var(--cw-gray-950)' }}>{openCount}</div>
            <div style={{ fontFamily: 'var(--font-ui)', fontSize: 11.5, color: 'var(--cw-gray-700)', marginTop: 2 }}>Open</div>
          </Card>
          <Card style={{ padding: '10px 16px', minWidth: 88 }}>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 24, fontWeight: 600, color: incidentCount ? 'var(--status-danger-fg)' : 'var(--cw-gray-950)' }}>{incidentCount}</div>
            <div style={{ fontFamily: 'var(--font-ui)', fontSize: 11.5, color: 'var(--cw-gray-700)', marginTop: 2 }}>Incidents</div>
          </Card>
        </div>
      </div>

      {/* filter bar */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, margin: '20px 0 18px', flexWrap: 'wrap' }}>
        <div style={{ width: 300 }}>
          <Input icon={icons.search} placeholder="Search issues, labels, assignee…" value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>
        <div style={{ width: 1, height: 24, background: 'var(--cw-gray-400)', margin: '0 4px' }} />
        {['all', 'bug', 'incident', 'task'].map((tp) => (
          <button key={tp} onClick={() => setTypeFilter(tp)} style={chip(typeFilter === tp)}>
            {tp === 'all' ? 'All types' : JR_TYPE[tp].label}
          </button>
        ))}
        <div style={{ width: 1, height: 24, background: 'var(--cw-gray-400)', margin: '0 4px' }} />
        <button onClick={() => setStatusFilter('all')} style={chip(statusFilter === 'all')}>All statuses</button>
        {JIRA.statuses.map((s) => (
          <button key={s.id} onClick={() => setStatusFilter(s.id)} style={chip(statusFilter === s.id)}>{s.label}</button>
        ))}
      </div>

      <Card style={{ overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', tableLayout: 'fixed' }}>
          <colgroup>
            <col style={{ width: 112 }} />
            <col />
            <col style={{ width: 116 }} />
            <col style={{ width: 130 }} />
            <col style={{ width: 168 }} />
            <col style={{ width: 96 }} />
          </colgroup>
          <thead>
            <tr>
              <th style={th}>Key</th>
              <th style={th}>Summary</th>
              <th style={th}>Priority</th>
              <th style={th}>Status</th>
              <th style={th}>Assignee</th>
              <th style={th}>Updated</th>
            </tr>
          </thead>
          <tbody>
            {issues.map((it) => {
              const pr = JR_PRIORITY[it.priority] || JR_PRIORITY.medium;
              return (
                <tr key={it.key}
                  onClick={it.run_id ? () => onOpenRun(it.run_id) : undefined}
                  style={{ cursor: it.run_id ? 'pointer' : 'default' }}
                  onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--cw-gray-50)'; }}
                  onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent'; }}>
                  <td style={{ ...td, fontFamily: 'var(--font-mono)', fontSize: 12.5, color: 'var(--cw-blue-800)', fontWeight: 500 }}>{it.key}</td>
                  <td style={{ ...td, overflow: 'hidden' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                      <JrTypeGlyph type={it.type} />
                      <div style={{ minWidth: 0 }}>
                        <div style={{ color: 'var(--cw-gray-950)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{it.summary}</div>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 4, flexWrap: 'wrap' }}>
                          {it.labels.map((l) => (
                            <span key={l} style={{ fontFamily: 'var(--font-mono)', fontSize: 10.5, color: 'var(--cw-gray-700)', background: 'var(--cw-gray-100)', borderRadius: 3, padding: '1px 6px' }}>{l}</span>
                          ))}
                          {it.run_id && (
                            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, fontFamily: 'var(--font-mono)', fontSize: 10.5, color: 'var(--cw-blue-800)' }}>
                              <Icon path={icons.link2} size={12} />{shortId(it.run_id)}
                            </span>
                          )}
                          {it.comments > 0 && (
                            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, fontFamily: 'var(--font-ui)', fontSize: 11, color: 'var(--cw-gray-700)' }}>
                              <Icon path={icons.message} size={12} />{it.comments}
                            </span>
                          )}
                        </div>
                      </div>
                    </div>
                  </td>
                  <td style={td}>
                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, color: pr.color, fontFamily: 'var(--font-ui)', fontSize: 12.5, fontWeight: 600 }}>
                      <Icon path={pr.icon} size={15} strokeWidth={2.4} />{pr.label}
                    </span>
                  </td>
                  <td style={td}><JrStatusPill status={it.status} /></td>
                  <td style={td}><JrAvatar name={it.assignee} /></td>
                  <td style={{ ...td, fontFamily: 'var(--font-ui)', fontSize: 12, color: 'var(--cw-gray-700)' }}>{fmtRel(it.updated_at)}</td>
                </tr>
              );
            })}
            {issues.length === 0 && (
              <tr>
                <td colSpan={6} style={{ ...td, height: 120, textAlign: 'center', color: 'var(--cw-gray-700)', fontFamily: 'var(--font-ui)', fontSize: 13 }}>
                  No issues match the current filters.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </Card>
    </div>
  );
}


export default Jira;
