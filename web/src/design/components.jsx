// fwqual — UI primitives. Forked from the CoreWeave Cloud Console kit,
// extended with canary-domain components. All styling via design-system tokens.


// ============ ICONS (inline Lucide-style, 2px stroke) ============
export const Icon = ({ path, size = 20, fill = 'none', strokeWidth = 2, ...p }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill={fill} stroke="currentColor"
    strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" {...p}>{path}</svg>
);

export const icons = {
  runs:      <><path d="M3 3v18h18"/><path d="m7 14 3-3 3 3 5-6"/></>,
  template:  <><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/><path d="M9 13h6M9 17h4"/></>,
  evidence:  <><path d="M20 6 9 17l-5-5"/></>,
  fleet:     <><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/></>,
  rack:      <><rect x="4" y="3" width="16" height="18" rx="1"/><path d="M4 9h16M4 15h16"/><path d="M8 6h.01M8 12h.01M8 18h.01"/></>,
  settings:  <><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></>,
  plus:      <><path d="M5 12h14M12 5v14"/></>,
  search:    <><circle cx="11" cy="11" r="7"/><path d="m20 20-3-3"/></>,
  chevDown:  <><path d="m6 9 6 6 6-6"/></>,
  chevRight: <><path d="m9 18 6-6-6-6"/></>,
  arrowLeft: <><path d="M19 12H5M12 19l-7-7 7-7"/></>,
  help:      <><circle cx="12" cy="12" r="10"/><path d="M9.1 9a3 3 0 0 1 5.8 1c0 2-3 2-3 4"/><path d="M12 17h.01"/></>,
  bell:      <><path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></>,
  bellOff:   <><path d="M8.7 3A6 6 0 0 1 18 8a21.3 21.3 0 0 0 .6 5"/><path d="M17 17H3s3-2 3-9a4.67 4.67 0 0 1 .3-1.7"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/><path d="m2 2 20 20"/></>,
  calClock:  <><path d="M21 7.5V6a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h6"/><path d="M3 10h18M16 2v4M8 2v4"/><circle cx="18" cy="18" r="4"/><path d="M18 16.5V18l1 1"/></>,
  user:      <><circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/></>,
  moreVert:  <><circle cx="12" cy="5" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="12" cy="19" r="1"/></>,
  check:     <><path d="M20 6 9 17l-5-5"/></>,
  checkCircle: <><circle cx="12" cy="12" r="10"/><path d="m9 12 2 2 4-4"/></>,
  x:         <><path d="M18 6 6 18M6 6l12 12"/></>,
  xCircle:   <><circle cx="12" cy="12" r="10"/><path d="m15 9-6 6M9 9l6 6"/></>,
  alert:     <><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><path d="M12 9v4M12 17h.01"/></>,
  spinner:   <><path d="M21 12a9 9 0 1 1-6.219-8.56"/></>,
  clock:     <><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></>,
  dot:       <><circle cx="12" cy="12" r="4"/></>,
  zap:       <><path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/></>,
  cpu:       <><rect x="4" y="4" width="16" height="16" rx="2"/><rect x="9" y="9" width="6" height="6"/><path d="M9 2v2M15 2v2M9 20v2M15 20v2M2 9h2M2 15h2M20 9h2M20 15h2"/></>,
  sparkles:  <><path d="m12 3-1.9 5.8a2 2 0 0 1-1.3 1.3L3 12l5.8 1.9a2 2 0 0 1 1.3 1.3L12 21l1.9-5.8a2 2 0 0 1 1.3-1.3L21 12l-5.8-1.9a2 2 0 0 1-1.3-1.3z"/></>,
  externalLink: <><path d="M15 3h6v6M10 14 21 3M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/></>,
  signature: <><path d="M3 17s2-3 5-3 4 2 6 2 5-3 5-3"/><path d="M3 21h18"/></>,
  filter:    <><path d="M22 3H2l8 9.46V19l4 2v-8.54z"/></>,
  refresh:   <><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8M21 3v5h-5M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16M8 16H3v5"/></>,
  moon:      <><path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9z"/></>,
  sun:       <><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></>,
  probe:     <><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></>,
  report:    <><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/><path d="M9 17v-3M12 17v-5M15 17v-2"/></>,
  sliders:   <><line x1="21" x2="14" y1="4" y2="4"/><line x1="10" x2="3" y1="4" y2="4"/><line x1="21" x2="12" y1="12" y2="12"/><line x1="8" x2="3" y1="12" y2="12"/><line x1="21" x2="16" y1="20" y2="20"/><line x1="12" x2="3" y1="20" y2="20"/><line x1="14" x2="14" y1="2" y2="6"/><line x1="8" x2="8" y1="10" y2="14"/><line x1="16" x2="16" y1="18" y2="22"/></>,
  plug:      <><path d="M12 22v-5M9 8V2M15 8V2M18 8v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V8z"/></>,
  board:     <><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M8 3v18M16 3v18"/><path d="M3 9h5M16 9h5M3 15h5"/></>,
  lineChart: <><path d="M3 3v16a2 2 0 0 0 2 2h16"/><path d="m19 9-5 5-4-4-3 3"/></>,
  lock:      <><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></>,
  bug:       <><path d="m8 2 1.88 1.88M14.12 3.88 16 2"/><path d="M9 7.13v-1a3.003 3.003 0 1 1 6 0v1"/><path d="M12 20c-3.3 0-6-2.7-6-6v-3a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v3c0 3.3-2.7 6-6 6"/><path d="M12 20v-9M6.53 9C4.6 8.8 3 7.1 3 5M6 13H2M3 21c0-2.1 1.7-3.9 3.8-4M20.97 5c0 2.1-1.6 3.8-3.5 4M22 13h-4M17.2 17c2.1.1 3.8 1.9 3.8 4"/></>,
  link2:     <><path d="M9 17H7A5 5 0 0 1 7 7h2M15 7h2a5 5 0 1 1 0 10h-2M8 12h8"/></>,
  message:   <><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></>,
  arrowUp:   <><path d="m5 12 7-7 7 7M12 19V5"/></>,
  arrowDown: <><path d="M12 5v14M5 12l7 7 7-7"/></>,
  equal:     <><path d="M5 9h14M5 15h14"/></>,
  jira:      <><path d="M12 2 5 9l7 7 7-7z"/><path d="M5 16l7 7 7-7"/></>,
  book:      <><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></>,
  database:  <><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14a9 3 0 0 0 18 0V5"/><path d="M3 12a9 3 0 0 0 18 0"/></>,
  send:      <><path d="M22 2 11 13M22 2l-7 20-4-9-9-4z"/></>,
  cornerDown:<><path d="m9 10-5 5 5 5"/><path d="M20 4v7a4 4 0 0 1-4 4H4"/></>,
};

// ============ LOGO MARK — QAgenticCow (CIA cow agent: shades + earpiece) ============
export const LogoMark = ({ size = 30 }) => (
  <svg width={size} height={size} viewBox="0 0 48 48" fill="none" aria-label="QAgenticCow">
    {/* ears */}
    <ellipse cx="9.5" cy="16.5" rx="6" ry="3.3" transform="rotate(-32 9.5 16.5)" fill="#4264FF"/>
    <ellipse cx="38.5" cy="16.5" rx="6" ry="3.3" transform="rotate(32 38.5 16.5)" fill="#4264FF"/>
    {/* horns */}
    <path d="M17 9 C13 6 12.4 3.4 14 2.4 C16 3.9 17.6 6.2 18.5 9 Z" fill="#4264FF"/>
    <path d="M31 9 C35 6 35.6 3.4 34 2.4 C32 3.9 30.4 6.2 29.5 9 Z" fill="#4264FF"/>
    {/* head */}
    <path d="M13 15 C13 10 16 8 19 8 L29 8 C32 8 35 10 35 15 L35 23 C35 32 30 36 24 37 C18 36 13 32 13 23 Z" fill="#4264FF"/>
    {/* muzzle (themed: dark navy in light, light in dark) */}
    <ellipse cx="24" cy="28.5" rx="8.6" ry="6" style={{ fill: 'var(--cw-gray-950)' }}/>
    {/* covert earpiece — bud on the cheek + coiled wire to the collar */}
    <circle cx="33.8" cy="23.2" r="1.6" fill="#14121a"/>
    <path d="M33.8 24.8 C35.7 26.8 35.4 29.8 34 31.8" fill="none" stroke="#14121a" strokeWidth="1.3" strokeLinecap="round"/>
    {/* sunglasses — two lenses, bridge, temple arm to the ear, + glints */}
    <ellipse cx="18.6" cy="18" rx="4.3" ry="3" fill="#14121a"/>
    <ellipse cx="29.4" cy="18" rx="4.3" ry="3" fill="#14121a"/>
    <rect x="22.6" y="17.2" width="2.8" height="1.5" rx="0.6" fill="#14121a"/>
    <path d="M33.7 17.6 L36.6 16.1" stroke="#14121a" strokeWidth="1.3" strokeLinecap="round"/>
    <ellipse cx="16.8" cy="16.7" rx="1.1" ry="0.7" fill="#ffffff" opacity="0.85"/>
    <ellipse cx="27.6" cy="16.7" rx="1.1" ry="0.7" fill="#ffffff" opacity="0.85"/>
    {/* nostrils */}
    <ellipse cx="21" cy="29" rx="1.3" ry="2" fill="#4264FF"/>
    <ellipse cx="27" cy="29" rx="1.3" ry="2" fill="#4264FF"/>
  </svg>
);

// ============ BUTTON ============
export const Button = ({ variant = 'primary', size = 'md', leftIcon, rightIcon, children, style, ...p }) => {
  const base = { fontFamily: 'var(--font-ui)', fontWeight: 600, borderRadius: 'var(--button-radius)', border: '1px solid transparent', cursor: 'pointer', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', gap: 8, lineHeight: 1, transition: 'background 120ms, border-color 120ms', whiteSpace: 'nowrap' };
  const sizes = { sm: { height: 28, fontSize: 12.5, padding: '0 10px' }, md: { height: 36, fontSize: 14, padding: '0 14px' }, lg: { height: 44, fontSize: 15, padding: '0 18px' } };
  const variants = {
    primary: { background: 'var(--cw-blue-800)', color: '#fff' },
    secondary: { background: 'var(--fw-surface)', color: 'var(--cw-gray-950)', borderColor: 'var(--cw-gray-400)' },
    ghost: { background: 'transparent', color: 'var(--cw-gray-950)' },
    danger: { background: 'var(--cw-red)', color: '#fff' },
  };
  return (
    <button style={{ ...base, ...sizes[size], ...variants[variant], ...style }}
      onMouseEnter={(e) => { if (variant === 'primary') e.currentTarget.style.background = 'var(--cw-blue-700)'; if (variant === 'secondary' || variant === 'ghost') e.currentTarget.style.background = 'var(--cw-gray-50)'; }}
      onMouseLeave={(e) => { e.currentTarget.style.background = variants[variant].background; }}
      {...p}>
      {leftIcon && <Icon path={leftIcon} size={size === 'sm' ? 14 : 16} />}{children}{rightIcon && <Icon path={rightIcon} size={size === 'sm' ? 14 : 16} />}
    </button>
  );
};

// ============ INPUT ============
export const Input = ({ icon, style, ...p }) => (
  <div style={{ position: 'relative', display: 'flex', alignItems: 'center', width: '100%' }}>
    {icon && <span style={{ position: 'absolute', left: 11, color: 'var(--cw-gray-700)', display: 'flex', pointerEvents: 'none' }}><Icon path={icon} size={16} /></span>}
    <input style={{ fontFamily: 'var(--font-sans)', fontSize: 14, height: 36, padding: icon ? '0 12px 0 34px' : '0 12px', border: '1px solid var(--cw-gray-400)', borderRadius: 'var(--input-radius)', background: 'var(--fw-surface)', color: 'var(--cw-gray-950)', outline: 'none', width: '100%', boxSizing: 'border-box', ...style }}
      onFocus={(e) => { e.target.style.borderColor = 'var(--cw-blue)'; e.target.style.boxShadow = '0 0 0 2px var(--cw-blue-300)'; }}
      onBlur={(e) => { e.target.style.borderColor = 'var(--cw-gray-400)'; e.target.style.boxShadow = 'none'; }}
      {...p} />
  </div>
);

// ============ CARD ============
export const Card = ({ children, style, ...p }) => (
  <div style={{ background: 'var(--fw-surface)', border: '1px solid var(--cw-gray-400)', borderRadius: 'var(--card-radius)', ...style }} {...p}>{children}</div>
);

// ============ TAG ============
export const Tag = ({ children, mono, style }) => (
  <span style={{ display: 'inline-flex', alignItems: 'center', fontFamily: mono ? 'var(--font-mono)' : 'var(--font-ui)', fontSize: 12, fontWeight: mono ? 500 : 600, padding: '3px 8px', borderRadius: 4, background: 'var(--cw-gray-100)', color: 'var(--cw-gray-950)', ...style }}>{children}</span>
);

// ============ RUN STATUS PILL ============
export const RUN_STATUS = {
  pending:    { bg: 'var(--cw-gray-100)',  fg: 'var(--cw-gray-900)', dot: 'var(--cw-gray-700)', label: 'Pending' },
  running:    { bg: 'var(--cw-blue-300)',  fg: 'var(--cw-blue-950)', dot: 'var(--cw-blue)',     label: 'Running' },
  passed:     { bg: 'var(--cw-green-100)',  fg: 'var(--status-success-fg)',           dot: 'var(--cw-green)',    label: 'Passed' },
  warning:    { bg: 'var(--cw-yellow-100)', fg: 'var(--status-warning-fg)',           dot: 'var(--cw-yellow)',   label: 'Warning' },
  failed:     { bg: 'var(--cw-red-100)',    fg: 'var(--status-danger-fg)',      dot: 'var(--cw-red)',      label: 'Failed' },
  signed_off: { bg: 'var(--cw-green-100)',  fg: 'var(--status-success-fg)',           dot: 'var(--cw-green)',    label: 'Signed off' },
  cancelled:  { bg: 'var(--cw-gray-100)',  fg: 'var(--cw-gray-900)', dot: 'var(--cw-gray-700)', label: 'Cancelled' },
};
export const StatusPill = ({ status, size = 'md' }) => {
  const s = RUN_STATUS[status] || RUN_STATUS.pending;
  const dims = size === 'sm' ? { fontSize: 11, padding: '2px 8px', h: 20 } : { fontSize: 12, padding: '3px 10px', h: 22 };
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontFamily: 'var(--font-ui)', fontWeight: 600, fontSize: dims.fontSize, padding: dims.padding, borderRadius: 4, height: dims.h, background: s.bg, color: s.fg, whiteSpace: 'nowrap' }}>
      <span style={{ width: 7, height: 7, borderRadius: '50%', background: s.dot }} />{s.label}
    </span>
  );
};

// ============ SEVERITY PILL (alert evidence) ============
export const SEVERITY = {
  critical: { bg: 'var(--cw-red-100)',    fg: 'var(--status-danger-fg)', dot: 'var(--cw-red)' },
  warning:  { bg: 'var(--cw-yellow-100)', fg: 'var(--status-warning-fg)', dot: 'var(--cw-yellow)' },
  info:     { bg: 'var(--cw-gray-100)',   fg: 'var(--cw-gray-900)', dot: 'var(--cw-gray-700)' },
  clear:    { bg: 'var(--cw-green-100)',  fg: 'var(--status-success-fg)', dot: 'var(--cw-green)' },
};
export const SeverityPill = ({ severity }) => {
  const s = SEVERITY[severity] || SEVERITY.info;
  const label = severity === 'clear' ? 'Clear' : severity.charAt(0).toUpperCase() + severity.slice(1);
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontFamily: 'var(--font-ui)', fontWeight: 600, fontSize: 11.5, padding: '2px 9px', borderRadius: 4, height: 20, background: s.bg, color: s.fg, whiteSpace: 'nowrap' }}>
      <span style={{ width: 6, height: 6, borderRadius: '50%', background: s.dot }} />{label}
    </span>
  );
};

// ============ EYEBROW ============
export const Eyebrow = ({ children, style }) => (
  <div style={{ fontFamily: 'var(--font-ui)', fontWeight: 700, fontSize: 11, letterSpacing: '0.07em', textTransform: 'uppercase', color: 'var(--cw-gray-700)', ...style }}>{children}</div>
);

// ============ formatting helpers ============
export const fmtRel = (iso) => {
  const then = new Date(iso).getTime();
  const now = new Date('2026-05-30T14:25:00Z').getTime();
  const s = Math.round((now - then) / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.round(s / 60); if (m < 60) return `${m}m ago`;
  const h = Math.round(m / 60); if (h < 24) return `${h}h ago`;
  const d = Math.round(h / 24); return `${d}d ago`;
};
export const fmtAbs = (iso) => {
  const dt = new Date(iso);
  return dt.toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false, timeZone: 'UTC' }) + ' UTC';
};
export const shortId = (id) => id.slice(0, 6) + '…' + id.slice(-4);

