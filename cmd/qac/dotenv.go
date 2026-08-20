package main

import (
	"bufio"
	"os"
	"strings"
	"unicode"
)

// loadDotEnv loads KEY/VALUE pairs from a .env file into the process
// environment for local development. It is intentionally tiny and
// dependency-free (the project's self-contained ethos): blank lines and
// "#" comments are skipped, an optional leading "export " is tolerated, and
// surrounding single/double quotes are stripped. The separator is tolerant —
// "KEY=VALUE" (canonical), "KEY: VALUE", or "KEY VALUE" all parse — so a
// hand-written file works regardless of which the operator reached for.
//
// Precedence: a variable already present in the environment is NEVER
// overwritten — the real shell environment always wins over the file. A
// missing .env is not an error.
//
// This loads the creds qac consumes (AWXCTL_SOURCEGRAPH_TOKEN,
// AWXCTL_VMAUTH_USERNAME, AWXCTL_VMAUTH_PASSWORD, ANTHROPIC_API_KEY). Never
// commit a real .env — it is gitignored, and the pr-security workflow scans
// for secrets.
//
// Returns: the variable NAMES it set (never values — safe to log for
// diagnostics), whether the file existed, and the count of non-blank,
// non-comment lines that had no "KEY=VALUE" form (skipped as malformed).
func loadDotEnv(path string) (loaded []string, present bool, malformed int) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, 0 // no .env is fine
	}
	defer f.Close()
	present = true

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimPrefix(sc.Text(), "\ufeff") // tolerate a UTF-8 BOM on the first line
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		// Tolerant separator: '=' (canonical), else ':', else the first run of
		// whitespace. Covers the common hand-written shapes (KEY=v, KEY: v, KEY v).
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			key, val, ok = strings.Cut(line, ":")
		}
		if !ok {
			if i := strings.IndexFunc(line, unicode.IsSpace); i > 0 {
				key, val, ok = line[:i], line[i+1:], true
			}
		}
		if !ok {
			malformed++ // a real line with no key/value separator
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key == "" {
			malformed++
			continue
		}
		loaded = append(loaded, key)
		if _, set := os.LookupEnv(key); !set {
			_ = os.Setenv(key, val)
		}
	}
	return loaded, present, malformed
}

// envAliases maps each canonical AWXCTL_* credential qac reads to the shorter
// bare names a hand-maintained .env commonly uses. This lets a personal creds
// file work without renaming everything to the AWXCTL_ convention.
var envAliases = map[string][]string{
	"AWXCTL_SOURCEGRAPH_TOKEN": {"SOURCEGRAPH_TOKEN"},
	"AWXCTL_VMAUTH_USERNAME":   {"VMAUTH_USERNAME", "VM_USER"},
	"AWXCTL_VMAUTH_PASSWORD":   {"VMAUTH_PASSWORD", "VM_PASSWORD"},
	"ANTHROPIC_API_KEY":        {"CLAUDE_API_KEY"},
}

// applyEnvAliases fills each canonical AWXCTL_* variable from the first
// non-empty alias when the canonical one is unset. The canonical name (and the
// real shell environment) always wins. Returns the canonical names it filled
// (for diagnostics — names only).
func applyEnvAliases() (filled []string) {
	for canonical, alts := range envAliases {
		if v, set := os.LookupEnv(canonical); set && v != "" {
			continue
		}
		for _, a := range alts {
			if v, ok := os.LookupEnv(a); ok && v != "" {
				_ = os.Setenv(canonical, v)
				filled = append(filled, canonical+"←"+a)
				break
			}
		}
	}
	return filled
}
