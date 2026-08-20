// Package prompt is the embedded registry of agent prompts/rubrics, keyed by
// the template's prompt_ref. Storing prompts as files (not Go string literals)
// keeps them reviewable and lets an operator diff rubric versions.
package prompt

import (
	"embed"
	"fmt"
)

//go:embed prompts/*.md
var fs embed.FS

// Get returns the prompt body registered under ref (a template prompt_ref,
// e.g. "fw-release-canary.summary.v1"). Returns an error when no prompt is
// registered for ref.
func Get(ref string) (string, error) {
	b, err := fs.ReadFile("prompts/" + ref + ".md")
	if err != nil {
		return "", fmt.Errorf("prompt %q not found: %w", ref, err)
	}
	return string(b), nil
}
