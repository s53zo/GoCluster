package pathreliability

import "testing"

func requireH3Mappings(t *testing.T) {
	t.Helper()
	if err := InitH3MappingsFromDir("data/h3"); err != nil {
		t.Skipf("InitH3Mappings unavailable: %v", err)
	}
}
