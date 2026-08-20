package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestGenrlccfixture_IsIdempotent runs `go generate ./internal/rlccclient/`
// from the repo root twice and asserts the diff is zero.
func TestGenrlccfixture_IsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping go-generate idempotency check")
	}
	for i := 0; i < 2; i++ {
		cmd := exec.Command("go", "generate", "./internal/rlccclient/")
		cmd.Dir = "../.."
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go generate iter %d: %v\n%s", i, err, out)
		}
	}
	// git status check — fixture_gen.go must be unchanged after a no-op regen.
	cmd := exec.Command("git", "status", "--porcelain", "internal/rlccclient/fixture_gen.go")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("fixture_gen.go changed after no-op regen:\n%s", out)
	}
}
