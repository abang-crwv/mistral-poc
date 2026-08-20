package rlccclient

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"qac/internal/sourcegraph"
)

const (
	rlccRepo = "github.com/coreweave/rack-lifecycle-controller"
	rlccPath = "chart/rack-lifecycle-controller/values.yaml"
	rlccRef  = "main"
)

// SourcegraphClient reads the RLCC chart values.yaml live from Sourcegraph.
// Caching lives one layer down (in sourcegraph.Client); this layer just
// parses and reports the chart tag.
type SourcegraphClient struct {
	sg       *sourcegraph.Client
	chartTag string // currently unknown without a side channel; defaults to ""
}

// NewSourcegraphClient wraps a sourcegraph.Client. ChartTag is left empty —
// the SHA returned by Fetch is the load-bearing pinning identifier; the
// chart tag is informational only.
func NewSourcegraphClient(sg *sourcegraph.Client) *SourcegraphClient {
	return &SourcegraphClient{sg: sg}
}

func (c *SourcegraphClient) catalog(ctx context.Context) (map[string]WorkflowGraph, SourceRef, error) {
	res, err := c.sg.Fetch(ctx, rlccRepo, rlccPath, rlccRef)
	if err != nil {
		return nil, SourceRef{}, fmt.Errorf("fetch rlcc values.yaml: %w", err)
	}
	m, err := ParseValuesYAML(bytes.NewReader(res.Body))
	if err != nil {
		return nil, SourceRef{}, fmt.Errorf("parse rlcc values.yaml: %w", err)
	}
	return m, SourceRef{ChartTag: c.chartTag, SHA: res.SHA}, nil
}

func (c *SourcegraphClient) ListWorkflows(ctx context.Context) ([]WorkflowSummary, SourceRef, error) {
	m, src, err := c.catalog(ctx)
	if err != nil {
		return nil, src, err
	}
	out := make([]WorkflowSummary, 0, len(m))
	for _, g := range m {
		out = append(out, g.WorkflowSummary)
	}
	// Sort to match MapClient's stable order.
	sortByName(out)
	return out, src, nil
}

func (c *SourcegraphClient) GetWorkflow(ctx context.Context, name string) (WorkflowGraph, SourceRef, error) {
	m, src, err := c.catalog(ctx)
	if err != nil {
		return WorkflowGraph{}, src, err
	}
	g, ok := m[name]
	if !ok {
		return WorkflowGraph{}, src, fmt.Errorf("%w: %s", ErrWorkflowNotFound, name)
	}
	return g, src, nil
}

func sortByName(s []WorkflowSummary) {
	sort.Slice(s, func(i, j int) bool { return s[i].Name < s[j].Name })
}
