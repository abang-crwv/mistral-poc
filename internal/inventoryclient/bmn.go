package inventoryclient

import (
	"fmt"
	"regexp"
	"strconv"
)

// BMN is one compute tray entry, the per-CT key fwqual uses for
// canary attribution. Deviceslot is the canonical key (matches
// ^<dc>-r<NNN>-node-[0-9]+-<region>$); BMNName is the short identifier
// emitted by Prometheus (e.g. "s90txs64"), or "" when inventory reports
// no BMN for the tray (a real data gap — callers render it as such, not
// as the deviceslot). CTPosition is parsed out of the deviceslot for
// convenient sort/display ordering.
type BMN struct {
	Deviceslot string `json:"deviceslot"`
	BMNName    string `json:"bmn_name"`
	Rack       string `json:"rack"`
	Zone       string `json:"zone"`
	CTPosition int    `json:"ct_position,omitempty"`
}

// deviceslotRe extracts the node position from a canonical deviceslot.
// Capture group 1 is the position; the remainder is captured implicitly
// so the regex is anchored.
var deviceslotRe = regexp.MustCompile(`-node-(\d+)-`)

// positionFromDeviceslot returns the CT position parsed from a deviceslot
// string, or 0 when the deviceslot doesn't match the canonical shape.
// fwqual stores 0 as "unknown" — callers should treat 0 as a sort key,
// not a real position.
func positionFromDeviceslot(slot string) int {
	m := deviceslotRe.FindStringSubmatch(slot)
	if len(m) < 2 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

// zoneFromRackID extracts the trailing "us-east-04b"-style suffix from
// a rack id like "dh1-r037-us-east-04b". The first two dash-separated
// components are dropped; everything after is the zone.
func zoneFromRackID(rackID string) string {
	dashes := 0
	for i, c := range rackID {
		if c == '-' {
			dashes++
			if dashes == 2 {
				return rackID[i+1:]
			}
		}
	}
	return ""
}

// ErrEmptyRack is returned by ResolveBMNs when the rack id is empty.
var ErrEmptyRack = fmt.Errorf("inventoryclient: empty rack id")
