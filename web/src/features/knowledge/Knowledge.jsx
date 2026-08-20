// fwqual — Knowledge.
// AI-indexed knowledge base: a plain-language Ask box grounded in the indexed
// corpus, a searchable domain Glossary, and an auto-maintained source Index.
// "Purely AI driven" — QA COW continuously indexes md runbooks, Confluence,
// and Jira; answers are synthesized by the model and cited back to sources.
import { useState } from 'react';
import { Icon, icons, Button, Input, Card, Tag, Eyebrow, fmtRel } from '@/design/components';

const _kbUseState = useState;

// ---- Glossary: the domain vocabulary that runs through fwqual ---------------
const GLOSSARY = [
  { term: 'Canary', cat: 'Process', def: 'A limited-blast-radius rollout: a firmware bundle is applied to a small set of racks and verified before fleet-wide rollout. The fw-release-canary template drives it.' },
  { term: 'Zap', cat: 'Process', def: 'The firmware flash/apply operation inside a canary step. Disruptive by design — racks reboot, NVLink flaps, and thermal alerts trip while a zap is in flight.' },
  { term: 'Bundle', cat: 'Firmware', def: 'A versioned firmware package (e.g. dell-xe9712-default-2.0.0-rc) qualified through the canary lifecycle: candidate → in canary → qualified → rollout → live.' },
  { term: 'XID', cat: 'Diagnostics', def: 'An NVIDIA GPU error code surfaced by the driver, identifying a class of GPU fault. XID 79 = GPU fell off the bus; XID 48 = double-bit (uncorrectable) ECC; XID 63/64 = ECC page retirement.' },
  { term: 'NVLink', abbr: 'NVL', cat: 'Hardware', def: 'NVIDIA high-bandwidth GPU-to-GPU interconnect. An NVL72 rack wires 72 GPUs into a single NVLink domain via the backend fabric; a missing link leaves the domain incomplete.' },
  { term: 'NVL72', abbr: 'GB200-NVL72', cat: 'Hardware', def: 'The Dell XE9712 rack SKU: 72 Blackwell GPUs in one NVLink domain. fwqual qualifies firmware against gb200-4x / gb300-4x instances on this SKU.' },
  { term: 'L10 / L11 / L12', cat: 'Diagnostics', def: 'Manufacturing and field diagnostic test levels. L11 FieldDiag is the rack-level field diagnostic; L12 covers backend-fabric / spine connectivity. A canary run gates on these passing.' },
  { term: 'FieldDiag', cat: 'Diagnostics', def: 'Dell field diagnostics run at the rack level (L11). Checks NVLink domain completeness, GPU presence, and link training before a rack is cleared.' },
  { term: 'CDU', abbr: 'Coolant Distribution Unit', cat: 'Hardware', def: 'The liquid-cooling manifold for a rack. Inlet/outlet temps and flow are watched on the Rack Power & Cooling dashboard during a zap.' },
  { term: 'PDU', abbr: 'Power Distribution Unit', cat: 'Hardware', def: 'Per-rack power feed. PDU draw is a leading indicator of a stuck or looping zap.' },
  { term: 'SKU', cat: 'Hardware', def: 'Stock-keeping unit — the hardware identifier for a rack/instance, e.g. XE9712-GB200-NVL72. Bundles are scoped to a SKU + instance type.' },
  { term: 'Silence', cat: 'Alerting', def: 'A maintenance window that suppresses PagerDuty alerts for a rack during known-disruptive QA. Keeps FRO on-call from being paged for expected zap noise.' },
  { term: 'Probe', cat: 'Process', def: 'A verification check wired into a template step. alert_probe reads Alertmanager per rack and folds max severity + counts into run state as evidence.' },
  { term: 'FRO', abbr: 'Field Reliability Operations', cat: 'Alerting', def: 'The org that owns the physical fleet and the GPU Fleet on-call rotation. fwqual silences route to the FRO On-Call PagerDuty service.' },
  { term: 'ECC', abbr: 'Error-Correcting Code', cat: 'Diagnostics', def: 'GPU memory error correction. Single-bit errors are corrected; a double-bit (uncorrectable) ECC event raises XID 48 and usually means an RMA.' },
  { term: 'RMA', abbr: 'Return Merchandise Authorization', cat: 'Process', def: 'The hardware-replacement workflow for a failed GPU/tray. Tracked in the Node Tracker repair lifecycle (triage → diagnosing → RMA pending → repair).' },
  { term: 'ULID', cat: 'Process', def: 'The 26-char lexicographically-sortable run identifier (01HZX8K3QJ7P2M5N8R4T6V9W2C). Every canary run, event, and piece of evidence keys off it.' },
  { term: 'Backend fabric', cat: 'Hardware', def: 'The NVLink switch fabric (spines + trays) inside a rack. An L12 misconnect — e.g. tray ↔ spine miswire — leaves the NVLink domain incomplete.' },
];

const CATS = ['All', 'Hardware', 'Firmware', 'Diagnostics', 'Process', 'Alerting'];
const CAT_TINT = {
  Hardware:    { bg: 'var(--cw-blue-200)',   fg: 'var(--cw-blue-950)' },
  Firmware:    { bg: 'var(--cw-green-100)',  fg: 'var(--status-success-fg)' },
  Diagnostics: { bg: 'var(--cw-yellow-100)', fg: 'var(--status-warning-fg)' },
  Process:     { bg: 'var(--cw-gray-100)',   fg: 'var(--cw-gray-900)' },
  Alerting:    { bg: 'var(--cw-red-100)',    fg: 'var(--status-danger-fg)' },
};

// ---- Indexed sources (auto-maintained by QA COW) ----------------------------
const KB_SOURCES = [
  { id: 'fw-canary-runbook',     title: 'fw-release-canary runbook',     type: 'Markdown',   chunks: 48, updated: '2026-05-30T12:10:00Z', status: 'indexed' },
  { id: 'xid-reference',         title: 'NVIDIA XID error reference',     type: 'Markdown',   chunks: 36, updated: '2026-05-29T08:00:00Z', status: 'indexed' },
  { id: 'l11-fielddiag',         title: 'L11 FieldDiag playbook',         type: 'Markdown',   chunks: 22, updated: '2026-05-30T09:42:00Z', status: 'indexed' },
  { id: 'nvlink-domain-repair',  title: 'NVLink domain repair guide',     type: 'Confluence', chunks: 31, updated: '2026-05-28T16:20:00Z', status: 'indexed' },
  { id: 'silence-policy',        title: 'Silence & on-call policy',       type: 'Confluence', chunks: 14, updated: '2026-05-30T11:05:00Z', status: 'indexed' },
  { id: 'bundle-lifecycle',      title: 'Bundle qualification lifecycle', type: 'Markdown',   chunks: 19, updated: '2026-05-30T13:55:00Z', status: 'indexing' },
  { id: 'rma-process',           title: 'GPU RMA process',                type: 'Confluence', chunks: 17, updated: '2026-05-27T10:30:00Z', status: 'indexed' },
  { id: 'cdu-coolant-ops',       title: 'CDU & coolant operations',       type: 'Markdown',   chunks: 12, updated: '2026-05-26T14:00:00Z', status: 'indexed' },
  { id: 'fwqa-jira',             title: 'FWQA board (issues + comments)',  type: 'Jira',       chunks: 64, updated: '2026-05-30T14:18:00Z', status: 'indexed' },
];

const TYPE_TINT = {
  Markdown:   { bg: 'var(--cw-gray-100)',  fg: 'var(--cw-gray-900)' },
  Confluence: { bg: 'var(--cw-blue-200)',  fg: 'var(--cw-blue-950)' },
  Jira:       { bg: 'var(--cw-blue-100)',  fg: 'var(--cw-blue-900)' },
};

const SUGGESTED = [
  'What does XID 79 mean?',
  'Why do I silence alerts during a zap?',
  'What does L11 FieldDiag check?',
  'When is a bundle considered qualified?',
];

// ---- tiny retrieval: rank sources/terms by token overlap --------------------
const STOP = new Set(['the','a','an','is','are','do','i','to','of','for','what','why','when','how','does','during','in','on','it','and','my','be','should']);
function tokenize(s) { return (s.toLowerCase().match(/[a-z0-9]+/g) || []).filter((w) => w.length > 1 && !STOP.has(w)); }
function rankSources(q) {
  const qt = tokenize(q);
  return KB_SOURCES
    .map((d) => { const hay = tokenize(d.title + ' ' + d.type); const score = qt.reduce((n, w) => n + (hay.some((h) => h.includes(w) || w.includes(h)) ? 1 : 0), 0); return { d, score }; })
    .filter((x) => x.score > 0).sort((a, b) => b.score - a.score).slice(0, 4).map((x) => x.d);
}
function bestTerm(q) {
  const qt = tokenize(q);
  let best = null, bestScore = 0;
  for (const t of GLOSSARY) {
    const hay = tokenize(t.term + ' ' + (t.abbr || '') + ' ' + t.def);
    const score = qt.reduce((n, w) => n + (hay.some((h) => h === w) ? 2 : hay.some((h) => h.includes(w)) ? 1 : 0), 0);
    if (score > bestScore) { bestScore = score; best = t; }
  }
  return bestScore >= 2 ? best : null;
}

// light inline markdown → HTML (bold + code), with escaping
function renderMd(text) {
  const esc = text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  return esc
    .replace(/\*\*([^*]+)\*\*/g, '<strong style="font-weight:600;color:var(--cw-gray-950)">$1</strong>')
    .replace(/`([^`]+)`/g, '<code style="font-family:var(--font-mono);font-size:0.92em;background:var(--cw-gray-100);padding:1px 5px;border-radius:3px">$1</code>');
}

function SourceChip({ d, onClick }) {
  const t = TYPE_TINT[d.type] || TYPE_TINT.Markdown;
  return (
    <button onClick={onClick} title={'Open ' + d.title} style={{
      display: 'inline-flex', alignItems: 'center', gap: 6, cursor: 'pointer',
      fontFamily: 'var(--font-ui)', fontSize: 11.5, fontWeight: 600, height: 24, padding: '0 9px',
      borderRadius: 4, border: '1px solid var(--cw-gray-400)', background: 'var(--fw-surface)', color: 'var(--cw-gray-900)',
    }}
      onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--cw-blue)'; e.currentTarget.style.background = 'var(--cw-gray-50)'; }}
      onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--cw-gray-400)'; e.currentTarget.style.background = 'var(--fw-surface)'; }}>
      <span style={{ width: 6, height: 6, borderRadius: 1.5, background: t.bg, border: '1px solid ' + t.fg, flexShrink: 0 }} />
      {d.title}
    </button>
  );
}

function AskPanel({ onCite }) {
  const [q, setQ] = _kbUseState('');
  const [loading, setLoading] = _kbUseState(false);
  const [ans, setAns] = _kbUseState(null);

  const submit = async (question) => {
    const text = (question != null ? question : q).trim();
    if (!text || loading) return;
    if (question != null) setQ(question);
    setLoading(true); setAns(null);
    const sources = rankSources(text);
    const grounding = GLOSSARY.map((t) => `${t.term}${t.abbr ? ' (' + t.abbr + ')' : ''} [${t.cat}]: ${t.def}`).join('\n');
    const srcList = KB_SOURCES.map((d) => `- ${d.title} (${d.type})`).join('\n');
    const complete = (typeof window !== 'undefined' && window.claude && window.claude.complete) || null;
    if (!complete) {
      setAns({ text: 'Model not connected. The knowledge base (glossary and sources) is shown above; live Q&A is wired in a later iteration.', sources, offline: true });
      setLoading(false);
      return;
    }
    try {
      const reply = await complete({
        messages: [{
          role: 'user',
          content:
            'You are the knowledge base for fwqual, a GPU firmware-qualification tool. Answer the operator\'s question using ONLY the indexed knowledge below. ' +
            'Be concise (2–4 sentences), technical, in the voice of an SRE runbook. Use **bold** for key terms and `code` for identifiers. ' +
            'If the answer is not in the knowledge, say so plainly.\n\n' +
            'GLOSSARY:\n' + grounding + '\n\nINDEXED SOURCES:\n' + srcList + '\n\nQuestion: ' + text,
        }],
      });
      setAns({ text: reply.trim(), sources });
    } catch (e) {
      const t = bestTerm(text);
      const fb = t
        ? `**${t.term}** — ${t.def}`
        : 'The live model is unavailable, so this answer is drawn straight from the index. No strongly-matching entry was found — try one of the suggested questions, or browse the glossary below.';
      setAns({ text: fb, sources, offline: true });
    } finally { setLoading(false); }
  };

  return (
    <Card style={{ padding: 0, overflow: 'hidden', maxWidth: 960 }}>
      <div style={{ padding: '18px 20px 16px' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
          <span style={{ color: 'var(--cw-blue)', display: 'flex' }}><Icon path={icons.sparkles} size={17} /></span>
          <span style={{ fontFamily: 'var(--font-ui)', fontSize: 13.5, fontWeight: 700, color: 'var(--cw-gray-950)' }}>Ask the knowledge base</span>
          <span style={{ marginLeft: 'auto', fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--cw-gray-600)' }}>{KB_SOURCES.length} sources indexed</span>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <div style={{ flex: 1 }}>
            <Input icon={icons.search} placeholder="Ask in plain language — e.g. what does XID 79 mean?"
              value={q} onChange={(e) => setQ(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') submit(); }}
              style={{ height: 42, fontSize: 14.5 }} />
          </div>
          <Button size="lg" leftIcon={icons.sparkles} onClick={() => submit()} disabled={loading || !q.trim()}
            style={{ opacity: loading || !q.trim() ? 0.55 : 1, cursor: loading || !q.trim() ? 'default' : 'pointer' }}>
            {loading ? 'Thinking…' : 'Ask'}
          </Button>
        </div>
        <div style={{ display: 'flex', gap: 7, marginTop: 12, flexWrap: 'wrap' }}>
          {SUGGESTED.map((s) => (
            <button key={s} onClick={() => submit(s)} disabled={loading} style={{
              fontFamily: 'var(--font-ui)', fontSize: 12, fontWeight: 500, height: 26, padding: '0 10px',
              borderRadius: 13, cursor: loading ? 'default' : 'pointer', border: '1px solid var(--cw-gray-400)',
              background: 'var(--cw-gray-50)', color: 'var(--cw-gray-800)',
            }}
              onMouseEnter={(e) => { if (!loading) e.currentTarget.style.borderColor = 'var(--cw-blue)'; }}
              onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--cw-gray-400)'; }}>{s}</button>
          ))}
        </div>
      </div>

      {(loading || ans) && (
        <div style={{ borderTop: '1px solid var(--cw-gray-300)', background: 'var(--cw-gray-50)', padding: '16px 20px 18px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
            <Eyebrow>Answer</Eyebrow>
            {ans && ans.offline && <Tag style={{ fontSize: 10, padding: '1px 6px', background: 'var(--cw-yellow-100)', color: 'var(--status-warning-fg)' }}>from index</Tag>}
            {ans && !ans.offline && <Tag style={{ fontSize: 10, padding: '1px 6px', background: 'var(--cw-blue-200)', color: 'var(--cw-blue-950)' }}>AI synthesized</Tag>}
          </div>
          {loading ? (
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, color: 'var(--cw-gray-700)', fontFamily: 'var(--font-sans)', fontSize: 14 }}>
              <span style={{ display: 'flex', animation: 'kbspin 0.9s linear infinite' }}><Icon path={icons.spinner} size={16} /></span>
              Searching the index and synthesizing…
            </div>
          ) : (
            <>
              <div style={{ fontFamily: 'var(--font-sans)', fontSize: 14.5, lineHeight: 1.6, color: 'var(--cw-gray-900)' }}
                dangerouslySetInnerHTML={{ __html: renderMd(ans.text).replace(/\n+/g, '<br/>') }} />
              {ans.sources.length > 0 && (
                <div style={{ marginTop: 14, paddingTop: 12, borderTop: '1px solid var(--cw-gray-300)' }}>
                  <div style={{ fontFamily: 'var(--font-ui)', fontSize: 11, fontWeight: 600, color: 'var(--cw-gray-700)', marginBottom: 8 }}>Sources consulted</div>
                  <div style={{ display: 'flex', gap: 7, flexWrap: 'wrap' }}>
                    {ans.sources.map((d) => <SourceChip key={d.id} d={d} onClick={() => onCite && onCite(d.id)} />)}
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </Card>
  );
}

function GlossaryList() {
  const [cat, setCat] = _kbUseState('All');
  const [query, setQuery] = _kbUseState('');
  const qt = query.trim().toLowerCase();
  const visible = GLOSSARY.filter((t) =>
    (cat === 'All' || t.cat === cat) &&
    (!qt || (t.term + ' ' + (t.abbr || '') + ' ' + t.def).toLowerCase().includes(qt))
  ).sort((a, b) => a.term.localeCompare(b.term));

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 14, flexWrap: 'wrap' }}>
        <h2 style={{ margin: 0, fontFamily: 'var(--font-display)', fontWeight: 700, fontSize: 17, color: 'var(--cw-gray-950)' }}>Glossary</h2>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11.5, color: 'var(--cw-gray-600)' }}>{visible.length} terms</span>
        <div style={{ marginLeft: 'auto', width: 220 }}>
          <Input icon={icons.search} placeholder="Filter terms" value={query} onChange={(e) => setQuery(e.target.value)} style={{ height: 32, fontSize: 13 }} />
        </div>
      </div>
      <div style={{ display: 'flex', gap: 6, marginBottom: 14, flexWrap: 'wrap' }}>
        {CATS.map((c) => {
          const active = cat === c;
          return (
            <button key={c} onClick={() => setCat(c)} style={{
              fontFamily: 'var(--font-ui)', fontSize: 12, fontWeight: 600, height: 28, padding: '0 11px',
              borderRadius: 4, cursor: 'pointer', border: '1px solid ' + (active ? 'var(--cw-blue)' : 'var(--cw-gray-400)'),
              background: active ? 'var(--cw-blue-200)' : 'var(--fw-surface)', color: active ? 'var(--cw-blue-800)' : 'var(--cw-gray-900)',
            }}>{c}</button>
          );
        })}
      </div>
      <Card style={{ overflow: 'hidden' }}>
        {visible.map((t, i) => {
          const tint = CAT_TINT[t.cat] || CAT_TINT.Process;
          return (
            <div key={t.term} style={{ padding: '14px 16px', borderTop: i ? '1px solid var(--cw-gray-200)' : 'none' }}>
              <div style={{ display: 'flex', alignItems: 'baseline', gap: 9, flexWrap: 'wrap', marginBottom: 5 }}>
                <span style={{ fontFamily: 'var(--font-sans)', fontSize: 15, fontWeight: 700, color: 'var(--cw-gray-950)' }}>{t.term}</span>
                {t.abbr && <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--cw-gray-700)' }}>{t.abbr}</span>}
                <Tag style={{ marginLeft: 'auto', fontSize: 10.5, padding: '1px 8px', background: tint.bg, color: tint.fg }}>{t.cat}</Tag>
              </div>
              <p style={{ margin: 0, fontFamily: 'var(--font-sans)', fontSize: 13.5, lineHeight: 1.55, color: 'var(--cw-gray-700)' }}>{t.def}</p>
            </div>
          );
        })}
        {visible.length === 0 && (
          <div style={{ padding: '28px 16px', textAlign: 'center', fontFamily: 'var(--font-sans)', fontSize: 13.5, color: 'var(--cw-gray-600)' }}>No terms match “{query}”.</div>
        )}
      </Card>
    </div>
  );
}

function IndexRail({ highlight }) {
  const [reindexing, setReindexing] = _kbUseState(false);
  const totalChunks = KB_SOURCES.reduce((n, d) => n + d.chunks, 0);
  const reindex = () => { setReindexing(true); setTimeout(() => setReindexing(false), 1600); };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <Card style={{ padding: '16px' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
          <span style={{ color: 'var(--cw-blue)', display: 'flex' }}><Icon path={icons.database} size={16} /></span>
          <span style={{ fontFamily: 'var(--font-ui)', fontSize: 13, fontWeight: 700, color: 'var(--cw-gray-950)' }}>Index</span>
        </div>
        <div style={{ display: 'flex', gap: 18, margin: '12px 0 4px' }}>
          <div>
            <div style={{ fontFamily: 'var(--font-sans)', fontSize: 22, fontWeight: 700, color: 'var(--cw-gray-950)', lineHeight: 1 }}>{KB_SOURCES.length}</div>
            <div style={{ fontFamily: 'var(--font-ui)', fontSize: 11, color: 'var(--cw-gray-700)', marginTop: 3 }}>sources</div>
          </div>
          <div>
            <div style={{ fontFamily: 'var(--font-sans)', fontSize: 22, fontWeight: 700, color: 'var(--cw-gray-950)', lineHeight: 1 }}>{totalChunks}</div>
            <div style={{ fontFamily: 'var(--font-ui)', fontSize: 11, color: 'var(--cw-gray-700)', marginTop: 3 }}>chunks</div>
          </div>
        </div>
        <p style={{ margin: '10px 0 0', fontFamily: 'var(--font-sans)', fontSize: 12, lineHeight: 1.5, color: 'var(--cw-gray-700)' }}>
          Auto-indexed by QA COW · embeddings <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11.5 }}>bge-large-en</span>
        </p>
        <div style={{ marginTop: 12 }}>
          <Button variant="secondary" size="sm" leftIcon={icons.refresh} onClick={reindex} disabled={reindexing}
            style={{ width: '100%', opacity: reindexing ? 0.6 : 1 }}>{reindexing ? 'Reindexing…' : 'Reindex now'}</Button>
        </div>
      </Card>

      <Card style={{ overflow: 'hidden' }}>
        <div style={{ padding: '12px 14px', borderBottom: '1px solid var(--cw-gray-200)' }}>
          <Eyebrow>Sources</Eyebrow>
        </div>
        {KB_SOURCES.map((d, i) => {
          const t = TYPE_TINT[d.type] || TYPE_TINT.Markdown;
          const on = highlight === d.id;
          return (
            <div key={d.id} style={{
              padding: '11px 14px', borderTop: i ? '1px solid var(--cw-gray-200)' : 'none',
              background: on ? 'var(--cw-blue-100)' : 'transparent', transition: 'background 200ms',
            }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', gap: 9 }}>
                <span style={{ color: 'var(--cw-gray-600)', display: 'flex', marginTop: 1, flexShrink: 0 }}>
                  <Icon path={d.type === 'Jira' ? icons.bug : d.type === 'Confluence' ? icons.book : icons.template} size={15} />
                </span>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontFamily: 'var(--font-sans)', fontSize: 13, fontWeight: 600, color: 'var(--cw-gray-950)' }}>{d.title}</div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 7, marginTop: 4 }}>
                    <Tag style={{ fontSize: 10, padding: '0 6px', background: t.bg, color: t.fg }}>{d.type}</Tag>
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10.5, color: 'var(--cw-gray-600)' }}>{d.chunks} chunks</span>
                    {d.status === 'indexing'
                      ? <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, marginLeft: 'auto', fontFamily: 'var(--font-ui)', fontSize: 10.5, fontWeight: 600, color: 'var(--cw-blue-800)' }}><span style={{ width: 6, height: 6, borderRadius: '50%', background: 'var(--cw-blue)' }} />indexing</span>
                      : <span style={{ marginLeft: 'auto', fontFamily: 'var(--font-ui)', fontSize: 10.5, color: 'var(--cw-gray-600)' }}>{fmtRel(d.updated)}</span>}
                  </div>
                </div>
              </div>
            </div>
          );
        })}
      </Card>
    </div>
  );
}

function Knowledge() {
  const [highlight, setHighlight] = _kbUseState(null);
  const cite = (id) => { setHighlight(id); setTimeout(() => setHighlight((h) => (h === id ? null : h)), 2400); };

  return (
    <div style={{ padding: '28px 32px 64px', background: 'var(--cw-bg-canvas, #FCFCFE)', flex: 1, overflow: 'auto' }}>
      <style>{`@keyframes kbspin { to { transform: rotate(360deg); } }`}</style>
      <div style={{ marginBottom: 22 }}>
        <h1 style={{ margin: 0, fontFamily: 'var(--font-display)', fontWeight: 700, fontSize: 28, color: 'var(--cw-gray-950)', letterSpacing: '-0.01em' }}>Knowledge</h1>
        <p style={{ margin: '6px 0 0', fontFamily: 'var(--font-sans)', fontSize: 14, color: 'var(--cw-gray-900)', maxWidth: 720, lineHeight: 1.5 }}>
          An AI-indexed glossary, runbooks, and docs. Ask in plain language — answers are synthesized from the indexed corpus and cited back to their sources.
        </p>
      </div>

      <AskPanel onCite={cite} />

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) 320px', gap: 22, marginTop: 26, alignItems: 'start' }}>
        <GlossaryList />
        <IndexRail highlight={highlight} />
      </div>
    </div>
  );
}


export default Knowledge;
