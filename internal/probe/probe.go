// Package probe defines the probe-type abstraction used by the engine.
// Probes split into three categories:
//
//   - gatherer  : captures state, produces evidence, no verdict (iter-4b)
//   - assertion : passes/fails via a checkpoint expression (iter-4c+)
//   - action    : performs an operation; reports completion (iter-4c+)
//
// iter-4b ships one gatherer (alertprobe.AlertProbe). The Category()
// metadata lets the engine branch by probe role in later iterations.
package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"qac/internal/alertcategoryclient"
	"qac/internal/alertclient"
	"qac/internal/awxclient"
	"qac/internal/firmwareclient"
	"qac/internal/gpuperfclient"
	"qac/internal/hpcverifclient"
	"qac/internal/inventoryclient"
	"qac/internal/lifecycleclient"
	"qac/internal/lokiclient"
	"qac/internal/rlccclient"
	"qac/internal/store"
)

// Category narrates a probe's role. See package-level doc.
type Category string

const (
	CategoryGatherer  Category = "gatherer"
	CategoryAssertion Category = "assertion"
	CategoryAction    Category = "action"
)

// Output is the canonical probe-result envelope. The engine persists it
// as an evidence row. StructuredData carries probe-shaped JSON without
// forcing downstream code to know the schema; Sources lists provenance
// strings (e.g., "alertmanager-stub"); ProbedAt is unix seconds.
type Output struct {
	StructuredData json.RawMessage `json:"structured_data"`
	Sources        []string        `json:"sources,omitempty"`
	ProbedAt       int64           `json:"probed_at"`
	// Verdict is the step verdict an assertion probe computes: "passed",
	// "warning", or "failed" (matching engine's StepCompletedPayload values).
	// Empty for gatherer probes (the engine leaves the run StatusRunning).
	Verdict string `json:"verdict,omitempty"`
}

// Clients aggregates the backend ports a Probe.Run may need. Grows
// additively in future iterations. iter-5e adds LifecycleClient,
// EvidenceWriter, and EventEmitter for rlcc_action_probe; existing probes
// (alert_probe, firmware_inventory_probe) ignore them.
type Clients struct {
	AlertClient         alertclient.Client
	AlertCategoryClient alertcategoryclient.Client // alert_probe (categories)
	FirmwareClient      firmwareclient.Client      // firmware_inventory_probe
	HPCVerifClient      hpcverifclient.Client      // hpc_verification_failure_probe
	GPUPerfClient       gpuperfclient.Client       // gpu_performance_probe
	AWXJobClient        awxclient.Client           // awx_job_probe (job history + failures)
	LokiClient          lokiclient.Client          // awx_job_probe (l11-fielddiag job link)
	InventoryResolver   inventoryclient.Resolver
	LifecycleClient     LifecycleClient // iter-5e
	EvidenceWriter      EvidenceWriter  // iter-5e
	EventEmitter        EventEmitter    // iter-5e
	EvidenceReader      EvidenceReader  // iter-5l
}

// LifecycleClient is the read-only port rlcc_action_probe uses to poll
// per-CT FLCC state and re-query RLCC ignores each tick. Satisfied by
// lifecycleclient.Client (MapClient + PromClient).
type LifecycleClient interface {
	QueryActionState(ctx context.Context, deviceslots []string, targetState string) (map[string]bool, error)
	QueryRLCCIgnored(ctx context.Context, rk lifecycleclient.RackKey) ([]lifecycleclient.IgnoredBMN, error)
	QueryFLCCState(ctx context.Context, deviceslots []string) (map[string]lifecycleclient.FLCCObservation, error)
}

// EvidenceWriter is a narrow port for writing per-CT evidence rows during a
// probe run. The engine still writes the aggregate evidence from the probe's
// returned Output; this is for the per-CT rows that use the
// evidence.deviceslot column. Satisfied by *store.Store.
type EvidenceWriter interface {
	PutEvidence(ctx context.Context, ev store.Evidence) error
}

// EvidenceReader is a read-only port for a gatherer that consumes a prior
// step's per-CT evidence (e.g. fail_cause_probe reading l11_fielddiag's
// failed-tray rows). Satisfied by *store.Store (ListEvidence, iter-5j).
type EvidenceReader interface {
	ListEvidence(ctx context.Context, runID, stepID string) ([]store.Evidence, error)
}

// EventEmitter is a narrow port for emitting events from inside a probe
// (e.g., BMNIgnored when a CT becomes RLCC-ignored mid-run). Narrower than
// the full store API. Satisfied by *StoreEmitter.
type EventEmitter interface {
	EmitBMNIgnored(ctx context.Context, runID string, payload BMNIgnoredPayload) error
}

// BMNIgnoredPayload mirrors engine.BMNIgnoredPayload for cross-package
// access; same JSON tags. Source is "rlcc" when emitted by rlcc_action_probe.
type BMNIgnoredPayload struct {
	Rack       string `json:"rack"`
	Deviceslot string `json:"deviceslot"`
	Source     string `json:"source"`
	Reason     string `json:"reason,omitempty"`
	MarkedAt   int64  `json:"marked_at"`
}

// StoreEmitter adapts *store.Store to the EventEmitter port.
type StoreEmitter struct{ s *store.Store }

// NewStoreEmitter returns an EventEmitter backed by s.
func NewStoreEmitter(s *store.Store) *StoreEmitter { return &StoreEmitter{s: s} }

// EmitBMNIgnored marshals p and appends a BMNIgnored event for runID.
func (e *StoreEmitter) EmitBMNIgnored(ctx context.Context, runID string, p BMNIgnoredPayload) error {
	payload, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal BMNIgnored: %w", err)
	}
	return e.s.AppendEvent(ctx, runID, "BMNIgnored", payload)
}

// Port satisfaction checks.
var (
	_ EvidenceWriter  = (*store.Store)(nil)
	_ EvidenceReader  = (*store.Store)(nil)
	_ EventEmitter    = (*StoreEmitter)(nil)
	_ LifecycleClient = (lifecycleclient.Client)(nil)
)

// StepContext is the per-step input to Probe.Run. RunID + StepID stitch
// evidence to events. Racks is the canary_racks list. Inputs is the full
// operator-supplied input map. Config is the parsed step.action.args block
// (nil for steps without one). iter-5e adds RLCCWorkflow (the pinned
// snapshot) and IgnoredDeviceslots (operator-ignored CTs from
// BMNIgnored(source=operator) events).
type StepContext struct {
	RunID              string
	StepID             string
	Racks              []string
	Inputs             map[string]any
	Config             map[string]any
	RLCCWorkflow       *rlccclient.WorkflowSnapshot // iter-5e
	IgnoredDeviceslots []string                     // iter-5e
}

// Probe is the contract every probe type satisfies. Goroutine-safe:
// the engine may run multiple probes concurrently in iter-4c+.
type Probe interface {
	Type() string
	Category() Category
	Run(ctx context.Context, clients Clients, sc StepContext) (Output, error)
}

// Registry maps probe type names to implementations. Last Register wins.
type Registry struct {
	mu    sync.RWMutex
	byKey map[string]Probe
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{byKey: map[string]Probe{}}
}

// Register stores p under its Type() key. Concurrent-safe.
func (r *Registry) Register(p Probe) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byKey[p.Type()] = p
}

// Get returns the probe registered under typeName. The bool is false if
// no probe is registered.
func (r *Registry) Get(typeName string) (Probe, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.byKey[typeName]
	return p, ok
}

// Info is a registered probe's public metadata, for the /api/probes surface.
type Info struct {
	Type     string   `json:"type"`
	Category Category `json:"category"`
}

// List returns Info for every registered probe, sorted by Type.
func (r *Registry) List() []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Info, 0, len(r.byKey))
	for _, p := range r.byKey {
		out = append(out, Info{Type: p.Type(), Category: p.Category()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}
