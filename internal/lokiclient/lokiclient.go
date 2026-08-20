// Package lokiclient queries CoreWeave Loki (LogQL) through the Grafana
// datasource proxy, authenticating with a Grafana service-account token.
//
// awx_job_probe uses it for the l11-fielddiag branch: that stage is one
// rack-wide AWX job whose limit spans every node mgmt IP in the nvlink domain,
// so it can't be found with `awxctl job info bmn <one-bmn>`. Instead RLCC logs
// a structured "RLCCAWXJobStarted" event carrying the AWX job link when it
// launches the job; this client reads that event and parses out the job id +
// region, which the probe then reads via `awxctl job info id`.
//
// Access pattern (ported from coreweave/frop internal/grafanaloki, which
// mirrors nodebot's coreweave_grafana.py):
//
//	GET {BaseURL}/api/datasources/proxy/uid/{lokiUID}/loki/api/v1/query_range
//	Authorization: Bearer <GRAFANA_SERVICE_ACCOUNT>
//
// The Loki datasource UID is resolved from the region prefix (see
// datasource.go). Like the sibling ports, the bare `qac` module means we carry
// only the shape we need.
package lokiclient

import (
	"context"
	"errors"
	"time"
)

// LogEntry is one Loki log line with its stream labels and timestamp.
type LogEntry struct {
	Timestamp time.Time
	Line      string
	Labels    map[string]string
}

// Client queries Loki via the Grafana datasource proxy. Implementations must
// be goroutine-safe.
type Client interface {
	// QueryRange runs a LogQL query against the region's Loki datasource over
	// [start, end], newest-first, capped at limit entries. An unknown region
	// returns ErrUnknownRegion; a source/transport failure returns a wrapped
	// ErrSourceUnavailable.
	QueryRange(ctx context.Context, region, logql string, start, end time.Time, limit int) ([]LogEntry, error)
}

// ErrSourceUnavailable signals a source-side failure (network, auth, bad
// response). Callers may map it onto a StepFailed event.
var ErrSourceUnavailable = errors.New("lokiclient: source unavailable")

// ErrUnknownRegion is returned when a region string has no known Loki
// datasource UID (see ResolveLokiUID).
var ErrUnknownRegion = errors.New("lokiclient: no Loki datasource for region")
