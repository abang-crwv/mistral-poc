package lokiclient

import "strings"

// regionLokiUID is the Loki datasource UID per fleet region prefix. Ported
// verbatim from coreweave/frop internal/grafanaloki/coreweave_logs_datasource.go
// (which mirrors NodeBot's REGION_DATASOURCES loki column). RLCC resolves logs
// by region prefix, uppercased.
var regionLokiUID = []struct {
	prefix string
	uid    string
}{
	{"US-CENTRAL", "af20t1e3rwcu8c"},
	{"US-WEST", "af20t1e3rwcu8c"},
	{"RNO2", "af20t1e3rwcu8c"},
	{"US-EAST", "bf9ywk3urkyrkf"},
	{"RDU1", "bf9ywk3urkyrkf"},
	{"CA-EAST", "bf9ywk3urkyrkf"},
	{"EU-SOUTH", "ef9dupn3z3s3kd"},
	{"EU-NORTH", "ef9dupn3z3s3kd"},
}

// ResolveLokiUID returns the Loki datasource UID for a region string (e.g.
// "us-central-03a", "US-CENTRAL-03"), matched by uppercased prefix. Empty when
// the prefix is unknown.
func ResolveLokiUID(region string) string {
	r := strings.ToUpper(strings.TrimSpace(region))
	for _, row := range regionLokiUID {
		if strings.HasPrefix(r, row.prefix) {
			return row.uid
		}
	}
	return ""
}
