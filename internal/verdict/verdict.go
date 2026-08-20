// Package verdict is the single source of qac's run-status vocabulary and
// the verdict→status rollup. It depends on nothing so both the store
// projection (internal/store) and the engine projection (internal/engine)
// can share it without an import cycle.
package verdict

// Verdict values an assertion probe produces (also the verdict-derived
// status strings — same literals).
const (
	Passed  = "passed"
	Warning = "warning"
	Failed  = "failed"
)

// Run status values.
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusPassed    = "passed"
	StatusWarning   = "warning"
	StatusFailed    = "failed"
	StatusSignedOff = "signed_off"
	StatusCancelled = "cancelled"
)

// IsTerminal reports whether a status is final — no projection should move
// a run off it. signed_off (operator) and cancelled (operator) are terminal;
// passed/failed/warning are walk outcomes that a later event could still
// roll up, so they are NOT terminal here.
func IsTerminal(status string) bool {
	return status == StatusSignedOff || status == StatusCancelled
}

// Worse returns the worse (higher-ranked) of two verdicts. Empty = no
// verdict yet (best). Used to roll up a run's worst step verdict.
func Worse(a, b string) string {
	rank := map[string]int{"": 0, Passed: 1, Warning: 2, Failed: 3}
	if rank[a] >= rank[b] {
		return a
	}
	return b
}

// ToStatus maps a (worst) verdict to a run status. Empty/unknown → running.
func ToStatus(v string) string {
	switch v {
	case Passed:
		return StatusPassed
	case Warning:
		return StatusWarning
	case Failed:
		return StatusFailed
	default:
		return StatusRunning
	}
}
