package lifecycleclient

import (
	"embed"
	"strings"
)

//go:embed queries/*.promql
var queryFS embed.FS

// loadQuery reads a .promql file from the embedded FS and trims trailing
// whitespace. Ports the tiphys/inventory/signal.go pattern verbatim;
// callers pass the file name without the queries/ prefix.
func loadQuery(name string) string {
	body, err := queryFS.ReadFile("queries/" + name)
	if err != nil {
		// Embedded files are baked at compile time; a missing file is
		// a programming error, not a runtime concern. Return empty so
		// tests fail loudly rather than panic in production.
		return ""
	}
	return strings.TrimRight(string(body), "\n\r\t ")
}

// renderRLCCIgnoredQuery substitutes the rack id into the embedded
// query template. The substitution is a single literal ReplaceAll —
// rack ids are constrained to [a-z0-9-]+ so no quoting hazards.
func renderRLCCIgnoredQuery(rack string) string {
	tmpl := loadQuery("rlcc_ignored.promql")
	return strings.ReplaceAll(tmpl, "{rack}", rack)
}

// renderActionStateQuery substitutes rack/node/state into the embedded
// action-state query template. Same single-literal ReplaceAll strategy as
// renderRLCCIgnoredQuery — label values are constrained character sets so
// there are no quoting hazards.
func renderActionStateQuery(rack, node, state string) string {
	tmpl := loadQuery("rlcc_action_state.promql")
	tmpl = strings.ReplaceAll(tmpl, "{rack}", rack)
	tmpl = strings.ReplaceAll(tmpl, "{node}", node)
	tmpl = strings.ReplaceAll(tmpl, "{state}", state)
	return tmpl
}

// renderFLCCStateQuery substitutes the deviceslot into the embedded
// flcc_node_state query. Single-literal ReplaceAll; deviceslots are a
// constrained character set so there are no quoting hazards.
func renderFLCCStateQuery(deviceslot string) string {
	tmpl := loadQuery("flcc_node_state.promql")
	return strings.ReplaceAll(tmpl, "{deviceslot}", deviceslot)
}

// parseActionLabels splits a canonical deviceslot into the (rack, node)
// label pair the RLCC state metric is keyed on. Format:
//
//	deviceslot: dh3-r012-node-07-us-east-01a
//	rack:       dh3-r012-us-east-01a
//	node:       node-07
//
// A deviceslot that doesn't contain "-node-" returns (deviceslot, "") so the
// caller still issues a (likely-empty) query rather than panicking.
func parseActionLabels(deviceslot string) (rack, node string) {
	idx := strings.Index(deviceslot, "-node-")
	if idx < 0 {
		return deviceslot, ""
	}
	prefix := deviceslot[:idx] // dh3-r012
	rest := deviceslot[idx+1:] // node-07-us-east-01a
	parts := strings.SplitN(rest, "-", 3)
	if len(parts) < 3 {
		return deviceslot, ""
	}
	node = parts[0] + "-" + parts[1] // node-07
	region := parts[2]               // us-east-01a
	rack = prefix + "-" + region     // dh3-r012-us-east-01a
	return rack, node
}
