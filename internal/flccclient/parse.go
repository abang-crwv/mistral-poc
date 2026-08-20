package flccclient

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// flccTop is the wire-shape of the FLCC chart values.yaml. fwqual only cares
// about flcc.config.workflows; everything else (apiServer, flcc.app,
// workflowOverrides, ...) is ignored.
type flccTop struct {
	FLCC struct {
		Config struct {
			Workflows []FLCCWorkflowGraph `yaml:"workflows"`
		} `yaml:"config"`
	} `yaml:"flcc"`
}

// ParseValuesYAML decodes the fleet-lifecycle-controller chart values.yaml
// into a name -> FLCCWorkflowGraph map. Derives PhaseCount (= len(Phases))
// and TerminalPhase (= last phase's State).
func ParseValuesYAML(r io.Reader) (map[string]FLCCWorkflowGraph, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	var top flccTop
	if err := yaml.Unmarshal(body, &top); err != nil {
		return nil, fmt.Errorf("yaml unmarshal: %w", err)
	}
	out := make(map[string]FLCCWorkflowGraph, len(top.FLCC.Config.Workflows))
	for _, g := range top.FLCC.Config.Workflows {
		g.PhaseCount = len(g.Phases)
		if g.PhaseCount > 0 {
			g.TerminalPhase = g.Phases[len(g.Phases)-1].State
		}
		out[g.Name] = g
	}
	return out, nil
}
