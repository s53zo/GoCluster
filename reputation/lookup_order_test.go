package reputation

import (
	"net/netip"
	"testing"
	"time"

	"dxcluster/config"
)

func TestLookupPrefersCymru(t *testing.T) {
	cfg := config.ReputationConfig{
		Enabled:                true,
		StateTTLSeconds:        3600,
		StateMaxEntries:        1000,
		PrefixTTLSeconds:       3600,
		PrefixMaxEntries:       1000,
		LookupCacheTTLSeconds:  3600,
		LookupCacheMaxEntries:  1000,
		IPv4BucketSize:         10,
		IPv4BucketRefillPerSec: 10,
		IPv6BucketSize:         10,
		IPv6BucketRefillPerSec: 10,
	}
	gate, err := NewGate(cfg, nil)
	if err != nil {
		t.Fatalf("NewGate: %v", err)
	}
	now := time.Unix(0, 0).UTC()
	gate.lookupCymruFunc = func(addr netip.Addr, _ time.Time) (LookupResult, bool, bool) {
		return LookupResult{ASN: "64500", CountryCode: "US", Source: cymruSource, FetchedAt: now}, true, false
	}
	gate.lookupIPInfoFunc = func(addr netip.Addr, _ time.Time) LookupResult {
		t.Fatalf("ipinfo fallback should not be called when cymru succeeds")
		return LookupResult{}
	}

	ip := netip.MustParseAddr("8.8.8.8")
	_, cymru, countryKey, _, ok := gate.lookupIP(ip, now)
	if !ok {
		t.Fatalf("expected lookup to succeed")
	}
	if cymru.Source != cymruSource || cymru.ASN != "64500" || countryKey != "US" {
		t.Fatalf("unexpected cymru result: %+v country=%s", cymru, countryKey)
	}
}

func TestLookupFallsBackToIPInfo(t *testing.T) {
	cfg := config.ReputationConfig{
		Enabled:                true,
		StateTTLSeconds:        3600,
		StateMaxEntries:        1000,
		PrefixTTLSeconds:       3600,
		PrefixMaxEntries:       1000,
		LookupCacheTTLSeconds:  3600,
		LookupCacheMaxEntries:  1000,
		IPv4BucketSize:         10,
		IPv4BucketRefillPerSec: 10,
		IPv6BucketSize:         10,
		IPv6BucketRefillPerSec: 10,
	}
	gate, err := NewGate(cfg, nil)
	if err != nil {
		t.Fatalf("NewGate: %v", err)
	}
	now := time.Unix(0, 0).UTC()
	gate.lookupCymruFunc = func(addr netip.Addr, _ time.Time) (LookupResult, bool, bool) {
		return LookupResult{}, false, false
	}
	gate.lookupIPInfoFunc = func(addr netip.Addr, _ time.Time) LookupResult {
		return LookupResult{ASN: "15169", CountryCode: "US", Source: ipinfoSource, FetchedAt: now}
	}

	ip := netip.MustParseAddr("8.8.8.8")
	ipinfo, cymru, countryKey, _, ok := gate.lookupIP(ip, now)
	if !ok {
		t.Fatalf("expected lookup to succeed")
	}
	if cymru.Source != "" {
		t.Fatalf("expected cymru to be empty, got %+v", cymru)
	}
	if ipinfo.Source != ipinfoSource || ipinfo.ASN != "15169" || countryKey != "US" {
		t.Fatalf("unexpected ipinfo result: %+v country=%s", ipinfo, countryKey)
	}
}
