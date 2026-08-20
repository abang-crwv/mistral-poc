package hpcverifclient

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

// renderFailuresQuery substitutes the rack id into the embedded query. Only the
// "{rack_name}" placeholder is replaced — the bare `rack_name` label references
// elsewhere in the query (group_left / label_replace target) carry no braces,
// so a single-literal ReplaceAll is unambiguous. Rack ids are a constrained
// character set, so there is no quoting hazard.
func renderFailuresQuery(rack string) string {
	tmpl := loadQuery("hpc_verification_failures.promql")
	return strings.ReplaceAll(tmpl, "{rack_name}", rack)
}

// renderRosterQuery substitutes the rack id into the embedded roster query. Same
// single-placeholder substitution as renderFailuresQuery — only "{rack_name}" is
// replaced; the bare `rack_name` label refs (group_left / label_replace target)
// carry no braces and survive untouched.
func renderRosterQuery(rack string) string {
	tmpl := loadQuery("hpc_verification_roster.promql")
	return strings.ReplaceAll(tmpl, "{rack_name}", rack)
}
