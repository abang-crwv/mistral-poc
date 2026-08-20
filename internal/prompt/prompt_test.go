package prompt

import (
	"strings"
	"testing"
)

func TestGet_CanaryRubric(t *testing.T) {
	body, err := Get("fw-release-canary.summary.v1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(body, "record_assessment") {
		t.Errorf("rubric should instruct the model to call record_assessment; got:\n%s", body)
	}
}

func TestGet_Unknown(t *testing.T) {
	if _, err := Get("does.not.exist"); err == nil {
		t.Fatal("expected error for unknown prompt_ref")
	}
}
