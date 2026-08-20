package api

import (
	"fmt"
	"regexp"
	"strings"

	"qac/internal/template"
)

// rackPattern enforces the canonical CoreWeave rack name format
// (post-2025-07-02): <datahall>-<rack>-<zone>, e.g. dh3-r012-us-east-01a
// or s0-r011-us-west-01a. The datahall prefix is letters+digits — real
// fleet uses dh3, dh1000, and s0; the earlier dh-only anchor rejected
// the s-prefixed racks that live in us-west. MIRRORED on the frontend
// in web/src/features/runs/newRunSchema.ts and the template's
// canary_racks validate rule — keep them in lockstep. Source: Glean
// FPA-1509 + Jordan Dahmen's "Rack Naming Convention" doc.
var rackPattern = regexp.MustCompile(`^[a-z]+\d+-r\d{3}-[a-z]+-[a-z]+-\d{2}[a-z]$`)

// ValidateRacks splits a comma-separated rack string, trims whitespace,
// validates each token against rackPattern, and returns the canonical
// joined form (no spaces) along with the parsed slice. Returns a
// user-facing error message on the first invalid token. Empty string
// means OK.
func ValidateRacks(input string) (canonical string, racks []string, errMsg string) {
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !rackPattern.MatchString(p) {
			return "", nil, fmt.Sprintf("Rack %q is not in the expected format (example: dh3-r012-us-east-01a)", p)
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return "", nil, "At least one rack is required"
	}
	return strings.Join(out, ","), out, ""
}

// ValidateInputs checks raw against tpl.Inputs and returns a user-facing
// error message naming the first failing input id. Empty string means OK.
// Extra keys in raw not declared in tpl.Inputs are silently ignored
// (forward-compat).
func ValidateInputs(tpl template.Template, raw map[string]any) string {
	for _, in := range tpl.Inputs {
		v, present := raw[in.ID]
		if !present || isZeroValue(v) {
			if in.Required {
				return fmt.Sprintf("input %s is required", in.ID)
			}
			continue
		}
		switch in.Type {
		case "text", "url", "textarea":
			s, ok := v.(string)
			if !ok {
				return fmt.Sprintf("input %s has wrong type (expected string)", in.ID)
			}
			if in.Validate != "" {
				re, err := regexp.Compile(in.Validate)
				if err != nil {
					return fmt.Sprintf("input %s validate regex invalid: %v", in.ID, err)
				}
				if !re.MatchString(s) {
					return fmt.Sprintf("input %s failed validation", in.ID)
				}
			}
		case "multi_text", "multi_url":
			arr, ok := v.([]any)
			if !ok {
				return fmt.Sprintf("input %s has wrong type (expected list of strings)", in.ID)
			}
			var re *regexp.Regexp
			if in.Validate != "" {
				var err error
				re, err = regexp.Compile(in.Validate)
				if err != nil {
					return fmt.Sprintf("input %s validate regex invalid: %v", in.ID, err)
				}
			}
			for i, item := range arr {
				s, ok := item.(string)
				if !ok {
					return fmt.Sprintf("input %s[%d] has wrong type (expected string)", in.ID, i)
				}
				if re != nil && !re.MatchString(s) {
					return fmt.Sprintf("input %s failed validation", in.ID)
				}
			}
		case "enum":
			s, ok := v.(string)
			if !ok {
				return fmt.Sprintf("input %s has wrong type (expected string)", in.ID)
			}
			found := false
			for _, opt := range in.Options {
				if s == opt {
					found = true
					break
				}
			}
			if !found {
				return fmt.Sprintf("input %s not in allowed options", in.ID)
			}
		}
	}
	return ""
}

// isZeroValue treats nil, "", and empty slices as "missing" for required
// checks so callers can send {"foo": ""} and have it rejected as required.
func isZeroValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []any:
		return len(x) == 0
	}
	return false
}
