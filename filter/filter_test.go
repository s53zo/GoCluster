package filter

import (
	"testing"

	"dxcluster/spot"
)

func TestNewFilterDefaultsAllowAllContinentsAndZones(t *testing.T) {
	f := NewFilter()
	s := &spot.Spot{
		Mode: "CW",
		Band: "20m",
		DXMetadata: spot.CallMetadata{
			Continent: "EU",
			CQZone:    14,
		},
		DEMetadata: spot.CallMetadata{
			Continent: "NA",
			CQZone:    5,
		},
	}
	if !f.Matches(s) {
		t.Fatalf("default filter should allow all continents/zones")
	}
}

func TestContinentFilters(t *testing.T) {
	f := NewFilter()
	f.SetDXContinent("EU", true)

	pass := &spot.Spot{
		Mode: "CW",
		Band: "20m",
		DXMetadata: spot.CallMetadata{
			Continent: "EU",
			CQZone:    14,
		},
		DEMetadata: spot.CallMetadata{
			Continent: "NA",
			CQZone:    5,
		},
	}
	if !f.Matches(pass) {
		t.Fatalf("expected EU DX continent to pass")
	}

	fail := &spot.Spot{
		Mode: "CW",
		Band: "20m",
		DXMetadata: spot.CallMetadata{
			Continent: "NA",
			CQZone:    14,
		},
		DEMetadata: spot.CallMetadata{
			Continent: "NA",
			CQZone:    5,
		},
	}
	if f.Matches(fail) {
		t.Fatalf("expected non-matching DX continent to be rejected")
	}

	missing := &spot.Spot{
		Mode: "CW",
		Band: "20m",
		DXMetadata: spot.CallMetadata{
			Continent: "",
			CQZone:    14,
		},
		DEMetadata: spot.CallMetadata{
			Continent: "EU",
			CQZone:    5,
		},
	}
	if f.Matches(missing) {
		t.Fatalf("expected missing DX continent to be rejected when DX continent filter is active")
	}
}

func TestZoneFilters(t *testing.T) {
	f := NewFilter()
	f.SetDXZone(14, true)

	pass := &spot.Spot{
		Mode: "CW",
		Band: "20m",
		DXMetadata: spot.CallMetadata{
			Continent: "EU",
			CQZone:    14,
		},
		DEMetadata: spot.CallMetadata{
			Continent: "NA",
			CQZone:    5,
		},
	}
	if !f.Matches(pass) {
		t.Fatalf("expected matching DX zone to pass")
	}

	otherZone := &spot.Spot{
		Mode: "CW",
		Band: "20m",
		DXMetadata: spot.CallMetadata{
			Continent: "EU",
			CQZone:    15,
		},
		DEMetadata: spot.CallMetadata{
			Continent: "NA",
			CQZone:    5,
		},
	}
	if f.Matches(otherZone) {
		t.Fatalf("expected non-matching DX zone to be rejected")
	}

	missing := &spot.Spot{
		Mode: "CW",
		Band: "20m",
		DXMetadata: spot.CallMetadata{
			Continent: "EU",
			CQZone:    0,
		},
		DEMetadata: spot.CallMetadata{
			Continent: "NA",
			CQZone:    5,
		},
	}
	if f.Matches(missing) {
		t.Fatalf("expected missing DX zone to be rejected when DX zone filter is active")
	}
}

func TestNormalizeDefaultsRestoresPermissiveFilters(t *testing.T) {
	var f Filter
	f.normalizeDefaults()
	if !f.AllDXContinents || !f.AllDEContinents || !f.AllDXZones || !f.AllDEZones {
		t.Fatalf("expected normalizeDefaults to restore permissive continent/zone flags")
	}
}
