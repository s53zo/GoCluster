package pathreliability

import (
	"testing"
	"time"
)

func TestStoreStatsByBandCounts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StaleAfterSeconds = 600
	cfg.CoarseFallbackEnabled = true
	store := NewStore(cfg, []string{"160m", "80m"})
	now := time.Now().UTC()

	store.Update(EncodeCell("FN31"), EncodeCell("FN32"), "", "", "160m", -5, 1.0, now)
	store.Update(InvalidCell, InvalidCell, "FN", "FN", "80m", -10, 1.0, now)

	stats := store.StatsByBand(now)
	if len(stats) != 2 {
		t.Fatalf("expected 2 bands, got %d", len(stats))
	}
	if stats[0].fine != 1 || stats[0].coarse != 0 {
		t.Fatalf("expected 160m fine=1 coarse=0, got fine=%d coarse=%d", stats[0].fine, stats[0].coarse)
	}
	if stats[1].fine != 0 || stats[1].coarse != 1 {
		t.Fatalf("expected 80m fine=0 coarse=1, got fine=%d coarse=%d", stats[1].fine, stats[1].coarse)
	}
}

func TestPredictorStatsByBand(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StaleAfterSeconds = 600
	cfg.CoarseFallbackEnabled = true
	predictor := NewPredictor(cfg, []string{"160m", "80m"})
	now := time.Now().UTC()

	predictor.Update(BucketBaseline, EncodeCell("FN31"), EncodeCell("FN32"), "", "", "160m", -5, 1.0, now, false)
	predictor.Update(BucketNarrowband, InvalidCell, InvalidCell, "FN", "FN", "80m", -7, 1.0, now, false)

	stats := predictor.StatsByBand(now)
	if len(stats) != 2 {
		t.Fatalf("expected 2 bands, got %d", len(stats))
	}
	if stats[0].Band != "160m" || stats[1].Band != "80m" {
		t.Fatalf("unexpected band order: %q, %q", stats[0].Band, stats[1].Band)
	}
	if stats[0].BaselineFine != 1 || stats[0].BaselineCoarse != 0 {
		t.Fatalf("expected 160m baseline fine=1 coarse=0, got fine=%d coarse=%d", stats[0].BaselineFine, stats[0].BaselineCoarse)
	}
	if stats[0].NarrowFine != 0 || stats[0].NarrowCoarse != 0 {
		t.Fatalf("expected 160m narrow fine=0 coarse=0, got fine=%d coarse=%d", stats[0].NarrowFine, stats[0].NarrowCoarse)
	}
	if stats[1].BaselineFine != 0 || stats[1].BaselineCoarse != 0 {
		t.Fatalf("expected 80m baseline fine=0 coarse=0, got fine=%d coarse=%d", stats[1].BaselineFine, stats[1].BaselineCoarse)
	}
	if stats[1].NarrowFine != 0 || stats[1].NarrowCoarse != 1 {
		t.Fatalf("expected 80m narrow fine=0 coarse=1, got fine=%d coarse=%d", stats[1].NarrowFine, stats[1].NarrowCoarse)
	}
}
