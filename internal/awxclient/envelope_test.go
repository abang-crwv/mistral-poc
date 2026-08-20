package awxclient_test

import (
	"context"

	"qac/internal/awxclient"
)

// §0.25 envelope assertion — awxclient exposes ONLY read-only job-info verbs:
// JobsForBMNs (`job info bmn`), JobByID (`job info id`), AnalyzeFailures
// (`job info id --errsort`). All three are observation, not orchestration. If
// anyone changes a signature or removes a method, this stops compiling.
//
// Go's type system cannot express "this interface has EXACTLY these methods
// and no others": adding a NEW method (e.g. a mutating Launch/Cancel/Retry)
// would not break this assertion. Reviewer attention + the envelope note in
// the package doc are the second line of defense — a new method must map to a
// read-only awxctl verb.
var _ interface {
	JobsForBMNs(context.Context, []string, awxclient.Options) ([]awxclient.Job, error)
	JobByID(context.Context, string, int) (awxclient.Job, error)
	AnalyzeFailures(context.Context, string, []int) ([]awxclient.FailureGroup, error)
} = (awxclient.Client)(nil)

// Both concrete implementations satisfy Client — keeps serve.go wiring honest.
var (
	_ awxclient.Client = (*awxclient.CLIClient)(nil)
	_ awxclient.Client = (*awxclient.MapClient)(nil)
)
