// Package inventoryclient is a self-contained fleet-id resolver.
// It is a surgical port of the interface and types from
// ~/Documents/argo/tiphys/inventory — only the surface needed by
// fwqual. The bare fwqual module path means we cannot import that
// project directly; the port carries forward the contract (Resolver,
// Hit, error types) so a future swap to a real VM/Netbox-backed
// resolver is a one-line change in cmd/qac/serve.go.
package inventoryclient

import (
	"context"
	"errors"
	"fmt"
)

// Hit is one resolution result. A given fleet ID may resolve to zero
// (not found), one (unambiguous), or multiple (duplicate; refuse) hits.
// Labels carries the raw key/value pairs that the production resolver
// would emit; fwqual reads instance_type/sku/variant/etc out of here.
type Hit struct {
	ID      string
	Region  string
	Kind    string
	Cluster string
	Labels  map[string]string
}

// Kind values for Hit.Kind. iter-4a reads only rack-kind hits;
// the other constants are ported forward for future iterations
// that exercise BMN/CDU/PowerShelf/NVLink lookups.
const (
	KindBMN        = "bmn"
	KindRack       = "rack"
	KindCDU        = "cdu"
	KindPowerShelf = "powershelf"
	KindNVLink     = "nvlink"
)

// DomainScope carries the NVLink-domain selector labels for a rack, read
// off baremetal_node_nvlink_domain_info. These are the labels the alert
// category queries scope by. Region and Zone are kept RAW (as stored on the
// metric, typically upper-cased, e.g. "CA-EAST-01A") — not lower-cased like
// Hit.Region — because the alert join matches on the stored label value.
// NVLinkDomain is the domain identifier (e.g. "DH1000-002-CA-EAST-01A").
type DomainScope struct {
	Region       string
	Zone         string
	Cluster      string
	NVLinkDomain string
}

// Resolver looks up a fleet ID. Implementations must be goroutine-safe.
type Resolver interface {
	Lookup(ctx context.Context, id string) ([]Hit, error)
	// ResolveBMNs returns the compute trays present in a rack. The
	// production resolver (a future iter-6+) will hit the same backend
	// as Lookup; MapResolver is fixture-backed for tests + iter-5b dev.
	// Returns ErrNotFound when the rack id has no BMN list seeded.
	ResolveBMNs(ctx context.Context, rackID string) ([]BMN, error)
	// ResolveDomainScope returns the NVLink-domain selector labels for a
	// rack (region/zone/cluster/nvlink_domain), read from
	// baremetal_node_nvlink_domain_info. Returns ErrNotFound when the rack
	// has no domain series. Used by alert_probe to scope its category
	// queries to the rack's NVLink domain.
	ResolveDomainScope(ctx context.Context, rackID string) (DomainScope, error)
}

// ErrNotFound is returned by ResolveRack (Task 8) when a rack id
// resolves to zero hits.
var ErrNotFound = errors.New("inventoryclient: not found")

// AmbiguousError is returned when an id resolves to multiple regions.
// Callers should refuse to act and surface the region list to the user.
type AmbiguousError struct {
	ID   string
	Hits []Hit
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("inventoryclient: %q resolves to %d regions: %v", e.ID, len(e.Hits), e.Regions())
}

// Regions returns the unique region list in order matching e.Hits.
func (e *AmbiguousError) Regions() []string {
	out := make([]string, len(e.Hits))
	for i, h := range e.Hits {
		out[i] = h.Region
	}
	return out
}
