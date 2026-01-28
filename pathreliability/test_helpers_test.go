package pathreliability

import "testing"

func requireH3Mappings(t *testing.T) {
	t.Helper()
	if err := InitH3Mappings(); err != nil {
		t.Skipf("InitH3Mappings unavailable: %v", err)
	}
}
