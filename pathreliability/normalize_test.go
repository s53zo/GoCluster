package pathreliability

import (
	"math"
	"testing"
	"time"
)

func TestGlyphForDB(t *testing.T) {
	cfg := DefaultConfig()
	if got := GlyphForDB(-12, "FT8", cfg); got != "+" {
		t.Fatalf("expected + for -12 dB, got %q", got)
	}
	if got := GlyphForDB(-16, "FT8", cfg); got != "=" {
		t.Fatalf("expected = for -16 dB, got %q", got)
	}
	if got := GlyphForDB(-20, "FT8", cfg); got != "-" {
		t.Fatalf("expected - for -20 dB, got %q", got)
	}
	if got := GlyphForDB(-30, "FT8", cfg); got != "!" {
		t.Fatalf("expected ! for -30 dB, got %q", got)
	}
	if got := GlyphForDB(-4, "CW", cfg); got != "-" {
		t.Fatalf("expected - for -4 dB in CW thresholds, got %q", got)
	}
}

func TestGlyphForDBUsesCustomSymbols(t *testing.T) {
	cfg := DefaultConfig()
	cfg.GlyphSymbols = GlyphSymbols{
		High:         "H",
		Medium:       "M",
		Low:          "L",
		Unlikely:     "U",
		Insufficient: "I",
	}
	if got := GlyphForDB(-12, "FT8", cfg); got != "H" {
		t.Fatalf("expected H for -12 dB, got %q", got)
	}
	if got := GlyphForDB(-16, "FT8", cfg); got != "M" {
		t.Fatalf("expected M for -16 dB, got %q", got)
	}
	if got := GlyphForDB(-20, "FT8", cfg); got != "L" {
		t.Fatalf("expected L for -20 dB, got %q", got)
	}
	if got := GlyphForDB(-30, "FT8", cfg); got != "U" {
		t.Fatalf("expected U for -30 dB, got %q", got)
	}
}

func TestMergeSamplesWeighted(t *testing.T) {
	cfg := DefaultConfig()
	receive := Sample{Value: -10, Weight: 10}
	transmit := Sample{Value: -5, Weight: 4}
	mergedDB, mergedWeight, ok := mergeSamples(receive, transmit, cfg, 0)
	if !ok {
		t.Fatalf("expected merge ok")
	}
	if mergedWeight <= 0 {
		t.Fatalf("expected positive merged weight")
	}
	if mergedDB >= -7 || mergedDB <= -11 {
		t.Fatalf("unexpected merged dB: %v", mergedDB)
	}
}

func TestSelectSampleMinFineWeight(t *testing.T) {
	fine := Sample{Value: -20, Weight: 2, AgeSec: 12}
	coarse := Sample{Value: -5, Weight: 10, AgeSec: 30}
	got := SelectSample(fine, coarse, nil, 5)
	if got.Value != coarse.Value || got.Weight != coarse.Weight {
		t.Fatalf("expected coarse when fine below min, got value=%v weight=%v", got.Value, got.Weight)
	}

	fine = Sample{Value: -10, Weight: 6, AgeSec: 10}
	coarse = Sample{Value: -4, Weight: 10, AgeSec: 20}
	got = SelectSample(fine, coarse, nil, 5)
	wantValue := (fine.Value*fine.Weight + coarse.Value*coarse.Weight) / (fine.Weight + coarse.Weight)
	if math.Abs(got.Value-wantValue) > 0.0001 {
		t.Fatalf("expected blended value %v, got %v", wantValue, got.Value)
	}
	if got.Weight != fine.Weight+coarse.Weight {
		t.Fatalf("expected blended weight %v, got %v", fine.Weight+coarse.Weight, got.Weight)
	}

	fine = Sample{Value: -18, Weight: 2, AgeSec: 5}
	neighbors := []Sample{{Value: -8, Weight: 4, AgeSec: 15}}
	got = SelectSample(fine, Sample{}, neighbors, 5)
	if got.Value != neighbors[0].Value || got.Weight != neighbors[0].Weight {
		t.Fatalf("expected neighbor fallback when coarse missing and fine below min, got value=%v weight=%v", got.Value, got.Weight)
	}
}

func TestPredictUsesInsufficientGlyph(t *testing.T) {
	cfg := DefaultConfig()
	cfg.GlyphSymbols.Insufficient = "I"
	predictor := NewPredictor(cfg, []string{"20m"})
	userCell := EncodeCell("FN31")
	dxCell := EncodeCell("FN32")
	res := predictor.Predict(userCell, dxCell, "FN", "FN", "20m", "FT8", 0, time.Now().UTC())
	if res.Glyph != "I" {
		t.Fatalf("expected insufficient glyph I, got %q", res.Glyph)
	}
}

func TestFT8EquivalentBandwidthOffsets(t *testing.T) {
	cfg := DefaultConfig()
	if got, ok := FT8Equivalent("CW", 0, cfg); !ok || got != -7 {
		t.Fatalf("expected CW 0 dB to map to -7 dB, got %v (ok=%v)", got, ok)
	}
	if got, ok := FT8Equivalent("RTTY", 0, cfg); !ok || got != -7 {
		t.Fatalf("expected RTTY 0 dB to map to -7 dB, got %v (ok=%v)", got, ok)
	}
	if got, ok := FT8Equivalent("PSK", 0, cfg); !ok || got != -7 {
		t.Fatalf("expected PSK 0 dB to map to -7 dB, got %v (ok=%v)", got, ok)
	}
	if got, ok := FT8Equivalent("FT4", 0, cfg); !ok || got != -3 {
		t.Fatalf("expected FT4 0 dB to map to -3 dB, got %v (ok=%v)", got, ok)
	}
	if got, ok := FT8Equivalent("FT8", 0, cfg); !ok || got != 0 {
		t.Fatalf("expected FT8 0 dB to map to 0 dB, got %v (ok=%v)", got, ok)
	}
}

func TestBucketForIngest(t *testing.T) {
	if got := BucketForIngest("FT8"); got != BucketBaseline {
		t.Fatalf("expected FT8 to map to baseline bucket")
	}
	if got := BucketForIngest("CW"); got != BucketNarrowband {
		t.Fatalf("expected CW to map to narrowband bucket")
	}
	if got := BucketForIngest("USB"); got != BucketNone {
		t.Fatalf("expected USB to skip ingest")
	}
}

func TestPredictNarrowbandFallsBackToBaseline(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinEffectiveWeight = 0.1
	predictor := NewPredictor(cfg, []string{"20m"})
	userCell := EncodeCell("FN31")
	dxCell := EncodeCell("FN32")
	now := time.Now().UTC()

	predictor.Update(BucketBaseline, userCell, dxCell, "FN", "FN", "20m", -5, 1.0, now, false)

	res := predictor.Predict(userCell, dxCell, "FN", "FN", "20m", "CW", 0, now)
	if res.Glyph == cfg.GlyphSymbols.Insufficient {
		t.Fatalf("expected fallback glyph, got insufficient")
	}
	if res.Glyph != GlyphForDB(-5, "CW", cfg) {
		t.Fatalf("unexpected glyph for fallback: %q", res.Glyph)
	}
}

func TestPredictBaselineIgnoresNarrowband(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinEffectiveWeight = 0.1
	predictor := NewPredictor(cfg, []string{"20m"})
	userCell := EncodeCell("FN31")
	dxCell := EncodeCell("FN32")
	now := time.Now().UTC()

	predictor.Update(BucketNarrowband, userCell, dxCell, "FN", "FN", "20m", -5, 1.0, now, false)

	res := predictor.Predict(userCell, dxCell, "FN", "FN", "20m", "FT8", 0, now)
	if res.Glyph != cfg.GlyphSymbols.Insufficient {
		t.Fatalf("expected insufficient when baseline is empty, got %q", res.Glyph)
	}
}
