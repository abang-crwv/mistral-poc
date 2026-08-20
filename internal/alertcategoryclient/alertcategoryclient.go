// Package alertcategoryclient is a self-contained, category-driven alert
// source. Each "category" is one embedded PromQL template that gathers a
// class of alerts (firing/pending) over a lookback window, scoped to a rack's
// NVLink domain, and returns generic per-series rows. The probe layer
// (internal/probe/alertprobe) iterates the category registry and groups the
// rows under each category id — adding a category is one registry entry plus
// one queries/<id>.promql file, no new method or type.
//
// Like the sibling lifecycleclient/inventoryclient ports, the bare `qac`
// module means we cannot import an external SDK: this package carries the
// shape it needs and runs through the shared super-region fan-out client
// (internal/vm).
package alertcategoryclient

import (
	"context"
	"errors"
	"time"
)

// DomainScope re-declares the rack's NVLink-domain selector labels (the port
// stays self-contained — no cross-package import of inventoryclient). Values
// are substituted verbatim into query selectors, so they must be the raw,
// stored label values (typically upper-cased zone/region).
type DomainScope struct {
	Region       string
	Zone         string
	Cluster      string
	NVLinkDomain string
}

// Lookback is the range-query window and step for a category gather. Zero
// fields fall back to package defaults.
type Lookback struct {
	Window time.Duration
	Step   time.Duration
}

// CategoryRow is one collapsed matrix series: its label set plus the
// first/last sample timestamps and the number of steps it was present over the
// lookback window.
type CategoryRow struct {
	Labels      map[string]string `json:"labels"`
	FirstSeen   int64             `json:"first_seen"`
	LastSeen    int64             `json:"last_seen"`
	SampleCount int               `json:"sample_count"`
}

// CategorySpec describes one alert category: a stable id, a human title, and
// the embedded PromQL template file that gathers it. queryFile is unexported —
// it is an implementation detail of the prom backend.
type CategorySpec struct {
	ID        string
	Title     string
	queryFile string
}

// Client gathers ranged alert rows for a named category, scoped to one rack's
// NVLink domain. Implementations must be goroutine-safe.
type Client interface {
	// Gather runs the category's range query over the lookback window and
	// returns one CategoryRow per result series. rack is the canonical rack
	// id (used by categories that scope on the rack / deviceslot label).
	Gather(ctx context.Context, categoryID string, scope DomainScope, rack string, lb Lookback) ([]CategoryRow, error)
	// Categories returns the registry of categories (id + title) so callers
	// iterate without hard-coding ids.
	Categories() []CategorySpec
}

// ErrSourceUnavailable signals a source-side failure (network, auth,
// rate-limit). Callers may map it onto a StepFailed event.
var ErrSourceUnavailable = errors.New("alertcategoryclient: source unavailable")

// ErrUnknownCategory is returned by Gather for an unregistered category id.
var ErrUnknownCategory = errors.New("alertcategoryclient: unknown category")
