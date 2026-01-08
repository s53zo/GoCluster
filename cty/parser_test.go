package cty

import (
	"strings"
	"testing"
)

const samplePLIST = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
<key>K1ABC</key>
	<dict>
		<key>Country</key>
		<string>Alpha</string>
		<key>Prefix</key>
		<string>K1ABC</string>
		<key>ExactCallsign</key>
		<true/>
	</dict>
<key>K1</key>
	<dict>
		<key>Country</key>
		<string>Alpha</string>
		<key>Prefix</key>
		<string>K1</string>
		<key>ExactCallsign</key>
		<false/>
	</dict>
<key>XM3</key>
	<dict>
		<key>Country</key>
		<string>Zed</string>
		<key>Prefix</key>
		<string>XM3</string>
		<key>ExactCallsign</key>
		<false/>
	</dict>
<key>W6</key>
	<dict>
		<key>Country</key>
		<string>Delta</string>
		<key>Prefix</key>
		<string>W6</string>
		<key>ExactCallsign</key>
		<false/>
	</dict>
<key>FO/</key>
	<dict>
		<key>Country</key>
		<string>Slashland</string>
		<key>Prefix</key>
		<string>FO/</string>
		<key>ExactCallsign</key>
		<false/>
	</dict>
</dict>
</plist>`

func loadSampleDatabase(t *testing.T) *CTYDatabase {
	t.Helper()
	db, err := LoadCTYDatabaseFromReader(strings.NewReader(samplePLIST))
	if err != nil {
		t.Fatalf("load sample database: %v", err)
	}
	return db
}

func TestLookupExactCallsign(t *testing.T) {
	db := loadSampleDatabase(t)
	info, ok := db.LookupCallsign("K1ABC")
	if !ok {
		t.Fatalf("expected K1ABC to resolve")
	}
	if info.Country != "Alpha" {
		t.Fatalf("expected Alpha, got %q", info.Country)
	}
}

func TestLookupLongestPrefix(t *testing.T) {
	db := loadSampleDatabase(t)
	info, ok := db.LookupCallsign("K1XYZ")
	if !ok {
		t.Fatalf("expected prefix match for K1XYZ")
	}
	if info.Prefix != "K1" {
		t.Fatalf("expected prefix K1, got %q", info.Prefix)
	}
}

func TestLookupDoesNotNormalizeInput(t *testing.T) {
	db := loadSampleDatabase(t)
	if _, ok := db.LookupCallsign("K1ABC/M"); !ok {
		t.Fatalf("expected prefix match for K1ABC/M")
	}
	info, ok := db.LookupCallsign("K1ABC")
	if !ok {
		t.Fatalf("expected normalized call to resolve")
	}
	if info.Prefix != "K1ABC" {
		t.Fatalf("expected prefix K1ABC, got %q", info.Prefix)
	}
}

func TestLongerPrefixFallback(t *testing.T) {
	db := loadSampleDatabase(t)
	info, ok := db.LookupCallsign("XM3A")
	if !ok {
		t.Fatalf("expected longest prefix match for XM3A")
	}
	if info.Country != "Zed" {
		t.Fatalf("expected Zed, got %q", info.Country)
	}
}

func TestLookupMetrics(t *testing.T) {
	db := loadSampleDatabase(t)
	if _, ok := db.LookupCallsign("K1XYZ"); !ok {
		t.Fatalf("expected K1XYZ to resolve")
	}
	if _, ok := db.LookupCallsign("K1XYZ"); !ok {
		t.Fatalf("expected K1XYZ cache hit to resolve")
	}
	db.LookupCallsign("ZZ9ZZA") // miss uncached
	db.LookupCallsign("ZZ9ZZA") // miss from cache

	metrics := db.Metrics()
	if metrics.TotalLookups != 4 {
		t.Fatalf("unexpected total lookups: %d", metrics.TotalLookups)
	}
	if metrics.CacheHits != 0 {
		t.Fatalf("unexpected cache hits: %d", metrics.CacheHits)
	}
	if metrics.CacheEntries != 0 {
		t.Fatalf("unexpected cache entries: %d", metrics.CacheEntries)
	}
	if metrics.Validated != 2 {
		t.Fatalf("unexpected validated count: %d", metrics.Validated)
	}
	if metrics.ValidatedFromCache != 0 {
		t.Fatalf("unexpected validated-from-cache count: %d", metrics.ValidatedFromCache)
	}
}

func TestLookupPrefixWithSlashInCallsign(t *testing.T) {
	db := loadSampleDatabase(t)
	info, ok := db.LookupCallsign("W6/UT5UF")
	if !ok {
		t.Fatalf("expected W6/UT5UF to resolve via prefix")
	}
	if info.Prefix != "W6" {
		t.Fatalf("expected prefix W6, got %q", info.Prefix)
	}
	if info.Country != "Delta" {
		t.Fatalf("expected Delta, got %q", info.Country)
	}
}

func TestLookupPrefixWithSlashKey(t *testing.T) {
	db := loadSampleDatabase(t)
	info, ok := db.LookupCallsign("FO/ABC")
	if !ok {
		t.Fatalf("expected FO/ABC to resolve via slash prefix key")
	}
	if info.Prefix != "FO/" {
		t.Fatalf("expected prefix FO/, got %q", info.Prefix)
	}
	if info.Country != "Slashland" {
		t.Fatalf("expected Slashland, got %q", info.Country)
	}
}

func TestLookupPortablePrefersShortestSegment(t *testing.T) {
	db := loadSampleDatabase(t)
	info, ok := db.LookupCallsignPortable("K1ABC/W6")
	if !ok {
		t.Fatalf("expected K1ABC/W6 to resolve")
	}
	if info.Prefix != "W6" {
		t.Fatalf("expected W6, got %q", info.Prefix)
	}
	info, ok = db.LookupCallsignPortable("W6/K1ABC")
	if !ok {
		t.Fatalf("expected W6/K1ABC to resolve")
	}
	if info.Prefix != "W6" {
		t.Fatalf("expected W6, got %q", info.Prefix)
	}
}

func TestLookupPortableFallsBackToFullCall(t *testing.T) {
	db := loadSampleDatabase(t)
	info, ok := db.LookupCallsignPortable("FO/ABC")
	if !ok {
		t.Fatalf("expected FO/ABC to resolve")
	}
	if info.Prefix != "FO/" {
		t.Fatalf("expected prefix FO/, got %q", info.Prefix)
	}
}

func TestLookupPortableIgnoresBeaconSuffix(t *testing.T) {
	db := loadSampleDatabase(t)
	info, ok := db.LookupCallsignPortable("K1ABC/B")
	if !ok {
		t.Fatalf("expected K1ABC/B to resolve")
	}
	if info.Prefix != "K1ABC" {
		t.Fatalf("expected prefix K1ABC, got %q", info.Prefix)
	}
}
