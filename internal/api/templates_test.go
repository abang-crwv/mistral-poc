package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

const demoTemplateYAML = `schema: qac.template/v1
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
`

// seedDemoTemplate inserts a known-good "demo" template using the iter-2
// newTestServer helper which returns (*httptest.Server, *store.Store).
func TestGetTemplates_ReturnsSeeded(t *testing.T) {
	srv, s, _ := newTestServer(t)
	if err := s.UpsertTemplate(context.Background(), "demo", 1, demoTemplateYAML, `{"id":"demo","version":1,"title":"Demo"}`); err != nil {
		t.Fatalf("UpsertTemplate: %v", err)
	}

	res, err := http.Get(srv.URL + "/api/templates")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct {
		Templates []struct {
			ID, Title string
			Version   int
		} `json:"templates"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Templates) != 1 {
		t.Fatalf("len = %d, want 1", len(body.Templates))
	}
	if body.Templates[0].ID != "demo" {
		t.Fatalf("ID = %q, want demo", body.Templates[0].ID)
	}
	if body.Templates[0].Title != "Demo" {
		t.Fatalf("Title = %q, want Demo", body.Templates[0].Title)
	}
}

func TestGetTemplate_HappyPath(t *testing.T) {
	srv, s, _ := newTestServer(t)
	if err := s.UpsertTemplate(context.Background(), "demo", 1, demoTemplateYAML, `{"id":"demo","version":1,"title":"Demo"}`); err != nil {
		t.Fatalf("UpsertTemplate: %v", err)
	}

	res, err := http.Get(srv.URL + "/api/templates/demo")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct {
		Template struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"template"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Template.ID != "demo" {
		t.Fatalf("template.id = %q, want demo", body.Template.ID)
	}
}

func TestGetTemplate_NotFound(t *testing.T) {
	srv, _, _ := newTestServer(t)

	res, err := http.Get(srv.URL + "/api/templates/ghost")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
	var env struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error.Code != "not_found" {
		t.Fatalf("code = %q, want not_found", env.Error.Code)
	}
}
