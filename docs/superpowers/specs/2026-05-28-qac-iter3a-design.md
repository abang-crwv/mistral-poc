# qac iteration 3a — template loader and template-aware run creation

**Date:** 2026-05-28
**Status:** Draft, pending user sign-off
**Predecessor specs:**
- `2026-05-27-qac-rebuild-design.md` (walking skeleton)
- `2026-05-28-qac-run-detail-design.md` (run-detail page + new-run modal)

**Branch:** `wp/add-qac` (continuing — no PR yet; deferred until canary actually verifies something)

---

## 1. Purpose

Introduce the **template** as a first-class object so the canary's shape is declared in YAML rather than baked into the form. The predecessor at `~/Documents/argo/jason/fwqual/` proved this model — a single `qac.template/v1` document captures inputs, steps, and per-step kinds (probe / review / ai_assess / signoff). Iter-3a ports that model into this rebuild, but **backend only**: templates are parsed, persisted, served over HTTP, and used to validate `POST /api/runs`. The UI is unchanged in this iteration; iter-3b will render template inputs and steps dynamically.

This iteration also refits the canonical template per the 2026-05-25 RLCC alignment: probe steps carry explicit `action:` and `checkpoint:` sub-objects, and the `capacity_check` and `alert_sweep` steps are removed from the rail (they become background functions in iter-4).

The canary still does no real work — but after iter-3a, a run is **created against a real template definition** with real input validation, and the iter-4 engine can read template steps directly.

## 2. Scope (what iter-3a ships)

1. **`templates/firmware-release-canary.yaml`** — canonical template authored fresh in this repo. RLCC-faithful schema. Canonical rack regex.
2. **`internal/template` package** — pure Go: `Parse([]byte) (Template, error)`, `Validate(Template) error`, plus types (`Template`, `Target`, `Input`, `Step`, `Action`, `Checkpoint`, `CheckpointBlocker`).
3. **Embedded templates.** `embed_dist.go` grows a second `//go:embed` for `templates/*.yaml`. The `internal/seed` package gets `LoadEmbeddedTemplates(ctx, store)` called at server boot — upserts every embedded template into the DB.
4. **`templates` table** + idempotent migration. Columns: `id TEXT PRIMARY KEY`, `version INTEGER NOT NULL`, `body_yaml TEXT NOT NULL`, `parsed_json TEXT NOT NULL`, `created_at INTEGER NOT NULL`, `updated_at INTEGER NOT NULL`.
5. **`runs.inputs` column** — `TEXT NOT NULL DEFAULT '{}'`, idempotent migration via `pragma_table_info` (same pattern as iter-2's bundle column). Stores the user-supplied input map as JSON.
6. **`internal/store/templates.go`** — `UpsertTemplate(ctx, t Template) error`, `ListTemplates(ctx) ([]TemplateSummary, error)`, `GetTemplate(ctx, id string) (Template, bool, error)`.
7. **`GET /api/templates`** — list of `{id, version, title}` summaries.
8. **`GET /api/templates/{id}`** — full parsed template as JSON; 404 `not_found` if absent.
9. **`POST /api/runs` evolution** — accepts both shapes:
   - **Legacy (iter-2):** `{ "bundle": "...", "rack": "dh3-r012-..." }`. Handler normalizes to `template_id="firmware-release-canary"`, `inputs={ bundle_tag: ..., canary_racks: [...] }`.
   - **New:** `{ "template_id": "...", "inputs": { ... } }`.
   After normalization the handler:
   - Looks up the template; returns **422 `template_not_found`** if unknown.
   - Validates `inputs` against `template.Inputs[]`; returns **400 `invalid_request`** with a message naming the failing input id on any violation.
   - Generates a ULID, appends `RunCreated` event (payload carries `template_id`, `template_version`, `inputs`).
   - Loads the projected run and returns 201 `{ run }`. If projection load fails after the event was successfully appended, returns **201 with a degraded body** `{ run: { id, created_at, status: "running" }, warning: "projection_unavailable" }` — never 500 after a successful side-effect.
10. **`qac template load <path>`** CLI subcommand — reads a YAML file, parses + validates, upserts to the DB, prints `{id, version}` to stdout.

Frontend is **not touched** in iter-3a. The existing new-run modal continues to POST the legacy `{bundle, rack}` shape, which the handler normalizes. Iter-3b will replace the modal with a dynamic form driven by the template definition.

## 3. Out of scope (deferred to iter-3b / iter-4)

- Dynamic rendering of template inputs in the new-run modal (iter-3b)
- Dynamic rendering of template steps as a rail in the run-detail page (iter-3b)
- Engine that actually advances steps (`engine.Advance`, action handlers, checkpoint evaluator) — iter-4
- Probe execution (no tool registry, no `cwf`/`flcc`/`rlcc`/`metrics` callers wired) — iter-4
- Template versioning UI / migration between template versions — later
- Multi-tenant template ownership — out forever (single-user sandbox tool)

## 4. Constraints carried forward

1. **Self-contained.** Port (don't import) from `~/Documents/argo/jason/fwqual/`. No `replace` directives. Module path remains bare `qac`.
2. **Single binary.** `make build` still produces one executable; embedded `templates/*.yaml` ship inside it.
3. **Cloud Console copy guide.** No new user-facing strings in iter-3a (UI untouched). Error messages from the API stay short and lowercase-first (matches iter-2's error envelope style):
   - `"unknown template_id: <id>"`
   - `"input <id> is required"`
   - `"input <id> has wrong type"`
   - `"input <id> failed validation"`
4. **Pinned stack.** One new top-level Go dependency: `gopkg.in/yaml.v3`. Vetted, last-modified 2024, used throughout Go ecosystem. No new frontend dependencies.

## 5. Template schema (`qac.template/v1`)

The canonical template document, after the 2026-05-25 RLCC refit:

```yaml
schema: qac.template/v1
id: fw-release-canary
version: 1
title: "FW release canary"

target:
  scope: rack
  instance_types: [gb200-4x, gb300-4x]

inputs:
  - id: bundle_tag
    label: "FW bundle tag"
    type: text
    required: true
    example: "dell-xe9712-default-2.0.0-rc"

  - id: instance_type
    label: "Instance type"
    type: enum
    options: [gb200-4x, gb300-4x]
    required: true

  - id: variant
    label: "GB generation"
    type: enum
    options: [gb200, gb300]
    required: true

  - id: canary_racks
    label: "Canary racks"
    type: multi_text
    required: true
    # Canonical CoreWeave rack format (post-2025-07-02 rename); mirrored in
    # internal/api/runs_validate.go (Go) and web/src/features/runs/newRunSchema.ts (TS)
    validate: "^dh\\d+-r\\d{3}-[a-z]+-[a-z]+-\\d{2}[a-z]$"

  - id: requester
    label: "Requester (from #havock)"
    type: text
    required: true

  - id: nitwit_receipts
    label: "nitwit receipt URLs"
    type: multi_url
    required: true

  - id: slack_thread
    label: "Slack thread URL for reply"
    type: url
    required: false

steps:
  - id: baseline_pre
    kind: probe
    title: "Baseline — pre-zap health snapshot"
    action:
      tool: metrics
      args:
        racks: "{{ inputs.canary_racks }}"
        mode: pre
      timeout: 5m
    checkpoint:
      targets: per_rack
      success_when: { stdout_contains: "baseline_ok" }
      blockers: []

  - id: zap_canary
    kind: probe
    title: "Zap canary rack(s) — FLCC node-zap workflow"
    action:
      tool: flcc
      multi_rack: true
      parallelism: all
      args: ["workflow", "run", "node-zap",
             "--rack", "{{ rack }}",
             "--bundle", "{{ inputs.bundle_tag }}",
             "--json"]
      timeout: 60m
    checkpoint:
      targets: per_rack
      success_when: { json_field: "status", equals: "completed" }
      fail_when:    { json_field: "status", equals: "failed" }
      blockers:
        - kind: ActionRequired
          when: { json_field: "status", equals: "halted" }
          message: "node-zap halted; manual intervention required"

  - id: l10_fielddiag
    kind: probe
    title: "L10 fielddiag — NVIDIA single-tray GPU screening"
    action:
      tool: flcc
      args: ["state", "report", "--racks", "{{ inputs.canary_racks | join:',' }}",
             "--state", "l10-fielddiag", "--json"]
      timeout: 18h
    checkpoint:
      targets: per_rack
      success_when: { json_field: "state.completed", equals: true }

  - id: l10_check
    kind: probe
    title: "L10 test — node-level HPC verification"
    action:
      tool: flcc
      args: ["state", "report", "--racks", "{{ inputs.canary_racks | join:',' }}",
             "--state", "l10-test", "--include-alerts", "--json"]
      timeout: 9h
    checkpoint:
      targets: per_rack
      success_when: { json_field: "state.completed", equals: true }

  - id: l11_fielddiag
    kind: probe
    title: "L11 fielddiag — whole-rack NVLink-domain diagnostic"
    action:
      tool: rlcc
      args: ["state", "report", "--racks", "{{ inputs.canary_racks | join:',' }}",
             "--state", "l11-fielddiag", "--json"]
      timeout: 4h
    checkpoint:
      targets: per_rack
      success_when: { json_field: "state.completed", equals: true }

  - id: l11_check
    kind: probe
    title: "L11 test — NVLink-domain group test"
    action:
      tool: rlcc
      args: ["state", "report", "--racks", "{{ inputs.canary_racks | join:',' }}",
             "--state", "l11-test", "--include-alerts", "--json"]
      timeout: 6h
    checkpoint:
      targets: per_rack
      success_when: { json_field: "state.completed", equals: true }

  - id: l12_seatrial
    kind: probe
    title: "L12 seatrial — backend misconnect screen"
    action:
      tool: rlcc
      args: ["state", "report", "--racks", "{{ inputs.canary_racks | join:',' }}",
             "--state", "l12-seatrial", "--json"]
      timeout: 1h
    checkpoint:
      targets: per_rack
      success_when: { json_field: "state.completed", equals: true }

  - id: l12_check
    kind: probe
    title: "L12 test — backend IB/RoCE fabric performance"
    action:
      tool: rlcc
      args: ["state", "report", "--racks", "{{ inputs.canary_racks | join:',' }}",
             "--state", "l12-test", "--include-alerts", "--json"]
      timeout: 6h
    checkpoint:
      targets: per_rack
      success_when: { json_field: "state.completed", equals: true }

  - id: baseline_post
    kind: probe
    title: "Baseline — post-zap delta vs baseline_pre"
    action:
      tool: metrics
      args:
        racks: "{{ inputs.canary_racks }}"
        mode: post
        compare_to: "step:baseline_pre"
      timeout: 5m
    checkpoint:
      targets: per_rack
      success_when: { stdout_contains: "delta_within_threshold" }

  - id: evidence_review
    kind: review
    title: "Per-rack evidence review"
    sources:
      - baseline_pre
      - zap_canary
      - l10_fielddiag
      - l10_check
      - l11_fielddiag
      - l11_check
      - l12_seatrial
      - l12_check
      - baseline_post
    enrichment:
      patterns: true
      glean: true
    optional_inputs:
      - id: rack_notes
        type: textarea
        scope: per_rack
        required: false

  - id: ai_summary
    kind: ai_assess
    title: "AI assessment of canary outcome"
    prompt_ref: fw-release-canary.summary.v1
    context_steps: [baseline_pre, zap_canary,
                    l10_fielddiag, l10_check, l11_fielddiag, l11_check,
                    l12_seatrial, l12_check, baseline_post]
    respect_suppressions: true
    cite_patterns: true
    cite_glean: true
    expected_verdict: [pass, fail, needs_review]

  - id: operator_signoff
    kind: signoff
    title: "Operator sign-off"
    final_verdict_options: [pass, fail]
    fields:
      - { id: signer_name, label: "Operator name", type: text,     required: true }
      - { id: signer_role, label: "Role",          type: text,     required: true }
      - { id: final_notes, label: "Final notes",   type: textarea, required: false }
    slack_reply: true
```

**Differences from predecessor:**
- `capacity_check` removed from rail (becomes background function in iter-4).
- `alert_sweep` removed from rail (becomes background function in iter-4).
- Each probe step gains explicit `action:` and `checkpoint:` sub-objects (RLCC-faithful). The predecessor's flat `tool/args/capture/timeout` are collapsed into `action:`; `checkpoint:` is new.
- `canary_racks.validate` uses canonical CoreWeave rack regex (mirrored from iter-2's `rackPattern`).

## 6. Go types (`internal/template/types.go`)

```go
type Template struct {
    Schema    string  `yaml:"schema"  json:"schema"`
    ID        string  `yaml:"id"      json:"id"`
    Version   int     `yaml:"version" json:"version"`
    Title     string  `yaml:"title"   json:"title"`
    Target    Target  `yaml:"target"  json:"target"`
    Inputs    []Input `yaml:"inputs"  json:"inputs"`
    Steps     []Step  `yaml:"steps"   json:"steps"`
}

type Target struct {
    Scope         string   `yaml:"scope"          json:"scope"`           // "rack" | "node" | "cluster"
    InstanceTypes []string `yaml:"instance_types" json:"instance_types"`
}

type Input struct {
    ID       string   `yaml:"id"       json:"id"`
    Label    string   `yaml:"label"    json:"label"`
    Type     string   `yaml:"type"     json:"type"`     // text | multi_text | enum | url | multi_url | textarea
    Required bool     `yaml:"required" json:"required"`
    Options  []string `yaml:"options,omitempty"  json:"options,omitempty"`
    Validate string   `yaml:"validate,omitempty" json:"validate,omitempty"` // regex
    Example  string   `yaml:"example,omitempty"  json:"example,omitempty"`
}

type Step struct {
    ID    string `yaml:"id"    json:"id"`
    Kind  string `yaml:"kind"  json:"kind"`  // probe | review | ai_assess | signoff
    Title string `yaml:"title" json:"title"`

    // probe-only
    Action     *Action     `yaml:"action,omitempty"     json:"action,omitempty"`
    Checkpoint *Checkpoint `yaml:"checkpoint,omitempty" json:"checkpoint,omitempty"`

    // review-only
    Sources        []string         `yaml:"sources,omitempty"         json:"sources,omitempty"`
    Enrichment     map[string]bool  `yaml:"enrichment,omitempty"      json:"enrichment,omitempty"`
    OptionalInputs []Input          `yaml:"optional_inputs,omitempty" json:"optional_inputs,omitempty"`

    // ai_assess-only
    PromptRef           string   `yaml:"prompt_ref,omitempty"           json:"prompt_ref,omitempty"`
    ContextSteps        []string `yaml:"context_steps,omitempty"        json:"context_steps,omitempty"`
    RespectSuppressions bool     `yaml:"respect_suppressions,omitempty" json:"respect_suppressions,omitempty"`
    CitePatterns        bool     `yaml:"cite_patterns,omitempty"        json:"cite_patterns,omitempty"`
    CiteGlean           bool     `yaml:"cite_glean,omitempty"           json:"cite_glean,omitempty"`
    ExpectedVerdict     []string `yaml:"expected_verdict,omitempty"     json:"expected_verdict,omitempty"`

    // signoff-only
    FinalVerdictOptions []string `yaml:"final_verdict_options,omitempty" json:"final_verdict_options,omitempty"`
    Fields              []Input  `yaml:"fields,omitempty"                json:"fields,omitempty"`
    SlackReply          bool     `yaml:"slack_reply,omitempty"           json:"slack_reply,omitempty"`
}

type Action struct {
    Tool        string         `yaml:"tool"                  json:"tool"`
    MultiRack   bool           `yaml:"multi_rack,omitempty"  json:"multi_rack,omitempty"`
    Parallelism string         `yaml:"parallelism,omitempty" json:"parallelism,omitempty"`
    Args        any            `yaml:"args,omitempty"        json:"args,omitempty"` // []string or map[string]any
    Timeout     string         `yaml:"timeout,omitempty"     json:"timeout,omitempty"`
}

type Checkpoint struct {
    Targets     string             `yaml:"targets"                 json:"targets"`     // "per_rack" | "rollup"
    SuccessWhen map[string]any     `yaml:"success_when,omitempty"  json:"success_when,omitempty"`
    FailWhen    map[string]any     `yaml:"fail_when,omitempty"     json:"fail_when,omitempty"`
    Blockers    []CheckpointBlocker `yaml:"blockers,omitempty"     json:"blockers,omitempty"`
}

type CheckpointBlocker struct {
    Kind    string         `yaml:"kind"            json:"kind"`    // "Informational" | "ActionRequired"
    When    map[string]any `yaml:"when"            json:"when"`
    Message string         `yaml:"message"         json:"message"`
}
```

## 7. Validation rules (`internal/template/validate.go`)

`Validate(t Template) error` checks:

1. `schema == "qac.template/v1"`.
2. `id` non-empty and matches `^[a-z0-9-]+$`.
3. `version >= 1`.
4. `title` non-empty.
5. `target.scope` in `{rack, node, cluster}`.
6. All `inputs[].id` non-empty and unique across the template.
7. `inputs[].type` in `{text, multi_text, enum, url, multi_url, textarea}`.
8. If `type == enum`, `options` is non-empty.
9. If `validate` set, regex compiles via `regexp.Compile`.
10. At least one `step`.
11. All `steps[].id` non-empty, unique, and match `^[a-z0-9_]+$`.
12. `steps[].kind` in `{probe, review, ai_assess, signoff}`.
13. If `kind == probe`: both `action` and `checkpoint` present; `action.tool` non-empty; `checkpoint.targets` in `{per_rack, rollup}`.
14. If `kind == review`: `sources` non-empty; every entry refers to a probe step earlier in `steps[]`.
15. If `kind == ai_assess`: `prompt_ref` non-empty; `context_steps` (if set) all refer to probe steps.
16. If `kind == signoff`: `final_verdict_options` non-empty; `fields[]` ids unique within the step.

Errors are returned as `fmt.Errorf("template %s: ...", t.ID)` so the caller can wrap them.

## 8. Input validation (`internal/api/runs_validate.go`)

New function `ValidateInputs(tpl template.Template, raw map[string]any) (errMsg string)` (returns `""` on success — mirrors iter-2's `ValidateRacks` shape):

For each declared `Input`:
- If `Required` and `raw[id]` absent or zero-value → `"input <id> is required"`.
- Type check:
  - `text`, `url`, `textarea` → must be `string`.
  - `multi_text`, `multi_url` → must be `[]any` of strings.
  - `enum` → must be `string` in `Options`.
- Mismatch → `"input <id> has wrong type"` or `"input <id> not in allowed options"`.
- If `Validate` regex set and value is `string` (or each element of `multi_text`) → regex match → `"input <id> failed validation"`.

Extra keys in `raw` that are not declared in `tpl.Inputs` are **silently ignored** (forward-compatible — old templates can still accept new optional inputs from the UI without erroring).

## 9. Database changes

### 9.1 New `templates` table

```sql
CREATE TABLE IF NOT EXISTS templates (
    id          TEXT PRIMARY KEY,
    version     INTEGER NOT NULL,
    body_yaml   TEXT NOT NULL,
    parsed_json TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);
```

- `body_yaml` is the original YAML (so we can show "source" later).
- `parsed_json` is `json.Marshal(template.Template{...})` for cheap reads — avoids re-parsing on every `GET /api/templates/{id}`.

### 9.2 `runs.inputs` column

Idempotent migration following iter-2's pattern:

```go
// addInputsColumnIfMissing checks pragma_table_info before ALTER TABLE.
// SQLite doesn't support ADD COLUMN IF NOT EXISTS, so we introspect first.
var n int
err := db.QueryRowContext(ctx,
    `SELECT count(*) FROM pragma_table_info('runs') WHERE name = 'inputs'`,
).Scan(&n)
// if n == 0, ALTER TABLE runs ADD COLUMN inputs TEXT NOT NULL DEFAULT '{}'
```

### 9.3 `RunCreated` event payload evolution

Before (iter-2):
```json
{ "bundle": "...", "rack": "dh3-r012-us-east-01a", "creator": "wpena" }
```

After (iter-3a):
```json
{
  "template_id": "firmware-release-canary",
  "template_version": 1,
  "inputs": {
    "bundle_tag": "dell-xe9712-default-2.0.0-rc",
    "canary_racks": ["dh3-r012-us-east-01a"]
  },
  "creator": "wpena"
}
```

The projection (`engine.Project`) derives `runs.bundle` from `inputs.bundle_tag` (if present) and `runs.rack` from `strings.Join(inputs.canary_racks, ",")` (if present) — preserving the iter-2 columns so the existing UI keeps working unchanged. `runs.inputs` stores the full map as JSON for future use.

**Backward-compat with iter-2 events:** existing iter-2 `RunCreated` events carry the flat `{bundle, rack}` shape. The projection treats those as the source of truth for `runs.bundle` and `runs.rack` when present (no `inputs` map in the payload), so iter-2 rows continue to project correctly after the migration.

**Convention, not contract:** the `bundle_tag` → `runs.bundle` and `canary_racks` → `runs.rack` mapping is a `fw-release-canary`-specific convention. Future templates with different input ids would need either matching ids or a per-template projection rule — out of scope for iter-3a (only one template ships).

## 10. HTTP API

### 10.1 `GET /api/templates`

```http
200 OK
{
  "templates": [
    { "id": "firmware-release-canary", "version": 1, "title": "FW release canary" }
  ]
}
```

### 10.2 `GET /api/templates/{id}`

```http
200 OK
{ "template": { ...full Template JSON... } }
```

```http
404 Not Found
{ "error": { "code": "not_found", "message": "template not found" } }
```

### 10.3 `POST /api/runs` (evolved)

**Body shape (legacy, still accepted):**
```json
{ "bundle": "dell-xe9712-default-2.0.0-rc", "rack": "dh3-r012-us-east-01a" }
```
Normalized at the handler edge to:
```json
{
  "template_id": "firmware-release-canary",
  "inputs": {
    "bundle_tag": "dell-xe9712-default-2.0.0-rc",
    "canary_racks": ["dh3-r012-us-east-01a"]
  }
}
```

**Body shape (new):**
```json
{
  "template_id": "firmware-release-canary",
  "inputs": {
    "bundle_tag": "...",
    "canary_racks": ["dh3-r012-...", "dh3-r013-..."],
    "instance_type": "gb200-4x",
    "variant": "gb200",
    "requester": "alice",
    "nitwit_receipts": ["https://..."]
  }
}
```

**Status codes:**
- `201 Created` — success. Body: `{ "run": { ... } }`.
- `201 Created` — degraded. Body: `{ "run": { "id", "created_at", "status": "running" }, "warning": "projection_unavailable" }`. Returned only when the `RunCreated` event was successfully appended but the immediate projection read failed (rare; client can refetch).
- `400 Bad Request` — `{ "error": { "code": "invalid_request", "message": "<details>" } }`. Used for malformed JSON, requests carrying both legacy keys (`bundle`/`rack`) and new keys (`template_id`/`inputs`) simultaneously, and `ValidateInputs` failures.
- `404 Not Found` — only used by `GET /api/templates/{id}` and `GET /api/runs/{id}` (unchanged from iter-2).
- **`422 Unprocessable Entity`** — `{ "error": { "code": "template_not_found", "message": "unknown template_id: <id>" } }`. New code, used only by `POST /api/runs` when `template_id` resolves to no template. 422 is the conventional status for "request was well-formed but semantically referenced a non-existent resource that the client should know about".
- `500 Internal Server Error` — only for genuinely unexpected failures (DB unavailable, embed corrupted). The 201-degraded path replaces the previous "500 after side-effect" failure mode.

## 11. CLI

### `qac template load <path>`

```text
$ qac template load templates/firmware-release-canary.yaml
loaded fw-release-canary v1
```

Reads the file from disk, calls `template.Parse` + `template.Validate`, then `store.UpsertTemplate(ctx, t)`. Exit code 0 on success, 1 on any error with the error message on stderr.

This subcommand is registered under the existing root cobra command in `cmd/qac/main.go`.

## 12. Boot sequence

1. `cmd/qac/main.go` opens the SQLite DB.
2. Runs migrations (now includes `addInputsColumnIfMissing` + `createTemplatesTableIfMissing`).
3. Calls `seed.LoadEmbeddedTemplates(ctx, store)`:
   - Walks `templates/*.yaml` from `TemplatesFS` (embed.FS).
   - For each file: parse, validate, `UpsertTemplate`.
   - Logs `loaded template <id> v<n>` per template.
4. Starts the HTTP server.

`LoadEmbeddedTemplates` always upserts (idempotent), so a stale-template scenario (developer edits the YAML, rebuilds, restarts) just refreshes the DB row.

## 13. File map

**New files:**
- `templates/firmware-release-canary.yaml` — canonical canary template (the full doc from §5)
- `internal/template/types.go` — Template, Target, Input, Step, Action, Checkpoint, CheckpointBlocker
- `internal/template/parse.go` — `Parse([]byte) (Template, error)` (yaml.Unmarshal)
- `internal/template/parse_test.go`
- `internal/template/validate.go` — `Validate(Template) error`
- `internal/template/validate_test.go`
- `internal/store/templates.go` — `UpsertTemplate`, `ListTemplates`, `GetTemplate`, `TemplateSummary`
- `internal/store/templates_test.go`
- `internal/api/templates.go` — `listTemplatesHandler`, `getTemplateHandler`
- `internal/api/templates_test.go`
- `cmd/qac/template.go` — `template load <path>` subcommand

**Modified files:**
- `embed_dist.go` — add `//go:embed templates/*.yaml` for `TemplatesFS`
- `go.mod` / `go.sum` — add `gopkg.in/yaml.v3`
- `internal/store/migrations.go` — `addInputsColumnIfMissing` + `createTemplatesTableIfMissing`
- `internal/store/migrations_test.go` — idempotency tests for both new migrations
- `internal/store/runs.go` — add `Inputs map[string]any` field to `Run`; update `ListRuns`/`GetRun` SELECTs
- `internal/store/events.go` — `runCreatedPayload` grows `TemplateID`, `TemplateVersion`, `Inputs`; projection derives `Bundle` + `Rack` from `Inputs`
- `internal/api/api.go` — register `GET /api/templates`, `GET /api/templates/{id}`
- `internal/api/runs.go` — `createRunHandler` gains `normalize()` helper; validates template + inputs; new 422 + 201-degraded paths
- `internal/api/runs_validate.go` — add `ValidateInputs(tpl, raw) string`
- `internal/api/api_test.go` — updated POST tests (covered in §14)
- `internal/seed/seed.go` — add `LoadEmbeddedTemplates(ctx, store)`
- `cmd/qac/main.go` — call `LoadEmbeddedTemplates` after migrations; register `template` subcommand

## 14. Testing

**`internal/template` package (~30 subtests):**
- Parse:
  - Round-trip the canonical YAML (assert id, version, title, len(inputs), len(steps)).
  - Unknown top-level field — parse should succeed (yaml.v3 default).
  - Malformed YAML — Parse returns error.
  - Empty input — Parse returns error.
- Validate (table-driven):
  - Happy path on canonical template.
  - Missing schema, wrong schema string.
  - Missing id, bad id chars.
  - Version < 1.
  - Empty title.
  - Bad target.scope.
  - Duplicate input ids.
  - Bad input type.
  - Enum without options.
  - Bad validate regex.
  - Zero steps.
  - Duplicate step ids.
  - Bad step kind.
  - Probe step missing action.
  - Probe step missing checkpoint.
  - Probe step bad checkpoint.targets.
  - Review step empty sources.
  - Review step references nonexistent probe.
  - Review step references later step (out of order).
  - ai_assess step missing prompt_ref.
  - signoff step empty final_verdict_options.
  - signoff step duplicate field ids.

**`internal/store` (+5 tests):**
- `TestMigrate_AddsInputsColumn` — runs on a v0 DB (no inputs col), asserts col present after.
- `TestMigrate_AddsInputsColumn_Idempotent` — runs twice; second run is a no-op.
- `TestMigrate_CreatesTemplatesTable_Idempotent` — runs twice; no error.
- `TestUpsertTemplate_InsertAndUpdate` — insert v1, upsert with same id v2; assert version updated.
- `TestListTemplates_And_GetTemplate` — seeds two templates, asserts list len and round-trip Get.

**`internal/api` (+12 tests):**
- `TestGetTemplates_ReturnsSeeded` — boots with seeded canonical template, list returns one summary.
- `TestGetTemplate_HappyPath` — full doc returned, fields match seed.
- `TestGetTemplate_NotFound` — 404, error envelope `not_found`.
- `TestCreateRun_LegacyShape_Happy` — `{bundle, rack}` body; 201; run created with normalized inputs.
- `TestCreateRun_LegacyShape_BadRack` — 400 invalid_request.
- `TestCreateRun_LegacyShape_EmptyBundle` — 400 invalid_request.
- `TestCreateRun_NewShape_Happy` — `{template_id, inputs}` body; 201.
- `TestCreateRun_NewShape_UnknownTemplate` — 422 template_not_found.
- `TestCreateRun_NewShape_MissingRequiredInput` — 400 invalid_request with "input X is required".
- `TestCreateRun_NewShape_WrongInputType` — 400 invalid_request with "input X has wrong type".
- `TestCreateRun_NewShape_RackRegexFailure` — 400 invalid_request with "input canary_racks failed validation".
- `TestValidateInputs_Table` — table-driven covering the type matrix (text, multi_text, enum, url, multi_url, textarea).

**Frontend tests:** unchanged at 18 — UI is not modified in iter-3a.

**Total target:** ≥76 tests (47 Go new + 21 Go iter-2 baseline + ~8 Go iter-1 baseline + 18 Vitest).

## 15. Acceptance criteria

1. `make build` produces a binary with `templates/firmware-release-canary.yaml` embedded (verify by `strings team/wpena/bin/qac | grep fw-release-canary`).
2. `make lint`, `make test` all green.
3. `make dev` boots; the embedded template is loaded into `templates` table on first start.
4. `qac template load templates/firmware-release-canary.yaml` prints `loaded fw-release-canary v1` and exits 0.
5. `curl /api/templates` returns the seeded template summary.
6. `curl /api/templates/fw-release-canary` returns the full parsed template.
7. `curl /api/templates/unknown` returns 404 `not_found`.
8. `POST /api/runs` with legacy `{bundle, rack}` still works (created run has normalized inputs in DB).
9. `POST /api/runs` with `{template_id: "firmware-release-canary", inputs: {...}}` creates a run.
10. `POST /api/runs` with unknown `template_id` returns 422 `template_not_found`.
11. `POST /api/runs` with input violating template schema (e.g., bad rack regex, missing required) returns 400 `invalid_request` naming the failing input id.
12. On post-create projection failure, response is 201 with `warning: "projection_unavailable"` body (covered by injected store stub in test; not an acceptance path the user exercises live).
13. Migration runs idempotently against an iter-2 DB — runs.inputs column is added if missing, templates table is created, no data loss in `runs` rows.
14. The iter-2 new-run modal in the UI still works unchanged (legacy `{bundle, rack}` shape passes through the normalize() path).

## 16. Risks and mitigations

- **YAML parsing surface.** `yaml.v3` handles a lot of corner cases (anchors, tagged scalars). Mitigation: validation pass is the gate, not the parse pass — even if YAML quirks let an odd doc through, `Validate` catches it.
- **Migration on existing DBs.** The iter-2 DB has runs without `inputs`. The `DEFAULT '{}'` clause + idempotent ALTER ensures backfill is trivial. New `inputs` will be empty `{}` for old rows; the projection treats missing inputs as `{}` so bundle/rack derivation falls back to the legacy `RunCreated` payload fields.
- **Projection failure as 201-degraded.** The semantics are "the event went in but we couldn't read it back immediately." This is rare but acknowledges the underlying truth: once the event is appended, the side-effect is durable. Returning 500 would be lying about durability.
- **Forward-compat with iter-3b.** The `Inputs map[string]any` field is structurally identical to what the dynamic form will produce. No further schema churn expected when iter-3b lands.
