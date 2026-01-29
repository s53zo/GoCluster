package solarweather

import (
	"testing"
	"time"
)

func TestSummaryBothActive(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.normalize()
	m := NewManager(cfg, nil)
	now := time.Date(2026, 1, 29, 12, 0, 0, 0, time.UTC)

	r3 := findRLevelIndex(cfg, "R3")
	g2 := findGLevelIndex(cfg, "G2")
	if r3 < 0 || g2 < 0 {
		t.Fatalf("expected default levels")
	}

	m.mu.Lock()
	m.state.RActiveUntil = make([]time.Time, len(cfg.rLevels))
	m.state.RActiveUntil[r3] = now.Add(10 * time.Minute)
	m.state.GOESTime = now
	m.state.GOESUpdatedAt = now
	m.state.Kp = cfg.gLevels[g2].MinKp
	m.state.KpTime = now
	m.state.KpUpdatedAt = now
	m.mu.Unlock()

	got := m.Summary(now)
	want := "SOLAR R3 G2 impacting 80m/60m/40m/30m/20m/17m/15m/12m/10m"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
