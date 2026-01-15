package pathreliability

import "testing"

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
