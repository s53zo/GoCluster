package solarweather

import (
	"testing"
	"time"
)

func findRLevelIndex(cfg Config, name string) int {
	for i, lvl := range cfg.rLevels {
		if lvl.Name == name {
			return i
		}
	}
	return -1
}

func findGLevelIndex(cfg Config, name string) int {
	for i, lvl := range cfg.gLevels {
		if lvl.Name == name {
			return i
		}
	}
	return -1
}

func TestSelectOverrideBands(t *testing.T) {
	cfg := DefaultConfig()
	cfg.normalize()

	r2 := findRLevelIndex(cfg, "R2")
	r3 := findRLevelIndex(cfg, "R3")
	r4 := findRLevelIndex(cfg, "R4")
	g2 := findGLevelIndex(cfg, "G2")
	g3 := findGLevelIndex(cfg, "G3")
	g4 := findGLevelIndex(cfg, "G4")
	if r2 < 0 || r3 < 0 || r4 < 0 || g2 < 0 || g3 < 0 || g4 < 0 {
		t.Fatalf("expected default levels to exist")
	}

	day := GateDecision{Daylight: true}
	high := GateDecision{HighLat: true}
	both := GateDecision{Daylight: true, HighLat: true}

	cases := []struct {
		name     string
		band     string
		rIdx     int
		gIdx     int
		gate     GateDecision
		wantKind OverrideKind
	}{
		{"R2 in-band", "80m", r2, -1, day, OverrideR},
		{"R2 out-of-band", "17m", r2, -1, day, OverrideNone},
		{"R3 in-band", "10m", r3, -1, day, OverrideR},
		{"R3 out-of-band", "160m", r3, -1, day, OverrideNone},
		{"R4 in-band", "12m", r4, -1, day, OverrideR},
		{"R4 out-of-band", "160m", r4, -1, day, OverrideNone},
		{"G2 in-band", "20m", -1, g2, high, OverrideG},
		{"G2 out-of-band", "40m", -1, g2, high, OverrideNone},
		{"G3 in-band", "40m", -1, g3, high, OverrideG},
		{"G3 out-of-band", "80m", -1, g3, high, OverrideNone},
		{"G4 in-band", "160m", -1, g4, high, OverrideG},
		{"G4 in-band upper", "10M", -1, g4, high, OverrideG},
		{"Unknown band", "2m", r3, g4, both, OverrideNone},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, kind := selectOverride(cfg, tc.band, tc.gate, tc.rIdx, tc.gIdx)
			if kind != tc.wantKind {
				t.Fatalf("expected %v, got %v", tc.wantKind, kind)
			}
		})
	}

	// Precedence: R only applies when the band is eligible; otherwise G can apply.
	if _, kind := selectOverride(cfg, "160m", both, r2, g4); kind != OverrideG {
		t.Fatalf("expected G when R band is ineligible")
	}
	if _, kind := selectOverride(cfg, "80m", both, r2, g4); kind != OverrideR {
		t.Fatalf("expected R to win when band is eligible")
	}

	// Unknown geometry suppresses overrides.
	if _, kind := selectOverride(cfg, "80m", GateDecision{Daylight: true, HighLat: true, DaylightUnknown: true}, r2, g4); kind != OverrideNone {
		t.Fatalf("expected no override when geometry is unknown")
	}
}

func TestSeedRLevelsLocked(t *testing.T) {
	cfg := DefaultConfig()
	cfg.normalize()
	m := NewManager(cfg, nil)
	now := time.Date(2026, 1, 29, 12, 0, 0, 0, time.UTC)
	sampleTime := now.Add(-30 * time.Minute)
	samples := []goesSample{{Time: sampleTime, Flux: 1e-4}}

	m.mu.Lock()
	seeded := m.seedRLevelsLocked(now, samples)
	m.mu.Unlock()
	if !seeded {
		t.Fatalf("expected seeded")
	}

	r3 := findRLevelIndex(m.cfg, "R3")
	if r3 < 0 {
		t.Fatalf("expected R3 level")
	}
	want := sampleTime.Add(m.cfg.rLevels[r3].Hold)
	m.mu.RLock()
	got := m.state.RActiveUntil[r3]
	m.mu.RUnlock()
	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
