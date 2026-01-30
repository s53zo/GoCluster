package uls

import (
	"strings"
	"testing"
)

func TestAllowlistMatchByJurisdiction(t *testing.T) {
	content := `
# comment
US:^N2WQ$
ADIF212:^LZ
W1AW
`
	al, err := LoadAllowlist(strings.NewReader(content))
	if err != nil {
		t.Fatalf("load allowlist: %v", err)
	}
	allowlist.Store(al)
	defer allowlist.Store(nil)

	if !AllowlistMatch(291, "N2WQ") {
		t.Fatalf("expected US allowlist to match N2WQ")
	}
	if AllowlistMatch(291, "K1ABC") {
		t.Fatalf("did not expect allowlist to match K1ABC")
	}
	if !AllowlistMatch(291, "W1AW") {
		t.Fatalf("expected default jurisdiction allowlist to match W1AW")
	}
	if !AllowlistMatch(212, "LZ5VV") {
		t.Fatalf("expected ADIF212 allowlist to match LZ5VV")
	}
}
