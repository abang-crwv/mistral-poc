// fwqual — app shell: global header + sidebar nav.
import { Icon, icons, Eyebrow, LogoMark } from '@/design/components';
import { PAGERDUTY } from '@/design/seed';

export const fwHeaderIconBtn = { background: 'transparent', border: 0, width: 36, height: 36, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', color: 'var(--cw-gray-700)', cursor: 'pointer', borderRadius: 4 };

export const Header = ({ health, onLock }) => (
  <header style={{ height: 60, background: 'var(--cw-bg-header, #FCFCFE)', borderBottom: '1px solid var(--cw-gray-400)', display: 'flex', alignItems: 'center', padding: '0 20px', gap: 16, flexShrink: 0 }}>
    <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
      <LogoMark size={26} />
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 8 }}>
        <span style={{ fontFamily: 'var(--font-display)', fontWeight: 800, fontSize: 17, color: 'var(--cw-gray-950)', letterSpacing: '-0.01em' }}>QAgenticCow</span>
        <span style={{ fontFamily: 'var(--font-ui)', fontSize: 11.5, color: 'var(--cw-gray-700)', fontWeight: 500 }}>Fleet / FRO</span>
      </div>
    </div>
    <div style={{ display: 'flex', alignItems: 'center', gap: 14, marginLeft: 'auto' }}>
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 7, fontFamily: 'var(--font-mono)', fontSize: 11.5, color: 'var(--cw-gray-900)' }}>
        <span style={{ width: 7, height: 7, borderRadius: '50%', background: 'var(--cw-green)' }} />
        {health}
      </span>
      <div style={{ width: 1, height: 24, background: 'var(--cw-gray-400)' }} />
      <button style={fwHeaderIconBtn}><Icon path={icons.bell} size={19} /></button>
      <button style={fwHeaderIconBtn}><Icon path={icons.help} size={19} /></button>
      <button style={fwHeaderIconBtn} onClick={onLock} title="Lock session"><Icon path={icons.lock} size={18} /></button>
      <button style={{ ...fwHeaderIconBtn, background: 'var(--cw-gray-100)', borderRadius: '50%', overflow: 'hidden' }} title="Special Agent Cow · FRO-007"><LogoMark size={20} /></button>
    </div>
  </header>
);

export const NavItem = ({ icon, label, active, onClick, badge }) => (
  <div onClick={onClick} style={{
    display: 'flex', alignItems: 'center', gap: 12, padding: '0 20px', margin: '1px 0',
    fontFamily: 'var(--font-sans)', fontSize: 14, cursor: 'pointer',
    color: active ? 'var(--cw-blue-800)' : 'var(--cw-gray-950)',
    background: active ? 'var(--cw-blue-200)' : 'transparent',
    fontWeight: active ? 600 : 400, position: 'relative', height: 40, boxSizing: 'border-box',
  }}
    onMouseEnter={(e) => { if (!active) e.currentTarget.style.background = 'var(--cw-gray-50)'; }}
    onMouseLeave={(e) => { if (!active) e.currentTarget.style.background = 'transparent'; }}>
    {active && <span style={{ position: 'absolute', left: 0, top: 8, bottom: 8, width: 2, background: 'var(--cw-blue)' }} />}
    <Icon path={icon} size={19} />
    <span>{label}</span>
    {badge != null && (
      <span style={{ marginLeft: 'auto', fontFamily: 'var(--font-ui)', fontSize: 11, fontWeight: 600, color: 'var(--cw-gray-900)', background: 'var(--cw-gray-100)', borderRadius: 10, padding: '1px 8px', minWidth: 20, textAlign: 'center' }}>{badge}</span>
    )}
  </div>
);

export const NavEyebrow = ({ children }) => (
  <div style={{ padding: '16px 20px 6px' }}>
    <Eyebrow>{children}</Eyebrow>
  </div>
);

export const Sidebar = ({ current, onNav, runCount }) => {
  const liveSilences = (typeof PAGERDUTY !== 'undefined')
    ? PAGERDUTY.silences.filter((s) => {
        const now = new Date('2026-05-30T14:25:00Z').getTime();
        return now >= new Date(s.starts_at).getTime() && now < new Date(s.ends_at).getTime();
      }).length
    : 0;
  return (
  <aside style={{ width: 248, background: 'var(--cw-bg-header, #FCFCFE)', borderRight: '1px solid var(--cw-gray-400)', display: 'flex', flexDirection: 'column', height: '100%', flexShrink: 0 }}>
    <div style={{ flex: 1, paddingTop: 6, overflow: 'auto' }}>
      <NavEyebrow>QA Workflows</NavEyebrow>
      <NavItem icon={icons.zap} label="Canary FW Release" active={current === 'runs'} onClick={() => onNav('runs')} badge={runCount} />
      <NavEyebrow>Troubleshooting</NavEyebrow>
      <NavItem icon={icons.cpu} label="Node Tracker" active={current === 'nodetracker'} onClick={() => onNav('nodetracker')} />
      <NavItem icon={icons.rack} label="Rack Tracker" active={current === 'racktracker'} onClick={() => onNav('racktracker')} />
      <NavItem icon={icons.board} label="L11 FieldDiag" active={current === 'l11dance'} onClick={() => onNav('l11dance')} />
      <NavItem icon={icons.bug} label="Jira" active={current === 'jira'} onClick={() => onNav('jira')} />
      <NavEyebrow>Library</NavEyebrow>
      <NavItem icon={icons.book} label="Knowledge" active={current === 'knowledge'} onClick={() => onNav('knowledge')} />
      <NavItem icon={icons.template} label="Templates" active={current === 'templates'} onClick={() => onNav('templates')} />
      <NavItem icon={icons.probe} label="Probes" active={current === 'probes'} onClick={() => onNav('probes')} />
      <NavItem icon={icons.sparkles} label="Agents" active={current === 'agents'} onClick={() => onNav('agents')} />
      <NavItem icon={icons.lineChart} label="Grafana Links" active={current === 'grafana'} onClick={() => onNav('grafana')} />
      <NavEyebrow>Records</NavEyebrow>
      <NavItem icon={icons.evidence} label="History" active={current === 'evidence'} onClick={() => onNav('evidence')} />
      <NavItem icon={icons.report} label="Reports" active={current === 'reports'} onClick={() => onNav('reports')} />
      <NavEyebrow>Alert Management</NavEyebrow>
      <NavItem icon={icons.bellOff} label="Silences" active={current === 'silencer'} onClick={() => onNav('silencer')} badge={liveSilences || null} />
      <NavEyebrow>User Settings</NavEyebrow>
      <NavItem icon={icons.bell} label="Browser Notifications" active={current === 'webpush'} onClick={() => onNav('webpush')} />
    </div>
    <div style={{ borderTop: '1px solid var(--cw-gray-400)' }}>
      <NavEyebrow>System</NavEyebrow>
      <NavItem icon={icons.plug} label="Integrations" active={current === 'integrations'} onClick={() => onNav('integrations')} />
      <NavItem icon={icons.settings} label="Settings" active={current === 'settings'} onClick={() => onNav('settings')} />
      <div style={{ padding: '12px 20px 14px' }}>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10.5, color: 'var(--cw-gray-700)', lineHeight: 1.6 }}>
          QAgenticCow · v0.4.2-iter4b<br />build 9f3c1a2 · db ~/.local/share
        </div>
      </div>
    </div>
  </aside>
  );
};

