package flccclient

import (
	"bytes"
	"context"
	"fmt"

	"qac/internal/sourcegraph"
)

const (
	flccRepo = "github.com/coreweave/fleet-lifecycle-controller"
	flccPath = "chart/values.yaml"
	flccRef  = "develop"
)

type SourcegraphClient struct {
	sg       *sourcegraph.Client
	chartTag string
}

func NewSourcegraphClient(sg *sourcegraph.Client) *SourcegraphClient {
	return &SourcegraphClient{sg: sg}
}

func (c *SourcegraphClient) catalog(ctx context.Context) (map[string]FLCCWorkflowGraph, SourceRef, error) {
	res, err := c.sg.Fetch(ctx, flccRepo, flccPath, flccRef)
	if err != nil {
		return nil, SourceRef{}, fmt.Errorf("fetch flcc values.yaml: %w", err)
	}
	m, err := ParseValuesYAML(bytes.NewReader(res.Body))
	if err != nil {
		return nil, SourceRef{}, fmt.Errorf("parse flcc values.yaml: %w", err)
	}
	return m, SourceRef{ChartTag: c.chartTag, SHA: res.SHA}, nil
}

func (c *SourcegraphClient) ListWorkflows(ctx context.Context) ([]FLCCWorkflowSummary, SourceRef, error) {
	m, src, err := c.catalog(ctx)
	if err != nil {
		return nil, src, err
	}
	out := make([]FLCCWorkflowSummary, 0, len(m))
	for _, g := range m {
		out = append(out, g.FLCCWorkflowSummary)
	}
	return out, src, nil
}

func (c *SourcegraphClient) GetWorkflow(ctx context.Context, name string) (FLCCWorkflowGraph, SourceRef, error) {
	m, src, err := c.catalog(ctx)
	if err != nil {
		return FLCCWorkflowGraph{}, src, err
	}
	g, ok := m[name]
	if !ok {
		return FLCCWorkflowGraph{}, src, fmt.Errorf("%w: %s", ErrWorkflowNotFound, name)
	}
	return g, src, nil
}

func (c *SourcegraphClient) TerminalPhase(ctx context.Context, name string) (string, error) {
	m, _, err := c.catalog(ctx)
	if err != nil {
		return "", err
	}
	g, ok := m[name]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrWorkflowNotFound, name)
	}
	return g.TerminalPhase, nil
}
