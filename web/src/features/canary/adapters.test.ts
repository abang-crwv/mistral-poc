import { describe, it, expect } from 'vitest';
import {
  deriveCurrentStep,
  adaptRun,
  detailFor,
  adaptFacts,
  adaptAlertEvidence,
  adaptTemplateSteps,
  stepStatesFor,
} from './adapters';
import type { RawRun, RawEvent } from './types';

const raw: RawRun = {
  id: 'r1',
  bundle: 'b1',
  template_id: 'firmware-release-canary',
  status: 'running',
  rack: 'dh3-r012-us-east-01a',
  created_by: 'wpena',
  created_at: 't0',
  updated_at: 't1',
  inputs: {
    bundle_tag: 'b1',
    canary_racks: ['dh3-r012-us-east-01a', 'dh3-r014-us-east-01a'],
    requester: 'k.osei',
    rlcc_workflow: 'wf',
  },
};

describe('adaptRun', () => {
  it('derives racks/requester from inputs and verdict from status', () => {
    const r = adaptRun(raw, []);
    expect(r.racks).toEqual(['dh3-r012-us-east-01a', 'dh3-r014-us-east-01a']);
    expect(r.requester).toBe('k.osei');
    expect(r.verdict).toBeUndefined(); // running
    expect(r.currentStep).toBeNull(); // no events
  });
  it('falls back to [rack] when inputs.canary_racks is absent', () => {
    const r = adaptRun({ ...raw, inputs: null }, []);
    expect(r.racks).toEqual(['dh3-r012-us-east-01a']);
  });
  it('maps verdict from terminal status', () => {
    expect(adaptRun({ ...raw, status: 'failed' }, []).verdict).toBe('fail');
    expect(adaptRun({ ...raw, status: 'signed_off' }, []).verdict).toBe('pass');
  });
});

describe('deriveCurrentStep', () => {
  it('is the last StepStarted without a later StepCompleted/StepFailed', () => {
    const evs: RawEvent[] = [
      { seq: 1, kind: 'StepStarted', payload: { step_id: 's1' }, occurred_at: 't' },
      { seq: 2, kind: 'StepCompleted', payload: { step_id: 's1' }, occurred_at: 't' },
      { seq: 3, kind: 'StepStarted', payload: { step_id: 's2' }, occurred_at: 't' },
    ];
    expect(deriveCurrentStep(evs)).toBe('s2');
  });
  it('is null when all started steps completed', () => {
    expect(
      deriveCurrentStep([
        { seq: 1, kind: 'StepStarted', payload: { step_id: 's1' }, occurred_at: 't' },
        { seq: 2, kind: 'StepCompleted', payload: { step_id: 's1' }, occurred_at: 't' },
      ]),
    ).toBeNull();
  });
});

describe('detailFor', () => {
  it('formats known kinds and falls back for unknown', () => {
    expect(
      detailFor('StepStarted', { step_id: 'l11_fielddiag', probe_type: 'rlcc_action_probe' }),
    ).toContain('l11_fielddiag');
    expect(detailFor('StepCompleted', { step_id: 'l11_fielddiag', verdict: 'failed' })).toContain(
      'failed',
    );
    expect(detailFor('Mystery', {})).toBe('Mystery');
  });
});

describe('adaptFacts / adaptAlertEvidence / adaptTemplateSteps / stepStatesFor', () => {
  it('maps facts and supplies a source label', () => {
    const f = adaptFacts(
      { instance_type: 'gb200-4x', sku: 'X', variant: 'v', region: 'US-EAST-01A' },
      ['rk1'],
    );
    expect(f.instance_type).toBe('gb200-4x');
    expect(f.source).toMatch(/inventory/);
  });
  it('passes the alert_probe aggregate payload through', () => {
    const e = adaptAlertEvidence({
      probed_at: 1,
      source: 's',
      max_severity: 'warning',
      alerts_clear: false,
      per_rack: {},
    });
    expect(e.max_severity).toBe('warning');
  });
  it('builds flat steps + index from a template', () => {
    const { flatSteps, stepIndex } = adaptTemplateSteps({
      id: 't',
      version: 10,
      title: 'T',
      steps: [
        { id: 'a', kind: 'probe', title: 'A', probe_type: 'alert_probe', timeout: '30s' },
        { id: 'b', kind: 'signoff', title: 'B' },
      ],
    });
    expect(flatSteps.map((s) => s.id)).toEqual(['a', 'b']);
    expect(stepIndex.b).toBe(1);
    expect(flatSteps[0].tool).toBe('alert_probe');
  });
  it('stepStatesFor marks before-current passed, current running, after pending', () => {
    const flat = [{ id: 'a' }, { id: 'b' }, { id: 'c' }].map((s) => ({
      ...s,
      title: '',
      kind: '',
      tool: '',
      live: false,
      timeout: '',
      desc: '',
    }));
    const idx = { a: 0, b: 1, c: 2 };
    const run = { ...adaptRun(raw, []), currentStep: 'b', status: 'running' as const };
    const st = stepStatesFor(run, flat, idx);
    expect(st).toEqual({ a: 'passed', b: 'running', c: 'pending' });
  });
});
