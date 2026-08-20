package rlccclient

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// rlccTop is the wire-shape of the chart's values.yaml. fwqual only cares
// about the workflows[] block; everything else (image, triggers, platforms,
// metrics, ...) is ignored by the unmarshaler.
type rlccTop struct {
	RLCC struct {
		Workflows []WorkflowGraph `yaml:"workflows"`
	} `yaml:"rlcc"`
}

// ParseValuesYAML decodes the rack-lifecycle-controller chart values.yaml
// into a name -> WorkflowGraph map. It derives RackKind from the workflow
// name prefix and ActionCount from len(Actions). Unknown handler kinds and
// unknown component kinds pass through verbatim (they're just strings under
// the hood).
func ParseValuesYAML(r io.Reader) (map[string]WorkflowGraph, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	var top rlccTop
	if err := yaml.Unmarshal(body, &top); err != nil {
		return nil, fmt.Errorf("yaml unmarshal: %w", err)
	}
	out := make(map[string]WorkflowGraph, len(top.RLCC.Workflows))
	for _, g := range top.RLCC.Workflows {
		g.RackKind = rackKindFromName(g.Name)
		g.ActionCount = len(g.Actions)
		g.Handlers = handlerKinds(g.Actions)
		out[g.Name] = g
	}
	return out, nil
}

// handlerKinds returns the sorted, de-duplicated set of handler kinds
// across a workflow's actions. Returns nil for an empty action list so
// the generated fixture and JSON match the zero value rather than an
// empty slice.
func handlerKinds(actions []Action) []string {
	if len(actions) == 0 {
		return nil
	}
	seen := make(map[HandlerKind]bool, len(actions))
	for _, a := range actions {
		seen[a.Handler] = true
	}
	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, string(h))
	}
	sort.Strings(out)
	return out
}

// rackKindFromName buckets a workflow name by its hardware prefix. Unknown
// prefixes return "" — the UI can group those under "other" or hide them.
func rackKindFromName(name string) string {
	for _, p := range []string{"gb200", "b300", "gbx00", "b200", "bx00"} {
		if strings.HasPrefix(name, p+"-") || name == p {
			return p
		}
	}
	return ""
}
