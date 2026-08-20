import { z } from 'zod';
import type { Template, TemplateInput } from './newRun.types';

// Mirrors internal/api/runs_validate.go:ValidateInputs. The frontend builds
// the same checks from the template's inputs[] so the wizard rejects bad
// values before POST, and the messages line up with the server's 400s.

const RAW_EMPTY = '';

/** What the wizard stores per field while editing: always a string. */
export type RawInputs = Record<string, string>;

const isMulti = (t: TemplateInput['type']) => t === 'multi_text' || t === 'multi_url';

/** Split a multi_* textarea into a trimmed, de-blanked list. */
export function splitMulti(raw: string): string[] {
  return raw
    .split(/[\n,]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

/**
 * Coerce the raw string map into the typed `inputs` object POSTed to the API:
 * multi_* become string[], everything else a trimmed string. Empty optional
 * values are dropped (the server treats "" / [] as absent for required checks,
 * but omitting keeps the event log and facts clean).
 */
export function coerceInputs(tpl: Template, raw: RawInputs): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const input of tpl.inputs) {
    const value = raw[input.id] ?? RAW_EMPTY;
    if (isMulti(input.type)) {
      const list = splitMulti(value);
      if (list.length > 0 || input.required) out[input.id] = list;
    } else {
      const trimmed = value.trim();
      if (trimmed !== RAW_EMPTY || input.required) out[input.id] = trimmed;
    }
  }
  return out;
}

function fieldExample(input: TemplateInput): string {
  return input.example ? ` (example: ${input.example})` : '';
}

function fieldSchema(input: TemplateInput): z.ZodTypeAny {
  const name = input.label || input.id;
  const re = input.validate ? new RegExp(input.validate) : null;

  // coerceInputs() omits empty non-required fields from the coerced object,
  // so the schema for non-required fields must accept `undefined` (.optional()).
  // Without this, zod reports "Required" for missing optional keys and the
  // wizard refuses to advance past Inputs.
  const maybeOptional = <T extends z.ZodTypeAny>(s: T): z.ZodTypeAny =>
    input.required ? s : s.optional();

  if (isMulti(input.type)) {
    let arr = z.array(z.string());
    if (input.required) arr = arr.min(1, `${name} is required`);
    const refined = arr.refine((items) => !re || items.every((s) => re.test(s)), {
      message: `${name}: one or more values are not in the expected format${fieldExample(input)}`,
    });
    return maybeOptional(refined);
  }

  if (input.type === 'enum') {
    const options = input.options ?? [];
    const refined = z.string().refine((v) => (!input.required && v === '') || options.includes(v), {
      message: `${name} must be one of: ${options.join(', ')}`,
    });
    return maybeOptional(refined);
  }

  // text | url | textarea
  let s = z.string();
  if (input.required) s = s.min(1, `${name} is required`);
  const refined = s.refine((v) => (!input.required && v === '') || !re || re.test(v), {
    message: `${name} is not in the expected format${fieldExample(input)}`,
  });
  return maybeOptional(refined);
}

/**
 * Build a zod object over the COERCED inputs (multi_* as string[]). Validate
 * with `buildInputsSchema(tpl).safeParse(coerceInputs(tpl, raw))`.
 */
export function buildInputsSchema(tpl: Template): z.ZodObject<Record<string, z.ZodTypeAny>> {
  const shape: Record<string, z.ZodTypeAny> = {};
  for (const input of tpl.inputs) shape[input.id] = fieldSchema(input);
  return z.object(shape);
}

/** Run the schema and return a flat { [inputId]: firstErrorMessage } map. */
export function validateInputs(tpl: Template, raw: RawInputs): Record<string, string> {
  const parsed = buildInputsSchema(tpl).safeParse(coerceInputs(tpl, raw));
  if (parsed.success) return {};
  const errors: Record<string, string> = {};
  for (const issue of parsed.error.issues) {
    const key = String(issue.path[0] ?? '');
    if (key && !errors[key]) errors[key] = issue.message;
  }
  return errors;
}
