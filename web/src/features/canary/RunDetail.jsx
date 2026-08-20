// fwqual — Run detail: step rail + discovered facts + per-rack evidence + event timeline.
import { useState } from 'react';
import { Icon, icons, Button, Card, Tag, Input, StatusPill, SeverityPill, Eyebrow, fmtAbs } from '@/design/components';
import AssessmentCard from './AssessmentCard';

const _cwUseState = useState;

// Table cell styles for the alert-evidence table (was previously sourced from
// the design module scope; localized here when RunDetail moved to props).
const fwTh = { textAlign: 'left', padding: '10px 16px', fontFamily: 'var(--font-ui)', fontSize: 12, fontWeight: 600, color: 'var(--cw-gray-950)', borderBottom: '1px solid var(--cw-gray-400)', whiteSpace: 'nowrap', userSelect: 'none' };
const fwTd = { padding: '13px 16px', fontFamily: 'var(--font-sans)', fontSize: 13.5, color: 'var(--cw-gray-950)', borderBottom: '1px solid var(--cw-gray-200)', verticalAlign: 'middle' };

// ---- step-state visual mapping --------------------------------------------
const STEP_STATE_VIS = {
  passed:  { color: 'var(--cw-green)',  icon: icons.checkCircle, ring: 'var(--cw-green-300)' },
  running: { color: 'var(--cw-blue)',   icon: icons.spinner,     ring: 'var(--cw-blue-400)' },
  failed:  { color: 'var(--cw-red)',    icon: icons.xCircle,     ring: 'var(--cw-red-300)' },
  warning: { color: 'var(--cw-yellow-700)', icon: icons.alert,   ring: 'var(--cw-yellow-300)' },
  pending: { color: 'var(--cw-gray-600)', icon: icons.dot,       ring: 'var(--cw-gray-300)' },
};

const KIND_LABEL = { probe: 'Probe', review: 'Review', ai_assess: 'AI assess', signoff: 'Sign-off' };

function StepRailItem({ step, index, state, selected, onSelect }) {
  const vis = STEP_STATE_VIS[state] || STEP_STATE_VIS.pending;
  const spinning = state === 'running';
  return (
    <div onClick={() => onSelect(step.id)} style={{
      display: 'flex', gap: 12, padding: '10px 12px', cursor: 'pointer', borderRadius: 4,
      background: selected ? 'var(--cw-blue-200)' : 'transparent',
      border: '1px solid ' + (selected ? 'var(--cw-blue-400)' : 'transparent'),
      position: 'relative',
    }}
      onMouseEnter={(e) => { if (!selected) e.currentTarget.style.background = 'var(--cw-gray-50)'; }}
      onMouseLeave={(e) => { if (!selected) e.currentTarget.style.background = 'transparent'; }}>
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', flexShrink: 0 }}>
        <span style={{ color: vis.color, display: 'flex', animation: spinning ? 'fwspin 1s linear infinite' : 'none' }}>
          <Icon path={vis.icon} size={18} fill={state === 'pending' ? 'currentColor' : 'none'} />
        </span>
      </div>
      <div style={{ minWidth: 0, flex: 1 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10.5, color: 'var(--cw-gray-700)' }}>{String(index + 1).padStart(2, '0')}</span>
          <span style={{ fontFamily: 'var(--font-sans)', fontSize: 13.5, fontWeight: selected ? 600 : 500, color: 'var(--cw-gray-950)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{step.title}</span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 4 }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10.5, color: 'var(--cw-gray-700)' }}>{step.tool || KIND_LABEL[step.kind].toLowerCase()}</span>
          {step.live
            ? <span style={{ fontFamily: 'var(--font-ui)', fontSize: 9.5, fontWeight: 700, letterSpacing: '0.04em', color: 'var(--status-success-fg)', background: 'var(--cw-green-100)', padding: '1px 6px', borderRadius: 3 }}>LIVE</span>
            : <span style={{ fontFamily: 'var(--font-ui)', fontSize: 9.5, fontWeight: 700, letterSpacing: '0.04em', color: 'var(--cw-gray-900)', background: 'var(--cw-gray-100)', padding: '1px 6px', borderRadius: 3 }}>DESIGNED</span>}
        </div>
      </div>
    </div>
  );
}

function StepRail({ steps = [], stepIndex = {}, states, selected, onSelect }) {
  return (
    <Card style={{ padding: 0, overflow: 'hidden', alignSelf: 'flex-start' }}>
      <div style={{ padding: '14px 16px', borderBottom: '1px solid var(--cw-gray-200)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Eyebrow>Canary step rail</Eyebrow>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--cw-gray-700)' }}>{steps.length} steps</span>
      </div>
      <div style={{ padding: 8 }}>
        {steps.map((s) => (
          <StepRailItem key={s.id} step={s} index={stepIndex[s.id]} state={states[s.id]} selected={selected === s.id} onSelect={onSelect} />
        ))}
      </div>
    </Card>
  );
}

// ---- discovered facts strip -----------------------------------------------
function FactsCard({ facts }) {
  if (!facts) return null;
  const rows = [
    ['Instance type', facts.instance_type, true],
    ['SKU', facts.sku, true],
    ['Variant', facts.variant, true],
    ['Region', facts.region, false],
    ['Nodes', facts.nodes, false],
  ];
  return (
    <Card style={{ padding: 0, overflow: 'hidden' }}>
      <div style={{ padding: '12px 16px', borderBottom: '1px solid var(--cw-gray-200)', display: 'flex', alignItems: 'center', gap: 8 }}>
        <Icon path={icons.cpu} size={15} style={{ color: 'var(--cw-gray-700)' }} />
        <Eyebrow>Discovered facts</Eyebrow>
        <span style={{ marginLeft: 'auto', fontFamily: 'var(--font-mono)', fontSize: 10.5, color: 'var(--cw-gray-700)' }}>{facts.source}</span>
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)' }}>
        {rows.map(([label, value, mono], i) => (
          <div key={label} style={{ padding: '12px 16px', borderRight: i < rows.length - 1 ? '1px solid var(--cw-gray-200)' : 'none' }}>
            <div style={{ fontFamily: 'var(--font-ui)', fontSize: 10.5, fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', color: 'var(--cw-gray-700)', marginBottom: 5 }}>{label}</div>
            <div style={{ fontFamily: mono ? 'var(--font-mono)' : 'var(--font-sans)', fontSize: mono ? 12.5 : 14, color: 'var(--cw-gray-950)', fontWeight: 500 }}>{value}</div>
          </div>
        ))}
      </div>
    </Card>
  );
}

// ---- alert evidence panel (the live alert_probe output) -------------------
function AlertEvidence({ evidence }) {
  const racks = Object.entries(evidence.per_rack);
  return (
    <div>
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10, padding: '12px 16px', marginBottom: 16,
        background: evidence.alerts_clear ? 'var(--cw-green-50)' : 'var(--cw-yellow-50)',
        border: '1px solid ' + (evidence.alerts_clear ? 'var(--cw-green-300)' : 'var(--cw-yellow-300)'),
        borderRadius: 4,
      }}>
        <Icon path={evidence.alerts_clear ? icons.checkCircle : icons.alert} size={18} style={{ color: evidence.alerts_clear ? 'var(--cw-green)' : 'var(--cw-yellow-700)' }} />
        <div style={{ fontFamily: 'var(--font-sans)', fontSize: 13.5, color: 'var(--cw-gray-950)' }}>
          {evidence.alerts_clear
            ? 'All canary racks are clear of active alerts at baseline.'
            : <>Active alerts present at baseline — max severity <strong>{evidence.max_severity}</strong>. Captured as evidence; no verdict (gatherer probe).</>}
        </div>
        <span style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 6, fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--cw-gray-700)', whiteSpace: 'nowrap' }}>
          <Icon path={icons.clock} size={13} /> {fmtAbs(evidence.probed_at)}
        </span>
      </div>

      {racks.map(([rack, body]) => (
        <div key={rack} style={{ marginBottom: 18 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 8 }}>
            <Icon path={icons.rack} size={15} style={{ color: 'var(--cw-gray-700)' }} />
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 600, color: 'var(--cw-gray-950)' }}>{rack}</span>
            <SeverityPill severity={body.alerts.length === 0 ? 'clear' : body.max_severity} />
            <div style={{ marginLeft: 'auto', display: 'flex', gap: 6 }}>
              {['critical', 'warning', 'info'].map((sev) => (
                <span key={sev} style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: body.count_by_severity[sev] ? 'var(--cw-gray-950)' : 'var(--cw-gray-600)' }}>
                  {body.count_by_severity[sev]} {sev}
                </span>
              ))}
            </div>
          </div>
          {body.alerts.length === 0 ? (
            <div style={{ padding: '14px 16px', fontFamily: 'var(--font-sans)', fontSize: 13, color: 'var(--cw-gray-700)', background: 'var(--cw-gray-50)', border: '1px solid var(--cw-gray-200)', borderRadius: 4 }}>No active alerts.</div>
          ) : (
            <Card style={{ overflow: 'hidden' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ background: 'var(--cw-gray-100)' }}>
                    <th style={{ ...fwTh, padding: '8px 14px' }}>Alert</th>
                    <th style={{ ...fwTh, padding: '8px 14px' }}>Severity</th>
                    <th style={{ ...fwTh, padding: '8px 14px' }}>Node</th>
                    <th style={{ ...fwTh, padding: '8px 14px' }}>Summary</th>
                    <th style={{ ...fwTh, padding: '8px 14px' }}>For</th>
                  </tr>
                </thead>
                <tbody>
                  {body.alerts.map((a, i) => (
                    <tr key={i}>
                      <td style={{ ...fwTd, padding: '10px 14px', fontFamily: 'var(--font-mono)', fontSize: 12.5 }}>{a.name}</td>
                      <td style={{ ...fwTd, padding: '10px 14px' }}><SeverityPill severity={a.severity} /></td>
                      <td style={{ ...fwTd, padding: '10px 14px', fontFamily: 'var(--font-mono)', fontSize: 12 }}>{a.node}</td>
                      <td style={{ ...fwTd, padding: '10px 14px', color: 'var(--cw-gray-900)' }}>{a.summary}</td>
                      <td style={{ ...fwTd, padding: '10px 14px', fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--cw-gray-700)' }}>{a.for}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Card>
          )}
        </div>
      ))}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 4 }}>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--cw-gray-700)' }}>source: {evidence.source}</span>
        <button style={{ marginLeft: 'auto', display: 'inline-flex', alignItems: 'center', gap: 6, background: 'transparent', border: 0, cursor: 'pointer', fontFamily: 'var(--font-ui)', fontSize: 12, fontWeight: 600, color: 'var(--cw-blue-800)' }}>
          View raw JSON <Icon path={icons.externalLink} size={13} />
        </button>
      </div>
    </div>
  );
}

// ---- generic / designed step panel ----------------------------------------
function DesignedStepPanel({ step, state, onAdvance, canAdvance = false }) {
  return (
    <div>
      {!step.live && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '12px 16px', marginBottom: 16, background: 'var(--cw-gray-50)', border: '1px dashed var(--cw-gray-500)', borderRadius: 4 }}>
          <Icon path={icons.clock} size={17} style={{ color: 'var(--cw-gray-700)' }} />
          <div style={{ fontFamily: 'var(--font-sans)', fontSize: 13.5, color: 'var(--cw-gray-900)' }}>
            This step is <strong>designed in the template</strong> but not yet wired to execute (iter-4c+). The engine stops after the first live probe.
          </div>
        </div>
      )}
      {state === 'failed' && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '12px 16px', marginBottom: 16, background: 'var(--cw-red-50)', border: '1px solid var(--cw-red-300)', borderRadius: 4 }}>
          <Icon path={icons.xCircle} size={18} style={{ color: 'var(--cw-red)' }} />
          <div style={{ fontFamily: 'var(--font-sans)', fontSize: 13.5, color: 'var(--cw-gray-950)' }}>
            Step failed — <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5 }}>node-zap status=failed</span> on a canary rack. The run is a terminal failure.
          </div>
        </div>
      )}
      <div style={{ display: 'grid', gridTemplateColumns: 'max-content 1fr', gap: '10px 24px', fontFamily: 'var(--font-sans)', fontSize: 13.5 }}>
        <span style={{ color: 'var(--cw-gray-700)' }}>Kind</span>
        <span><Tag>{KIND_LABEL[step.kind]}</Tag></span>
        {step.tool && (<>
          <span style={{ color: 'var(--cw-gray-700)' }}>Tool</span>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13 }}>{step.tool}</span>
        </>)}
        {step.timeout && (<>
          <span style={{ color: 'var(--cw-gray-700)' }}>Timeout</span>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13 }}>{step.timeout}</span>
        </>)}
        <span style={{ color: 'var(--cw-gray-700)' }}>Description</span>
        <span style={{ color: 'var(--cw-gray-950)', maxWidth: 560, lineHeight: 1.55 }}>{step.desc}</span>
      </div>
      {canAdvance && (step.kind === 'review' || step.kind === 'ai_assess') && (
        <div style={{ marginTop: 18, paddingTop: 16, borderTop: '1px solid var(--cw-gray-200)', display: 'flex', alignItems: 'center', gap: 12 }}>
          <Button variant="primary" leftIcon={icons.check} onClick={() => onAdvance({ step_id: step.id })}>Advance</Button>
          <span style={{ fontFamily: 'var(--font-sans)', fontSize: 12.5, color: 'var(--cw-gray-700)' }}>
            Advance the run past this {KIND_LABEL[step.kind].toLowerCase()} gate.
          </span>
        </div>
      )}
    </div>
  );
}

// ---- sign-off panel -------------------------------------------------------
function SignoffField({ label, children }) {
  return (
    <div style={{ marginBottom: 14 }}>
      <label style={{ display: 'block', fontFamily: 'var(--font-ui)', fontSize: 12.5, fontWeight: 600, color: 'var(--cw-gray-950)', marginBottom: 6 }}>{label}</label>
      {children}
    </div>
  );
}

function SignoffPanel({ run, step, state, onSignoff }) {
  const [signerName, setSignerName] = _cwUseState('');
  const [signerRole, setSignerRole] = _cwUseState('');
  const [verdict, setVerdict] = _cwUseState('pass');

  if (run.status === 'signed_off') {
    return (
      <div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '14px 16px', marginBottom: 16, background: 'var(--cw-green-50)', border: '1px solid var(--cw-green-300)', borderRadius: 4 }}>
          <Icon path={icons.signature} size={18} style={{ color: 'var(--cw-green)' }} />
          <div style={{ fontFamily: 'var(--font-sans)', fontSize: 13.5, color: 'var(--cw-gray-950)' }}>
            Signed off <strong>pass</strong> by <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5 }}>{run.requester}</span> · reply posted to #havock.
          </div>
        </div>
        <div style={{ fontFamily: 'var(--font-sans)', fontSize: 13.5, color: 'var(--cw-gray-900)', lineHeight: 1.6, maxWidth: 620 }}>
          Final notes: All L10/L11/L12 stages completed clean on the single canary rack. Post-zap metrics delta within threshold. Recommending fleet-wide rollout of <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5 }}>{run.bundle}</span>.
        </div>
      </div>
    );
  }

  const nameOk = signerName.trim().length > 0;
  const roleOk = signerRole.trim().length > 0;
  const valid = nameOk && roleOk;

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '12px 16px', marginBottom: 16, background: 'var(--cw-gray-50)', border: '1px solid var(--cw-gray-200)', borderRadius: 4 }}>
        <Icon path={icons.signature} size={17} style={{ color: 'var(--cw-gray-700)' }} />
        <div style={{ fontFamily: 'var(--font-sans)', fontSize: 13.5, color: 'var(--cw-gray-900)' }}>
          Record the operator sign-off verdict for <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5 }}>{step.id}</span>.
        </div>
      </div>
      <div style={{ maxWidth: 420 }}>
        <SignoffField label="Signer name">
          <Input placeholder="k.osei" value={signerName} onChange={(e) => setSignerName(e.target.value)} style={{ fontFamily: 'var(--font-mono)', fontSize: 13 }} />
        </SignoffField>
        <SignoffField label="Signer role">
          <Input placeholder="fleet release engineer" value={signerRole} onChange={(e) => setSignerRole(e.target.value)} style={{ fontSize: 13 }} />
        </SignoffField>
        <SignoffField label="Verdict">
          <select value={verdict} onChange={(e) => setVerdict(e.target.value)}
            style={{ fontFamily: 'var(--font-sans)', fontSize: 13, height: 36, padding: '0 12px', border: '1px solid var(--cw-gray-400)', borderRadius: 'var(--input-radius)', background: 'var(--fw-surface)', color: 'var(--cw-gray-950)', outline: 'none', width: '100%', boxSizing: 'border-box', cursor: 'pointer' }}
            onFocus={(e) => { e.target.style.borderColor = 'var(--cw-blue)'; e.target.style.boxShadow = '0 0 0 2px var(--cw-blue-300)'; }}
            onBlur={(e) => { e.target.style.borderColor = 'var(--cw-gray-400)'; e.target.style.boxShadow = 'none'; }}>
            <option value="pass">pass</option>
            <option value="fail">fail</option>
          </select>
        </SignoffField>
        <Button variant="primary" leftIcon={icons.signature} disabled={!valid} style={!valid ? { opacity: 0.5, cursor: 'not-allowed' } : {}}
          onClick={() => { if (valid) onSignoff({ step_id: step.id, verdict, signer_name: signerName.trim(), signer_role: signerRole.trim() }); }}>
          Sign off
        </Button>
      </div>
    </div>
  );
}

// ---- right-hand step panel router -----------------------------------------
function StepPanel({ run, step, state, evidence, assessment, onSignoff, onAdvance }) {
  const isCurrent = run.currentStep === step.id;
  const vis = STEP_STATE_VIS[state] || STEP_STATE_VIS.pending;
  const stateLabel = { passed: 'Passed', running: 'Running', failed: 'Failed', warning: 'Warning', pending: 'Pending' }[state];
  return (
    <Card style={{ padding: 0, overflow: 'hidden' }}>
      <div style={{ padding: '16px 20px', borderBottom: '1px solid var(--cw-gray-200)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <span style={{ color: vis.color, display: 'flex', animation: state === 'running' ? 'fwspin 1s linear infinite' : 'none' }}>
            <Icon path={vis.icon} size={20} fill={state === 'pending' ? 'currentColor' : 'none'} />
          </span>
          <h2 style={{ margin: 0, fontFamily: 'var(--font-display)', fontWeight: 700, fontSize: 18, color: 'var(--cw-gray-950)' }}>{step.title}</h2>
          <span style={{ fontFamily: 'var(--font-ui)', fontSize: 11.5, fontWeight: 600, color: vis.color, marginLeft: 2 }}>{stateLabel}</span>
          <div style={{ marginLeft: 'auto' }}>
            {step.live
              ? <span style={{ fontFamily: 'var(--font-ui)', fontSize: 10, fontWeight: 700, letterSpacing: '0.05em', color: 'var(--status-success-fg)', background: 'var(--cw-green-100)', padding: '3px 8px', borderRadius: 3 }}>LIVE TODAY</span>
              : <span style={{ fontFamily: 'var(--font-ui)', fontSize: 10, fontWeight: 700, letterSpacing: '0.05em', color: 'var(--cw-gray-900)', background: 'var(--cw-gray-100)', padding: '3px 8px', borderRadius: 3 }}>DESIGNED</span>}
          </div>
        </div>
        <div style={{ marginTop: 6, fontFamily: 'var(--font-mono)', fontSize: 11.5, color: 'var(--cw-gray-700)' }}>
          {step.id}
        </div>
      </div>
      <div style={{ padding: 20 }}>
        {step.kind === 'signoff'
          ? <SignoffPanel run={run} step={step} state={state} onSignoff={onSignoff} />
          : (step.id === 'alert_baseline' && evidence && evidence.per_rack)
            ? <AlertEvidence evidence={evidence} />
            : (step.kind === 'ai_assess' && assessment)
              ? <AssessmentCard assessment={assessment} />
              : <DesignedStepPanel step={step} state={state} onAdvance={onAdvance} canAdvance={isCurrent} />}
      </div>
    </Card>
  );
}

// ---- event timeline -------------------------------------------------------
const EVENT_VIS = {
  RunCreated:      { color: 'var(--cw-blue)',  icon: icons.plus },
  FactsDiscovered: { color: 'var(--cw-gray-700)', icon: icons.cpu },
  StepStarted:     { color: 'var(--cw-blue)',  icon: icons.spinner },
  StepCompleted:   { color: 'var(--cw-green)', icon: icons.check },
  StepFailed:      { color: 'var(--cw-red)',   icon: icons.x },
  OperatorAction:  { color: 'var(--cw-green)', icon: icons.signature },
};
function Timeline({ events }) {
  return (
    <Card style={{ padding: 0, overflow: 'hidden' }}>
      <div style={{ padding: '14px 20px', borderBottom: '1px solid var(--cw-gray-200)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Eyebrow>Event log</Eyebrow>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--cw-gray-700)' }}>{events.length} events · source of truth</span>
      </div>
      <div style={{ padding: '8px 20px 16px' }}>
        {events.map((e, i) => {
          const vis = EVENT_VIS[e.kind] || EVENT_VIS.RunCreated;
          const last = i === events.length - 1;
          return (
            <div key={e.seq} style={{ display: 'flex', gap: 14, position: 'relative' }}>
              <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', flexShrink: 0 }}>
                <span style={{ width: 28, height: 28, borderRadius: '50%', background: 'var(--fw-surface)', border: '1.5px solid ' + vis.color, color: vis.color, display: 'flex', alignItems: 'center', justifyContent: 'center', marginTop: 8 }}>
                  <Icon path={vis.icon} size={14} />
                </span>
                {!last && <span style={{ width: 1.5, flex: 1, background: 'var(--cw-gray-300)', minHeight: 14 }} />}
              </div>
              <div style={{ paddingTop: 10, paddingBottom: last ? 0 : 10, flex: 1 }}>
                <div style={{ display: 'flex', alignItems: 'baseline', gap: 10 }}>
                  <span style={{ fontFamily: 'var(--font-ui)', fontSize: 13.5, fontWeight: 600, color: 'var(--cw-gray-950)' }}>{e.kind}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10.5, color: 'var(--cw-gray-600)' }}>seq {e.seq}</span>
                  <span style={{ marginLeft: 'auto', fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--cw-gray-700)' }}>{fmtAbs(e.occurred_at)}</span>
                </div>
                <div style={{ fontFamily: 'var(--font-sans)', fontSize: 13, color: 'var(--cw-gray-900)', marginTop: 3 }}>{e.detail}</div>
              </div>
            </div>
          );
        })}
      </div>
    </Card>
  );
}

// ---- run detail page ------------------------------------------------------
function RunDetail({
  run,
  events = [],
  facts,
  evidence,
  assessment = null,
  steps = [],
  stepIndex = {},
  states = {},
  templateVersion = 0,
  // eslint-disable-next-line no-unused-vars
  showRailGroups = false,
  onBack,
  onSignoff = () => {},
  onAdvance = () => {},
  onCancel = () => {},
}) {
  const defaultStep = run.currentStep || (steps[0] && steps[0].id) || 'alert_baseline';
  const [selected, setSelected] = _cwUseState(defaultStep);
  const step = steps.find((s) => s.id === selected) || steps[0] || { id: selected, kind: '', title: selected, tool: '', live: false, timeout: '', desc: '' };
  const signoffStep = steps.find((s) => s.kind === 'signoff');

  const metaRow = [
    ['Bundle', run.bundle, true],
    ['Template', `${run.template_id} v${templateVersion}`, true],
    ['Requester', run.requester, true],
    ['Created by', run.created_by, true],
    ['Created', fmtAbs(run.created_at), false],
  ];

  return (
    <div style={{ padding: '24px 32px 64px', background: 'var(--cw-bg-canvas, #FCFCFE)', flex: 1, overflow: 'auto' }}>
      <button onClick={onBack} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, background: 'transparent', border: 0, color: 'var(--cw-blue-800)', fontFamily: 'var(--font-ui)', fontSize: 13, fontWeight: 600, cursor: 'pointer', padding: 0, marginBottom: 16 }}>
        <Icon path={icons.arrowLeft} size={15} /> Canary Runs
      </button>

      {/* header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 16, marginBottom: 14 }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <h1 style={{ margin: 0, fontFamily: 'var(--font-display)', fontWeight: 700, fontSize: 26, color: 'var(--cw-gray-950)', letterSpacing: '-0.01em' }}>Canary run</h1>
            <StatusPill status={run.status} />
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 8 }}>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13, color: 'var(--cw-gray-900)' }}>{run.id}</span>
            <span style={{ display: 'flex', gap: 5 }}>{run.racks.map((rk) => <Tag key={rk} mono>{rk}</Tag>)}</span>
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8, flexShrink: 0 }}>
          <Button variant="secondary" leftIcon={icons.refresh} size="md">Refresh</Button>
          {(run.status === 'running' || run.status === 'pending') && (
            <Button variant="secondary" leftIcon={icons.xCircle} size="md" onClick={() => onCancel()}>Cancel run</Button>
          )}
          {(run.status === 'warning' || run.status === 'passed') && signoffStep && (
            <Button variant="primary" leftIcon={icons.signature} onClick={() => setSelected(signoffStep.id)}>Sign off</Button>
          )}
        </div>
      </div>

      {/* meta strip */}
      <Card style={{ padding: 0, overflow: 'hidden', marginBottom: 16 }}>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)' }}>
          {metaRow.map(([label, value, mono], i) => (
            <div key={label} style={{ padding: '12px 16px', borderRight: i < metaRow.length - 1 ? '1px solid var(--cw-gray-200)' : 'none' }}>
              <div style={{ fontFamily: 'var(--font-ui)', fontSize: 10.5, fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', color: 'var(--cw-gray-700)', marginBottom: 5 }}>{label}</div>
              <div style={{ fontFamily: mono ? 'var(--font-mono)' : 'var(--font-sans)', fontSize: mono ? 12.5 : 13.5, color: 'var(--cw-gray-950)', fontWeight: 500, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{value}</div>
            </div>
          ))}
        </div>
      </Card>

      <div style={{ marginBottom: 16 }}>
        <FactsCard facts={facts} />
      </div>

      {/* rail + panel */}
      <div style={{ display: 'grid', gridTemplateColumns: '320px 1fr', gap: 16, alignItems: 'start', marginBottom: 16 }}>
        <StepRail steps={steps} stepIndex={stepIndex} states={states} selected={selected} onSelect={setSelected} />
        <StepPanel run={run} step={step} state={states[selected]} evidence={evidence} assessment={assessment} onSignoff={onSignoff} onAdvance={onAdvance} />
      </div>

      <Timeline events={events} />
    </div>
  );
}


export default RunDetail;
