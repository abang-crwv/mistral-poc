package alertclient

import (
	"testing"

	"qac/internal/inventoryclient"
)

// TestFixtureCoverage_AllInventoryRacksHaveAlertEntry asserts every rack
// id in inventoryclient.SeedDemoFixtures also appears in SeedDemoAlerts.
// Without this guard, adding a rack to one fixture and forgetting the
// other would silently break end-to-end probe runs against the new rack.
func TestFixtureCoverage_AllInventoryRacksHaveAlertEntry(t *testing.T) {
	alerts := SeedDemoAlerts()
	inv := inventoryclient.SeedDemoFixtures()
	for rackID := range inv {
		if _, ok := alerts[rackID]; !ok {
			t.Errorf("inventoryclient rack %q has no SeedDemoAlerts entry", rackID)
		}
	}
}
