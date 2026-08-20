export type RunStatus = 'pending' | 'running' | 'passed' | 'warning' | 'failed' | 'signed_off';

export type Run = {
  id: string;
  bundle: string;
  template_id: string;
  created_at: string;
  updated_at: string;
  status: RunStatus;
  rack: string;
  created_by: string;
};

export type Event = {
  seq: number;
  kind: string;
  occurred_at: string;
  // payload shape is keyed by `kind`; consumers narrow at the point of use.
  payload: unknown;
};
