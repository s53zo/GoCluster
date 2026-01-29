package solarweather

import "time"

const (
	wmmEpoch = 2025.0
	g10Base  = -29351.8
	g11Base  = -1410.8
	h11Base  = 4545.4
	g10Dot   = 12.0
	g11Dot   = 9.7
	h11Dot   = -21.5
)

// DipoleAxisECEF returns the unit geomagnetic dipole axis in ECEF using WMM2025 n=1 coefficients.
func DipoleAxisECEF(now time.Time) Vec3 {
	decYear := decimalYear(now.UTC())
	delta := decYear - wmmEpoch
	g10 := g10Base + g10Dot*delta
	g11 := g11Base + g11Dot*delta
	h11 := h11Base + h11Dot*delta

	// Use the north-pointing axis. Sign does not matter because |p·m| is used for gating.
	v := Vec3{X: -g11, Y: -h11, Z: -g10}
	return v.Normalize()
}

func decimalYear(t time.Time) float64 {
	y := t.Year()
	start := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(y+1, 1, 1, 0, 0, 0, 0, time.UTC)
	elapsed := t.Sub(start)
	duration := end.Sub(start)
	if duration <= 0 {
		return float64(y)
	}
	return float64(y) + float64(elapsed)/float64(duration)
}
