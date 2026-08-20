// fwqual — seed data. Mirrors the real domain: fw-release-canary template,
// event-sourced runs, alert_probe per-rack evidence, discovered facts.

// ---- Template step model (fw-release-canary v3, 13 steps) -----------------
// `live` = wired and executing today (iter-4b). `designed` = in template, not yet wired.
export const TEMPLATE = {
  id: 'fw-release-canary',
  version: 3,
  title: 'FW release canary',
  scope: 'rack',
  instanceTypes: ['gb200-4x', 'gb300-4x'],
};

export const STEP_GROUPS = [
  {
    id: 'baseline_pre_group', label: 'Baseline',
    steps: [
      { id: 'alert_baseline', title: 'Pre-zap alert baseline', kind: 'probe', tool: 'alert_probe', live: true, timeout: '30s', desc: 'Sweeps Alertmanager for active alerts on each canary rack and rolls them up per-rack. Gatherer — no verdict.' },
      { id: 'baseline_pre', title: 'Pre-zap health snapshot', kind: 'probe', tool: 'metrics', live: false, timeout: '5m', desc: 'Captures a metrics baseline per rack. success_when stdout contains baseline_ok.' },
    ],
  },
  {
    id: 'zap_group', label: 'Zap',
    steps: [
      { id: 'zap_canary', title: 'Zap canary rack(s)', kind: 'probe', tool: 'flcc', live: false, timeout: '60m', desc: 'FLCC node-zap workflow per rack, parallel. success_when status=completed; ActionRequired on halted.' },
    ],
  },
  {
    id: 'l10_group', label: 'L10 — node',
    steps: [
      { id: 'l10_fielddiag', title: 'L10 fielddiag', kind: 'probe', tool: 'flcc', live: false, timeout: '18h', desc: 'NVIDIA single-tray GPU screening. success_when state.completed=true.' },
      { id: 'l10_check', title: 'L10 test — HPC verification', kind: 'probe', tool: 'flcc', live: false, timeout: '9h', desc: 'Node-level HPC verification, includes alerts.' },
    ],
  },
  {
    id: 'l11_group', label: 'L11 — rack',
    steps: [
      { id: 'l11_fielddiag', title: 'L11 fielddiag', kind: 'probe', tool: 'rlcc', live: false, timeout: '4h', desc: 'Whole-rack NVLink-domain diagnostic.' },
      { id: 'l11_check', title: 'L11 test — NVLink group', kind: 'probe', tool: 'rlcc', live: false, timeout: '6h', desc: 'NVLink-domain group test, includes alerts.' },
    ],
  },
  {
    id: 'l12_group', label: 'L12 — fabric',
    steps: [
      { id: 'l12_seatrial', title: 'L12 seatrial', kind: 'probe', tool: 'rlcc', live: false, timeout: '1h', desc: 'Backend misconnect screen.' },
      { id: 'l12_check', title: 'L12 test — IB/RoCE fabric', kind: 'probe', tool: 'rlcc', live: false, timeout: '6h', desc: 'Backend IB/RoCE fabric performance.' },
    ],
  },
  {
    id: 'post_group', label: 'Post',
    steps: [
      { id: 'baseline_post', title: 'Post-zap delta vs baseline', kind: 'probe', tool: 'metrics', live: false, timeout: '5m', desc: 'Post-zap delta against baseline_pre. success_when delta_within_threshold.' },
    ],
  },
  {
    id: 'signoff_group', label: 'Review & sign-off',
    steps: [
      { id: 'evidence_review', title: 'Per-rack evidence review', kind: 'review', tool: null, live: false, desc: 'Operator reviews evidence across all racks, enriched with pattern + Glean context.' },
      { id: 'ai_summary', title: 'AI assessment', kind: 'ai_assess', tool: null, live: false, desc: 'Drafts a pass / fail / needs_review assessment citing patterns and Glean.' },
      { id: 'operator_signoff', title: 'Operator sign-off', kind: 'signoff', tool: null, live: false, desc: 'Final verdict (pass / fail), signer, and Slack reply to #havock.' },
    ],
  },
];

export const FLAT_STEPS = STEP_GROUPS.flatMap((g) => g.steps);
export const STEP_INDEX = Object.fromEntries(FLAT_STEPS.map((s, i) => [s.id, i]));

// ---- Runs (the runs-table projection) -------------------------------------
export const RUNS = [
  {
    id: '01HZX8K3QJ7P2M5N8R4T6V9W2C',
    bundle: 'dell-xe9712-default-2.0.0-rc',
    template_id: 'fw-release-canary',
    status: 'running',
    racks: ['dh3-r012-us-east-01a', 'dh3-r014-us-east-01a'],
    requester: 'k.osei',
    created_by: 'wpena',
    created_at: '2026-05-30T14:21:08Z',
    updated_at: '2026-05-30T14:22:55Z',
    currentStep: 'alert_baseline',
  },
  {
    id: '01HZWQ2F8M4N6P9R3T5V7X1Z0B',
    bundle: 'dell-xe9712-default-1.9.4',
    template_id: 'fw-release-canary',
    status: 'signed_off',
    racks: ['dh5-r007-us-west-02b'],
    requester: 'm.tran',
    created_by: 'wpena',
    created_at: '2026-05-28T09:02:41Z',
    updated_at: '2026-05-29T17:48:12Z',
    currentStep: 'operator_signoff',
    verdict: 'pass',
  },
  {
    id: '01HZW0A5C7E9G2J4L6N8Q1S3U',
    bundle: 'dell-xe9680-default-3.1.0-rc',
    template_id: 'fw-release-canary',
    status: 'failed',
    racks: ['dh2-r003-us-east-02a', 'dh2-r005-us-east-02a'],
    requester: 'k.osei',
    created_by: 'wpena',
    created_at: '2026-05-27T11:14:22Z',
    updated_at: '2026-05-27T13:55:09Z',
    currentStep: 'zap_canary',
  },
  {
    id: '01HZV6T8W0Y2A4C6E8G1J3L5N',
    bundle: 'hgx-h200-fw-2026-05-canary-2',
    template_id: 'fw-release-canary',
    status: 'warning',
    racks: ['dh4-r021-eu-west-01a'],
    requester: 's.alvarez',
    created_by: 'wpena',
    created_at: '2026-05-26T16:40:55Z',
    updated_at: '2026-05-27T08:11:30Z',
    currentStep: 'evidence_review',
  },
  {
    id: '01HZU9P1R3T5V7X9Z2B4D6F8H',
    bundle: 'dell-xe9712-default-2.0.0-rc',
    template_id: 'fw-release-canary',
    status: 'pending',
    racks: ['dh6-r009-us-west-03a'],
    requester: 'm.tran',
    created_by: 'wpena',
    created_at: '2026-05-30T13:58:02Z',
    updated_at: '2026-05-30T13:58:02Z',
    currentStep: null,
  },
];

// ---- Discovered facts (from inventory client, per run) --------------------
export const FACTS = {
  '01HZX8K3QJ7P2M5N8R4T6V9W2C': {
    instance_type: 'gb200-4x', sku: 'XE9712-GB200-NVL72', variant: 'air-cooled-rev-c',
    region: 'US-EAST-01A', nodes: 18, source: 'inventory · cwf where',
  },
  '01HZWQ2F8M4N6P9R3T5V7X1Z0B': {
    instance_type: 'gb200-4x', sku: 'XE9712-GB200-NVL72', variant: 'liquid-cooled-rev-d',
    region: 'US-WEST-02B', nodes: 9, source: 'inventory · cwf where',
  },
  '01HZW0A5C7E9G2J4L6N8Q1S3U': {
    instance_type: 'gb300-4x', sku: 'XE9680-GB300', variant: 'liquid-cooled-rev-a',
    region: 'US-EAST-02A', nodes: 16, source: 'inventory · cwf where',
  },
  '01HZV6T8W0Y2A4C6E8G1J3L5N': {
    instance_type: 'gb200-4x', sku: 'HGX-H200-8G', variant: 'air-cooled-rev-b',
    region: 'EU-WEST-01A', nodes: 8, source: 'inventory · cwf where',
  },
};

// ---- alert_probe evidence (live step output) ------------------------------
// Shape matches internal/probe/alertprobe: per_rack { alerts, max_severity, count_by_severity }.
export const ALERT_EVIDENCE = {
  '01HZX8K3QJ7P2M5N8R4T6V9W2C': {
    probed_at: '2026-05-30T14:22:51Z',
    source: 'alertmanager-stub',
    alerts_clear: false,
    max_severity: 'warning',
    per_rack: {
      'dh3-r012-us-east-01a': {
        max_severity: 'warning',
        count_by_severity: { critical: 0, warning: 2, info: 1 },
        alerts: [
          { name: 'GPUThermalMargin', severity: 'warning', node: 'dh3-r012-n04', summary: 'Tray 4 inlet temp 4°C above fleet median', for: '12m' },
          { name: 'NVLinkFlapRate', severity: 'warning', node: 'dh3-r012-n11', summary: 'Link flap rate 0.3/min on port 6', for: '7m' },
          { name: 'BMCClockDrift', severity: 'info', node: 'dh3-r012-n02', summary: 'BMC NTP offset 1.8s', for: '41m' },
        ],
      },
      'dh3-r014-us-east-01a': {
        max_severity: 'info',
        count_by_severity: { critical: 0, warning: 0, info: 1 },
        alerts: [
          { name: 'BMCClockDrift', severity: 'info', node: 'dh3-r014-n09', summary: 'BMC NTP offset 1.2s', for: '33m' },
        ],
      },
    },
  },
};

// ---- Event log (folded into run state) ------------------------------------
export const EVENTS = {
  '01HZX8K3QJ7P2M5N8R4T6V9W2C': [
    { seq: 0, kind: 'RunCreated', occurred_at: '2026-05-30T14:21:08Z', detail: 'template fw-release-canary · 2 racks · requester k.osei' },
    { seq: 1, kind: 'FactsDiscovered', occurred_at: '2026-05-30T14:21:12Z', detail: 'gb200-4x · XE9712-GB200-NVL72 · air-cooled-rev-c · 18 nodes' },
    { seq: 2, kind: 'StepStarted', occurred_at: '2026-05-30T14:22:44Z', detail: 'alert_baseline · alert_probe on 2 racks' },
    { seq: 3, kind: 'StepCompleted', occurred_at: '2026-05-30T14:22:55Z', detail: 'alert_baseline · evidence captured · max_severity warning' },
  ],
  '01HZWQ2F8M4N6P9R3T5V7X1Z0B': [
    { seq: 0, kind: 'RunCreated', occurred_at: '2026-05-28T09:02:41Z', detail: 'template fw-release-canary · 1 rack · requester m.tran' },
    { seq: 1, kind: 'FactsDiscovered', occurred_at: '2026-05-28T09:02:46Z', detail: 'gb200-4x · XE9712-GB200-NVL72 · liquid-cooled-rev-d · 9 nodes' },
    { seq: 2, kind: 'StepCompleted', occurred_at: '2026-05-28T09:05:10Z', detail: 'alert_baseline · clear' },
    { seq: 3, kind: 'StepCompleted', occurred_at: '2026-05-29T15:20:00Z', detail: 'l12_check · pass' },
    { seq: 4, kind: 'OperatorAction', occurred_at: '2026-05-29T17:48:12Z', detail: 'signoff · verdict pass · m.tran' },
  ],
  '01HZW0A5C7E9G2J4L6N8Q1S3U': [
    { seq: 0, kind: 'RunCreated', occurred_at: '2026-05-27T11:14:22Z', detail: 'template fw-release-canary · 2 racks · requester k.osei' },
    { seq: 1, kind: 'FactsDiscovered', occurred_at: '2026-05-27T11:14:28Z', detail: 'gb300-4x · XE9680-GB300 · liquid-cooled-rev-a · 16 nodes' },
    { seq: 2, kind: 'StepCompleted', occurred_at: '2026-05-27T11:18:02Z', detail: 'alert_baseline · clear' },
    { seq: 3, kind: 'StepStarted', occurred_at: '2026-05-27T11:40:11Z', detail: 'zap_canary · flcc node-zap on 2 racks' },
    { seq: 4, kind: 'StepFailed', occurred_at: '2026-05-27T13:55:09Z', detail: 'zap_canary · dh2-r005 node-zap status=failed' },
  ],
  '01HZV6T8W0Y2A4C6E8G1J3L5N': [
    { seq: 0, kind: 'RunCreated', occurred_at: '2026-05-26T16:40:55Z', detail: 'template fw-release-canary · 1 rack · requester s.alvarez' },
    { seq: 1, kind: 'FactsDiscovered', occurred_at: '2026-05-26T16:41:00Z', detail: 'gb200-4x · HGX-H200-8G · air-cooled-rev-b · 8 nodes' },
    { seq: 2, kind: 'StepCompleted', occurred_at: '2026-05-27T02:10:44Z', detail: 'l12_check · pass with warnings' },
    { seq: 3, kind: 'StepStarted', occurred_at: '2026-05-27T08:11:30Z', detail: 'evidence_review · awaiting operator' },
  ],
  '01HZU9P1R3T5V7X9Z2B4D6F8H': [
    { seq: 0, kind: 'RunCreated', occurred_at: '2026-05-30T13:58:02Z', detail: 'template fw-release-canary · 1 rack · requester m.tran' },
  ],
};

// ---- Per-step status for a run (derived for the rail) ---------------------
// Returns map stepId -> 'passed'|'running'|'failed'|'warning'|'pending'|'skipped'
export function stepStatesFor(run) {
  const cur = run.currentStep ? STEP_INDEX[run.currentStep] : -1;
  const out = {};
  FLAT_STEPS.forEach((s, i) => {
    if (run.status === 'pending') { out[s.id] = 'pending'; return; }
    if (run.status === 'signed_off') { out[s.id] = 'passed'; return; }
    if (i < cur) out[s.id] = 'passed';
    else if (i === cur) {
      if (run.status === 'failed') out[s.id] = 'failed';
      else if (run.status === 'warning') out[s.id] = 'warning';
      else out[s.id] = 'running';
    } else out[s.id] = 'pending';
  });
  return out;
}

// ---- Firmware bundle tracker ----------------------------------------------
// Per-bundle rollout view: each firmware bundle moves left→right through the
// qualification lifecycle. Complements the per-run Canary Runs page.
//   stage: candidate | canary | qualified | rollout | live
export const BUNDLE_STAGES = [
  { id: 'candidate', label: 'Candidate',  tint: 'gray',   hint: 'Submitted, not yet scheduled' },
  { id: 'canary',    label: 'In Canary',  tint: 'blue',   hint: 'Verifying on canary racks' },
  { id: 'qualified', label: 'Qualified',  tint: 'green',  hint: 'Passed — ready to roll' },
  { id: 'rollout',   label: 'Rolling Out', tint: 'yellow', hint: 'Deploying to the fleet' },
  { id: 'live',      label: 'Live',        tint: 'green',  hint: 'Fleet standard' },
];

export const BUNDLES = [
  { id: 'dell-xe9712-default-2.1.0-rc', stage: 'candidate', instance: 'gb200-4x', sku: 'XE9712-GB200-NVL72', owner: 'k.osei', updated_at: '2026-05-30T13:40:00Z', note: 'Queued behind 2.0.0-rc canary' },
  { id: 'dell-xe9712-default-2.0.0-rc', stage: 'canary', instance: 'gb200-4x', sku: 'XE9712-GB200-NVL72', owner: 'k.osei', updated_at: '2026-05-30T14:22:55Z', run_id: '01HZX8K3QJ7P2M5N8R4T6V9W2C', run_status: 'running', canary_racks: 2 },
  { id: 'hgx-h200-fw-2026-05-canary-2', stage: 'canary', instance: 'h200-8g', sku: 'HGX-H200-8G', owner: 's.alvarez', updated_at: '2026-05-27T08:11:30Z', run_id: '01HZV6T8W0Y2A4C6E8G1J3L5N', run_status: 'warning', canary_racks: 1 },
  { id: 'dell-xe9680-default-3.1.0-rc', stage: 'canary', instance: 'gb300-4x', sku: 'XE9680-GB300', owner: 'k.osei', updated_at: '2026-05-27T13:55:09Z', run_id: '01HZW0A5C7E9G2J4L6N8Q1S3U', run_status: 'failed', canary_racks: 2, blocked: true },
  { id: 'dell-xe9712-default-1.9.4', stage: 'qualified', instance: 'gb200-4x', sku: 'XE9712-GB200-NVL72', owner: 'm.tran', updated_at: '2026-05-29T17:48:12Z', run_id: '01HZWQ2F8M4N6P9R3T5V7X1Z0B', qualified_at: '2026-05-29T17:48:12Z' },
  { id: 'dell-xe9712-default-1.9.2', stage: 'rollout', instance: 'gb200-4x', sku: 'XE9712-GB200-NVL72', owner: 'm.tran', updated_at: '2026-05-30T11:05:00Z', racks_done: 142, racks_total: 318 },
  { id: 'dell-xe9680-default-3.0.4', stage: 'live', instance: 'gb300-4x', sku: 'XE9680-GB300', owner: 'm.tran', updated_at: '2026-05-12T10:00:00Z', racks_total: 96 },
  { id: 'hgx-h200-fw-2026-04', stage: 'live', instance: 'h200-8g', sku: 'HGX-H200-8G', owner: 's.alvarez', updated_at: '2026-04-30T10:00:00Z', racks_total: 54 },
];

// ---- Grafana dashboard links ----------------------------------------------
// Curated dashboards operators jump to while watching a canary run. Links are
// templated with $rack / $region / $run_id, filled from the run context.
export const GRAFANA_BASE = 'https://grafana.cw.internal/d';
export const GRAFANA_LINKS = [
  { id: 'rack-power',   title: 'Rack Power & Cooling',     category: 'Rack',    scope: 'rack',   vars: ['$rack'],            uid: 'rk-pwr-01',  desc: 'Per-rack PDU draw, inlet/outlet temps, and CDU loop pressure. First stop when thermal alerts fire.' },
  { id: 'gpu-thermals', title: 'GPU Thermals & Clocks',    category: 'Node',    scope: 'node',   vars: ['$rack', '$node'],   uid: 'gpu-th-04',  desc: 'Per-GPU temperature, clock throttle, and power-cap events across every tray in a node.' },
  { id: 'nvlink',       title: 'NVLink Domain Health',     category: 'Fabric',  scope: 'rack',   vars: ['$rack'],            uid: 'nvl-dom-02', desc: 'NVLink flap rate, per-port bandwidth, and CRC errors across the NVL72 domain.' },
  { id: 'ib-roce',      title: 'IB / RoCE Backend Fabric', category: 'Fabric',  scope: 'fabric', vars: ['$region'],          uid: 'ib-fab-07',  desc: 'Backend fabric throughput, congestion, and misconnect screen for the L12 seatrial.' },
  { id: 'node-zap',     title: 'Node-Zap Progress',        category: 'Firmware', scope: 'run',   vars: ['$run_id'],          uid: 'zap-prog-1', desc: 'Live AWX firmware-zap workflow status, per-node bundle apply, and halt/retry counts.' },
  { id: 'fw-inventory', title: 'Fleet Firmware Inventory', category: 'Firmware', scope: 'fleet', vars: [],                   uid: 'fw-inv-00',  desc: 'Installed bundle versions across the fleet, grouped by SKU and region. Tracks rollout coverage.' },
  { id: 'alertmanager', title: 'Alertmanager Overview',    category: 'Alerts',  scope: 'fleet',  vars: ['$region'],          uid: 'am-ovw-03',  desc: 'Active alerts by severity and service, with the silence overlay from the PagerDuty integration.' },
  { id: 'hpc-verify',   title: 'HPC Verification (L12)',   category: 'Node',    scope: 'rack',   vars: ['$rack'],            uid: 'hpc-ver-05', desc: 'Throughput vs. pre-zap baseline, delta thresholds, and the post-zap metrics comparison.' },
];

// ---- Web push notifications -----------------------------------------------
// Operators subscribe a browser; QA COW sends a Web Push when a rule fires.
// The push is delivered to the OS notification center by the browser.
export const WEB_PUSH = {
  agent: {
    connected: true,
    browser: 'Chrome 126 · macOS 15.4',
    endpoint: 'https://fcm.googleapis.com/fcm/send/dQw4w9Wg…aXcQ',
    permission: 'granted',         // Notification permission for this origin
    delivered_today: 14,
    last_delivered: '2026-05-30T14:22:55Z',
  },
  rules: [
    { id: 'run_failed',       label: 'Canary run failed',          desc: 'A run hits a terminal failure on any step.',        style: 'alert',  sound: true,  enabled: true },
    { id: 'step_failed',      label: 'Step failed mid-run',         desc: 'A probe or action step fails while the run is live.', style: 'alert',  sound: true,  enabled: true },
    { id: 'awaiting_review',  label: 'Run awaiting your review',    desc: 'A run reaches evidence review and needs an operator.', style: 'banner', sound: false, enabled: true },
    { id: 'signoff_done',     label: 'Run signed off',              desc: 'A run is signed off pass or fail.',                  style: 'banner', sound: false, enabled: true },
    { id: 'silence_expiring', label: 'Alert silence expiring',      desc: 'A PagerDuty silence has under 30 min left.',         style: 'banner', sound: true,  enabled: true },
    { id: 'baseline_alerts',  label: 'New alerts at baseline',      desc: 'alert_probe captures active alerts pre-zap.',        style: 'banner', sound: false, enabled: false },
    { id: 'run_complete',     label: 'Canary run completed',        desc: 'All steps finished, ready for review or sign-off.',  style: 'banner', sound: false, enabled: true },
  ],
  // most recent delivery, used for the preview banner
  recent: {
    rule: 'awaiting_review',
    title: 'Run awaiting your review',
    body: 'hgx-h200-fw-2026-05-canary-2 · dh4-r021-eu-west-01a reached evidence review.',
    at: '2026-05-30T14:22:55Z',
  },
};

// ---- PagerDuty alert silencer ---------------------------------------------
// During canary zap + L10/L11/L12 diagnostics, racks reboot, flap NVLink, and
// trip thermal alerts — all expected. Silences put the rack's PagerDuty service
// into a maintenance window so on-call isn't paged for known-disruptive QA work.
export const PAGERDUTY = {
  service: {
    name: 'GPU Fleet — FRO On-Call',
    subdomain: 'coreweave.pagerduty.com',
    serviceId: 'PXY7K2A',
    integration: 'Events API v2',
    escalation_policy: 'FRO Primary → SRE Secondary',
    status: 'connected',
    last_sync: '2026-05-30T14:24:30Z',
  },
  // status derived against NOW (2026-05-30T14:25:00Z) in the page; stored as-is here.
  silences: [
    {
      id: 'mw_01HZX8M2', rack: 'dh3-r012-us-east-01a', run_id: '01HZX8K3QJ7P2M5N8R4T6V9W2C',
      reason: 'FW canary zap — dell-xe9712-default-2.0.0-rc',
      created_by: 'k.osei', auto: true,
      starts_at: '2026-05-30T14:22:10Z', ends_at: '2026-05-30T18:22:10Z', suppressed: 3,
    },
    {
      id: 'mw_01HZX8M9', rack: 'dh3-r014-us-east-01a', run_id: '01HZX8K3QJ7P2M5N8R4T6V9W2C',
      reason: 'FW canary zap — dell-xe9712-default-2.0.0-rc',
      created_by: 'k.osei', auto: true,
      starts_at: '2026-05-30T14:22:10Z', ends_at: '2026-05-30T18:22:10Z', suppressed: 1,
    },
    {
      id: 'mw_01HZU9P3', rack: 'dh6-r009-us-west-03a', run_id: '01HZU9P1R3T5V7X9Z2B4D6F8H',
      reason: 'Pre-silence ahead of queued canary run',
      created_by: 'm.tran', auto: false,
      starts_at: '2026-05-30T13:55:00Z', ends_at: '2026-05-30T14:50:00Z', suppressed: 9,
    },
    {
      id: 'mw_01HZW7T1', rack: 'dh5-r007-us-west-02b', run_id: null,
      reason: 'Scheduled tray reseat — DC-OPS-4471',
      created_by: 'm.tran', auto: false,
      starts_at: '2026-05-30T15:00:00Z', ends_at: '2026-05-30T17:00:00Z', suppressed: 0,
    },
    {
      id: 'mw_01HZW0A8', rack: 'dh2-r003-us-east-02a', run_id: '01HZW0A5C7E9G2J4L6N8Q1S3U',
      reason: 'FW canary zap — dell-xe9680-default-3.1.0-rc',
      created_by: 'k.osei', auto: true,
      starts_at: '2026-05-27T11:40:00Z', ends_at: '2026-05-27T13:55:09Z', suppressed: 22,
    },
    {
      id: 'mw_01HZW0AC', rack: 'dh2-r005-us-east-02a', run_id: '01HZW0A5C7E9G2J4L6N8Q1S3U',
      reason: 'FW canary zap — dell-xe9680-default-3.1.0-rc',
      created_by: 'k.osei', auto: true,
      starts_at: '2026-05-27T11:40:00Z', ends_at: '2026-05-27T13:55:09Z', suppressed: 18,
    },
  ],
};

// ---- Jira — FW QA issue tracker -------------------------------------------
// Bugs / incidents / tasks filed from canary failures, regressions, and
// tooling work. Mirrors the FWQA project board; many issues link a canary run.
export const JIRA = {
  project: { key: 'FWQA', name: 'Firmware Qualification', board: 'FWQA — Canary', host: 'coreweave.atlassian.net' },
  // workflow columns, in board order
  statuses: [
    { id: 'backlog',     label: 'Backlog',     tint: 'gray' },
    { id: 'todo',        label: 'To Do',       tint: 'gray' },
    { id: 'in_progress', label: 'In Progress', tint: 'blue' },
    { id: 'in_review',   label: 'In Review',   tint: 'yellow' },
    { id: 'done',        label: 'Done',        tint: 'green' },
  ],
  issues: [
    {
      key: 'FWQA-412', type: 'incident', priority: 'highest', status: 'in_progress',
      summary: 'node-zap halts on dh2-r005 — bundle 3.1.0-rc fails apply',
      assignee: 'k.osei', reporter: 'wpena', updated_at: '2026-05-30T13:10:00Z',
      run_id: '01HZW0A5C7E9G2J4L6N8Q1S3U', labels: ['node-zap', 'gb300-4x'], comments: 7,
    },
    {
      key: 'FWQA-409', type: 'bug', priority: 'high', status: 'in_review',
      summary: 'GPUThermalMargin warning persists post-zap on dh3-r012 tray 4',
      assignee: 's.alvarez', reporter: 'k.osei', updated_at: '2026-05-30T11:42:00Z',
      run_id: '01HZX8K3QJ7P2M5N8R4T6V9W2C', labels: ['thermals', 'alert_probe'], comments: 3,
    },
    {
      key: 'FWQA-405', type: 'bug', priority: 'high', status: 'todo',
      summary: 'NVLinkFlapRate threshold too sensitive — flags healthy racks at baseline',
      assignee: 'm.tran', reporter: 's.alvarez', updated_at: '2026-05-29T16:20:00Z',
      run_id: '01HZV6T8W0Y2A4C6E8G1J3L5N', labels: ['nvlink', 'probe-tuning'], comments: 5,
    },
    {
      key: 'FWQA-398', type: 'task', priority: 'medium', status: 'in_progress',
      summary: 'Wire baseline_post metrics delta step into live template (iter-5)',
      assignee: 'wpena', reporter: 'wpena', updated_at: '2026-05-30T09:05:00Z',
      run_id: null, labels: ['template', 'metrics'], comments: 2,
    },
    {
      key: 'FWQA-391', type: 'bug', priority: 'medium', status: 'todo',
      summary: 'BMCClockDrift info alerts not deduped across reseats',
      assignee: 's.alvarez', reporter: 'm.tran', updated_at: '2026-05-28T14:48:00Z',
      run_id: null, labels: ['bmc', 'alert_probe'], comments: 1,
    },
    {
      key: 'FWQA-384', type: 'task', priority: 'low', status: 'backlog',
      summary: 'Add per-rack Grafana deep-link to evidence review panel',
      assignee: null, reporter: 'k.osei', updated_at: '2026-05-26T10:15:00Z',
      run_id: null, labels: ['grafana', 'ux'], comments: 0,
    },
    {
      key: 'FWQA-377', type: 'bug', priority: 'high', status: 'done',
      summary: 'Silence window not auto-extended when zap retries past ends_at',
      assignee: 'm.tran', reporter: 'k.osei', updated_at: '2026-05-29T18:02:00Z',
      run_id: '01HZWQ2F8M4N6P9R3T5V7X1Z0B', labels: ['pagerduty', 'silencer'], comments: 4,
    },
    {
      key: 'FWQA-370', type: 'incident', priority: 'highest', status: 'done',
      summary: 'L12 seatrial false misconnect on EU-WEST fabric — stale topology cache',
      assignee: 's.alvarez', reporter: 's.alvarez', updated_at: '2026-05-25T20:30:00Z',
      run_id: null, labels: ['fabric', 'l12'], comments: 9,
    },
  ],
};

// ---- Fix trackers — hardware I'm working OUTSIDE the canary QA flow --------
// Personal worklist of nodes / racks taken offline for repair: the failure,
// where it is in the repair lifecycle, the next action, and a linked ticket.
// Shared repair-lifecycle status model for both trackers.
export const FIX_STATUSES = [
  { id: 'triage',      label: 'Triage',      tint: 'gray',   desc: 'Just picked up — root cause not yet confirmed.' },
  { id: 'diagnosing',  label: 'Diagnosing',  tint: 'blue',   desc: 'Running diagnostics / playbook to isolate the fault.' },
  { id: 'rma_pending', label: 'RMA Pending', tint: 'yellow', desc: 'Faulted part identified — awaiting RMA approval / parts.' },
  { id: 'repair',      label: 'In Repair',   tint: 'blue',   desc: 'DC tech is physically swapping / reseating hardware.' },
  { id: 'verifying',   label: 'Verifying',   tint: 'yellow', desc: 'Hardware back — re-running screens to confirm the fix.' },
  { id: 'blocked',     label: 'Blocked',     tint: 'red',    desc: 'Stuck on a dependency outside my control.' },
  { id: 'resolved',    label: 'Resolved',    tint: 'green',  desc: 'Fixed and verified — ready to return to the fleet.' },
];

// Individual GPU nodes / trays
export const NODE_FIXES = [
  { id: 'dh2-r005-n04', rack: 'dh2-r005-us-east-02a', region: 'US-EAST-02A', instance: 'gb300-4x', issue: 'GPU3 fell off the bus mid-zap', xid: 79, status: 'diagnosing', priority: 'high',   jira: 'FWQA-412', opened_at: '2026-05-29T08:10:00Z', updated_at: '2026-05-30T13:05:00Z', next: 'Run XID 149 playbook to confirm faulted tray, then file RMA.' },
  { id: 'dh3-r012-n07', rack: 'dh3-r012-us-east-01a', region: 'US-EAST-01A', instance: 'gb200-4x', issue: 'GPU6 double-bit ECC, uncorrectable', xid: 48, status: 'rma_pending', priority: 'high',   jira: 'FWQA-409', opened_at: '2026-05-28T15:42:00Z', updated_at: '2026-05-30T11:40:00Z', next: 'RMA approved — awaiting replacement tray from depot (ETA 06-02).' },
  { id: 'dh2-r003-n11', rack: 'dh2-r003-us-east-02a', region: 'US-EAST-02A', instance: 'gb300-4x', issue: 'NVLink down on 2 lanes, port 5', xid: null, status: 'repair',      priority: 'medium', jira: null,       opened_at: '2026-05-29T19:20:00Z', updated_at: '2026-05-30T09:55:00Z', next: 'DC tech reseating NVLink bridge on tray 2.' },
  { id: 'dh5-r007-n02', rack: 'dh5-r007-us-west-02b', region: 'US-WEST-02B', instance: 'gb200-4x', issue: 'HBM thermal throttle, GPU1 hot', xid: null, status: 'verifying',   priority: 'medium', jira: null,       opened_at: '2026-05-27T12:00:00Z', updated_at: '2026-05-30T07:30:00Z', next: 'Re-running L10 fielddiag after CDU manifold clean.' },
  { id: 'dh4-r021-n09', rack: 'dh4-r021-eu-west-01a', region: 'EU-WEST-01A', instance: 'hgx-h200', issue: 'BMC unresponsive, no IPMI',     xid: null, status: 'blocked',     priority: 'high',   jira: 'FWQA-391', opened_at: '2026-05-30T05:15:00Z', updated_at: '2026-05-30T12:48:00Z', next: 'Blocked on smart-hands window in EU-WEST (next slot 06-01 09:00 CET).' },
  { id: 'dh3-r014-n01', rack: 'dh3-r014-us-east-01a', region: 'US-EAST-01A', instance: 'gb200-4x', issue: 'PCIe link degraded to x8',       xid: null, status: 'triage',      priority: 'low',    jira: null,       opened_at: '2026-05-30T10:25:00Z', updated_at: '2026-05-30T10:25:00Z', next: 'Pull PCIe AER logs, compare against baseline.' },
  { id: 'dh2-r005-n12', rack: 'dh2-r005-us-east-02a', region: 'US-EAST-02A', instance: 'gb300-4x', issue: 'Power-good flap on tray boot',    xid: null, status: 'resolved',    priority: 'low',    jira: null,       opened_at: '2026-05-25T14:00:00Z', updated_at: '2026-05-28T16:20:00Z', next: 'Reseated 12VHPWR — verified clean over 48h. Returned to fleet.' },
];

// Whole racks
export const RACK_FIXES = [
  { id: 'dh2-r005-us-east-02a', region: 'US-EAST-02A', instance: 'gb300-4x', nodes: 18, issue: 'L11 FieldDiag fail — NVLink domain incomplete', status: 'diagnosing', priority: 'high',   jira: 'FWQA-412', opened_at: '2026-05-29T08:05:00Z', updated_at: '2026-05-30T13:10:00Z', next: 'Working XID 149 playbook end-to-end to localize the faulted GPU/tray.' },
  { id: 'dh4-r021-eu-west-01a', region: 'EU-WEST-01A', instance: 'hgx-h200', nodes: 16, issue: 'L12 backend fabric misconnect, tray 4 ↔ spine', status: 'repair',     priority: 'high',   jira: 'FWQA-370', opened_at: '2026-05-25T18:30:00Z', updated_at: '2026-05-30T10:05:00Z', next: 'DC tech recabling tray 4 uplinks per corrected topology map.' },
  { id: 'dh3-r012-us-east-01a', region: 'US-EAST-01A', instance: 'gb200-4x', nodes: 18, issue: 'CDU coolant pressure below threshold',          status: 'rma_pending', priority: 'medium', jira: null,       opened_at: '2026-05-28T09:40:00Z', updated_at: '2026-05-30T08:15:00Z', next: 'Awaiting replacement CDU pump from facilities (RMA filed).' },
  { id: 'dh5-r007-us-west-02b', region: 'US-WEST-02B', instance: 'gb200-4x', nodes: 18, issue: 'PSU shelf fault, redundancy lost',               status: 'verifying',  priority: 'medium', jira: null,       opened_at: '2026-05-26T11:10:00Z', updated_at: '2026-05-30T06:50:00Z', next: 'PSU swapped — monitoring rail telemetry for 24h before return.' },
  { id: 'dh2-r003-us-east-02a', region: 'US-EAST-02A', instance: 'gb300-4x', nodes: 18, issue: 'Whole-rack power-capping anomaly under load',     status: 'triage',     priority: 'low',    jira: null,       opened_at: '2026-05-30T09:30:00Z', updated_at: '2026-05-30T09:30:00Z', next: 'Correlate cap events with PDU telemetry; suspect firmware.' },
  { id: 'dh6-r018-us-west-02b', region: 'US-WEST-02B', instance: 'gb200-4x', nodes: 18, issue: 'Intermittent busbar contact, rack-wide brownouts', status: 'resolved',   priority: 'low',    jira: null,       opened_at: '2026-05-22T13:00:00Z', updated_at: '2026-05-27T17:45:00Z', next: 'Busbar re-torqued to spec — clean for 5 days. Returned to fleet.' },
];

