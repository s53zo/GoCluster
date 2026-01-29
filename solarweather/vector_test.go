package solarweather

import (
	"math"
	"testing"
)

func TestWrapToPiMapsPiToPositive(t *testing.T) {
	if got := wrapToPi(math.Pi); got != math.Pi {
		t.Fatalf("expected +pi, got %v", got)
	}
	if got := wrapToPi(-math.Pi); got != math.Pi {
		t.Fatalf("expected +pi for -pi, got %v", got)
	}
}
