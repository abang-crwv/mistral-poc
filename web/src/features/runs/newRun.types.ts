// Types for the canary New Run wizard. These mirror the Go wire shapes:
//   - Template / TemplateInput  -> internal/template/types.go (Template, Input)
//   - TemplateSummary           -> internal/store/templates.go (TemplateSummary)
//   - InventoryPreview / *      -> internal/api/inventory_preview.go (previewResponse)
//   - RlccWorkflowSummary       -> internal/rlccclient/types.go (WorkflowSummary)
//   - CreateRunInput            -> internal/api/runs.go (createRunRequest, new shape)
// Keep these in lockstep with the backend; a mismatch is a silent contract break.

// ---- Templates ------------------------------------------------------------

export type TemplateInputType = 'text' | 'multi_text' | 'enum' | 'url' | 'multi_url' | 'textarea';

export type TemplateInput = {
  id: string;
  label: string;
  type: TemplateInputType;
  required: boolean;
  options?: string[];
  /** RE2 source string; applied per value (per item for multi_*). */
  validate?: string;
  example?: string;
};

export type TemplateTarget = {
  scope: string;
  instance_types: string[];
};

/** Full canonical template object from GET /api/templates/{id} ({ template }). */
export type Template = {
  schema: string;
  id: string;
  version: number;
  title: string;
  target: TemplateTarget;
  inputs: TemplateInput[];
  // steps are not needed by the wizard; left loosely typed on purpose.
  steps?: unknown[];
};

/** Row from GET /api/templates ({ templates }). */
export type TemplateSummary = {
  id: string;
  version: number;
  title: string;
};

// ---- Inventory preview ----------------------------------------------------

export type PreviewBMN = {
  deviceslot: string;
  bmn_name: string;
  ct_position?: number;
};

export type PreviewIgnored = {
  deviceslot: string;
  bmn_name?: string;
};

export type PreviewRack = {
  rack: string;
  zone: string;
  instance_type?: string;
  sku?: string;
  bmns: PreviewBMN[];
  /** CTs already ignored upstream in RLCC (source=rlcc). Informational. */
  rlcc_ignored?: PreviewIgnored[];
};

export type PreviewError = {
  rack: string;
  message: string;
};

/** GET /api/inventory/preview?racks=a,b,c */
export type InventoryPreview = {
  racks: PreviewRack[];
  errors?: PreviewError[];
};

// ---- RLCC workflows (context panel) ---------------------------------------

export type RlccWorkflowSummary = {
  name: string;
  description: string;
  lifecycle: string;
  current_states: string[];
  desired_state: string;
  rack_kind: string;
  action_count: number;
  /** Sorted, de-duplicated handler kinds in the workflow (e.g. 'l11-fielddiag').
   *  The canary picker shows only workflows whose handlers include the L11 step. */
  handlers: string[];
};

// ---- Create run (new shape) -----------------------------------------------

/** One operator-supplied ignore entry posted with the run. */
export type IgnoredBMN = {
  rack: string;
  deviceslot: string;
  reason?: string;
};

/**
 * POST /api/runs (iter-3a+ shape). `inputs` is keyed by template input id —
 * e.g. { bundle_tag: string, canary_racks: string[], requester?: string }.
 */
export type CreateRunInput = {
  template_id: string;
  inputs: Record<string, unknown>;
  ignored_bmns?: IgnoredBMN[];
};
