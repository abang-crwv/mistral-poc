package lifecycleclient

import (
	"context"
	"sync"
)

// MapClient is a deterministic in-memory Client for tests and the
// iter-5b offline backend. Production uses PromClient.
type MapClient struct {
	ignored map[RackKey][]IgnoredBMN

	// iter-5e: per-targetState tick tracking for QueryActionState. mu
	// guards only these maps; QueryRLCCIgnored reads the immutable
	// `ignored` table without locking.
	mu          sync.Mutex
	actionTicks map[string]int             // targetState -> ticks until completion (override)
	tickCount   map[string]int             // targetState -> calls so far
	flcc        map[string]FLCCObservation // deviceslot -> seeded observation (per-slot override)

	// iter-5g: tick-based FLCC completion (mirrors QueryActionState). When
	// flccCompleteAfter > 0, QueryFLCCState returns {State:"ready"} for
	// all unseeded deviceslots once flccTicks >= flccCompleteAfter.
	flccCompleteAfter int
	flccTicks         int
}

// NewMapClient returns a MapClient backed by the given (rack, zone) →
// ignored BMN list table. Nil tables are normalized to empty.
func NewMapClient(ignored map[RackKey][]IgnoredBMN) *MapClient {
	if ignored == nil {
		ignored = map[RackKey][]IgnoredBMN{}
	}
	return &MapClient{ignored: ignored}
}

// QueryRLCCIgnored satisfies Client. Returns nil when the rack has no
// seeded ignores (positive "no ignores" observation, not a miss).
func (m *MapClient) QueryRLCCIgnored(_ context.Context, rack RackKey) ([]IgnoredBMN, error) {
	if v, ok := m.ignored[rack]; ok {
		out := make([]IgnoredBMN, len(v))
		copy(out, v)
		return out, nil
	}
	return nil, nil
}

// SetActionCompleteAfter overrides the number of QueryActionState calls
// before deviceslots of targetState report complete. Default is 3.
func (m *MapClient) SetActionCompleteAfter(targetState string, ticks int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.actionTicks == nil {
		m.actionTicks = map[string]int{}
	}
	m.actionTicks[targetState] = ticks
}

// QueryActionState satisfies Client. Each call increments a per-targetState
// tick counter and reports every requested deviceslot as complete once the
// counter reaches completeAfter (default 3). Deterministic for tests.
func (m *MapClient) QueryActionState(_ context.Context, deviceslots []string, targetState string) (map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tickCount == nil {
		m.tickCount = map[string]int{}
	}
	m.tickCount[targetState]++
	completeAfter := 3
	if n, ok := m.actionTicks[targetState]; ok && n > 0 {
		completeAfter = n
	}
	done := m.tickCount[targetState] >= completeAfter
	out := make(map[string]bool, len(deviceslots))
	for _, ds := range deviceslots {
		out[ds] = done
	}
	return out, nil
}

// SetFLCCObservation seeds the FLCC observation returned by QueryFLCCState
// for a deviceslot. Takes precedence over SetFLCCCompleteAfter for that
// deviceslot. Deterministic for tests.
func (m *MapClient) SetFLCCObservation(deviceslot string, obs FLCCObservation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.flcc == nil {
		m.flcc = map[string]FLCCObservation{}
	}
	m.flcc[deviceslot] = obs
}

// SetFLCCCompleteAfter configures tick-based FLCC completion: QueryFLCCState
// returns {State:"ready"} for all unseeded deviceslots once the call counter
// reaches ticks. Per-slot seedings (SetFLCCObservation) take precedence.
// Default (0) means all unseeded deviceslots return a zero-value observation
// (in-progress) indefinitely.
func (m *MapClient) SetFLCCCompleteAfter(ticks int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flccCompleteAfter = ticks
}

// QueryFLCCState satisfies Client. Per-slot seedings (SetFLCCObservation) are
// returned directly. For unseeded deviceslots, if SetFLCCCompleteAfter was
// called with ticks > 0, returns {State:"ready"} once the internal call
// counter reaches that threshold; otherwise returns a zero-value observation.
func (m *MapClient) QueryFLCCState(_ context.Context, deviceslots []string) (map[string]FLCCObservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// flccTicks advances every call regardless of per-slot seeding; seeded slots
	// take precedence and never consult the tick-based default.
	m.flccTicks++
	done := m.flccCompleteAfter > 0 && m.flccTicks >= m.flccCompleteAfter
	out := make(map[string]FLCCObservation, len(deviceslots))
	for _, ds := range deviceslots {
		if seeded, ok := m.flcc[ds]; ok {
			out[ds] = seeded
			continue
		}
		if done {
			out[ds] = FLCCObservation{State: "ready"}
		} else {
			out[ds] = FLCCObservation{}
		}
	}
	return out, nil
}
