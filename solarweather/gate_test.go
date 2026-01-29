package solarweather

import (
	"math"
	"testing"
)

func TestDaylightFractionSplit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	sun := latLonToVec(0, 0).Normalize()
	a := latLonToVec(0, 45).Normalize()
	b := latLonToVec(0, 135).Normalize()
	frac, nearTerm, unknown := daylightFraction(a, b, sun, cfg)
	if unknown || nearTerm {
		t.Fatalf("expected normal case, unknown=%v near=%v", unknown, nearTerm)
	}
	if math.Abs(frac-0.5) > 1e-6 {
		t.Fatalf("expected 0.5, got %v", frac)
	}
}

func TestHighLatFractionDipoleMeridian(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	a := latLonToVec(0, 0).Normalize()
	b := latLonToVec(90, 0).Normalize()
	dipole := Vec3{Z: 1}
	frac, maxAbs := highLatFractionDipole(a, b, dipole, 45, cfg)
	if math.Abs(frac-0.5) > 1e-6 {
		t.Fatalf("expected 0.5, got %v", frac)
	}
	if math.Abs(maxAbs-90) > 1e-6 {
		t.Fatalf("expected max abs 90 deg, got %v", maxAbs)
	}
}

func TestDaylightHysteresisHold(t *testing.T) {
	cfg := DefaultConfig()
	if got := applyDaylightHysteresis(false, 0.6, false, false, cfg); !got {
		t.Fatalf("expected enter to be true")
	}
	if got := applyDaylightHysteresis(true, 0.4, false, false, cfg); got {
		t.Fatalf("expected exit to be false")
	}
	if got := applyDaylightHysteresis(true, 0.5, true, false, cfg); !got {
		t.Fatalf("expected near-terminator hold to keep true")
	}
}

func TestHighLatHysteresis(t *testing.T) {
	cfg := DefaultConfig()
	lEdge := 55.0
	enterMax := lEdge + cfg.HighLat.EnterMaxAbsOffset
	exitMax := lEdge + cfg.HighLat.ExitMaxAbsOffset
	if got := applyHighLatHysteresis(false, enterMax+0.1, 0.0, lEdge, cfg); !got {
		t.Fatalf("expected enter to be true")
	}
	if got := applyHighLatHysteresis(true, exitMax-0.1, cfg.HighLat.ExitFrac-0.01, lEdge, cfg); got {
		t.Fatalf("expected exit to be false")
	}
}
