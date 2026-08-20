import type {
  RawRun,
  RawEvent,
  RawTemplate,
  CanaryRun,
  RunEvent,
  Facts,
  AlertEvidence,
  CanaryStep,
  RunStatus,
} from './types';

export function deriveVerdict(status: RunStatus): 'pass' | 'fail' | undefined {
  if (status === 'failed') return 'fail';
  if (status === 'passed' || status === 'signed_off') return 'pass';
  return undefined;
}

function stepId(payload: unknown): string | undefined {
  return payload && typeof payload === 'object'
    ? ((payload as Record<string, unknown>).step_id as string | undefined)
    : undefined;
}

// The current step is the last StepStarted whose step_id has no later
// StepCompleted/StepFailed. Returns null if none is open.
export function deriveCurrentStep(events: RawEvent[]): string | null {
  const open = new Set<string>();
  const order: string[] = [];
  for (const e of events) {
    const sid = stepId(e.payload);
    if (!sid) continue;
    if (e.kind === 'StepStarted') {
      if (!open.has(sid)) order.push(sid);
      open.add(sid);
    } else if (e.kind === 'StepCompleted' || e.kind === 'StepFailed') open.delete(sid);
  }
  for (let i = order.length - 1; i >= 0; i--) if (open.has(order[i])) return order[i];
  return null;
}

export function adaptRun(raw: RawRun, events: RawEvent[]): CanaryRun {
  const inputs = raw.inputs ?? {};
  const racks =
    inputs.canary_racks && inputs.canary_racks.length
      ? inputs.canary_racks
      : raw.rack
        ? [raw.rack]
        : [];
  return {
    id: raw.id,
    bundle: raw.bundle,
    template_id: raw.template_id,
    status: raw.status,
    racks,
    requester: inputs.requester ?? raw.created_by,
    created_by: raw.created_by,
    created_at: raw.created_at,
    updated_at: raw.updated_at,
    currentStep: deriveCurrentStep(events),
    verdict: deriveVerdict(raw.status),
  };
}

type AnyRecord = Record<string, unknown>;

const p = (payload: unknown): AnyRecord =>
  payload && typeof payload === 'object' ? (payload as AnyRecord) : {};

export function detailFor(kind: string, payload: unknown): string {
  // Event payloads are loosely typed JSON; read fields defensively.
  const d = p(payload) as {
    template_id?: string;
    inputs?: { canary_racks?: string[]; requester?: string };
    instance_type?: string;
    sku?: string;
    variant?: string;
    step_id?: string;
    probe_type?: string;
    verdict?: string;
    error?: string;
    action?: string;
    signer_name?: string;
    deviceslot?: string;
    source?: string;
    message?: string;
    final_verdict?: string;
    reason?: string;
  };
  switch (kind) {
    case 'RunCreated':
      return `template ${d.template_id ?? '—'}${d.inputs?.canary_racks ? ` · ${d.inputs.canary_racks.length} racks` : ''}${d.inputs?.requester ? ` · requester ${d.inputs.requester}` : ''}`;
    case 'FactsDiscovered':
      return [d.instance_type, d.sku, d.variant].filter(Boolean).join(' · ') || 'facts discovered';
    case 'StepStarted':
      return `${d.step_id ?? '—'}${d.probe_type ? ` · ${d.probe_type}` : ''}`;
    case 'StepCompleted':
      return `${d.step_id ?? '—'}${d.verdict ? ` · verdict ${d.verdict}` : ' · completed'}`;
    case 'StepFailed':
      return `${d.step_id ?? '—'} · failed${d.error ? `: ${d.error}` : ''}`;
    case 'OperatorAction':
      return `${d.step_id ?? '—'} · ${d.action ?? 'action'}${d.verdict ? ` · ${d.verdict}` : ''}${d.signer_name ? ` · ${d.signer_name}` : ''}`;
    case 'BMNIgnored':
      return `${d.deviceslot ?? '—'} ignored${d.source ? ` · ${d.source}` : ''}`;
    case 'FactsDiscoveryFailed':
      return `facts discovery failed${d.error ? `: ${d.error}` : ''}`;
    case 'BMNDiscoveryFailed':
      return `BMN discovery failed${d.message ? `: ${d.message}` : ''}`;
    case 'RunCompleted':
      return `run complete${d.final_verdict ? ` · ${d.final_verdict}` : ''}`;
    case 'RunCancelled':
      return `run cancelled${d.reason ? `: ${d.reason}` : ''}`;
    default:
      return kind;
  }
}

export function adaptEvents(events: RawEvent[]): RunEvent[] {
  return events.map((e) => ({
    seq: e.seq,
    kind: e.kind,
    occurred_at: e.occurred_at,
    detail: detailFor(e.kind, e.payload),
  }));
}

export function adaptFacts(
  facts: Record<string, unknown> | null | undefined,
  racks: string[],
): Facts {
  const f = (facts ?? {}) as {
    instance_type?: string;
    sku?: string;
    variant?: string;
    region?: string;
    nodes?: number;
  };
  return {
    instance_type: f.instance_type,
    sku: f.sku,
    variant: f.variant,
    region: f.region,
    nodes: typeof f.nodes === 'number' ? f.nodes : racks.length ? racks.length * 18 : undefined,
    source: 'inventory · cwf where',
  };
}

export function adaptAlertEvidence(
  aggregatePayload: Record<string, unknown> | null | undefined,
): AlertEvidence {
  return (aggregatePayload ?? {}) as AlertEvidence;
}

export function adaptTemplateSteps(tpl: RawTemplate): {
  flatSteps: CanaryStep[];
  stepIndex: Record<string, number>;
} {
  const liveProbes = new Set([
    'alert_probe',
    'firmware_inventory_probe',
    'hpc_verification_failure_probe',
    'gpu_performance_probe',
    'awx_job_probe',
    'rlcc_action_probe',
    'fail_cause_probe',
  ]);
  const flatSteps: CanaryStep[] = (tpl.steps ?? []).map((s) => ({
    id: s.id,
    title: s.title,
    kind: s.kind,
    tool: s.action?.tool ?? s.probe_type ?? '',
    live: !!s.probe_type && liveProbes.has(s.probe_type),
    timeout: s.timeout ?? '',
    desc: '',
  }));
  const stepIndex: Record<string, number> = {};
  flatSteps.forEach((s, i) => {
    stepIndex[s.id] = i;
  });
  return { flatSteps, stepIndex };
}

// Parameterized port of the design's stepStatesFor (was bound to seed FLAT_STEPS).
export function stepStatesFor(
  run: CanaryRun,
  flatSteps: CanaryStep[],
  stepIndex: Record<string, number>,
): Record<string, string> {
  const cur = run.currentStep ? (stepIndex[run.currentStep] ?? -1) : -1;
  const out: Record<string, string> = {};
  flatSteps.forEach((s, i) => {
    if (run.status === 'pending') {
      out[s.id] = 'pending';
      return;
    }
    if (run.status === 'signed_off') {
      out[s.id] = 'passed';
      return;
    }
    if (i < cur) out[s.id] = 'passed';
    else if (i === cur)
      out[s.id] =
        run.status === 'failed' ? 'failed' : run.status === 'warning' ? 'warning' : 'running';
    else out[s.id] = 'pending';
  });
  return out;
}
