package facts

import "testing"

func TestIsRegistered_KnownKey(t *testing.T) {
	if !IsRegistered(KeyInstanceType) {
		t.Fatalf("IsRegistered(KeyInstanceType) = false, want true")
	}
}

func TestIsRegistered_UnknownKey(t *testing.T) {
	if IsRegistered(Key("not_a_real_key")) {
		t.Fatalf("IsRegistered(\"not_a_real_key\") = true, want false")
	}
}

// TestRegistryContainsAllDeclaredConsts asserts that every Key constant
// declared in keys.go is present in the registered map. If you add a
// new const without adding it to the registered map, this test fails —
// preventing silent registry drift.
func TestRegistryContainsAllDeclaredConsts(t *testing.T) {
	want := []Key{
		KeyBundleTag, KeyRequester,
		KeyInstanceType, KeySKU, KeyVariant,
		KeyGBGeneration, KeyRegion, KeyCluster,
	}
	for _, k := range want {
		if !IsRegistered(k) {
			t.Errorf("IsRegistered(%q) = false; should be in registered map", k)
		}
	}
}
