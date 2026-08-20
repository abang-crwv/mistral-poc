package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"qac/internal/verdict"
)

// runCreatedPayload mirrors the JSON shape of the RunCreated event.
// Both shapes are tolerated:
//
//	legacy (iter-2):  {bundle, rack, template_id, created_by}
//	new   (iter-3a):  {template_id, inputs:{bundle_tag, canary_racks, ...}, created_by}
//
// When `inputs` is present the projection derives bundle/rack from it;
// otherwise it falls back to the flat fields so iter-2 events keep
// projecting correctly after migration.
type runCreatedPayload struct {
	TemplateID string          `json:"template_id"`
	Bundle     string          `json:"bundle"`
	Rack       string          `json:"rack"`
	CreatedBy  string          `json:"created_by"`
	Inputs     json.RawMessage `json:"inputs"`
	// RLCCWorkflow is the iter-5d snapshot — engine.RunCreatedPayload has
	// the typed *rlccclient.WorkflowSnapshot form. store passes the bytes
	// through opaquely (no rlccclient import); the projection in
	// AppendEvent doesn't read it. omitempty on a pointer means pre-iter-5d
	// rows decode to nil cleanly.
	RLCCWorkflow *json.RawMessage `json:"rlcc_workflow,omitempty"`
}

// factsDiscoveredPayload mirrors the wire shape of FactsDiscovered.
// `Facts` is a map of registered fact-key strings to arbitrary JSON
// values; the projection JSON-encodes each value before UPSERT so the
// store's TEXT column is always valid JSON.
type factsDiscoveredPayload struct {
	Scope  string         `json:"scope"`
	Source string         `json:"source"`
	Facts  map[string]any `json:"facts"`
}

// factsDiscoveryFailedPayload mirrors the wire shape of
// FactsDiscoveryFailed. iter-4a only emits this at scope="run" for
// inventory hard-fail; later iterations may emit at rack/node scope
// for soft-fail probe semantics.
type factsDiscoveryFailedPayload struct {
	Scope      string   `json:"scope"`
	Source     string   `json:"source"`
	Error      string   `json:"error"`
	Unresolved []string `json:"unresolved"`
}

// iter-5b payload types — mirrored from engine.go to avoid an import
// cycle with internal/engine. JSON tags must match.
type bmnDiscoveredPayload struct {
	Rack         string         `json:"rack"`
	Zone         string         `json:"zone"`
	Source       string         `json:"source"`
	DiscoveredAt int64          `json:"discovered_at"`
	BMNs         []bmnDiscEntry `json:"bmns"`
}
type bmnDiscEntry struct {
	Deviceslot string `json:"deviceslot"`
	BMNName    string `json:"bmn_name"`
	CTPosition int    `json:"ct_position,omitempty"`
}
type bmnDiscoveryFailedPayload struct {
	Rack    string `json:"rack"`
	Source  string `json:"source"`
	Message string `json:"message"`
}
type bmnIgnoredPayload struct {
	Rack       string `json:"rack"`
	Deviceslot string `json:"deviceslot"`
	Source     string `json:"source"`
	Reason     string `json:"reason,omitempty"`
	MarkedAt   int64  `json:"marked_at"`
}

// stepStartedPayload mirrors the wire shape of StepStarted (emitted by
// internal/engine). Decoded inside AppendEvent so a malformed payload
// rolls back the transaction.
type stepStartedPayload struct {
	StepID    string   `json:"step_id"`
	StepType  string   `json:"step_type,omitempty"`
	Racks     []string `json:"racks,omitempty"`
	StartedAt int64    `json:"started_at,omitempty"`
}

// stepCompletedPayload mirrors the wire shape of StepCompleted. iter-4b
// gatherers leave Verdict empty; iter-4c+ assertion probes populate it.
// Both projections (store and engine.Project) branch on Verdict via the
// shared internal/verdict package to advance worst_verdict and run status.
type stepCompletedPayload struct {
	StepID      string   `json:"step_id"`
	EvidenceID  string   `json:"evidence_id,omitempty"`
	DurationMS  int64    `json:"duration_ms,omitempty"`
	CompletedAt int64    `json:"completed_at,omitempty"`
	Sources     []string `json:"sources,omitempty"`
	Verdict     string   `json:"verdict,omitempty"`
}

// stepFailedPayload mirrors the wire shape of StepFailed. Triggers a
// runs.status flip to 'failed'.
type stepFailedPayload struct {
	StepID     string `json:"step_id"`
	Error      string `json:"error"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	FailedAt   int64  `json:"failed_at,omitempty"`
}

// operatorActionPayload mirrors the wire shape of OperatorAction. Action
// "signoff" pins the run status to signed_off (terminal).
type operatorActionPayload struct {
	StepID string `json:"step_id"`
	Action string `json:"action"`
}

// AppendEvent inserts an event and updates the runs projection in one
// transaction.
func (s *Store) AppendEvent(ctx context.Context, runID, kind string, payload []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	switch kind {
	case "RunCreated":
		var p runCreatedPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("decode RunCreated payload: %w", err)
		}
		createdBy := p.CreatedBy
		if createdBy == "" {
			createdBy = "wpena"
		}

		// Derive bundle + rack from inputs when present. The mapping
		// (bundle_tag -> bundle, canary_racks -> rack) is convention
		// for the canonical canary template; future templates with
		// different input ids would need either matching ids or a
		// per-template projection rule.
		bundle, rack := p.Bundle, p.Rack
		inputsJSON := "{}"
		if len(p.Inputs) > 0 {
			inputsJSON = string(p.Inputs)
			var raw map[string]any
			if err := json.Unmarshal(p.Inputs, &raw); err != nil {
				return fmt.Errorf("decode RunCreated.inputs: %w", err)
			}
			if v, ok := raw["bundle_tag"].(string); ok && v != "" {
				bundle = v
			}
			if v, ok := raw["canary_racks"].([]any); ok {
				parts := make([]string, 0, len(v))
				for _, item := range v {
					if s, ok := item.(string); ok && s != "" {
						parts = append(parts, s)
					}
				}
				if len(parts) > 0 {
					rack = strings.Join(parts, ",")
				}
			}
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO runs (id, template_id, bundle, created_at, updated_at, status, rack, created_by, inputs)
			VALUES (?, ?, ?, ?, ?, 'pending', ?, ?, ?)
			ON CONFLICT(id) DO NOTHING`,
			runID, p.TemplateID, bundle, now, now, rack, createdBy, inputsJSON,
		); err != nil {
			return fmt.Errorf("insert run: %w", err)
		}

	case "StepStarted":
		var p stepStartedPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("decode StepStarted payload: %w", err)
		}
		_ = p // shape validated; projection below uses none of the fields
		// Guard: do not downgrade a terminal signed_off run back to running.
		// Mirrors the engine.Project guard: if st.Status != StatusSignedOff { st.Status = StatusRunning }.
		if _, err := tx.ExecContext(ctx, `
			UPDATE runs SET status = 'running', updated_at = ? WHERE id = ? AND status NOT IN (?, ?)`,
			now, runID, verdict.StatusSignedOff, verdict.StatusCancelled,
		); err != nil {
			return fmt.Errorf("update run (StepStarted): %w", err)
		}

	case "StepCompleted":
		var p stepCompletedPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("decode StepCompleted payload: %w", err)
		}
		var curWorst string
		if err := tx.QueryRowContext(ctx, `SELECT worst_verdict FROM runs WHERE id = ?`, runID).Scan(&curWorst); err != nil {
			return fmt.Errorf("read worst_verdict (StepCompleted): %w", err)
		}
		nw := verdict.Worse(curWorst, p.Verdict)
		if _, err := tx.ExecContext(ctx,
			`UPDATE runs SET worst_verdict = ?, status = ?, updated_at = ? WHERE id = ? AND status NOT IN (?, ?)`,
			nw, verdict.ToStatus(nw), now, runID, verdict.StatusSignedOff, verdict.StatusCancelled); err != nil {
			return fmt.Errorf("update run (StepCompleted): %w", err)
		}

	case "RunCompleted":
		// The walk ended with no operator gate. Finalize a non-signed-off
		// run from its accumulated worst_verdict: empty worst (every step a
		// gatherer, no failure) => passed; otherwise the verdict's status.
		// Without this a baseline-only run sits at 'running' forever.
		var curWorst string
		if err := tx.QueryRowContext(ctx, `SELECT worst_verdict FROM runs WHERE id = ?`, runID).Scan(&curWorst); err != nil {
			return fmt.Errorf("read worst_verdict (RunCompleted): %w", err)
		}
		final := verdict.StatusPassed
		if curWorst != "" {
			final = verdict.ToStatus(curWorst)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE runs SET status = ?, updated_at = ? WHERE id = ? AND status NOT IN (?, ?)`,
			final, now, runID, verdict.StatusSignedOff, verdict.StatusCancelled,
		); err != nil {
			return fmt.Errorf("update run (RunCompleted): %w", err)
		}
		// Fall through to tail event INSERT.

	case "RunCancelled":
		// Operator cancelled — terminal, overrides any prior status.
		if _, err := tx.ExecContext(ctx,
			`UPDATE runs SET status = ?, updated_at = ? WHERE id = ?`,
			verdict.StatusCancelled, now, runID,
		); err != nil {
			return fmt.Errorf("update run (RunCancelled): %w", err)
		}
		// Fall through to tail event INSERT.

	case "StepFailed":
		var p stepFailedPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("decode StepFailed payload: %w", err)
		}
		_ = p // shape validated; projection below uses none of the fields
		if _, err := tx.ExecContext(ctx,
			`UPDATE runs SET status = ?, worst_verdict = ?, updated_at = ? WHERE id = ? AND status NOT IN (?, ?)`,
			verdict.StatusFailed, verdict.Failed, now, runID, verdict.StatusSignedOff, verdict.StatusCancelled,
		); err != nil {
			return fmt.Errorf("update run (StepFailed): %w", err)
		}
		// Fall through to tail event INSERT.

	case "OperatorAction":
		var p operatorActionPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("decode OperatorAction payload: %w", err)
		}
		if p.Action == "signoff" {
			if _, err := tx.ExecContext(ctx,
				`UPDATE runs SET status = ?, updated_at = ? WHERE id = ?`,
				verdict.StatusSignedOff, now, runID); err != nil {
				return fmt.Errorf("update run (OperatorAction signoff): %w", err)
			}
		} else {
			if _, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = ? WHERE id = ?`, now, runID); err != nil {
				return fmt.Errorf("update run (OperatorAction): %w", err)
			}
		}

	case "FactsDiscovered":
		var p factsDiscoveredPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("decode FactsDiscovered payload: %w", err)
		}
		// We need the event row's autoincrement id to populate
		// facts.source_event_id. Do the events INSERT here (inside
		// the same tx), use res.LastInsertId() to recover the id,
		// then UPSERT each fact. Return early so the tail INSERT
		// (below the switch) doesn't double-insert this event kind.
		res, err := tx.ExecContext(ctx, `
			INSERT INTO events (run_id, seq, kind, payload, occurred_at)
			VALUES (?, COALESCE((SELECT MAX(seq) FROM events WHERE run_id = ?), 0) + 1, ?, ?, ?)`,
			runID, runID, kind, string(payload), now,
		)
		if err != nil {
			return fmt.Errorf("insert event (FactsDiscovered): %w", err)
		}
		evtID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("last insert id (FactsDiscovered): %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE runs SET updated_at = ? WHERE id = ?`,
			now, runID,
		); err != nil {
			return fmt.Errorf("update run (FactsDiscovered): %w", err)
		}
		for k, v := range p.Facts {
			valueJSON, err := json.Marshal(v)
			if err != nil {
				return fmt.Errorf("marshal fact value (%s): %w", k, err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO facts (run_id, scope, key, value, source, source_event_id, discovered_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(run_id, scope, key) DO UPDATE SET
				  value           = excluded.value,
				  source          = excluded.source,
				  source_event_id = excluded.source_event_id,
				  discovered_at   = excluded.discovered_at`,
				runID, p.Scope, k, string(valueJSON), p.Source, evtID, now,
			); err != nil {
				return fmt.Errorf("upsert fact (%s,%s,%s): %w", runID, p.Scope, k, err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
		return nil

	case "FactsDiscoveryFailed":
		var p factsDiscoveryFailedPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("decode FactsDiscoveryFailed payload: %w", err)
		}
		_ = p // payload is stored verbatim via the tail INSERT; decode validates shape
		// Pin worst_verdict=failed (not just status) so a later StepCompleted
		// can't roll the worst-wins rollup back to passed. signed_off-guarded
		// to match engine.Project and the StepFailed case.
		if _, err := tx.ExecContext(ctx,
			`UPDATE runs SET status = ?, worst_verdict = ?, updated_at = ? WHERE id = ? AND status NOT IN (?, ?)`,
			verdict.StatusFailed, verdict.Failed, now, runID, verdict.StatusSignedOff, verdict.StatusCancelled,
		); err != nil {
			return fmt.Errorf("update run (FactsDiscoveryFailed): %w", err)
		}
		// Fall through to tail event INSERT.

	case "BMNDiscovered":
		var p bmnDiscoveredPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("decode BMNDiscovered payload: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO events (run_id, seq, kind, payload, occurred_at)
			VALUES (?, COALESCE((SELECT MAX(seq) FROM events WHERE run_id = ?), 0) + 1, ?, ?, ?)`,
			runID, runID, kind, string(payload), now,
		); err != nil {
			return fmt.Errorf("insert event (BMNDiscovered): %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE runs SET updated_at = ? WHERE id = ?`, now, runID,
		); err != nil {
			return fmt.Errorf("update run (BMNDiscovered): %w", err)
		}
		for _, b := range p.BMNs {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO rack_deviceslots (run_id, rack, deviceslot, bmn_name, zone, ct_position, discovered_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(run_id, rack, deviceslot) DO UPDATE SET
				  bmn_name      = excluded.bmn_name,
				  zone          = excluded.zone,
				  ct_position   = excluded.ct_position,
				  discovered_at = excluded.discovered_at`,
				runID, p.Rack, b.Deviceslot, b.BMNName, p.Zone, b.CTPosition, p.DiscoveredAt,
			); err != nil {
				return fmt.Errorf("upsert rack_deviceslots (%s,%s): %w", p.Rack, b.Deviceslot, err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
		return nil

	case "BMNDiscoveryFailed":
		var p bmnDiscoveryFailedPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("decode BMNDiscoveryFailed payload: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO events (run_id, seq, kind, payload, occurred_at)
			VALUES (?, COALESCE((SELECT MAX(seq) FROM events WHERE run_id = ?), 0) + 1, ?, ?, ?)`,
			runID, runID, kind, string(payload), now,
		); err != nil {
			return fmt.Errorf("insert event (BMNDiscoveryFailed): %w", err)
		}
		// Pin worst_verdict=failed (not just status) so a later StepCompleted
		// can't roll the worst-wins rollup back to passed. signed_off-guarded
		// to match engine.Project and the StepFailed case.
		if _, err := tx.ExecContext(ctx,
			`UPDATE runs SET status = ?, worst_verdict = ?, updated_at = ? WHERE id = ? AND status NOT IN (?, ?)`,
			verdict.StatusFailed, verdict.Failed, now, runID, verdict.StatusSignedOff, verdict.StatusCancelled,
		); err != nil {
			return fmt.Errorf("update run (BMNDiscoveryFailed): %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
		return nil

	case "BMNIgnored":
		var p bmnIgnoredPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("decode BMNIgnored payload: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO events (run_id, seq, kind, payload, occurred_at)
			VALUES (?, COALESCE((SELECT MAX(seq) FROM events WHERE run_id = ?), 0) + 1, ?, ?, ?)`,
			runID, runID, kind, string(payload), now,
		); err != nil {
			return fmt.Errorf("insert event (BMNIgnored): %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE runs SET updated_at = ? WHERE id = ?`, now, runID,
		); err != nil {
			return fmt.Errorf("update run (BMNIgnored): %w", err)
		}
		var reason any
		if p.Reason != "" {
			reason = p.Reason
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO bmn_ignores (run_id, rack, deviceslot, source, reason, marked_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(run_id, deviceslot, source) DO UPDATE SET
			  rack      = excluded.rack,
			  reason    = excluded.reason,
			  marked_at = excluded.marked_at`,
			runID, p.Rack, p.Deviceslot, p.Source, reason, p.MarkedAt,
		); err != nil {
			return fmt.Errorf("upsert bmn_ignores (%s,%s,%s): %w", runID, p.Deviceslot, p.Source, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
		return nil

	default:
		// Unknown kinds are stored; projection ignores them.
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO events (run_id, seq, kind, payload, occurred_at)
		VALUES (?, COALESCE((SELECT MAX(seq) FROM events WHERE run_id = ?), 0) + 1, ?, ?, ?)`,
		runID, runID, kind, string(payload), now,
	); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
