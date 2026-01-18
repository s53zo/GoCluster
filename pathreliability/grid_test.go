package pathreliability

import "testing"

func TestGrid2FromLatLon(t *testing.T) {
	cases := []struct {
		lat  float64
		lon  float64
		want string
	}{
		{46.0, 14.0, "JN"},
		{-45.0, -70.0, "FE"},
		{0.0, 0.0, "JJ"},
		{91.0, 0.0, ""},
		{0.0, 181.0, ""},
	}
	for _, tc := range cases {
		got := Grid2FromLatLon(tc.lat, tc.lon)
		if got != tc.want {
			t.Fatalf("Grid2FromLatLon(%v,%v) = %q, want %q", tc.lat, tc.lon, got, tc.want)
		}
	}
}
