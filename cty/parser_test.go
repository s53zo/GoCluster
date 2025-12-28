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
	if _, ok := db.cacheGet("K1ABC/M"); !ok {
		t.Fatalf("expected cache entry keyed by raw input")
	}
	if _, ok := db.cacheGet("K1ABC"); ok {
		t.Fatalf("did not expect normalized cache entry before lookup")
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

func TestLookupCachesHits(t *testing.T) {
	db := loadSampleDatabase(t)
	call := "K1XYZ"
	info, ok := db.LookupCallsign(call)
	if !ok {
		t.Fatalf("expected prefix match for %s", call)
	}
	entry, ok := db.cacheGet(normalizeCallsign(call))
	if !ok {
		t.Fatalf("expected cache entry for %s", call)
	}
	if !entry.ok || entry.info == nil {
		t.Fatalf("expected cached hit with info")
	}
	if entry.info.Prefix != info.Prefix {
		t.Fatalf("cache info mismatch: want %s got %s", info.Prefix, entry.info.Prefix)
	}
	again, ok := db.LookupCallsign(call)
	if !ok || again != entry.info {
		t.Fatalf("expected cached pointer to be reused")
	}
}

func TestLookupCachesMisses(t *testing.T) {
	db := loadSampleDatabase(t)
	call := "ZZ9ZZA"
	if _, ok := db.LookupCallsign(call); ok {
		t.Fatalf("expected %s to miss", call)
	}
	norm := normalizeCallsign(call)
	entry, ok := db.cacheGet(norm)
	if !ok {
		t.Fatalf("expected cache miss entry for %s", call)
	}
	if entry.ok || entry.info != nil {
		t.Fatalf("expected cached miss entry to record failure")
	}
	if _, ok := db.LookupCallsign(call); ok {
		t.Fatalf("expected cached miss to stay false")
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
	if metrics.CacheHits != 2 {
		t.Fatalf("unexpected cache hits: %d", metrics.CacheHits)
	}
	if metrics.CacheEntries != 2 {
		t.Fatalf("unexpected cache entries: %d", metrics.CacheEntries)
	}
	if metrics.Validated != 2 {
		t.Fatalf("unexpected validated count: %d", metrics.Validated)
	}
	if metrics.ValidatedFromCache != 1 {
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
