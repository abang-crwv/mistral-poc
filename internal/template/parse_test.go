package template

import (
	"strings"
	"testing"
)

const minimalValidYAML = `
schema: qac.template/v1
id: smoke
version: 1
title: "Smoke test"
target:
  scope: rack
  instance_types: [gb200-4x]
inputs:
  - id: bundle_tag
    label: "FW bundle tag"
    type: text
    required: true
steps:
  - id: noop
    kind: probe
    title: "no-op"
    action:
      tool: metrics
      timeout: 5m
    checkpoint:
      targets: per_rack
      success_when:
        stdout_contains: "ok"
`

func TestParse_RoundTripCanonical(t *testing.T) {
	tpl, err := Parse([]byte(minimalValidYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tpl.Schema != "qac.template/v1" {
		t.Fatalf("Schema = %q, want qac.template/v1", tpl.Schema)
	}
	if tpl.ID != "smoke" {
		t.Fatalf("ID = %q, want smoke", tpl.ID)
	}
	if tpl.Version != 1 {
		t.Fatalf("Version = %d, want 1", tpl.Version)
	}
	if len(tpl.Inputs) != 1 {
		t.Fatalf("Inputs len = %d, want 1", len(tpl.Inputs))
	}
	if len(tpl.Steps) != 1 {
		t.Fatalf("Steps len = %d, want 1", len(tpl.Steps))
	}
	if tpl.Steps[0].Action == nil {
		t.Fatal("Steps[0].Action is nil; expected populated")
	}
	if tpl.Steps[0].Action.Tool != "metrics" {
		t.Fatalf("Steps[0].Action.Tool = %q, want metrics", tpl.Steps[0].Action.Tool)
	}
	if tpl.Steps[0].Checkpoint == nil {
		t.Fatal("Steps[0].Checkpoint is nil; expected populated")
	}
}

func TestParse_MalformedYAML(t *testing.T) {
	_, err := Parse([]byte("this: is: not: yaml: at: all"))
	if err == nil {
		t.Fatal("Parse on malformed YAML returned nil error; expected non-nil")
	}
}

func TestParse_EmptyInput(t *testing.T) {
	tpl, err := Parse([]byte(""))
	if err != nil {
		t.Fatalf("Parse on empty input: %v", err)
	}
	// Empty YAML yields a zero-value Template. Validate (Task 4) will
	// reject it. Parse itself just returns the zero value.
	if tpl.Schema != "" {
		t.Fatalf("Schema on empty input = %q, want empty", tpl.Schema)
	}
}

func TestParse_UnknownFieldTolerated(t *testing.T) {
	yamlWithExtra := minimalValidYAML + "\nfuture_field: 42\n"
	tpl, err := Parse([]byte(yamlWithExtra))
	if err != nil {
		t.Fatalf("Parse with unknown field returned error: %v", err)
	}
	// The unknown field is silently dropped. Existing fields parse normally.
	if tpl.ID != "smoke" {
		t.Fatalf("ID after extra field = %q, want smoke", tpl.ID)
	}
}

// Sanity check: the canonical YAML contains a multi-line `args` array.
// We assert Parse doesn't choke when Action.Args is a YAML sequence.
func TestParse_ActionArgsAsList(t *testing.T) {
	in := strings.Replace(minimalValidYAML,
		"    action:\n      tool: metrics\n      timeout: 5m",
		"    action:\n      tool: rlcc\n      args: [\"state\", \"report\", \"--racks\", \"dh3-r012-us-east-01a\"]\n      timeout: 5m",
		1)
	tpl, err := Parse([]byte(in))
	if err != nil {
		t.Fatalf("Parse with list args: %v", err)
	}
	args, ok := tpl.Steps[0].Action.Args.([]any)
	if !ok {
		t.Fatalf("Action.Args type = %T, want []any", tpl.Steps[0].Action.Args)
	}
	if len(args) != 4 {
		t.Fatalf("Action.Args len = %d, want 4", len(args))
	}
}
