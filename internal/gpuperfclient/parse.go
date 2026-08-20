package gpuperfclient

import (
	"fmt"
	"strings"
)

// renderQuery generates the scoped PromQL for a spec. Every signal shares the
// same rack scoping — join onto the rack's nodes via the node_context helper on
// (node, cluster, cluster_org, zone) — and differs only in the inner source
// expression: a recording rule (kindRule) or the _sum/_count mean of an OTel
// histogram (kindHistogramMean). The rack id is a constrained identifier, so
// interpolating it directly carries no quoting hazard (same approach as the
// sibling ports' ReplaceAll).
func renderQuery(spec MetricSpec, rack string) string {
	dims := strings.Join(spec.Dims, ", ")
	return fmt.Sprintf(`%s by (%s) (
  %s
  * on(node, cluster, cluster_org, zone) group_left()
  max by (node, cluster, cluster_org, zone) (
    node:node_context:limit_1{rack_name=%q, state!~%q, workflow!~%q}
  )
)`, spec.Agg, dims, innerExpr(spec), rack, flccIgnorableStates, flccIgnorableWorkflows)
}

// FLCC-ignorable node states/workflows, excluded from the rack scope so a node
// in RMA / broken / triage / debug / dev (or a broken-collect / rma workflow)
// can't contribute junk or half-run perf rows to the pack. node_context carries
// state and workflow with the same FLCC vocabulary as the hpcverif failures
// query's label_flcc_coreweave_com_{state,workflow}. A node missing the label
// has an empty value, which matches neither pattern, so it is kept.
const (
	flccIgnorableStates    = "rma|broken|triage|debug|dev|hold|onboard|fail|storage"
	flccIgnorableWorkflows = "broken-collect|rma.*"
)

// innerExpr is the source expression joined to the rack scope. A recording rule
// is used verbatim; a histogram takes its _sum/_count mean (per series, before
// the outer aggregation collapses it by dims).
func innerExpr(spec MetricSpec) string {
	if spec.Kind == kindHistogramMean {
		return fmt.Sprintf("(%s_sum / %s_count)", spec.Rule, spec.Rule)
	}
	return spec.Rule
}
