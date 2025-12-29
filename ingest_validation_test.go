package main

import (
	"strings"
	"testing"
	"time"

	"dxcluster/cty"
	"dxcluster/spot"
)

const ingestSamplePLIST = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
<key>DL</key>
	<dict>
		<key>Country</key>
		<string>Germany</string>
		<key>Prefix</key>
		<string>DL</string>
		<key>ADIF</key>
		<integer>230</integer>
		<key>CQZone</key>
		<integer>14</integer>
		<key>ITUZone</key>
		<integer>28</integer>
		<key>Continent</key>
		<string>EU</string>
		<key>ExactCallsign</key>
		<false/>
	</dict>
<key>K1</key>
	<dict>
		<key>Country</key>
		<string>United States</string>
		<key>Prefix</key>
		<string>K1</string>
		<key>ADIF</key>
		<integer>291</integer>
		<key>CQZone</key>
		<integer>5</integer>
		<key>ITUZone</key>
		<integer>8</integer>
		<key>Continent</key>
		<string>NA</string>
		<key>ExactCallsign</key>
		<false/>
	</dict>
</dict>
</plist>`

func loadIngestCTY(t *testing.T) *cty.CTYDatabase {
	t.Helper()
	db, err := cty.LoadCTYDatabaseFromReader(strings.NewReader(ingestSamplePLIST))
	if err != nil {
		t.Fatalf("load ingest CTY: %v", err)
	}
	return db
}

func TestCTYInfoCacheTTLAndEviction(t *testing.T) {
	cache := newCTYInfoCache(1, 10*time.Second)
	now := time.Now()
	infoA := &cty.PrefixInfo{Country: "A"}
	infoB := &cty.PrefixInfo{Country: "B"}

	cache.set("CALLA", infoA, true, now)
	if got, ok := cache.get("CALLA", now); !ok || got != infoA {
		t.Fatalf("expected cache hit for CALLA")
	}

	cache.set("CALLB", infoB, true, now)
	if _, ok := cache.get("CALLA", now); ok {
		t.Fatalf("expected CALLA to be evicted")
	}
	if got, ok := cache.get("CALLB", now); !ok || got != infoB {
		t.Fatalf("expected cache hit for CALLB")
	}

	expired := now.Add(11 * time.Second)
	if _, ok := cache.get("CALLB", expired); ok {
		t.Fatalf("expected CALLB to expire")
	}
}

func TestIngestValidatorPreservesGrids(t *testing.T) {
	licCache = newLicenseCache(5 * time.Minute)
	db := loadIngestCTY(t)
	v := newIngestValidator(func() *cty.CTYDatabase { return db }, make(chan *spot.Spot, 1), nil, 8, time.Minute)
	v.isLicensedUS = func(call string) bool { return true }

	s := spot.NewSpotNormalized("DL1ABC", "K1ABC", 14074.0, "FT8")
	s.SourceType = spot.SourcePSKReporter
	s.DXMetadata.Grid = "JN58"
	s.DEMetadata.Grid = "FN42"

	if !v.validateSpot(s) {
		t.Fatalf("expected spot to pass validation")
	}
	if s.DXMetadata.Grid != "JN58" {
		t.Fatalf("expected DX grid to be preserved, got %q", s.DXMetadata.Grid)
	}
	if s.DEMetadata.Grid != "FN42" {
		t.Fatalf("expected DE grid to be preserved, got %q", s.DEMetadata.Grid)
	}
	if s.DXMetadata.ADIF != 230 {
		t.Fatalf("expected DX ADIF 230, got %d", s.DXMetadata.ADIF)
	}
	if s.DEMetadata.ADIF != 291 {
		t.Fatalf("expected DE ADIF 291, got %d", s.DEMetadata.ADIF)
	}
}

func TestIngestValidatorDropsUnlicensedUSSpotter(t *testing.T) {
	licCache = newLicenseCache(5 * time.Minute)
	db := loadIngestCTY(t)
	var gotSource, gotRole, gotCall, gotMode string
	var gotFreq float64
	reported := false

	v := newIngestValidator(func() *cty.CTYDatabase { return db }, make(chan *spot.Spot, 1), nil, 8, time.Minute)
	v.unlicensedReporter = func(source, role, call, mode string, freq float64) {
		reported = true
		gotSource = source
		gotRole = role
		gotCall = call
		gotMode = mode
		gotFreq = freq
	}
	v.isLicensedUS = func(call string) bool { return false }

	s := spot.NewSpotNormalized("DL1ABC", "K1ABC", 14074.0, "FT8")
	s.SourceNode = "PSKREPORTER"

	if v.validateSpot(s) {
		t.Fatalf("expected unlicensed spotter to be dropped")
	}
	if !reported {
		t.Fatalf("expected unlicensed reporter to be invoked")
	}
	if gotSource != "PSKREPORTER" {
		t.Fatalf("expected source PSKREPORTER, got %q", gotSource)
	}
	if gotRole != "DE" {
		t.Fatalf("expected role DE, got %q", gotRole)
	}
	if gotCall != "K1ABC" {
		t.Fatalf("expected call K1ABC, got %q", gotCall)
	}
	if gotMode != "FT8" {
		t.Fatalf("expected mode FT8, got %q", gotMode)
	}
	if gotFreq != 14074.0 {
		t.Fatalf("expected frequency 14074.0, got %.1f", gotFreq)
	}
}
