package template

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCanonicalCanaryTemplate_ParsesAndValidates loads the templates/firmware-release-canary.yaml
// file from the repo, asserts Parse + Validate both succeed, and spot-checks a few
// expected fields so a regression in the file is caught immediately.
func TestCanonicalCanaryTemplate_ParsesAndValidates(t *testing.T) {
	path := filepath.Join("..", "..", "templates", "firmware-release-canary.yaml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	tpl, err := Parse(body)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := Validate(tpl); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if tpl.ID != "firmware-release-canary" {
		t.Fatalf("ID = %q, want firmware-release-canary", tpl.ID)
	}
	if tpl.Version != 11 {
		t.Fatalf("Version = %d, want 11", tpl.Version)
	}
	// 15 rail steps after the iter-5l fail_cause insertion:
	// alert_baseline, firmware_baseline, baseline_pre, zap_canary,
	// l10_fielddiag, l10_check, l11_fielddiag, l11_check, l12_seatrial,
	// l12_check, baseline_post, fail_cause, evidence_review, ai_summary,
	// operator_signoff = 15.
	if len(tpl.Steps) != 15 {
		t.Fatalf("Steps len = %d, want 15", len(tpl.Steps))
	}
}
