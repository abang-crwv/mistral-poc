// fwqual — New canary run modal. Two steps:
//   1. Details — operator supplies bundle_tag, canary_racks, requester, workflow.
//      instance_type / sku / variant are discovered from inventory, not asked.
//   2. Trays — one page per canary rack: the compute trays are enumerated from
//      inventory and the operator marks any tray to ignore before creating the
//      run. "Next" pages through the racks; "Create run" is on the last rack.
import { useState, useEffect } from 'react';
import { Icon, icons, Button, Input, Card, Tag } from '@/design/components';
import { useInventoryPreview } from '@/features/runs/useInventoryPreview';

const _cwUseState = useState;

const RACK_RE = /^[a-z]+\d+-r\d{3}-[a-z]+-[a-z]+-\d{2}[a-z]$/;
const key = (rack, ds) => `${rack}|${ds}`;

function NewRunModal({ open, onClose, onCreate, workflows = [] }) {
  const [bundle, setBundle] = _cwUseState('');
  const [racksText, setRacksText] = _cwUseState('');
  const [requester, setRequester] = _cwUseState('');
  const [workflow, setWorkflow] = _cwUseState('');
  const [touched, setTouched] = _cwUseState(false);
  const [step, setStep] = _cwUseState('details'); // 'details' | 'trays'
  const [ignored, setIgnored] = _cwUseState(() => new Set());

  // Fresh wizard each time the modal opens.
  useEffect(() => {
    if (open) {
      setStep('details');
      setIgnored(new Set());
      setTouched(false);
    }
  }, [open]);

  const racks = racksText.split(/[\s,]+/).map((r) => r.trim()).filter(Boolean);
  const badRacks = racks.filter((r) => !RACK_RE.test(r));
  const bundleOk = bundle.trim().length > 3;
  const racksOk = racks.length > 0 && badRacks.length === 0;
  const requesterOk = requester.trim().length > 0;
  const workflowOk = workflow.trim().length > 0;
  const detailsValid = bundleOk && racksOk && requesterOk && workflowOk;

  // Only resolve trays once we reach the trays step.
  const preview = useInventoryPreview(step === 'trays' ? racks : []);

  if (!open) return null;

  const toggleIgnore = (rack, ds) => {
    setIgnored((prev) => {
      const next = new Set(prev);
      const k = key(rack, ds);
      if (next.has(k)) next.delete(k);
      else next.add(k);
      return next;
    });
  };

  const submit = () => {
    const ignored_bmns = [...ignored].map((k) => {
      const [rack, deviceslot] = k.split('|');
      return { rack, deviceslot };
    });
    onCreate({ bundle: bundle.trim(), racks, requester: requester.trim(), rlcc_workflow: workflow, ignored_bmns });
  };

  const field = (label, hint, child) => (
    <div style={{ marginBottom: 18 }}>
      <label style={{ display: 'block', fontFamily: 'var(--font-ui)', fontSize: 13, fontWeight: 600, color: 'var(--cw-gray-950)', marginBottom: 6 }}>{label}</label>
      {child}
      {hint && <p style={{ margin: '6px 0 0', fontFamily: 'var(--font-sans)', fontSize: 12, color: 'var(--cw-gray-700)' }}>{hint}</p>}
    </div>
  );

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(33,31,38,0.5)', display: 'flex', alignItems: 'flex-start', justifyContent: 'center', zIndex: 100, padding: '64px 16px', overflow: 'auto' }}
      onClick={onClose}>
      <Card style={{ width: 560, maxWidth: '100%', boxShadow: 'var(--shadow-overlay, 0 12px 40px rgba(13,15,18,0.24))', borderRadius: 6 }} onClick={(e) => e.stopPropagation()}>
        <div style={{ padding: '18px 22px', borderBottom: '1px solid var(--cw-gray-200)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <div>
            <h2 style={{ margin: 0, fontFamily: 'var(--font-display)', fontWeight: 700, fontSize: 18, color: 'var(--cw-gray-950)' }}>New canary run</h2>
            <p style={{ margin: '4px 0 0', fontFamily: 'var(--font-sans)', fontSize: 13, color: 'var(--cw-gray-700)' }}>
              {step === 'details'
                ? <>Step 1 of 2 · Details · Template <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5 }}>firmware-prezap-baseline</span></>
                : <>Step 2 of 2 · Trays · {racks.length} rack{racks.length === 1 ? '' : 's'}</>}
            </p>
          </div>
          <button onClick={onClose} style={{ background: 'transparent', border: 0, cursor: 'pointer', color: 'var(--cw-gray-700)', display: 'flex', padding: 4 }}><Icon path={icons.x} size={20} /></button>
        </div>

        {step === 'details' ? (
          <div style={{ padding: 22 }}>
            {field('FW bundle tag', 'The firmware bundle under test, e.g. dell-xe9712-default-2.0.0-rc.',
              <Input placeholder="dell-xe9712-default-2.0.0-rc" value={bundle} onChange={(e) => setBundle(e.target.value)} style={{ fontFamily: 'var(--font-mono)', fontSize: 13 }} />
            )}

            {field('Canary racks', 'Comma- or space-separated. Format: dh{n}-r{nnn}-{region}-{nn}{a}.',
              <div>
                <textarea placeholder="dh3-r012-us-east-01a, dh3-r014-us-east-01a" value={racksText} onChange={(e) => setRacksText(e.target.value)}
                  rows={2} style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'var(--font-mono)', fontSize: 13, padding: '8px 12px', border: '1px solid ' + (touched && !racksOk ? 'var(--cw-red)' : 'var(--cw-gray-400)'), borderRadius: 'var(--input-radius)', resize: 'vertical', outline: 'none', color: 'var(--cw-gray-950)' }}
                  onFocus={(e) => { setTouched(true); e.target.style.boxShadow = '0 0 0 2px var(--cw-blue-300)'; }} onBlur={(e) => { e.target.style.boxShadow = 'none'; }} />
                {touched && badRacks.length > 0 && (
                  <p style={{ margin: '6px 0 0', fontFamily: 'var(--font-sans)', fontSize: 12, color: 'var(--cw-red-700)' }}>Invalid rack format: {badRacks.map((r) => <span key={r} style={{ fontFamily: 'var(--font-mono)' }}>{r}</span>).reduce((a, b) => [a, ', ', b])}</p>
                )}
              </div>
            )}

            {field('Requester', null,
              <Input placeholder="k.osei" value={requester} onChange={(e) => setRequester(e.target.value)} style={{ fontFamily: 'var(--font-mono)', fontSize: 13 }} />
            )}

            {field('RLCC Workflow', 'The RLCC bringup workflow whose L11/L12 actions the canary drives.',
              <select value={workflow} onChange={(e) => setWorkflow(e.target.value)}
                style={{ fontFamily: 'var(--font-mono)', fontSize: 13, height: 36, padding: '0 12px', border: '1px solid var(--cw-gray-400)', borderRadius: 'var(--input-radius)', background: 'var(--fw-surface)', color: workflow ? 'var(--cw-gray-950)' : 'var(--cw-gray-700)', outline: 'none', width: '100%', boxSizing: 'border-box', cursor: 'pointer' }}
                onFocus={(e) => { e.target.style.borderColor = 'var(--cw-blue)'; e.target.style.boxShadow = '0 0 0 2px var(--cw-blue-300)'; }}
                onBlur={(e) => { e.target.style.borderColor = 'var(--cw-gray-400)'; e.target.style.boxShadow = 'none'; }}>
                <option value="" disabled>Select a workflow…</option>
                {workflows.map((w) => <option key={w} value={w}>{w}</option>)}
              </select>
            )}

            <div style={{ display: 'flex', gap: 10, padding: '12px 14px', background: 'var(--cw-blue-100)', border: '1px solid var(--cw-blue-300)', borderRadius: 4 }}>
              <Icon path={icons.cpu} size={16} style={{ color: 'var(--cw-blue-800)', flexShrink: 0, marginTop: 1 }} />
              <p style={{ margin: 0, fontFamily: 'var(--font-sans)', fontSize: 12.5, color: 'var(--cw-gray-950)', lineHeight: 1.5 }}>
                Next, you'll review each rack's compute trays and choose any to ignore before the run is created.
              </p>
            </div>
          </div>
        ) : (
          <TraysStep racks={racks} preview={preview} ignored={ignored} onToggle={toggleIgnore} />
        )}

        <div style={{ padding: '14px 22px', borderTop: '1px solid var(--cw-gray-200)', display: 'flex', justifyContent: 'flex-end', gap: 10 }}>
          {step === 'details' ? (
            <>
              <Button variant="secondary" onClick={onClose}>Cancel</Button>
              <Button variant="primary" disabled={!detailsValid} style={!detailsValid ? { opacity: 0.5, cursor: 'not-allowed' } : {}}
                onClick={() => { if (detailsValid) setStep('trays'); }}>
                Next: review trays
              </Button>
            </>
          ) : (
            <>
              <Button variant="secondary" leftIcon={icons.arrowLeft} onClick={() => setStep('details')}>Back</Button>
              <Button variant="primary" onClick={submit}>Create run</Button>
            </>
          )}
        </div>
      </Card>
    </div>
  );
}

// TraysStep renders ALL canary racks on one scrollable page, each as a
// section listing its compute trays with an ignore toggle. RLCC-already-
// ignored trays are shown disabled (excluded upstream; not part of the
// operator ignore set).
function TraysStep({ racks, preview, ignored, onToggle }) {
  if (preview.isLoading) {
    return <div style={{ padding: 22 }}><p style={{ margin: 0, fontFamily: 'var(--font-sans)', fontSize: 13, color: 'var(--cw-gray-700)' }}>Resolving trays…</p></div>;
  }
  if (preview.isError) {
    return <div style={{ padding: 22 }}><p style={{ margin: 0, fontFamily: 'var(--font-sans)', fontSize: 13, color: 'var(--cw-red-700)' }}>Could not resolve inventory. You can still create the run; trays will be discovered server-side.</p></div>;
  }
  return (
    <div style={{ padding: 22, maxHeight: '60vh', overflow: 'auto' }}>
      {racks.map((rack, idx) => (
        <RackSection key={rack} rack={rack} preview={preview} ignored={ignored} onToggle={onToggle} first={idx === 0} />
      ))}
    </div>
  );
}

// RackSection is one rack's tray list within the all-racks trays page.
function RackSection({ rack, preview, ignored, onToggle, first }) {
  const pr = (preview.data?.racks ?? []).find((r) => r.rack === rack);
  const err = (preview.data?.errors ?? []).find((e) => e.rack === rack);
  const rlccIgnored = new Set((pr?.rlcc_ignored ?? []).map((b) => b.deviceslot));
  const bmns = [...(pr?.bmns ?? [])].sort((a, b) => (a.ct_position ?? 0) - (b.ct_position ?? 0));
  const ignoredCount = bmns.filter((b) => ignored.has(key(rack, b.deviceslot))).length;

  return (
    <div style={{ marginTop: first ? 0 : 22 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4, flexWrap: 'wrap' }}>
        <Tag mono>{rack}</Tag>
        {pr?.instance_type && <Tag>{pr.instance_type}</Tag>}
        {pr?.sku && <Tag>{pr.sku}</Tag>}
      </div>
      {err ? (
        <p style={{ margin: '10px 0 0', fontFamily: 'var(--font-sans)', fontSize: 13, color: 'var(--cw-red-700)' }}>
          {rack} did not resolve: {err.message}. It will be reported as unresolved when the run is created.
        </p>
      ) : (
        <>
          <p style={{ margin: '6px 0 12px', fontFamily: 'var(--font-sans)', fontSize: 12.5, color: 'var(--cw-gray-700)' }}>
            {bmns.length} compute trays · {ignoredCount} ignored. Click a tray to toggle.
          </p>
          <div style={{ border: '1px solid var(--cw-gray-200)', borderRadius: 4 }}>
            {bmns.map((b, i) => {
              const isRLCC = rlccIgnored.has(b.deviceslot);
              const isIgnored = ignored.has(key(rack, b.deviceslot));
              const dimmed = isRLCC || isIgnored;
              // Some nodes report no short BMN in inventory (resolver returns
              // empty). Show "—" rather than a blank cell.
              const bmnLabel = b.bmn_name || '—';
              return (
                <div key={b.deviceslot}
                  onClick={() => { if (!isRLCC) onToggle(rack, b.deviceslot); }}
                  style={{
                    display: 'flex', alignItems: 'center', gap: 10, padding: '8px 12px',
                    borderTop: i === 0 ? 'none' : '1px solid var(--cw-gray-200)',
                    cursor: isRLCC ? 'not-allowed' : 'pointer',
                    background: isIgnored ? 'var(--cw-gray-100)' : 'transparent',
                  }}>
                  <span style={{ width: 26, fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--cw-gray-700)', flexShrink: 0 }}>{b.ct_position ?? '—'}</span>
                  <span style={{ flex: 1, minWidth: 0, fontFamily: 'var(--font-mono)', fontSize: 12.5, color: dimmed ? 'var(--cw-gray-700)' : 'var(--cw-gray-950)', textDecoration: dimmed ? 'line-through' : 'none', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{b.deviceslot}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--cw-gray-700)', flexShrink: 0 }}>{bmnLabel}</span>
                  {isRLCC ? (
                    <span style={{ fontFamily: 'var(--font-ui)', fontSize: 10.5, fontWeight: 600, color: 'var(--cw-gray-700)', background: 'var(--cw-gray-100)', padding: '2px 7px', borderRadius: 3, flexShrink: 0 }}>RLCC-ignored</span>
                  ) : (
                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, fontFamily: 'var(--font-ui)', fontSize: 11, fontWeight: 600, color: isIgnored ? 'var(--cw-red-700)' : 'var(--cw-gray-700)', flexShrink: 0 }}>
                      <Icon path={isIgnored ? icons.xCircle : icons.dot} size={14} />{isIgnored ? 'Ignored' : 'Ignore'}
                    </span>
                  )}
                </div>
              );
            })}
          </div>
        </>
      )}
    </div>
  );
}

export default NewRunModal;
