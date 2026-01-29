package pathreliability

import (
	"math"
	"testing"
	"time"
)

func TestGlyphForPower(t *testing.T) {
	cfg := DefaultConfig()
	if got := GlyphForPower(cfg.powerFromDB(-12), "FT8", cfg); got != "+" {
		t.Fatalf("expected + for -12 dB, got %q", got)
	}
	if got := GlyphForPower(cfg.powerFromDB(-16), "FT8", cfg); got != "=" {
		t.Fatalf("expected = for -16 dB, got %q", got)
	}
	if got := GlyphForPower(cfg.powerFromDB(-20), "FT8", cfg); got != "-" {
		t.Fatalf("expected - for -20 dB, got %q", got)
	}
	if got := GlyphForPower(cfg.powerFromDB(-30), "FT8", cfg); got != "!" {
		t.Fatalf("expected ! for -30 dB, got %q", got)
	}
	if got := GlyphForPower(cfg.powerFromDB(-4), "CW", cfg); got != "-" {
		t.Fatalf("expected - for -4 dB in CW thresholds, got %q", got)
	}
}

func TestClassForPower(t *testing.T) {
	cfg := DefaultConfig()
	if got := ClassForPower(cfg.powerFromDB(-12), "FT8", cfg); got != "HIGH" {
		t.Fatalf("expected HIGH for -12 dB, got %q", got)
	}
	if got := ClassForPower(cfg.powerFromDB(-16), "FT8", cfg); got != "MEDIUM" {
		t.Fatalf("expected MEDIUM for -16 dB, got %q", got)
	}
	if got := ClassForPower(cfg.powerFromDB(-20), "FT8", cfg); got != "LOW" {
		t.Fatalf("expected LOW for -20 dB, got %q", got)
	}
	if got := ClassForPower(cfg.powerFromDB(-30), "FT8", cfg); got != "UNLIKELY" {
		t.Fatalf("expected UNLIKELY for -30 dB, got %q", got)
	}
}

func TestGlyphForPowerUsesCustomSymbols(t *testing.T) {
	cfg := DefaultConfig()
	cfg.GlyphSymbols = GlyphSymbols{
		High:         "H",
		Medium:       "M",
		Low:          "L",
		Unlikely:     "U",
		Insufficient: "I",
	}
	if got := GlyphForPower(cfg.powerFromDB(-12), "FT8", cfg); got != "H" {
		t.Fatalf("expected H for -12 dB, got %q", got)
	}
	if got := GlyphForPower(cfg.powerFromDB(-16), "FT8", cfg); got != "M" {
		t.Fatalf("expected M for -16 dB, got %q", got)
	}
	if got := GlyphForPower(cfg.powerFromDB(-20), "FT8", cfg); got != "L" {
		t.Fatalf("expected L for -20 dB, got %q", got)
	}
	if got := GlyphForPower(cfg.powerFromDB(-30), "FT8", cfg); got != "U" {
		t.Fatalf("expected U for -30 dB, got %q", got)
	}
}

func TestMergeSamplesWeighted(t *testing.T) {
	cfg := DefaultConfig()
	receive := Sample{Value: cfg.powerFromDB(-10), Weight: 10}
	transmit := Sample{Value: cfg.powerFromDB(-5), Weight: 4}
	mergedPower, mergedWeight, ok := mergeSamples(receive, transmit, cfg, 0)
	if !ok {
		t.Fatalf("expected merge ok")
	}
	if mergedWeight <= 0 {
		t.Fatalf("expected positive merged weight")
	}
	mergedDB := powerToDB(mergedPower)
	if mergedDB >= -7 || mergedDB <= -11 {
		t.Fatalf("unexpected merged dB: %v", mergedDB)
	}
}

func TestSelectSampleMinFineWeight(t *testing.T) {
	fine := Sample{Value: -20, Weight: 2, AgeSec: 12}
	coarse := Sample{Value: -5, Weight: 10, AgeSec: 30}
	got := SelectSample(fine, coarse, 5, 20)
	if got.Value != coarse.Value || got.Weight != coarse.Weight {
		t.Fatalf("expected coarse when fine below min, got value=%v weight=%v", got.Value, got.Weight)
	}

	fine = Sample{Value: -10, Weight: 6, AgeSec: 10}
	coarse = Sample{Value: -4, Weight: 10, AgeSec: 20}
	got = SelectSample(fine, coarse, 5, 20)
	wantValue := (fine.Value*fine.Weight + coarse.Value*coarse.Weight) / (fine.Weight + coarse.Weight)
	if math.Abs(got.Value-wantValue) > 0.0001 {
		t.Fatalf("expected blended value %v, got %v", wantValue, got.Value)
	}
	if got.Weight != fine.Weight+coarse.Weight {
		t.Fatalf("expected blended weight %v, got %v", fine.Weight+coarse.Weight, got.Weight)
	}

	fine = Sample{Value: -18, Weight: 2, AgeSec: 5}
	got = SelectSample(fine, Sample{}, 5, 20)
	if got.Value != fine.Value || got.Weight != fine.Weight {
		t.Fatalf("expected fine sample when coarse missing, got value=%v weight=%v", got.Value, got.Weight)
	}
}

func TestSelectSampleFineOnlyThreshold(t *testing.T) {
	fine := Sample{Value: -12, Weight: 25, AgeSec: 10}
	coarse := Sample{Value: -6, Weight: 100, AgeSec: 30}
	got := SelectSample(fine, coarse, 5, 20)
	if got.Value != fine.Value || got.Weight != fine.Weight {
		t.Fatalf("expected fine-only when fine meets threshold, got value=%v weight=%v", got.Value, got.Weight)
	}
}

func TestPredictUsesInsufficientGlyph(t *testing.T) {
	requireH3Mappings(t)
	cfg := DefaultConfig()
	cfg.GlyphSymbols.Insufficient = "I"
	predictor := NewPredictor(cfg, []string{"20m"})
	userCell := EncodeCell("FN31")
	dxCell := EncodeCell("FN32")
	userCoarse := EncodeCoarseCell("FN31")
	dxCoarse := EncodeCoarseCell("FN32")
	res := predictor.Predict(userCell, dxCell, userCoarse, dxCoarse, "20m", "FT8", 0, time.Now().UTC())
	if res.Glyph != "I" {
		t.Fatalf("expected insufficient glyph I, got %q", res.Glyph)
	}
	if res.Source != SourceInsufficient {
		t.Fatalf("expected insufficient source, got %v", res.Source)
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
	if got, ok := FT8Equivalent("WSPR", 0, cfg); !ok || got != 26 {
		t.Fatalf("expected WSPR 0 dB to map to 26 dB, got %v (ok=%v)", got, ok)
	}
	if got, ok := FT8Equivalent("FT4", 0, cfg); !ok || got != -3 {
		t.Fatalf("expected FT4 0 dB to map to -3 dB, got %v (ok=%v)", got, ok)
	}
	if got, ok := FT8Equivalent("FT8", 0, cfg); !ok || got != 0 {
		t.Fatalf("expected FT8 0 dB to map to 0 dB, got %v (ok=%v)", got, ok)
	}
}

func TestBucketForIngest(t *testing.T) {
	if got := BucketForIngest("FT8"); got != BucketCombined {
		t.Fatalf("expected FT8 to map to combined bucket")
	}
	if got := BucketForIngest("CW"); got != BucketCombined {
		t.Fatalf("expected CW to map to combined bucket")
	}
	if got := BucketForIngest("WSPR"); got != BucketCombined {
		t.Fatalf("expected WSPR to map to combined bucket")
	}
	if got := BucketForIngest("USB"); got != BucketNone {
		t.Fatalf("expected USB to skip ingest")
	}
}

func TestPowerFromDBClampsToRange(t *testing.T) {
	cfg := DefaultConfig()
	high := cfg.powerFromDB(cfg.ClampMax + 10)
	low := cfg.powerFromDB(cfg.ClampMin - 10)
	if math.Abs(powerToDB(high)-cfg.ClampMax) > 0.2 {
		t.Fatalf("expected high clamp near %v dB, got %v dB", cfg.ClampMax, powerToDB(high))
	}
	if math.Abs(powerToDB(low)-cfg.ClampMin) > 0.2 {
		t.Fatalf("expected low clamp near %v dB, got %v dB", cfg.ClampMin, powerToDB(low))
	}
}

func TestPredictUsesCombinedBuckets(t *testing.T) {
	requireH3Mappings(t)
	cfg := DefaultConfig()
	cfg.MinEffectiveWeight = 0.1
	predictor := NewPredictor(cfg, []string{"20m"})
	userCell := EncodeCell("FN31")
	dxCell := EncodeCell("FN32")
	userCoarse := EncodeCoarseCell("FN31")
	dxCoarse := EncodeCoarseCell("FN32")
	now := time.Now().UTC()

	predictor.Update(BucketCombined, userCell, dxCell, userCoarse, dxCoarse, "20m", -5, 1.0, now, false)

	resCW := predictor.Predict(userCell, dxCell, userCoarse, dxCoarse, "20m", "CW", 0, now)
	if resCW.Glyph == cfg.GlyphSymbols.Insufficient {
		t.Fatalf("expected combined glyph for CW, got insufficient")
	}
	if resCW.Glyph != GlyphForPower(cfg.powerFromDB(-5), "CW", cfg) {
		t.Fatalf("unexpected CW glyph: %q", resCW.Glyph)
	}
	if resCW.Source != SourceCombined {
		t.Fatalf("expected combined source for CW, got %v", resCW.Source)
	}

	resFT8 := predictor.Predict(userCell, dxCell, userCoarse, dxCoarse, "20m", "FT8", 0, now)
	if resFT8.Glyph == cfg.GlyphSymbols.Insufficient {
		t.Fatalf("expected combined glyph for FT8, got insufficient")
	}
	if resFT8.Glyph != GlyphForPower(cfg.powerFromDB(-5), "FT8", cfg) {
		t.Fatalf("unexpected FT8 glyph: %q", resFT8.Glyph)
	}
	if resFT8.Source != SourceCombined {
		t.Fatalf("expected combined source for FT8, got %v", resFT8.Source)
	}
}

func TestPredictUsesCoarseWhenFineMissing(t *testing.T) {
	requireH3Mappings(t)
	now := time.Now().UTC()
	userCell := EncodeCell("FN31")
	dxCell := EncodeCell("FN32")
	userCoarse := EncodeCoarseCell("FN31")
	dxCoarse := EncodeCoarseCell("FN32")

	cfg := DefaultConfig()
	cfg.MinEffectiveWeight = 0.1
	predictor := NewPredictor(cfg, []string{"20m"})
	predictor.Update(BucketCombined, InvalidCell, InvalidCell, userCoarse, dxCoarse, "20m", -5, 1.0, now, false)

	res := predictor.Predict(userCell, dxCell, userCoarse, dxCoarse, "20m", "FT8", 0, now)
	if res.Glyph == cfg.GlyphSymbols.Insufficient {
		t.Fatalf("expected coarse fallback glyph when enabled, got insufficient")
	}
}
