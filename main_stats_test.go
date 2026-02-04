package main

import (
	"testing"

	"dxcluster/spot"
)

func TestSourceStatsLabel(t *testing.T) {
	cases := []struct {
		name string
		spot *spot.Spot
		want string
	}{
		{"manual", &spot.Spot{SourceType: spot.SourceManual}, "HUMAN"},
		{"rbn-digital", &spot.Spot{SourceType: spot.SourceRBN, SourceNode: "RBN-DIGITAL"}, "RBN-DIGITAL"},
		{"rbn", &spot.Spot{SourceType: spot.SourceRBN, SourceNode: "RBN"}, "RBN"},
		{"ft8", &spot.Spot{SourceType: spot.SourceFT8}, "RBN-FT"},
		{"psk", &spot.Spot{SourceType: spot.SourcePSKReporter}, "PSKREPORTER"},
		{"peer", &spot.Spot{SourceType: spot.SourcePeer}, "PEER"},
		{"upstream", &spot.Spot{SourceType: spot.SourceUpstream}, "UPSTREAM"},
		{"node-fallback", &spot.Spot{SourceNode: "PSKREPORTER"}, "PSKREPORTER"},
		{"other", &spot.Spot{}, "OTHER"},
	}
	for _, tc := range cases {
		if got := sourceStatsLabel(tc.spot); got != tc.want {
			t.Fatalf("%s: expected %s, got %s", tc.name, tc.want, got)
		}
	}
}
