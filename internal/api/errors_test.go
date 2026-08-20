package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWriteError_Envelope(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, 404, "not_found", "Run not found")

	if w.Code != 404 {
		t.Fatalf("code = %d, want 404", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Fatalf("code = %q, want not_found", body.Error.Code)
	}
	if body.Error.Message != "Run not found" {
		t.Fatalf("message = %q, want Run not found", body.Error.Message)
	}
}

func TestWriteJSON_Body(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, 200, map[string]any{"ok": true})

	if w.Code != 200 {
		t.Fatalf("code = %d, want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("ok = %v, want true", body["ok"])
	}
}
