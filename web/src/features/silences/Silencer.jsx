// fwqual — PagerDuty Alert Silencer.
// Puts canary racks into a PagerDuty maintenance window for the duration of
// disruptive QA work (zap, L10/L11/L12) so on-call isn't paged for expected noise.
import { useState } from 'react';
import { Icon, icons, Button, Input, Card, Tag, fmtRel, shortId } from '@/design/components';
import { RUNS, PAGERDUTY } from '@/design/seed';

const _slUseState = useState;

const SL_NOW = new Date('2026-05-30T14:25:00Z').getTime();

// derive a live status + a human window phrase from the stored start/end stamps
const silenceStatus = (s) => {
  const start = new Date(s.starts_at).getTime();
  const end = new Date(s.ends_at).getTime();
  if (SL_NOW < start) return 'scheduled';
  if (SL_NOW >= end) return 'ended';
  if (end - SL_NOW <= 30 * 60 * 1000) return 'expiring';
  return 'active';
};

const fmtSpan = (ms) => {
  const m = Math.round(Math.abs(ms) / 60000);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60), rem = m % 60;
  return rem ? `${h}h ${rem}m` : `${h}h`;
};

const windowPhrase = (s, status) => {
  const start = new Date(s.starts_at).getTime();
  const end = new Date(s.ends_at).getTime();
  if (status === 'scheduled') return { lead: 'starts in', val: fmtSpan(start - SL_NOW) };
  if (status === 'ended') return { lead: 'ended', val: fmtSpan(SL_NOW - end) + ' ago' };
  return { lead: 'ends in', val: fmtSpan(end - SL_NOW) };
};

const SL_STATUS = {
  active:    { bg: 'var(--cw-green-100)',  fg: 'var(--status-success-fg)', dot: 'var(--cw-green)',  label: 'Active' },
  expiring:  { bg: 'var(--cw-yellow-100)', fg: 'var(--status-warning-fg)', dot: 'var(--cw-yellow)', label: 'Expiring' },
  scheduled: { bg: 'var(--cw-blue-200)',   fg: 'var(--cw-blue-950)',       dot: 'var(--cw-blue)',   label: 'Scheduled' },
  ended:     { bg: 'var(--cw-gray-100)',   fg: 'var(--cw-gray-900)',       dot: 'var(--cw-gray-700)', label: 'Ended' },
};
const SilencePill = ({ status }) => {
  const s = SL_STATUS[status];
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontFamily: 'var(--font-ui)', fontWeight: 600, fontSize: 11.5, padding: '2px 9px', borderRadius: 4, height: 20, background: s.bg, color: s.fg, whiteSpace: 'nowrap' }}>
      <span style={{ width: 6, height: 6, borderRadius: '50%', background: s.dot }} />{s.label}
    </span>
  );
};

const slTh = { textAlign: 'left', padding: '10px 16px', fontFamily: 'var(--font-ui)', fontSize: 12, fontWeight: 600, color: 'var(--cw-gray-950)', borderBottom: '1px solid var(--cw-gray-400)', whiteSpace: 'nowrap', userSelect: 'none' };
const slTd = { padding: '12px 16px', fontFamily: 'var(--font-sans)', fontSize: 13.5, color: 'var(--cw-gray-950)', borderBottom: '1px solid var(--cw-gray-200)', verticalAlign: 'middle' };

const slField = { display: 'flex', flexDirection: 'column', gap: 6 };
const slLabel = { fontFamily: 'var(--font-ui)', fontSize: 11.5, fontWeight: 600, color: 'var(--cw-gray-900)', letterSpacing: '0.01em' };
const slSelect = { fontFamily: 'var(--font-sans)', fontSize: 14, height: 36, padding: '0 10px', border: '1px solid var(--cw-gray-400)', borderRadius: 'var(--input-radius)', background: 'var(--fw-surface)', color: 'var(--cw-gray-950)', outline: 'none', width: '100%', boxSizing: 'border-box' };

// link-style action button used inside table rows
const RowAction = ({ children, onClick, danger }) => (
  <button onClick={onClick} style={{
    fontFamily: 'var(--font-ui)', fontSize: 12.5, fontWeight: 600, background: 'transparent',
    border: 0, cursor: 'pointer', padding: '4px 6px', borderRadius: 4,
    color: danger ? 'var(--cw-red)' : 'var(--cw-blue-800)',
  }}
    onMouseEnter={(e) => e.currentTarget.style.background = 'var(--cw-gray-50)'}
    onMouseLeave={(e) => e.currentTarget.style.background = 'transparent'}>{children}</button>
);

function Silencer() {
  const [silences, setSilences] = _slUseState(() => PAGERDUTY.silences.map((s) => ({ ...s })));
  const [showForm, setShowForm] = _slUseState(false);
  const [form, setForm] = _slUseState({ rack: '', run_id: '', reason: '', duration: '4' });

  const pd = PAGERDUTY.service;
  const withStatus = silences.map((s) => ({ ...s, _status: silenceStatus(s) }));
  const live = withStatus.filter((s) => s._status === 'active' || s._status === 'expiring');
  const racksCovered = new Set(withStatus.filter((s) => s._status !== 'ended').map((s) => s.rack)).size;
  const expiringCount = withStatus.filter((s) => s._status === 'expiring').length;
  const suppressed24h = withStatus.reduce((n, s) => n + (s.suppressed || 0), 0);

  // sort: live work first, then scheduled, then ended — each newest-first within group
  const order = { expiring: 0, active: 1, scheduled: 2, ended: 3 };
  const rows = [...withStatus].sort((a, b) =>
    (order[a._status] - order[b._status]) || (new Date(b.starts_at) - new Date(a.starts_at)));

  const endNow = (id) => setSilences((xs) => xs.map((s) => s.id === id ? { ...s, ends_at: new Date(SL_NOW).toISOString() } : s));
  const extend = (id) => setSilences((xs) => xs.map((s) => s.id === id ? { ...s, ends_at: new Date(new Date(s.ends_at).getTime() + 2 * 3600 * 1000).toISOString() } : s));

  const createSilence = () => {
    if (!form.rack.trim()) return;
    const dur = form.duration === 'open' ? 24 : Number(form.duration);
    const racks = form.rack.split(',').map((r) => r.trim()).filter(Boolean);
    const fresh = racks.map((rack, i) => ({
      id: 'mw_new' + Date.now() + i,
      rack,
      run_id: form.run_id || null,
      reason: form.reason.trim() || 'Manual silence',
      created_by: 'wpena', auto: false,
      starts_at: new Date(SL_NOW).toISOString(),
      ends_at: new Date(SL_NOW + dur * 3600 * 1000).toISOString(),
      suppressed: 0,
    }));
    setSilences((xs) => [...fresh, ...xs]);
    setForm({ rack: '', run_id: '', reason: '', duration: '4' });
    setShowForm(false);
  };

  return (
    <div style={{ padding: '28px 32px 64px', background: 'var(--cw-bg-canvas, #FCFCFE)', flex: 1, overflow: 'auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 6 }}>
        <div>
          <h1 style={{ margin: 0, fontFamily: 'var(--font-display)', fontWeight: 700, fontSize: 28, color: 'var(--cw-gray-950)', letterSpacing: '-0.01em' }}>Silences</h1>
          <p style={{ margin: '6px 0 0', fontFamily: 'var(--font-sans)', fontSize: 14, color: 'var(--cw-gray-900)', maxWidth: 660 }}>
            Suppress PagerDuty pages on racks under qualification. Canary runs open a silence automatically for the zap and L10/L11/L12 window so on-call isn't paged for expected reboot, NVLink-flap, and thermal noise.
          </p>
        </div>
        <Button variant="primary" leftIcon={icons.plus} onClick={() => setShowForm((v) => !v)}>New silence</Button>
      </div>

      {/* ---- PagerDuty connection ---- */}
      <Card style={{ display: 'flex', alignItems: 'center', gap: 16, padding: '14px 18px', margin: '22px 0 0' }}>
        <div style={{ width: 40, height: 40, borderRadius: 6, background: 'var(--cw-green-100)', color: 'var(--cw-green)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
          <Icon path={icons.bellOff} size={20} />
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <span style={{ fontFamily: 'var(--font-sans)', fontSize: 15, fontWeight: 600, color: 'var(--cw-gray-950)' }}>PagerDuty · {pd.name}</span>
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontFamily: 'var(--font-ui)', fontSize: 11.5, fontWeight: 600, padding: '2px 9px', borderRadius: 4, background: 'var(--cw-green-100)', color: 'var(--status-success-fg)' }}>
              <span style={{ width: 6, height: 6, borderRadius: '50%', background: 'var(--cw-green)' }} />Connected
            </span>
          </div>
          <div style={{ display: 'flex', gap: 16, fontFamily: 'var(--font-mono)', fontSize: 11.5, color: 'var(--cw-gray-700)', flexWrap: 'wrap' }}>
            <span>{pd.subdomain}</span>
            <span>{pd.integration}</span>
            <span>esc · {pd.escalation_policy}</span>
            <span>synced {fmtRel(pd.last_sync)}</span>
          </div>
        </div>
        <div style={{ marginLeft: 'auto' }}>
          <Button variant="secondary" size="sm" rightIcon={icons.externalLink}>Open in PagerDuty</Button>
        </div>
      </Card>

      {/* ---- stats ---- */}
      <div style={{ display: 'flex', gap: 16, margin: '16px 0 0' }}>
        <StatCard label="Active silences" value={live.length} unit="in effect" tint="green" />
        <StatCard label="Racks covered" value={racksCovered} unit="racks" tint="blue" />
        <StatCard label="Expiring soon" value={expiringCount} unit="< 30 min left" tint="yellow" />
        <StatCard label="Pages suppressed" value={suppressed24h} unit="last 24h" tint="gray" />
      </div>

      {/* ---- new silence form ---- */}
      {showForm && (
        <Card style={{ padding: '18px 20px', margin: '16px 0 0' }}>
          <div style={{ fontFamily: 'var(--font-display)', fontWeight: 700, fontSize: 16, color: 'var(--cw-gray-950)', marginBottom: 14 }}>New silence</div>
          <div style={{ display: 'grid', gridTemplateColumns: '1.4fr 1fr', gap: 14 }}>
            <div style={slField}>
              <label style={slLabel}>Rack(s)</label>
              <Input placeholder="rack id — comma-separated for multiple" value={form.rack} onChange={(e) => setForm({ ...form, rack: e.target.value })} />
            </div>
            <div style={slField}>
              <label style={slLabel}>Associated run</label>
              <select style={slSelect} value={form.run_id} onChange={(e) => setForm({ ...form, run_id: e.target.value })}>
                <option value="">None — manual silence</option>
                {RUNS.map((r) => <option key={r.id} value={r.id}>{shortId(r.id)} · {r.bundle}</option>)}
              </select>
            </div>
            <div style={slField}>
              <label style={slLabel}>Reason</label>
              <Input placeholder="What is this silence for?" value={form.reason} onChange={(e) => setForm({ ...form, reason: e.target.value })} />
            </div>
            <div style={slField}>
              <label style={slLabel}>Duration</label>
              <select style={slSelect} value={form.duration} onChange={(e) => setForm({ ...form, duration: e.target.value })}>
                <option value="1">1 hour</option>
                <option value="2">2 hours</option>
                <option value="4">4 hours</option>
                <option value="8">8 hours</option>
                <option value="open">Until I end it (24h cap)</option>
              </select>
            </div>
          </div>
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10, marginTop: 16 }}>
            <Button variant="ghost" onClick={() => setShowForm(false)}>Cancel</Button>
            <Button variant="primary" leftIcon={icons.bellOff} onClick={createSilence}>Silence rack</Button>
          </div>
        </Card>
      )}

      {/* ---- silences table ---- */}
      <Card style={{ overflow: 'hidden', margin: '20px 0 0' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ background: 'var(--cw-gray-100)' }}>
              <th style={slTh}>Rack</th>
              <th style={slTh}>Reason</th>
              <th style={slTh}>Window</th>
              <th style={slTh}>Created by</th>
              <th style={{ ...slTh, textAlign: 'right' }}>Pages</th>
              <th style={slTh}>Status</th>
              <th style={{ ...slTh, width: 150 }}></th>
            </tr>
          </thead>
          <tbody>
            {rows.map((s) => {
              const ph = windowPhrase(s, s._status);
              const dim = s._status === 'ended';
              return (
                <tr key={s.id} style={{ opacity: dim ? 0.6 : 1 }}>
                  <td style={slTd}>
                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 7 }}>
                      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5, color: 'var(--cw-gray-950)' }}>{s.rack}</span>
                      {s.auto && <Tag style={{ fontSize: 10.5, padding: '1px 6px', background: 'var(--cw-blue-200)', color: 'var(--cw-blue-950)' }}>auto</Tag>}
                    </span>
                  </td>
                  <td style={{ ...slTd, maxWidth: 280 }}>
                    <div style={{ color: 'var(--cw-gray-950)' }}>{s.reason}</div>
                    {s.run_id && <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--cw-blue-800)', marginTop: 2 }}>{shortId(s.run_id)}</div>}
                  </td>
                  <td style={{ ...slTd, whiteSpace: 'nowrap' }}>
                    <span style={{ color: 'var(--cw-gray-700)', fontSize: 12.5 }}>{ph.lead} </span>
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5, color: 'var(--cw-gray-950)' }}>{ph.val}</span>
                  </td>
                  <td style={{ ...slTd }}><span style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5 }}>{s.created_by}</span></td>
                  <td style={{ ...slTd, textAlign: 'right', fontFamily: 'var(--font-mono)', fontSize: 12.5, color: s.suppressed ? 'var(--cw-gray-950)' : 'var(--cw-gray-700)' }}>{s.suppressed}</td>
                  <td style={slTd}><SilencePill status={s._status} /></td>
                  <td style={{ ...slTd, textAlign: 'right', whiteSpace: 'nowrap' }}>
                    {(s._status === 'active' || s._status === 'expiring') && (
                      <span style={{ display: 'inline-flex', gap: 2 }}>
                        <RowAction onClick={() => extend(s.id)}>Extend +2h</RowAction>
                        <RowAction onClick={() => endNow(s.id)} danger>End now</RowAction>
                      </span>
                    )}
                    {s._status === 'scheduled' && <RowAction onClick={() => endNow(s.id)} danger>Cancel</RowAction>}
                    {s._status === 'ended' && <span style={{ fontFamily: 'var(--font-ui)', fontSize: 12, color: 'var(--cw-gray-700)' }}>—</span>}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </Card>
    </div>
  );
}


export default Silencer;
