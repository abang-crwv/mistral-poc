package alertcategoryclient

import (
	"embed"
	"strings"
)

//go:embed queries/*.promql
var queryFS embed.FS

// loadQuery reads a .promql file from the embedded FS and trims trailing
// whitespace. Mirrors lifecycleclient.loadQuery; callers pass the file name
// without the queries/ prefix. A missing file (programming error, baked at
// compile time) returns empty so Gather fails loudly rather than panicking.
func loadQuery(name string) string {
	body, err := queryFS.ReadFile("queries/" + name)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(body), "\n\r\t ")
}

// renderQuery substitutes scope + rack placeholders into a category template.
// Empty optional scope labels render as ".*" so a missing label doesn't
// collapse the selector to "match only the empty value". Label values are
// constrained character sets (region/zone/cluster/nvlink_domain/rack ids), so
// a literal ReplaceAll carries no quoting hazard — same strategy as
// lifecycleclient.
func renderQuery(tmpl string, scope DomainScope, rack string) string {
	r := strings.NewReplacer(
		"{region}", orWildcard(scope.Region),
		"{zone}", orWildcard(scope.Zone),
		"{cluster}", orWildcard(scope.Cluster),
		"{nvlink_domain}", orWildcard(scope.NVLinkDomain),
		"{rack}", rack,
	)
	return r.Replace(tmpl)
}

// orWildcard maps an empty/blank value to the ".*" regex so an absent optional
// scope label widens the selector rather than matching only empty.
func orWildcard(v string) string {
	if strings.TrimSpace(v) == "" {
		return ".*"
	}
	return v
}
