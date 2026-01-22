package cty

import (
	"math"
	"testing"
)

func TestGrid4FromLatLon(t *testing.T) {
	tests := []struct {
		name   string
		lat    float64
		lon    float64
		want   string
		wantOK bool
	}{
		{name: "origin", lat: 0, lon: 0, want: "JJ00", wantOK: true},
		{name: "max_edge", lat: 89.9999, lon: 179.9999, want: "RR99", wantOK: true},
		{name: "north_pole_clamp", lat: 90, lon: 180, want: "RR99", wantOK: true},
		{name: "invalid_nan", lat: math.NaN(), lon: 0, want: "", wantOK: false},
		{name: "invalid_out_of_range", lat: 95, lon: 0, want: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Grid4FromLatLon(tt.lat, tt.lon)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v want %v (grid=%q)", ok, tt.wantOK, got)
			}
			if ok && got != tt.want {
				t.Fatalf("grid=%q want %q", got, tt.want)
			}
		})
	}
}
