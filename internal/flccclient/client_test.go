package flccclient

import (
	"bytes"
	"os"
	"reflect"
	"testing"
)

func TestFixtureMatchesTestdata(t *testing.T) {
	body, err := os.ReadFile("testdata/flcc-values.yaml")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	parsed, err := ParseValuesYAML(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("parse testdata: %v", err)
	}
	if !reflect.DeepEqual(parsed, Fixture) {
		t.Fatalf("flccclient Fixture drift detected.\n\nparsed: %#v\n\nFixture: %#v\n\nRun `go generate ./internal/flccclient/` to refresh Fixture.",
			parsed, Fixture)
	}
}
