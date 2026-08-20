// Package facts owns the run-metadata model: the registered fact-key
// vocabulary, scope strings, and the Emit helper that ties an event
// append to a projection write.
//
// Fact rows live in the store at scope=run | rack:<id> | node:<id>.
// Producers must use registered Key constants — unregistered keys
// are rejected by Emit (ErrUnknownFactKey). To add a new fact:
//
//  1. Add a typed const here.
//  2. Add it to the `registered` map.
//  3. Run TestRegistryContainsAllDeclaredConsts to confirm.
//
// The registry is intentional friction — agentic consumers code
// against this list, so additions get a code change rather than a
// runtime surprise.
package facts

import "errors"

// Key is the typed identifier for a single fact attribute. The
// underlying string is what gets stored in the DB column.
type Key string

const (
	// Operator-supplied (source="operator", scope="run").
	KeyBundleTag Key = "bundle_tag"
	KeyRequester Key = "requester"

	// Inventory-discovered (source="inventory", scope="rack:<id>").
	KeyInstanceType Key = "instance_type"
	KeySKU          Key = "sku"
	KeyVariant      Key = "variant"
	KeyGBGeneration Key = "gb_generation"
	KeyRegion       Key = "region"
	KeyCluster      Key = "cluster"
)

// registered is the closed set of writable fact keys. Producers
// (including operator-input promotion in createRunHandler) must use
// a Key whose value appears here.
var registered = map[Key]bool{
	KeyBundleTag:    true,
	KeyRequester:    true,
	KeyInstanceType: true,
	KeySKU:          true,
	KeyVariant:      true,
	KeyGBGeneration: true,
	KeyRegion:       true,
	KeyCluster:      true,
}

// IsRegistered reports whether k is a known fact key.
func IsRegistered(k Key) bool { return registered[k] }

// ErrUnknownFactKey is returned by Emit (Task 5) when a caller passes
// a Key that is not in the registry. Wrappers should use errors.Is to
// detect it.
var ErrUnknownFactKey = errors.New("facts: unknown fact key")
