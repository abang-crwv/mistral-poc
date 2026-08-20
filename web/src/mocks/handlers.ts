import { http, HttpResponse } from 'msw';
import type { Run } from '@/features/runs/runs.types';
import type {
  InventoryPreview,
  RlccWorkflowSummary,
  Template,
  TemplateSummary,
} from '@/features/runs/newRun.types';

// `let` (not `const`) so the POST handler can prepend new runs.
export let fixtureRuns: Run[] = [
  {
    id: '01HZX0000000000000000000A',
    bundle: 'gb200-fw-2026-05-canary-3',
    template_id: 'firmware-release-canary',
    created_at: '2026-05-27T15:04:05Z',
    updated_at: '2026-05-27T15:04:05Z',
    status: 'pending',
    rack: 'dh3-r012-us-east-01a',
    created_by: 'wpena',
  },
  {
    id: '01HZX0000000000000000000B',
    bundle: 'gb200-fw-2026-05-canary-2',
    template_id: 'firmware-release-canary',
    created_at: '2026-05-27T14:00:00Z',
    updated_at: '2026-05-27T14:30:00Z',
    status: 'passed',
    rack: 'dh4-r066-us-east-11a',
    created_by: 'wpena',
  },
  {
    id: '01HZX0000000000000000000C',
    bundle: 'gb200-fw-2026-05-canary-1',
    template_id: 'firmware-release-canary',
    created_at: '2026-05-27T13:00:00Z',
    updated_at: '2026-05-27T13:15:00Z',
    status: 'failed',
    rack: 'dh3-r012-us-east-01a',
    created_by: 'wpena',
  },
];

// Mirrors templates/firmware-release-canary.yaml (inputs only — the wizard
// doesn't need steps). canary_racks carries an `example` so the validation
// message reads nicely in the form.
const templateSummaries: TemplateSummary[] = [
  { id: 'firmware-release-canary', version: 9, title: 'FW release canary' },
];

const firmwareReleaseCanary: Template = {
  schema: 'qac.template/v1',
  id: 'firmware-release-canary',
  version: 9,
  title: 'FW release canary',
  target: { scope: 'rack', instance_types: ['gb200-4x', 'gb300-4x'] },
  inputs: [
    {
      id: 'bundle_tag',
      label: 'FW bundle tag',
      type: 'text',
      required: true,
      example: 'dell-xe9712-default-2.0.0-rc',
    },
    {
      id: 'canary_racks',
      label: 'Canary racks',
      type: 'multi_text',
      required: true,
      validate: '^dh\\d+-r\\d{3}-[a-z]+-[a-z]+-\\d{2}[a-z]$',
      example: 'dh3-r012-us-east-01a',
    },
    {
      id: 'requester',
      label: 'Requester (from #havock)',
      type: 'text',
      required: false,
    },
    {
      id: 'rlcc_workflow',
      label: 'RLCC bringup workflow',
      type: 'text',
      required: true,
      example: 'gb200-rack-bringup-v4',
    },
  ],
};

// Real GET /api/templates/{id} returns steps[]; the canary adapter builds the
// step rail from these. A trimmed-but-real-shaped subset of the canonical
// firmware-release-canary template.
const firmwareReleaseCanarySteps = [
  {
    id: 'firmware_baseline',
    kind: 'probe',
    title: 'Firmware inventory',
    probe_type: 'firmware_inventory_probe',
    timeout: '60s',
  },
  {
    id: 'alert_baseline',
    kind: 'probe',
    title: 'Alert baseline',
    probe_type: 'alert_probe',
    timeout: '30s',
  },
  {
    id: 'l11_fielddiag',
    kind: 'probe',
    title: 'L11 field diag',
    probe_type: 'rlcc_action_probe',
    timeout: '45m',
  },
  { id: 'operator_review', kind: 'review', title: 'Operator review' },
  { id: 'signoff', kind: 'signoff', title: 'Sign off' },
];

// A small BMN fixture per known rack.
function previewRack(rack: string, instance: string, sku: string, zone: string, cts: number) {
  const bmns = Array.from({ length: cts }, (_, i) => ({
    deviceslot: `ct-${String(i + 1).padStart(2, '0')}`,
    bmn_name: `${rack}-bmn-${String(i + 1).padStart(2, '0')}`,
    ct_position: i + 1,
  }));
  return {
    rack,
    zone,
    instance_type: instance,
    sku,
    bmns,
    rlcc_ignored: cts > 6 ? [{ deviceslot: 'ct-04', bmn_name: `${rack}-bmn-04` }] : [],
  };
}

const knownRacks: Record<string, ReturnType<typeof previewRack>> = {
  'dh3-r012-us-east-01a': previewRack(
    'dh3-r012-us-east-01a',
    'gb200-4x',
    'XE9712-GB200-NVL72',
    'us-east-01a',
    8,
  ),
  'dh3-r014-us-east-01a': previewRack(
    'dh3-r014-us-east-01a',
    'gb200-4x',
    'XE9712-GB200-NVL72',
    'us-east-01a',
    6,
  ),
  'dh4-r066-us-east-11a': previewRack(
    'dh4-r066-us-east-11a',
    'gb300-4x',
    'XE9680-GB300',
    'us-east-11a',
    8,
  ),
};

// Canary `inputs` blobs keyed by run id (real /api/runs rows carry an `inputs`
// JSON object the canary adapter reads for racks/requester/workflow).
const runInputs: Record<
  string,
  { bundle_tag: string; canary_racks: string[]; requester: string; rlcc_workflow: string }
> = {
  '01HZX0000000000000000000A': {
    bundle_tag: 'gb200-fw-2026-05-canary-3',
    canary_racks: ['dh3-r012-us-east-01a', 'dh3-r014-us-east-01a'],
    requester: 'k.osei',
    rlcc_workflow: 'gb200-rack-bringup-v4',
  },
  '01HZX0000000000000000000B': {
    bundle_tag: 'gb200-fw-2026-05-canary-2',
    canary_racks: ['dh4-r066-us-east-11a'],
    requester: 'k.osei',
    rlcc_workflow: 'gb200-rack-bringup-v4',
  },
  '01HZX0000000000000000000C': {
    bundle_tag: 'gb200-fw-2026-05-canary-1',
    canary_racks: ['dh3-r012-us-east-01a'],
    requester: 'm.tan',
    rlcc_workflow: 'gb200-rack-bringup-v4',
  },
};

// Facts keyed by run id (real GET /api/runs/{id}/facts → {facts:{…}}).
const runFacts: Record<string, Record<string, unknown>> = {
  '01HZX0000000000000000000A': {
    instance_type: 'gb200-4x',
    sku: 'XE9712-GB200-NVL72',
    variant: 'default',
    region: 'us-east-01a',
  },
};

// alert_baseline (alert_probe) aggregate evidence keyed by run id. The aggregate
// row has deviceslot == null and the alert_probe Output as its payload.
const runAlertEvidence: Record<string, Record<string, unknown>> = {
  '01HZX0000000000000000000A': {
    probed_at: 1748358245,
    source: 'prometheus · alert_probe',
    alerts_clear: true,
    max_severity: 'none',
    per_rack: {
      'dh3-r012-us-east-01a': { max_severity: 'none', count_by_severity: {}, alerts: [] },
      'dh3-r014-us-east-01a': { max_severity: 'none', count_by_severity: {}, alerts: [] },
    },
  },
};

const rlccWorkflows: RlccWorkflowSummary[] = [
  {
    name: 'gb200-rack-bringup-v4',
    description: 'Full GB200 NVL72 rack bringup',
    lifecycle: 'bringup',
    current_states: ['l10-fielddiag', 'l11-fielddiag', 'l12-seatrial'],
    desired_state: 'l12-test',
    rack_kind: 'gb200-4x',
    action_count: 14,
    handlers: ['await-checkpoint', 'flcc-workflow', 'l11-fielddiag', 'provision'],
  },
  {
    // No l11-fielddiag — the canary picker must hide this one.
    name: 'gb200-rack-power-off',
    description: 'Power off a GB200 rack',
    lifecycle: 'stable',
    current_states: ['ready'],
    desired_state: 'powered-off',
    rack_kind: 'gb200-4x',
    action_count: 2,
    handlers: ['await-checkpoint', 'power-operation'],
  },
];

export const handlers = [
  http.get('/api/health', () =>
    HttpResponse.json({ ok: true, version: 'test', db_path: '/tmp/test.db' }),
  ),

  http.get('/api/runs', () =>
    HttpResponse.json({
      runs: fixtureRuns.map((r) => ({ ...r, inputs: runInputs[r.id] ?? null })),
    }),
  ),

  http.get('/api/runs/:id', ({ params }) => {
    const id = params.id as string;
    const run = fixtureRuns.find((r) => r.id === id);
    if (!run) {
      return HttpResponse.json(
        { error: { code: 'not_found', message: 'Run not found' } },
        { status: 404 },
      );
    }
    const inputs = runInputs[id] ?? null;
    const racks = inputs?.canary_racks ?? [run.rack];
    return HttpResponse.json({
      run: { ...run, inputs },
      racks,
      events: [
        {
          seq: 1,
          kind: 'RunCreated',
          occurred_at: run.created_at,
          payload: { template_id: run.template_id, inputs },
        },
      ],
    });
  }),

  http.get('/api/runs/:id/facts', ({ params }) =>
    HttpResponse.json({ facts: runFacts[params.id as string] ?? {} }),
  ),

  http.get('/api/runs/:id/evidence', ({ params, request }) => {
    const url = new URL(request.url);
    const step = url.searchParams.get('step');
    const payload = step === 'alert_baseline' ? runAlertEvidence[params.id as string] : undefined;
    const evidence = payload ? [{ deviceslot: null, payload }] : [];
    return HttpResponse.json({ evidence });
  }),

  http.get('/api/probes', () =>
    HttpResponse.json({
      probes: [
        {
          type: 'alert_probe',
          category: 'gatherer',
          title: 'Alert history',
          description: 'Sweeps firing and pending alerts across each canary rack.',
        },
        {
          type: 'awx_job_probe',
          category: 'gatherer',
          title: 'AWX zap jobs',
          description:
            'Gathers the firmware-zap AWX jobs per node plus the rack-wide l11-fielddiag, with failure signatures.',
        },
        {
          type: 'fail_cause_probe',
          category: 'gatherer',
          title: 'Fail cause',
          description:
            "Attributes a likely failure cause from a prior step's failed-tray evidence.",
        },
        {
          type: 'firmware_inventory_probe',
          category: 'gatherer',
          title: 'Firmware inventory',
          description: 'Captures per-deviceslot firmware versions for each rack.',
        },
        {
          type: 'gpu_performance_probe',
          category: 'gatherer',
          title: 'GPU performance',
          description: 'Snapshots the HPC-verification performance pack per rack.',
        },
        {
          type: 'hpc_verification_failure_probe',
          category: 'gatherer',
          title: 'HPC verification failures',
          description: 'Reports nodes whose HPC-verification message is non-OK.',
        },
        {
          type: 'rlcc_action_probe',
          category: 'action',
          title: 'RLCC action',
          description: 'Drives and polls a per-CT RLCC action to a target state.',
        },
      ],
    }),
  ),

  http.get('/api/agents', () =>
    HttpResponse.json({
      agents: [
        {
          type: 'canary_assessor',
          purpose:
            'Reasons over per-rack canary evidence and emits an advisory verdict with confidence and ranked likely causes.',
          prompt_ref: 'fw-release-canary.summary.v1',
          model: 'fixture',
          live: false,
        },
      ],
    }),
  ),

  http.get('/api/templates', () => HttpResponse.json({ templates: templateSummaries })),

  http.get('/api/templates/:id', ({ params }) => {
    if (params.id !== firmwareReleaseCanary.id) {
      return HttpResponse.json(
        { error: { code: 'not_found', message: 'Template not found' } },
        { status: 404 },
      );
    }
    return HttpResponse.json({
      template: { ...firmwareReleaseCanary, steps: firmwareReleaseCanarySteps },
    });
  }),

  http.get('/api/inventory/preview', ({ request }) => {
    const url = new URL(request.url);
    const raw = url.searchParams.get('racks') ?? '';
    const body: InventoryPreview = { racks: [], errors: [] };
    for (const r of raw
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)) {
      const known = knownRacks[r];
      if (known) body.racks.push(known);
      else body.errors!.push({ rack: r, message: 'rack not found' });
    }
    return HttpResponse.json(body);
  }),

  http.get('/api/rlcc/workflows', () =>
    HttpResponse.json({ workflows: rlccWorkflows, source: 'fixture' }),
  ),

  // Accepts the canonical {template_id, inputs, ignored_bmns} shape (and the
  // legacy {bundle, rack}). Derives the row from inputs for the fixture.
  http.post('/api/runs', async ({ request }) => {
    const body = (await request.json()) as {
      bundle?: string;
      rack?: string;
      template_id?: string;
      inputs?: { bundle_tag?: string; canary_racks?: string[] };
    };
    const bundle = body.inputs?.bundle_tag ?? body.bundle ?? 'unknown-bundle';
    const rack = body.inputs?.canary_racks?.[0] ?? body.rack ?? 'dh3-r012-us-east-01a';
    const newRun: Run = {
      id: '01HZX0000000000000000000Z',
      bundle,
      template_id: body.template_id ?? 'firmware-release-canary',
      rack,
      status: 'pending',
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
      created_by: 'wpena',
    };
    fixtureRuns = [newRun, ...fixtureRuns];
    return HttpResponse.json({ run: newRun }, { status: 201 });
  }),
];
