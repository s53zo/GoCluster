package reputation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dxcluster/config"
)

func TestGateProbationAndRamp(t *testing.T) {
	tmp := t.TempDir()
	snapshot := filepath.Join(tmp, "location.csv")
	writeSnapshot(t, snapshot, []string{
		"start_ip,end_ip,country_code,country,continent_code,asn",
		"203.0.113.0,203.0.113.255,US,United States,NA,AS64500",
	})

	cfg := config.ReputationConfig{
		Enabled:                  true,
		IPInfoSnapshotPath:       snapshot,
		ReputationDir:            tmp,
		InitialWaitSeconds:       10,
		RampWindowSeconds:        10,
		PerBandStart:             1,
		PerBandCap:               2,
		TotalCapStart:            2,
		TotalCapPostRamp:         4,
		TotalCapRampDelaySeconds: 0,
		IPv4BucketSize:           100,
		IPv4BucketRefillPerSec:   100,
		IPv6BucketSize:           100,
		IPv6BucketRefillPerSec:   100,
		StateTTLSeconds:          3600,
		StateMaxEntries:          1000,
		PrefixTTLSeconds:         3600,
		PrefixMaxEntries:         1000,
		LookupCacheTTLSeconds:    3600,
		LookupCacheMaxEntries:    1000,
		DisagreementResetOnNew:   true,
		ResetOnNewASN:            true,
		CountryFlipScope:         "country",
	}

	gate, err := NewGate(cfg, nil)
	if err != nil {
		t.Fatalf("NewGate failed: %v", err)
	}
	if err := gate.LoadSnapshot(); err != nil {
		t.Fatalf("LoadSnapshot failed: %v", err)
	}

	call := "K1ABC"
	ip := "203.0.113.10"
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	gate.RecordLogin(call, ip, start)

	if dec := gate.Check(Request{Call: call, Band: "20m", IP: ip, Now: start.Add(5 * time.Second)}); dec.Reason != DropProbation {
		t.Fatalf("expected probation drop, got %+v", dec)
	}

	t1 := start.Add(11 * time.Second)
	if dec := gate.Check(Request{Call: call, Band: "20m", IP: ip, Now: t1}); !dec.Allow {
		t.Fatalf("expected allow after wait, got %+v", dec)
	}
	if dec := gate.Check(Request{Call: call, Band: "20m", IP: ip, Now: t1}); dec.Reason != DropBandCap {
		t.Fatalf("expected band cap drop, got %+v", dec)
	}
	if dec := gate.Check(Request{Call: call, Band: "40m", IP: ip, Now: t1}); !dec.Allow {
		t.Fatalf("expected second-band allow, got %+v", dec)
	}
	if dec := gate.Check(Request{Call: call, Band: "80m", IP: ip, Now: t1}); dec.Reason != DropTotalCap {
		t.Fatalf("expected total cap drop, got %+v", dec)
	}

	t2 := start.Add(21 * time.Second)
	if dec := gate.Check(Request{Call: call, Band: "20m", IP: ip, Now: t2}); !dec.Allow {
		t.Fatalf("expected allow after ramp, got %+v", dec)
	}
	if dec := gate.Check(Request{Call: call, Band: "20m", IP: ip, Now: t2}); !dec.Allow {
		t.Fatalf("expected second per-band allow, got %+v", dec)
	}
	if dec := gate.Check(Request{Call: call, Band: "20m", IP: ip, Now: t2}); dec.Reason != DropBandCap {
		t.Fatalf("expected band cap drop after ramp, got %+v", dec)
	}
	if dec := gate.Check(Request{Call: call, Band: "40m", IP: ip, Now: t2}); !dec.Allow {
		t.Fatalf("expected allow within total cap, got %+v", dec)
	}
	if dec := gate.Check(Request{Call: call, Band: "80m", IP: ip, Now: t2}); !dec.Allow {
		t.Fatalf("expected allow within total cap, got %+v", dec)
	}
	if dec := gate.Check(Request{Call: call, Band: "15m", IP: ip, Now: t2}); dec.Reason != DropTotalCap {
		t.Fatalf("expected total cap drop after ramp, got %+v", dec)
	}
}

func TestPrefixLimiterDrop(t *testing.T) {
	tmp := t.TempDir()
	snapshot := filepath.Join(tmp, "location.csv")
	writeSnapshot(t, snapshot, []string{
		"start_ip,end_ip,country_code,country,continent_code,asn",
		"203.0.113.0,203.0.113.255,US,United States,NA,AS64500",
	})

	cfg := config.ReputationConfig{
		Enabled:                true,
		IPInfoSnapshotPath:     snapshot,
		ReputationDir:          tmp,
		InitialWaitSeconds:     1,
		RampWindowSeconds:      10,
		PerBandStart:           10,
		PerBandCap:             10,
		TotalCapStart:          10,
		TotalCapPostRamp:       10,
		IPv4BucketSize:         1,
		IPv4BucketRefillPerSec: 1,
		StateTTLSeconds:        3600,
		StateMaxEntries:        1000,
		PrefixTTLSeconds:       3600,
		PrefixMaxEntries:       1000,
		LookupCacheTTLSeconds:  3600,
		LookupCacheMaxEntries:  1000,
		DisagreementResetOnNew: true,
		ResetOnNewASN:          true,
		CountryFlipScope:       "country",
	}

	gate, err := NewGate(cfg, nil)
	if err != nil {
		t.Fatalf("NewGate failed: %v", err)
	}
	if err := gate.LoadSnapshot(); err != nil {
		t.Fatalf("LoadSnapshot failed: %v", err)
	}

	call := "K1ABC"
	ip := "203.0.113.10"
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	gate.RecordLogin(call, ip, start)

	now := start.Add(2 * time.Second)
	if dec := gate.Check(Request{Call: call, Band: "20m", IP: ip, Now: now}); !dec.Allow {
		t.Fatalf("expected first prefix allow, got %+v", dec)
	}
	if dec := gate.Check(Request{Call: call, Band: "40m", IP: ip, Now: now}); dec.Reason != DropPrefixCap {
		t.Fatalf("expected prefix cap drop, got %+v", dec)
	}
}

func writeSnapshot(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
}
