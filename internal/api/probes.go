package api

import (
	"net/http"

	"qac/internal/engine"
)

// probeCopy is presentation copy for the Probes page. The registry is the
// source of truth for which probes exist and their category; this only adds a
// human title + description. A registered probe with no entry falls back to
// its type as the title and an empty description, so new probes still list.
var probeCopy = map[string]struct{ Title, Description string }{
	"alert_probe": {
		"Alert history",
		"Sweeps firing and pending alerts across each canary rack's NVLink domain over a lookback window, grouped by category. Gatherer — no verdict.",
	},
	"firmware_inventory_probe": {
		"Firmware inventory",
		"Captures per-deviceslot firmware versions for each rack from redfish, plus per-node bundle convergence (current vs target bundle — did the release land), so a cross-run diff shows what moved and which nodes are off target. Gatherer.",
	},
	"hpc_verification_failure_probe": {
		"HPC verification status",
		"Reports each rack's HPC-verification status — passed, failed, stale, or not verified — by combining non-OK message failures with the tested-node roster and last-run freshness, after filtering by lifecycle state. Gatherer.",
	},
	"gpu_performance_probe": {
		"GPU performance",
		"Snapshots the HPC-verification performance pack per rack — GPU Blaze, NVBandwidth, NCCL, Megatron, and IB metrics. Gatherer.",
	},
	"awx_job_probe": {
		"AWX zap jobs",
		"Gathers the firmware-zap AWX jobs per node (node-zap, dpu-zap, fielddiag) plus the rack-wide l11-fielddiag, with status, per-node failure rate, and failure signatures. Gatherer.",
	},
	"rlcc_action_probe": {
		"RLCC action",
		"Drives and polls a per-CT RLCC action to a target state, honoring RLCC ignores.",
	},
	"fail_cause_probe": {
		"Fail cause",
		"Reads a prior step's failed-tray evidence to attribute a likely failure cause. Gatherer.",
	},
}

// listProbesHandler serves GET /api/probes: every registered probe with its
// category (from the registry) and presentation copy (from probeCopy).
func listProbesHandler(eng *engine.Engine) http.HandlerFunc {
	type view struct {
		Type        string `json:"type"`
		Category    string `json:"category"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		infos := eng.Probes()
		out := make([]view, 0, len(infos))
		for _, p := range infos {
			c := probeCopy[p.Type]
			title := c.Title
			if title == "" {
				title = p.Type
			}
			out = append(out, view{
				Type:        p.Type,
				Category:    string(p.Category),
				Title:       title,
				Description: c.Description,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"probes": out})
	}
}
