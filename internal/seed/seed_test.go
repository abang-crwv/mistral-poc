package seed

import (
	"context"
	"path/filepath"
	"testing"
	"testing/fstest"

	"qac/internal/store"
)

func TestLoadEmbeddedTemplates_UpsertsValidTemplate(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	fsys := fstest.MapFS{
		"templates/demo.yaml": &fstest.MapFile{
			Data: []byte(`schema: qac.template/v1
id: demo
version: 1
title: "Demo"
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
`),
		},
	}

	if err := LoadEmbeddedTemplates(ctx, s, fsys); err != nil {
		t.Fatalf("LoadEmbeddedTemplates: %v", err)
	}

	got, ok, err := s.GetTemplate(ctx, "demo")
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if !ok {
		t.Fatal("template not present after seed")
	}
	if got.Version != 1 {
		t.Fatalf("Version = %d, want 1", got.Version)
	}
}

func TestLoadEmbeddedTemplates_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	fsys := fstest.MapFS{
		"templates/demo.yaml": &fstest.MapFile{
			Data: []byte(`schema: qac.template/v1
id: demo
version: 1
title: "Demo"
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
`),
		},
	}

	for i := 0; i < 3; i++ {
		if err := LoadEmbeddedTemplates(ctx, s, fsys); err != nil {
			t.Fatalf("LoadEmbeddedTemplates #%d: %v", i, err)
		}
	}

	got, err := s.ListTemplates(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (idempotent)", len(got))
	}
}

func TestLoadEmbeddedTemplates_RejectsInvalidYAML(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	fsys := fstest.MapFS{
		"templates/bad.yaml": &fstest.MapFile{
			Data: []byte("schema: wrong/v1\nid: bad\n"),
		},
	}
	err = LoadEmbeddedTemplates(ctx, s, fsys)
	if err == nil {
		t.Fatal("LoadEmbeddedTemplates on invalid YAML returned nil; want non-nil")
	}
}
