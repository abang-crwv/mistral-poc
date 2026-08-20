// fwqual — Acknowledge gate, MGS "codec call" styling.
// Incoming transmission from FRO HQ on 140.85. Tall portrait windows flank a
// central frequency dial (PTT / MEMORY); the hail types out in white at the
// bottom; ACCEPT enters the console.
import { useState, useEffect } from 'react';
import { LogoMark } from '@/design/components';

const _cdcUseState = useState;
const _cdcUseEffect = useEffect;

const CDC_GREEN = '#5fe39b';
const CDC_DIM = '#2c6b49';
const CDC_DIGIT = '#e4fbf0';
const HAIL = "This channel is restricted. Your identity is on the wire, and everything you touch here is live fleet hardware. Confirm you're cleared, operator.";

// rising signal meter behind the frequency digits (staggered bars)
const SignalMeter = ({ live }) => (
  <div style={{ display: 'flex', alignItems: 'flex-end', gap: 4, height: 56 }}>
    {Array.from({ length: 13 }).map((_, i) => {
      const base = 11 + i * 3.4; // staircase rise
      return (
        <span key={i} className="cdc-bar" style={{
          width: 8, background: i < 7 ? CDC_DIGIT : CDC_DIM, borderRadius: 1,
          height: base, animationPlayState: live ? 'running' : 'paused', animationDelay: (i * 0.07) + 's',
        }} />
      );
    })}
  </div>
);

// FRO HQ portrait — operator silhouette (round head + bust) with the real
// handlebar mustache cut from the source art, baked into one phosphor-green PNG.
const MustacheFace = ({ size = 210 }) => (
  <img src="/assets/fro-hq-avatar.png" width={size} height={size} alt="FRO HQ operator" style={{ display: 'block' }} />
);

// operator portrait (you) — line-drawn comms silhouette
const OperatorFace = () => (
  <svg viewBox="0 0 48 56" width="86%" height="86%" fill="none" stroke={CDC_GREEN} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
    <path d="M24 8 C16 8 12 14 12 22 C12 29 15 33 18 35 L18 39 C11 40 5 44 5 52 L43 52 C43 44 37 40 30 39 L30 35 C33 33 36 29 36 22 C36 14 32 8 24 8 Z" opacity="0.9"/>
    <path d="M12 20 L7 18 M36 20 L41 18" opacity="0.75"/>
    <circle cx="19" cy="23" r="1.6" fill={CDC_GREEN} stroke="none"/>
    <circle cx="29" cy="23" r="1.6" fill={CDC_GREEN} stroke="none"/>
    <path d="M20 30 C22 31.5 26 31.5 28 30" opacity="0.7"/>
  </svg>
);

const CodecPortrait = ({ children, name, sub, talking, tintCow }) => (
  <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 16 }}>
    <div className={'cdc-frame' + (talking ? ' cdc-talking' : '')} style={{
      width: 236, height: 324, position: 'relative', overflow: 'hidden', borderRadius: 3,
      background: 'radial-gradient(ellipse at 50% 40%, #0e2a1d 0%, #04100a 100%)',
      border: '3px solid ' + CDC_GREEN,
      boxShadow: 'inset 0 0 30px rgba(95,227,155,0.18), 0 0 26px rgba(95,227,155,0.45), 0 0 56px rgba(95,227,155,0.18)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
    }}>
      <div style={{ width: '92%', height: '92%', display: 'flex', alignItems: 'center', justifyContent: 'center',
        filter: tintCow ? 'grayscale(1) brightness(1.3) sepia(1) hue-rotate(76deg) saturate(6)' : 'none' }}>
        {children}
      </div>
      <div className="cdc-scan" />
      <div style={{ position: 'absolute', inset: 0, pointerEvents: 'none', background: 'repeating-linear-gradient(0deg, rgba(0,0,0,0.34) 0px, rgba(0,0,0,0.34) 1px, transparent 1px, transparent 3px)' }} />
    </div>
    <div style={{ textAlign: 'center', maxWidth: 240, padding: '8px 16px', borderRadius: 3,
      background: 'rgba(8,26,18,0.85)', border: '1px solid rgba(95,227,155,0.45)',
      boxShadow: '0 0 14px rgba(95,227,155,0.22), inset 0 0 10px rgba(0,0,0,0.5)' }}>
      <div style={{ fontFamily: 'var(--font-mono)', fontSize: 16, fontWeight: 700, letterSpacing: '0.08em', color: CDC_GREEN, textShadow: '0 0 3px rgba(95,227,155,0.45)' }}>{name}</div>
      {sub && <div style={{ fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 600, color: '#7fd9aa', marginTop: 4, letterSpacing: '0.12em' }}>{sub}</div>}
    </div>
  </div>
);

// the MGS plate label (dark plate, tracked caps)
const Plate = ({ children }) => (
  <div style={{ fontFamily: 'var(--font-mono)', fontSize: 17, fontWeight: 700, letterSpacing: '0.42em', color: CDC_GREEN,
    background: 'rgba(20,52,37,0.7)', padding: '5px 18px 5px 24px', textShadow: '0 0 8px rgba(95,227,155,0.5)' }}>{children}</div>
);

const CodecArrow = ({ dir }) => (
  <div className="cdc-pulse" style={{ color: CDC_GREEN, fontSize: 30, lineHeight: 1, textShadow: '0 0 10px rgba(95,227,155,0.6)' }}>
    {dir === 'left' ? '\u25C0' : '\u25B6'}
  </div>
);

function LoginCodec({ onAcknowledge }) {
  const [typed, setTyped] = _cdcUseState('');
  const done = typed.length >= HAIL.length;

  _cdcUseEffect(() => {
    let i = 0;
    const id = setInterval(() => { i += 1; setTyped(HAIL.slice(0, i)); if (i >= HAIL.length) clearInterval(id); }, 28);
    return () => clearInterval(id);
  }, []);

  _cdcUseEffect(() => {
    const onKey = (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onAcknowledge(); } };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onAcknowledge]);

  return (
    <div className="cdc-root" style={{
      height: '100vh', width: '100%', background: '#030605', color: CDC_GREEN,
      display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
      padding: '24px 32px', boxSizing: 'border-box', position: 'relative', overflow: 'hidden',
    }}>
      <style>{`
        @keyframes cdcBar { 0%,100% { transform: scaleY(0.6); } 50% { transform: scaleY(1); } }
        .cdc-bar { transform-origin: bottom; animation: cdcBar 0.8s ease-in-out infinite; }
        @keyframes cdcScan { 0% { top: -34%; } 100% { top: 134%; } }
        .cdc-scan { position: absolute; left: 0; right: 0; height: 30%; pointer-events: none;
          background: linear-gradient(180deg, transparent, rgba(95,227,155,0.12) 50%, transparent); animation: cdcScan 3.2s linear infinite; }
        @keyframes cdcFlicker { 0%,100% { opacity: 1; } 92% { opacity: 1; } 94% { opacity: 0.88; } 96% { opacity: 1; } }
        .cdc-console { animation: cdcFlicker 6s infinite; }
        @keyframes cdcBlink { 0%,49% { opacity: 1; } 50%,100% { opacity: 0; } }
        .cdc-blink { animation: cdcBlink 1s steps(1) infinite; }
        @keyframes cdcRing { 0%,100% { box-shadow: inset 0 0 30px rgba(95,227,155,0.18), 0 0 26px rgba(95,227,155,0.45), 0 0 56px rgba(95,227,155,0.18); }
          50% { box-shadow: inset 0 0 30px rgba(95,227,155,0.34), 0 0 40px rgba(95,227,155,0.75), 0 0 80px rgba(95,227,155,0.3); } }
        .cdc-talking { animation: cdcRing 0.85s ease-in-out infinite; }
        @keyframes cdcPulse { 0%,100% { opacity: 0.45; } 50% { opacity: 1; } }
        .cdc-pulse { animation: cdcPulse 1.2s ease-in-out infinite; }
        .cdc-accept:hover { background: ${CDC_GREEN} !important; color: #04140c !important; box-shadow: 0 0 30px rgba(95,227,155,0.7) !important; }
      `}</style>

      {/* pixelated Porsche backdrop, terminal-rendered */}
      <img src="/assets/porsche-ascii.png" alt="" style={{
        position: 'absolute', left: '50%', top: '50%', transform: 'translate(-50%,-50%)',
        width: 'min(128vw, 2300px)', imageRendering: 'pixelated', opacity: 0.72, zIndex: 0,
        pointerEvents: 'none', mixBlendMode: 'screen',
      }} />
      {/* soft scrim to keep the lower dialogue legible over the ASCII */}
      <div style={{ position: 'absolute', left: 0, right: 0, bottom: 0, height: '46%', zIndex: 0, pointerEvents: 'none',
        background: 'linear-gradient(180deg, transparent, rgba(3,6,5,0.72) 60%, rgba(3,6,5,0.9))' }} />

      {/* CRT scanlines + vignette */}
      <div style={{ position: 'absolute', inset: 0, pointerEvents: 'none', zIndex: 2, background: 'repeating-linear-gradient(0deg, rgba(0,0,0,0.24) 0px, rgba(0,0,0,0.24) 1px, transparent 1px, transparent 3px)' }} />
      <div style={{ position: 'absolute', inset: 0, pointerEvents: 'none', zIndex: 2, background: 'radial-gradient(ellipse at center, transparent 60%, rgba(0,0,0,0.74) 100%)' }} />

      <div className="cdc-console" style={{ position: 'relative', zIndex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
        {/* portraits + central dial */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 44 }}>
          <CodecPortrait name="FRO HQ" sub="HQ · SECURE" talking={!done}>
            <MustacheFace size={210} />
          </CodecPortrait>

          <CodecArrow dir="left" />

          {/* central frequency module */}
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 11, width: 376 }}>
            <Plate>PTT</Plate>
            <div style={{ width: '100%', height: 98, background: 'linear-gradient(180deg, #0a1f16, #061410 100%)', border: '1px solid ' + CDC_DIM, borderRadius: 4, position: 'relative', overflow: 'hidden' }}>
              {/* signal meter sits behind the digits */}
              <div style={{ position: 'absolute', left: 20, top: 0, bottom: 0, display: 'flex', alignItems: 'center', opacity: 0.55 }}>
                <SignalMeter live={!done} />
              </div>
              <div style={{ position: 'absolute', inset: 0, display: 'flex', alignItems: 'center', justifyContent: 'center', fontFamily: 'var(--font-mono)', fontSize: 54, fontWeight: 700, color: CDC_DIGIT, letterSpacing: '0.01em', textShadow: '0 0 16px rgba(228,251,240,0.65)' }}>
                140.85
              </div>
            </div>
            <Plate>MEMORY</Plate>
          </div>

          <CodecArrow dir="right" />

          <CodecPortrait name="SPECIAL AGENT COW" sub="FRO-007" tintCow>
            <LogoMark size={210} />
          </CodecPortrait>
        </div>

        {/* dialogue — white subtitle at the bottom */}
        <div style={{ marginTop: 56, maxWidth: 1040, minHeight: 110, textAlign: 'center' }}>
          <p style={{ margin: 0, fontFamily: 'var(--font-sans)', fontSize: 34, fontWeight: 600, lineHeight: 1.45, color: '#f1fff8', textShadow: '0 1px 10px rgba(0,0,0,0.8)', letterSpacing: '0.01em' }}>
            {typed}<span className="cdc-blink" style={{ display: done ? 'none' : 'inline', color: CDC_GREEN }}>▮</span>
          </p>
        </div>

        {/* accept prompt */}
        <div style={{ marginTop: 36, height: 64, display: 'flex', alignItems: 'center' }}>
          <button className={'cdc-accept' + (done ? ' cdc-pulse' : '')} onClick={onAcknowledge} style={{
            fontFamily: 'var(--font-mono)', fontSize: 18, fontWeight: 700, letterSpacing: '0.18em',
            color: CDC_GREEN, background: 'transparent', border: '1px solid ' + CDC_GREEN, borderRadius: 3,
            padding: '16px 36px', cursor: 'pointer', whiteSpace: 'nowrap', transition: 'all 140ms',
            textShadow: '0 0 8px rgba(95,227,155,0.5)',
          }}>
            ▶ ACCEPT TRANSMISSION
          </button>
        </div>
      </div>

      {/* freq / channel chrome, bottom corners */}
      <div style={{ position: 'absolute', bottom: 18, left: 24, fontFamily: 'var(--font-mono)', fontSize: 11, letterSpacing: '0.14em', color: CDC_DIM }}>
        QAGENTICCOW · SECURE CODEC
      </div>
      <div style={{ position: 'absolute', bottom: 18, right: 24, fontFamily: 'var(--font-mono)', fontSize: 11, letterSpacing: '0.14em', color: CDC_DIM }}>
        ENTER · ACCEPT
      </div>
    </div>
  );
}


export default LoginCodec;
