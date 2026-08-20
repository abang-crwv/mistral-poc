export type RunStatus = 'pending' | 'running' | 'passed' | 'warning' | 'failed' | 'signed_off';

export interface CanaryRun {
  id: string;
  bundle: string;
  template_id: string;
  status: RunStatus;
  racks: string[];
  requester: string;
  created_by: string;
  created_at: string;
  updated_at: string;
  currentStep: string | null;
  verdict?: 'pass' | 'fail';
}

export interface RunEvent {
  seq: number;
  kind: string;
  occurred_at: string;
  detail: string;
}
export interface Facts {
  instance_type?: string;
  sku?: string;
  variant?: string;
  region?: string;
  nodes?: number;
  source: string;
}
export interface AlertEvidence {
  probed_at?: number;
  source?: string;
  alerts_clear?: boolean;
  max_severity?: string;
  per_rack?: Record<
    string,
    {
      max_severity: string;
      count_by_severity: Record<string, number>;
      alerts: Array<Record<string, unknown>>;
    }
  >;
}
export interface CanaryStep {
  id: string;
  title: string;
  kind: string;
  tool: string;
  live: boolean;
  timeout: string;
  desc: string;
}

// Raw API shapes (what lib/api returns).
export interface RawRun {
  id: string;
  bundle: string;
  template_id: string;
  status: RunStatus;
  rack: string;
  created_by: string;
  created_at: string;
  updated_at: string;
  inputs?: {
    bundle_tag?: string;
    canary_racks?: string[];
    requester?: string;
    rlcc_workflow?: string;
  } | null;
}
export interface RawEvent {
  seq: number;
  kind: string;
  payload: unknown;
  occurred_at: string;
}
export interface RawTemplate {
  id: string;
  version: number;
  title: string;
  steps: Array<{
    id: string;
    kind: string;
    title: string;
    action?: { tool?: string } | null;
    probe_type?: string;
    timeout?: string;
  }>;
}
