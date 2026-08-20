package lifecycleclient

import (
	"strings"
	"testing"
)

func TestRenderRLCCIgnoredQuery_SubstitutesRack(t *testing.T) {
	q := renderRLCCIgnoredQuery("dh3-r012-us-east-01a")
	if !strings.Contains(q, `rack="dh3-r012-us-east-01a"`) {
		t.Errorf("query missing rack label: %q", q)
	}
	if strings.Contains(q, "{rack}") {
		t.Errorf("query still has unsubstituted token: %q", q)
	}
}

func TestRenderRLCCIgnoredQuery_EmbeddedFileLoaded(t *testing.T) {
	// Indirect check: rendering with empty rack still produces output
	// (proves the embed is non-empty).
	q := renderRLCCIgnoredQuery("")
	if q == "" {
		t.Errorf("rendered query is empty — embed.FS not loading the .promql file")
	}
}

func TestParseActionLabels(t *testing.T) {
	cases := []struct {
		name       string
		deviceslot string
		wantRack   string
		wantNode   string
	}{
		{
			name:       "well-formed",
			deviceslot: "dh3-r012-node-07-us-east-01a",
			wantRack:   "dh3-r012-us-east-01a",
			wantNode:   "node-07",
		},
		{
			name:       "no -node- segment falls back to (deviceslot, \"\")",
			deviceslot: "garbage",
			wantRack:   "garbage",
			wantNode:   "",
		},
		{
			name:       "fewer than 3 trailing dash-segments falls back",
			deviceslot: "dh3-r012-node-07",
			wantRack:   "dh3-r012-node-07",
			wantNode:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rack, node := parseActionLabels(tc.deviceslot)
			if rack != tc.wantRack {
				t.Errorf("rack = %q, want %q", rack, tc.wantRack)
			}
			if node != tc.wantNode {
				t.Errorf("node = %q, want %q", node, tc.wantNode)
			}
		})
	}
}
