package telnet

import (
	"testing"

	"dxcluster/pathreliability"
)

func TestPathPredictionStatsSnapshotSplit(t *testing.T) {
	s := &Server{}

	s.recordPathPrediction(pathreliability.Result{Source: pathreliability.SourceCombined, Weight: 2}, false, false)
	s.recordPathPrediction(pathreliability.Result{Source: pathreliability.SourceInsufficient, Weight: 0}, false, false)
	s.recordPathPrediction(pathreliability.Result{Source: pathreliability.SourceInsufficient, Weight: 0.25}, false, false)

	stats := s.PathPredictionStatsSnapshot()
	if stats.Total != 3 {
		t.Fatalf("expected total=3, got %d", stats.Total)
	}
	if stats.Combined != 1 {
		t.Fatalf("expected combined=1, got %d", stats.Combined)
	}
	if stats.Insufficient != 2 {
		t.Fatalf("expected insufficient=2, got %d", stats.Insufficient)
	}
	if stats.NoSample != 1 {
		t.Fatalf("expected no_sample=1, got %d", stats.NoSample)
	}
	if stats.LowWeight != 1 {
		t.Fatalf("expected low_weight=1, got %d", stats.LowWeight)
	}

	after := s.PathPredictionStatsSnapshot()
	if after.Total != 0 || after.Combined != 0 || after.Insufficient != 0 || after.NoSample != 0 || after.LowWeight != 0 {
		t.Fatalf("expected zeroed snapshot, got %+v", after)
	}
}
