package uls

import "testing"

func TestIsLicensedUSFailsOpenDuringRefresh(t *testing.T) {
	SetRefreshInProgress(true)
	defer SetRefreshInProgress(false)

	if ok := IsLicensedUS("W1AW"); !ok {
		t.Fatal("expected IsLicensedUS to fail open during refresh")
	}
}
