package dedup

import (
	"testing"
	"time"

	"dxcluster/spot"
)

// Purpose: Verify secondary dedupe splits by source class.
// Key aspects: Ensures human and skimmer both pass, duplicates suppressed within class.
// Upstream: go test execution.
// Downstream: NewSecondaryDeduper and ShouldForward.
func TestSecondaryDeduperSplitsBySourceClass(t *testing.T) {
	d := NewSecondaryDeduper(time.Minute, false)
	now := time.Unix(1_700_000_000, 0).UTC()

	// Purpose: Build a spot with minimal metadata for dedupe tests.
	// Key aspects: Sets SourceType, time, and DE metadata fields.
	// Upstream: TestSecondaryDeduperSplitsBySourceClass.
	// Downstream: spot.NewSpot.
	makeSpot := func(source spot.SourceType, at time.Time) *spot.Spot {
		s := spot.NewSpot("K1ABC", "W1XYZ", 14074.0, "FT8")
		s.SourceType = source
		s.Time = at
		s.DEMetadata.ADIF = 291
		s.DEMetadata.CQZone = 5
		return s
	}

	skimmer := makeSpot(spot.SourceRBN, now)
	human := makeSpot(spot.SourceManual, now)

	if !d.ShouldForward(skimmer) {
		t.Fatal("expected skimmer spot to pass secondary dedupe")
	}
	if !d.ShouldForward(human) {
		t.Fatal("expected human spot to pass secondary dedupe even when skimmer already seen")
	}

	if d.ShouldForward(makeSpot(spot.SourceManual, now.Add(10*time.Second))) {
		t.Fatal("expected human duplicate to be suppressed within window")
	}
	if d.ShouldForward(makeSpot(spot.SourceRBN, now.Add(10*time.Second))) {
		t.Fatal("expected skimmer duplicate to be suppressed within window")
	}
}

// Purpose: Verify secondary dedupe still hashes when DE grid2 is missing.
// Key aspects: Missing grid2 should not bypass duplicates within the window.
// Upstream: go test execution.
// Downstream: SecondaryDeduper.ShouldForward.
func TestSecondaryDeduperWithMissingDEGrid2(t *testing.T) {
	d := NewSecondaryDeduper(5*time.Minute, false)
	now := time.Unix(1_700_000_100, 0).UTC()

	makeSpot := func(at time.Time) *spot.Spot {
		s := spot.NewSpot("K1ABC", "W1XYZ", 14074.0, "FT8")
		s.Time = at
		s.DEMetadata.ADIF = 291
		s.DEMetadata.CQZone = 5
		s.DEMetadata.Grid = ""
		s.EnsureNormalized()
		return s
	}

	if !d.ShouldForward(makeSpot(now)) {
		t.Fatal("expected first spot to pass secondary dedupe")
	}
	if d.ShouldForward(makeSpot(now.Add(30 * time.Second))) {
		t.Fatal("expected duplicate to be suppressed even without DE grid2")
	}
}
