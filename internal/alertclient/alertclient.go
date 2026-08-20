// Package alertclient is a self-contained alert source port. It mirrors
// internal/inventoryclient's contract: the Client interface is what the
// engine and probe code depends on; concrete implementations
// (MapAlertClient today, AlertmanagerClient in iter-4c) plug in via
// interface satisfaction. The bare fwqual module path means we cannot
// import an external alertmanager SDK directly; the port carries the
// shape we need.
package alertclient

import (
	"context"
	"errors"
)

// Alert is one firing alert observed at the source. Severity is one of
// "critical" | "warning" | "info" — the iter-4b alert_probe ranks these
// when building max_severity rollups.
type Alert struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Severity   string `json:"severity"`
	FiredAt    int64  `json:"fired_at"`
	Summary    string `json:"summary"`
	Scope      string `json:"scope"`                // rack id
	Deviceslot string `json:"deviceslot,omitempty"` // node/CT slot key; empty for rack-only alerts (iter-5l)
}

// Client reads alerts from a backing source. Implementations must be
// goroutine-safe (probes may run in parallel goroutines in iter-4c+).
type Client interface {
	ListByRack(ctx context.Context, rackID string) ([]Alert, error)
	// ListByDeviceslot returns alerts scoped to a single node/CT deviceslot,
	// or an empty slice on miss. Used by fail_cause_probe to pin a failed
	// tray's cause. (iter-5l)
	ListByDeviceslot(ctx context.Context, deviceslot string) ([]Alert, error)
}

// ErrSourceUnavailable signals a transient or terminal source-side
// failure (network, auth, rate-limit). Callers may map this onto a
// StepFailed event. iter-4b's MapAlertClient surfaces it for one
// rack id ("failing-source-rack") to support engine error-path tests.
var ErrSourceUnavailable = errors.New("alertclient: source unavailable")
