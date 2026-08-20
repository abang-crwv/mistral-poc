// fwqual — Web Push.
// Configures which run events QA COW sends as Web Push notifications to a
// subscribed browser; the browser delivers them to the OS notification center.
import { useState } from 'react';
import { Icon, icons, LogoMark, Button, Card, Eyebrow, fmtRel } from '@/design/components';
import { WEB_PUSH } from '@/design/seed';

const _wpUseState = useState;

// ---- small switch toggle --------------------------------------------------
const WpSwitch = ({ on, onClick, disabled }) => (
  <button onClick={disabled ? undefined : onClick} aria-pressed={on} style={{
    width: 38, height: 22, borderRadius: 11, border: 0, padding: 2, cursor: disabled ? 'default' : 'pointer',
    background: on ? 'var(--cw-blue-700)' : 'var(--cw-gray-500)', opacity: disabled ? 0.4 : 1,
    display: 'inline-flex', alignItems: 'center', transition: 'background 120ms', flexShrink: 0,
  }}>
    <span style={{ width: 18, height: 18, borderRadius: '50%', background: '#fff', transform: on ? 'translateX(16px)' : 'translateX(0)', transition: 'transform 120ms', boxShadow: '0 1px 2px rgba(0,0,0,0.3)' }} />
  </button>
);

// ---- a realistic OS push notification banner ------------------------------
const PushNotification = ({ title, body, app = 'QAgenticCow', when = 'now' }) => (
  <div style={{
    display: 'flex', gap: 12, padding: '12px 13px', borderRadius: 14, width: '100%', boxSizing: 'border-box',
    background: 'rgba(245,245,248,0.92)', border: '0.5px solid rgba(0,0,0,0.08)',
    boxShadow: '0 10px 34px rgba(13,15,18,0.28)',
  }}>
    <div style={{ width: 38, height: 38, borderRadius: 9, background: '#211f26', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
      <LogoMark size={26} />
    </div>
    <div style={{ flex: 1, minWidth: 0 }}>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 8 }}>
        <span style={{ fontFamily: '-apple-system, var(--font-sans)', fontSize: 12, fontWeight: 700, color: '#1d1d1f', letterSpacing: '0.01em' }}>{app}</span>
        <span style={{ marginLeft: 'auto', fontFamily: '-apple-system, var(--font-sans)', fontSize: 11.5, color: 'rgba(60,60,67,0.6)' }}>{when}</span>
      </div>
      <div style={{ fontFamily: '-apple-system, var(--font-sans)', fontSize: 13.5, fontWeight: 600, color: '#1d1d1f', marginTop: 2 }}>{title}</div>
      <div style={{ fontFamily: '-apple-system, var(--font-sans)', fontSize: 13, color: 'rgba(60,60,67,0.78)', marginTop: 1, lineHeight: 1.35 }}>{body}</div>
    </div>
  </div>
);

const WP_STYLE_LABEL = { banner: 'Banner', alert: 'Alert' };

function WebPush() {
  const a = WEB_PUSH.agent;
  const [rules, setRules] = _wpUseState(() => WEB_PUSH.rules.map((r) => ({ ...r })));
  const [preview, setPreview] = _wpUseState(WEB_PUSH.recent);

  const enabledCount = rules.filter((r) => r.enabled).length;

  const toggle = (id) => setRules((xs) => xs.map((r) => r.id === id ? { ...r, enabled: !r.enabled } : r));
  const setStyle = (id, style) => setRules((xs) => xs.map((r) => r.id === id ? { ...r, style } : r));
  const toggleSound = (id) => setRules((xs) => xs.map((r) => r.id === id ? { ...r, sound: !r.sound } : r));

  const sendTest = () => setPreview({
    title: 'Test notification',
    body: 'QAgenticCow web push is delivering to this browser.',
    at: new Date().toISOString(),
  });

  return (
    <div style={{ padding: '28px 32px 64px', background: 'var(--cw-bg-canvas, #FCFCFE)', flex: 1, overflow: 'auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 6 }}>
        <div>
          <h1 style={{ margin: 0, fontFamily: 'var(--font-display)', fontWeight: 700, fontSize: 28, color: 'var(--cw-gray-950)', letterSpacing: '-0.01em' }}>Browser Notifications</h1>
          <p style={{ margin: '6px 0 0', fontFamily: 'var(--font-sans)', fontSize: 14, color: 'var(--cw-gray-900)', maxWidth: 660 }}>
            Push run events to your browser. Subscribe this browser and QAgenticCow sends a Web Push — delivered to your OS notification center — whenever an enabled rule fires.
          </p>
        </div>
        <Button variant="secondary" leftIcon={icons.bell} onClick={sendTest}>Send test</Button>
      </div>

      {/* ---- push subscription status ---- */}
      <Card style={{ display: 'flex', alignItems: 'center', gap: 16, padding: '14px 18px', margin: '22px 0 0' }}>
        <div style={{ width: 40, height: 40, borderRadius: 6, background: 'var(--cw-blue-200)', color: 'var(--cw-blue-800)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
          <Icon path={icons.bell} size={20} />
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <span style={{ fontFamily: 'var(--font-sans)', fontSize: 15, fontWeight: 600, color: 'var(--cw-gray-950)' }}>This browser</span>
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontFamily: 'var(--font-ui)', fontSize: 11.5, fontWeight: 600, padding: '2px 9px', borderRadius: 4, background: 'var(--cw-green-100)', color: 'var(--status-success-fg)' }}>
              <span style={{ width: 6, height: 6, borderRadius: '50%', background: 'var(--cw-green)' }} />Subscribed
            </span>
          </div>
          <div style={{ display: 'flex', gap: 16, fontFamily: 'var(--font-mono)', fontSize: 11.5, color: 'var(--cw-gray-700)', flexWrap: 'wrap' }}>
            <span>{a.browser}</span>
            <span>permission · {a.permission}</span>
            <span>{a.endpoint}</span>
            <span>{a.delivered_today} delivered today</span>
            <span>last {fmtRel(a.last_delivered)}</span>
          </div>
        </div>
        <div style={{ marginLeft: 'auto' }}>
          <Button variant="secondary" size="sm" rightIcon={icons.externalLink}>Manage permission</Button>
        </div>
      </Card>

      {/* ---- two columns: rules + live preview ---- */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 360px', gap: 20, margin: '20px 0 0', alignItems: 'start' }}>
        {/* rules */}
        <Card style={{ overflow: 'hidden' }}>
          <div style={{ padding: '13px 18px', borderBottom: '1px solid var(--cw-gray-200)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Eyebrow>Notification rules</Eyebrow>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11.5, color: 'var(--cw-gray-700)' }}>{enabledCount} of {rules.length} enabled</span>
          </div>
          <div>
            {rules.map((r, i) => (
              <div key={r.id} style={{
                display: 'flex', alignItems: 'center', gap: 16, padding: '13px 18px',
                borderBottom: i < rules.length - 1 ? '1px solid var(--cw-gray-200)' : 'none',
                opacity: r.enabled ? 1 : 0.55,
              }}>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontFamily: 'var(--font-sans)', fontSize: 14, fontWeight: 600, color: 'var(--cw-gray-950)' }}>{r.label}</div>
                  <div style={{ fontFamily: 'var(--font-sans)', fontSize: 12.5, color: 'var(--cw-gray-700)', marginTop: 2 }}>{r.desc}</div>
                </div>
                {/* style segmented control */}
                <div style={{ display: 'flex', border: '1px solid var(--cw-gray-400)', borderRadius: 4, overflow: 'hidden', flexShrink: 0 }}>
                  {['banner', 'alert'].map((st) => {
                    const on = r.style === st;
                    return (
                      <button key={st} onClick={() => r.enabled && setStyle(r.id, st)} style={{
                        fontFamily: 'var(--font-ui)', fontSize: 11.5, fontWeight: 600, padding: '5px 11px', border: 0, cursor: r.enabled ? 'pointer' : 'default',
                        background: on ? 'var(--cw-blue-200)' : 'var(--fw-surface)', color: on ? 'var(--cw-blue-800)' : 'var(--cw-gray-700)',
                      }}>{WP_STYLE_LABEL[st]}</button>
                    );
                  })}
                </div>
                {/* sound */}
                <button onClick={() => r.enabled && toggleSound(r.id)} title="Play sound" style={{
                  width: 30, height: 30, borderRadius: 4, border: '1px solid var(--cw-gray-400)', cursor: r.enabled ? 'pointer' : 'default',
                  background: r.sound ? 'var(--cw-blue-200)' : 'var(--fw-surface)', color: r.sound ? 'var(--cw-blue-800)' : 'var(--cw-gray-600)',
                  display: 'inline-flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0,
                }}>
                  <Icon path={r.sound ? icons.bell : icons.bellOff} size={15} />
                </button>
                <WpSwitch on={r.enabled} onClick={() => toggle(r.id)} />
              </div>
            ))}
          </div>
        </Card>

        {/* preview */}
        <div style={{ position: 'sticky', top: 0 }}>
          <Eyebrow style={{ marginBottom: 10 }}>Preview</Eyebrow>
          <div style={{ borderRadius: 16, padding: 18, background: 'linear-gradient(150deg, #3a4a6b, #25304a)' }}>
            <PushNotification title={preview.title} body={preview.body} when={fmtRel(preview.at)} />
          </div>
          <p style={{ fontFamily: 'var(--font-sans)', fontSize: 12.5, color: 'var(--cw-gray-700)', margin: '12px 2px 0', lineHeight: 1.5 }}>
            How an enabled rule appears in the top-right of your screen. Alert-style notifications stay until dismissed; banners auto-hide.
          </p>
        </div>
      </div>
    </div>
  );
}


export default WebPush;
