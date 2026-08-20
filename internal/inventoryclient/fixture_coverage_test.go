package inventoryclient

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// rackIDPattern matches the canonical CoreWeave rack format used by
// fw-release-canary's canary_racks regex.
var rackIDPattern = regexp.MustCompile(`dh\d+-r\d{3}-[a-z]+-[a-z]+-\d{2}[a-z]`)

// TestSeedDemoFixtures_CoversReferences asserts every rack id mentioned
// in cmd/qac/seed_demo.go and templates/firmware-release-canary.yaml
// resolves in SeedDemoFixtures. Drift means the demo wiring would 422
// on inventory_unresolved, which is a deployment-time surprise the
// fixture coverage test prevents.
func TestSeedDemoFixtures_CoversReferences(t *testing.T) {
	// Find the project root by walking up until we see go.mod.
	root, err := findProjectRoot()
	if err != nil {
		t.Fatalf("findProjectRoot: %v", err)
	}

	fixtures := SeedDemoFixtures()

	for _, relPath := range []string{
		"cmd/qac/seed_demo.go",
		"templates/firmware-release-canary.yaml",
	} {
		body, err := os.ReadFile(filepath.Join(root, relPath))
		if err != nil {
			t.Fatalf("read %s: %v", relPath, err)
		}
		ids := rackIDPattern.FindAllString(string(body), -1)
		for _, id := range ids {
			if _, ok := fixtures[id]; !ok {
				t.Errorf("%s references rack id %q which is missing from SeedDemoFixtures", relPath, id)
			}
		}
	}
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
