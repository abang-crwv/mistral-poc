// Package api exposes the JSON HTTP surface backed by internal/store
// and internal/engine.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// errorBody is the wire format for every non-2xx response.
type errorBody struct {
	Error errorDetail `json:"error"`
}
type errorDetail struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Stable error codes — keep these short and snake_case; the frontend
// branches on them.
const (
	codeNotFound            = "not_found"
	codeInvalidRequest      = "invalid_request"
	codeInternal            = "internal"
	codeTemplateNotFound    = "template_not_found"
	codeInventoryUnresolved = "inventory_unresolved"
	codeForbidden           = "forbidden"
	codeBMNsUnresolved      = "bmns_unresolved"
	codeRLCCWorkflowUnknown = "rlcc_workflow_unknown" // iter-5d
)

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(errorBody{Error: errorDetail{Code: code, Message: message}}); err != nil {
		slog.Error("encode error envelope", "err", err)
	}
}

// writeErrorWithDetails encodes a non-2xx response with structured
// details. Today the only producer is the 422 inventory_unresolved
// path, which carries the unresolved rack list under "unresolved".
func writeErrorWithDetails(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(errorBody{Error: errorDetail{Code: code, Message: message, Details: details}}); err != nil {
		slog.Error("encode error envelope (with details)", "err", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("encode response", "err", err)
	}
}
