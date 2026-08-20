// fwqual — Acknowledge gate.
// A pre-app screen: agent mark + authorized-use notice + a single
// "Acknowledge" button. No credentials — clicking it enters the console.
import { Icon, icons, LogoMark } from '@/design/components';

function Login({ onAcknowledge }) {
  return (
    <div style={{
      height: '100vh', width: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'var(--cw-bg-canvas, #FCFCFE)', padding: 24, boxSizing: 'border-box',
    }}>
      {/* faint classified-style backdrop band */}
      <div style={{ position: 'fixed', inset: 0, pointerEvents: 'none', overflow: 'hidden' }}>
        <div style={{ position: 'absolute', top: '50%', left: '50%', transform: 'translate(-50%,-50%)', width: 720, height: 720, borderRadius: '50%', background: 'radial-gradient(circle, var(--cw-blue-100) 0%, transparent 70%)', opacity: 0.5 }} />
      </div>

      <div style={{
        position: 'relative', width: 440, maxWidth: '100%', background: 'var(--fw-surface)',
        border: '1px solid var(--cw-gray-400)', borderRadius: 'var(--card-radius)',
        boxShadow: 'var(--shadow-overlay, 0 12px 40px rgba(13,15,18,0.24))', overflow: 'hidden',
      }}>
        {/* brand bar */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '20px 28px', borderBottom: '1px solid var(--cw-gray-200)' }}>
          <LogoMark size={34} />
          <div style={{ display: 'flex', flexDirection: 'column' }}>
            <span style={{ fontFamily: 'var(--font-display)', fontWeight: 800, fontSize: 19, color: 'var(--cw-gray-950)', letterSpacing: '-0.01em', lineHeight: 1.1 }}>QAgenticCow</span>
            <span style={{ fontFamily: 'var(--font-ui)', fontSize: 11.5, color: 'var(--cw-gray-700)', fontWeight: 500, marginTop: 2 }}>Firmware Canary Verification · Fleet / FRO</span>
          </div>
        </div>

        {/* notice */}
        <div style={{ padding: '24px 28px 26px' }}>
          <div style={{ display: 'inline-flex', alignItems: 'center', gap: 7, fontFamily: 'var(--font-ui)', fontSize: 11, fontWeight: 700, letterSpacing: '0.07em', textTransform: 'uppercase', color: 'var(--cw-blue-800)', background: 'var(--cw-blue-200)', padding: '3px 9px', borderRadius: 4 }}>
            <Icon path={icons.lock || icons.user} size={13} /> Authorized access only
          </div>
          <p style={{ fontFamily: 'var(--font-sans)', fontSize: 13.5, lineHeight: 1.6, color: 'var(--cw-gray-900)', margin: '14px 0 0' }}>
            This is a restricted CoreWeave operations console. Activity is associated with your identity and logged. Firmware actions taken here apply to live fleet hardware. By continuing you confirm you are authorized and accept the monitoring and handling terms.
          </p>

          <div style={{ marginTop: 22 }}>
            <button onClick={onAcknowledge} style={{
              width: '100%', height: 44, border: 0, borderRadius: 'var(--button-radius)', cursor: 'pointer',
              background: '#4264ff', color: '#ffffff', fontFamily: 'var(--font-ui)', fontWeight: 600, fontSize: 15,
              display: 'inline-flex', alignItems: 'center', justifyContent: 'center', gap: 8,
            }}
              onMouseEnter={(e) => { e.currentTarget.style.background = '#3552ed'; }}
              onMouseLeave={(e) => { e.currentTarget.style.background = '#4264ff'; }}>
              Acknowledge
            </button>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: 16, fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--cw-gray-700)' }}>
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
              <span style={{ width: 6, height: 6, borderRadius: '50%', background: 'var(--cw-green)' }} /> session · Special Agent Cow
            </span>
            <span>clearance · FRO-007</span>
          </div>
        </div>
      </div>
    </div>
  );
}


export default Login;
