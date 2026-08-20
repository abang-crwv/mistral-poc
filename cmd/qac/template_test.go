package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"qac/internal/store"
)

const smokeTemplateYAML = `schema: qac.template/v1
id: cli-smoke
version: 1
title: "CLI smoke"
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

func TestTemplateLoadCmd_UpsertsTemplateAndPrints(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "smoke.yaml")
	if err := os.WriteFile(yamlPath, []byte(smokeTemplateYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	dbPath := filepath.Join(dir, "fwqual.db")

	cmd := templateCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"load", "--db", dbPath, yamlPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), "cli-smoke") {
		t.Fatalf("output = %q, want it to mention cli-smoke", out.String())
	}

	// Verify the upsert took effect.
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	row, ok, err := s.GetTemplate(context.Background(), "cli-smoke")
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if !ok {
		t.Fatal("template not present after load")
	}
	if row.Version != 1 {
		t.Fatalf("version = %d, want 1", row.Version)
	}
}
