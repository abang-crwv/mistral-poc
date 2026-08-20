package firmwareclient

import (
	"embed"
	"strings"
)

//go:embed queries/*.promql
var queryFS embed.FS

// loadQuery reads a .promql file from the embedded FS and trims trailing
// whitespace. Mirrors lifecycleclient.loadQuery.
func loadQuery(name string) string {
	body, err := queryFS.ReadFile("queries/" + name)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(body), "\n\r\t ")
}

// renderInventoryQuery substitutes the rack id into the embedded query. The
// rack id IS the topology rack_name (an exact-match label), so a single-literal
// ReplaceAll is enough; rack ids are a constrained character set so there is no
// quoting hazard.
func renderInventoryQuery(rack string) string {
	tmpl := loadQuery("firmware_inventory.promql")
	return strings.ReplaceAll(tmpl, "{rack_name}", rack)
}

// renderBundleQuery substitutes the rack id into the fwbundle-convergence
// query. Same exact-match rack_name substitution as renderInventoryQuery.
func renderBundleQuery(rack string) string {
	tmpl := loadQuery("firmware_bundle.promql")
	return strings.ReplaceAll(tmpl, "{rack_name}", rack)
}
