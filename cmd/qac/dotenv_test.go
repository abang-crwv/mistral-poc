package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := `# a comment
AWXCTL_SOURCEGRAPH_TOKEN=sg-abc123
export AWXCTL_VMAUTH_USERNAME = vm-user
AWXCTL_VMAUTH_PASSWORD="quoted secret"

QAC_DOTENV_SINGLE='single'
QAC_COLON: colon-value
QAC_SPACE space-value
noseparatorhere
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	// A var already in the environment must NOT be overwritten (real env wins).
	t.Setenv("AWXCTL_SOURCEGRAPH_TOKEN", "real-token-wins")
	// File-only keys: ensure unset before, and clean up after (loadDotEnv uses
	// os.Setenv directly, so it escapes t.Setenv's auto-restore).
	for _, k := range []string{"AWXCTL_VMAUTH_USERNAME", "AWXCTL_VMAUTH_PASSWORD", "QAC_DOTENV_SINGLE", "QAC_COLON", "QAC_SPACE"} {
		_ = os.Unsetenv(k)
		t.Cleanup(func() { _ = os.Unsetenv(k) })
	}

	loaded, present, malformed := loadDotEnv(path)
	if !present {
		t.Fatal("present = false, want true (file exists)")
	}
	if malformed != 1 {
		t.Errorf("malformed = %d, want 1 (the 'noseparatorhere' line)", malformed)
	}
	if len(loaded) != 6 {
		t.Errorf("loaded names = %v, want 6", loaded)
	}

	cases := map[string]string{
		"AWXCTL_SOURCEGRAPH_TOKEN": "real-token-wins", // precedence: real env wins
		"AWXCTL_VMAUTH_USERNAME":   "vm-user",         // '=' + export prefix + spaces trimmed
		"AWXCTL_VMAUTH_PASSWORD":   "quoted secret",   // double quotes stripped
		"QAC_DOTENV_SINGLE":        "single",          // single quotes stripped
		"QAC_COLON":                "colon-value",     // ':' separator
		"QAC_SPACE":                "space-value",     // whitespace separator
	}
	for k, want := range cases {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestApplyEnvAliases(t *testing.T) {
	// Bare names fill the canonical AWXCTL_* vars when those are unset.
	t.Setenv("AWXCTL_SOURCEGRAPH_TOKEN", "")
	_ = os.Unsetenv("AWXCTL_SOURCEGRAPH_TOKEN")
	t.Setenv("SOURCEGRAPH_TOKEN", "sg-tok")
	t.Setenv("VM_USER", "vmu")
	t.Setenv("VM_PASSWORD", "vmp")
	t.Cleanup(func() {
		for _, k := range []string{"AWXCTL_SOURCEGRAPH_TOKEN", "AWXCTL_VMAUTH_USERNAME", "AWXCTL_VMAUTH_PASSWORD"} {
			_ = os.Unsetenv(k)
		}
	})

	applyEnvAliases()

	if got := os.Getenv("AWXCTL_SOURCEGRAPH_TOKEN"); got != "sg-tok" {
		t.Errorf("AWXCTL_SOURCEGRAPH_TOKEN = %q, want sg-tok (from SOURCEGRAPH_TOKEN)", got)
	}
	if got := os.Getenv("AWXCTL_VMAUTH_USERNAME"); got != "vmu" {
		t.Errorf("AWXCTL_VMAUTH_USERNAME = %q, want vmu (from VM_USER)", got)
	}
	if got := os.Getenv("AWXCTL_VMAUTH_PASSWORD"); got != "vmp" {
		t.Errorf("AWXCTL_VMAUTH_PASSWORD = %q, want vmp (from VM_PASSWORD)", got)
	}
}

func TestApplyEnvAliases_CanonicalWins(t *testing.T) {
	t.Setenv("AWXCTL_SOURCEGRAPH_TOKEN", "canonical")
	t.Setenv("SOURCEGRAPH_TOKEN", "alias")
	applyEnvAliases()
	if got := os.Getenv("AWXCTL_SOURCEGRAPH_TOKEN"); got != "canonical" {
		t.Errorf("canonical should win: got %q, want canonical", got)
	}
}

func TestApplyEnvAliases_Anthropic(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_API_KEY", "sk-test")
	applyEnvAliases()
	if os.Getenv("ANTHROPIC_API_KEY") != "sk-test" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want sk-test (from CLAUDE_API_KEY alias)", os.Getenv("ANTHROPIC_API_KEY"))
	}
}

func TestLoadDotEnv_MissingFileIsNoError(t *testing.T) {
	// Must not panic or error when the file is absent; present must be false.
	loaded, present, malformed := loadDotEnv(filepath.Join(t.TempDir(), "does-not-exist.env"))
	if present || len(loaded) != 0 || malformed != 0 {
		t.Errorf("missing file: present=%v loaded=%v malformed=%d, want false/[]/0", present, loaded, malformed)
	}
}
