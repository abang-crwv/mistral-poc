package api

import (
	"strings"
	"testing"

	"qac/internal/template"
)

func TestValidateRacks(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		wantCanon   string
		wantInvalid bool
	}{
		{"single canonical", "dh3-r012-us-east-01a", "dh3-r012-us-east-01a", false},
		{"s-prefixed us-west rack", "s0-r011-us-west-01a", "s0-r011-us-west-01a", false},
		{"four-digit datahall", "dh1000-r003-ca-east-01a", "dh1000-r003-ca-east-01a", false},
		{"two racks comma+space", "dh3-r012-us-east-01a, dh4-r066-us-east-11a", "dh3-r012-us-east-01a,dh4-r066-us-east-11a", false},
		{"trailing comma tolerated", "dh3-r012-us-east-01a,", "dh3-r012-us-east-01a", false},
		{"empty input rejected", "", "", true},
		{"whitespace only rejected", "   ", "", true},
		{"capitals rejected", "DH3-R012-US-EAST-01A", "", true},
		{"as1-r07 dummy rejected", "as1-r07", "", true},
		{"CP5 rejected", "CP5-v2a", "", true},
		{"one bad in mixed list rejected", "dh3-r012-us-east-01a, oops", "", true},
		{"missing zero pad rejected", "dh3-r12-us-east-01a", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			canon, _, errMsg := ValidateRacks(tc.input)
			if tc.wantInvalid {
				if errMsg == "" {
					t.Fatalf("expected error, got canon=%q", canon)
				}
				return
			}
			if errMsg != "" {
				t.Fatalf("unexpected error: %s", errMsg)
			}
			if canon != tc.wantCanon {
				t.Fatalf("canon = %q, want %q", canon, tc.wantCanon)
			}
		})
	}
}

func TestValidateInputs(t *testing.T) {
	textInput := template.Input{ID: "bundle_tag", Label: "FW bundle tag", Type: "text", Required: true}
	multiTextInput := template.Input{
		ID: "canary_racks", Label: "Canary racks", Type: "multi_text", Required: true,
		Validate: `^[a-z]+\d+-r\d{3}-[a-z]+-[a-z]+-\d{2}[a-z]$`,
	}
	enumInput := template.Input{
		ID: "instance_type", Label: "Instance type", Type: "enum", Required: true,
		Options: []string{"gb200-4x", "gb300-4x"},
	}
	urlInput := template.Input{ID: "slack_thread", Label: "Slack thread URL", Type: "url", Required: false}
	multiURL := template.Input{ID: "nitwit_receipts", Label: "nitwit URLs", Type: "multi_url", Required: true}

	tpl := template.Template{
		Inputs: []template.Input{textInput, multiTextInput, enumInput, urlInput, multiURL},
	}

	cases := []struct {
		name      string
		raw       map[string]any
		wantMatch string // empty = expect success
	}{
		{
			name: "happy path",
			raw: map[string]any{
				"bundle_tag":      "dell-xe9712-default-2.0.0-rc",
				"canary_racks":    []any{"dh3-r012-us-east-01a"},
				"instance_type":   "gb200-4x",
				"nitwit_receipts": []any{"https://nitwit.example.com/r/1"},
			},
		},
		{
			name: "missing required text",
			raw: map[string]any{
				"canary_racks":    []any{"dh3-r012-us-east-01a"},
				"instance_type":   "gb200-4x",
				"nitwit_receipts": []any{"https://x"},
			},
			wantMatch: "bundle_tag is required",
		},
		{
			name: "wrong type for text",
			raw: map[string]any{
				"bundle_tag":      42,
				"canary_racks":    []any{"dh3-r012-us-east-01a"},
				"instance_type":   "gb200-4x",
				"nitwit_receipts": []any{"https://x"},
			},
			wantMatch: "bundle_tag has wrong type",
		},
		{
			name: "enum not in options",
			raw: map[string]any{
				"bundle_tag":      "x",
				"canary_racks":    []any{"dh3-r012-us-east-01a"},
				"instance_type":   "shoebox",
				"nitwit_receipts": []any{"https://x"},
			},
			wantMatch: "instance_type",
		},
		{
			name: "multi_text element fails regex",
			raw: map[string]any{
				"bundle_tag":      "x",
				"canary_racks":    []any{"not-a-rack"},
				"instance_type":   "gb200-4x",
				"nitwit_receipts": []any{"https://x"},
			},
			wantMatch: "canary_racks failed validation",
		},
		{
			name: "multi_text wrong shape (string, not list)",
			raw: map[string]any{
				"bundle_tag":      "x",
				"canary_racks":    "dh3-r012-us-east-01a",
				"instance_type":   "gb200-4x",
				"nitwit_receipts": []any{"https://x"},
			},
			wantMatch: "canary_racks has wrong type",
		},
		{
			name: "optional url omitted is fine",
			raw: map[string]any{
				"bundle_tag":      "x",
				"canary_racks":    []any{"dh3-r012-us-east-01a"},
				"instance_type":   "gb200-4x",
				"nitwit_receipts": []any{"https://x"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateInputs(tpl, tc.raw)
			if tc.wantMatch == "" {
				if got != "" {
					t.Fatalf("got %q, want empty (success)", got)
				}
				return
			}
			if !strings.Contains(got, tc.wantMatch) {
				t.Fatalf("got %q, want substring %q", got, tc.wantMatch)
			}
		})
	}
}
