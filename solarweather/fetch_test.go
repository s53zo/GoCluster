package solarweather

import (
	"math"
	"testing"
	"time"
)

func TestParseGOES(t *testing.T) {
	payload := `[
{"time_tag":"2026-01-28T00:00:00Z","flux":1e-6,"energy":"0.1-0.8nm"},
{"time_tag":"2026-01-28T00:01:00Z","flux":2e-6,"energy":"0.1-0.8nm"},
{"time_tag":"2026-01-28T00:01:00Z","flux":5e-9,"energy":"0.05-0.4nm"}
]`
	samples, ok := parseGOES([]byte(payload), "0.1-0.8nm")
	if !ok {
		t.Fatalf("expected ok")
	}
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}
	if math.Abs(samples[1].Flux-2e-6) > 1e-12 {
		t.Fatalf("expected flux 2e-6, got %v", samples[1].Flux)
	}
	want := time.Date(2026, 1, 28, 0, 1, 0, 0, time.UTC)
	if !samples[1].Time.Equal(want) {
		t.Fatalf("expected %v, got %v", want, samples[1].Time)
	}
}

func TestParseKp(t *testing.T) {
	payload := `[["time_tag","Kp"],["2026-01-22 00:00:00.000","4.33"],["2026-01-22 03:00:00.000","5.33"]]`
	kp, ts, ok := parseKp([]byte(payload))
	if !ok {
		t.Fatalf("expected ok")
	}
	if math.Abs(kp-5.33) > 1e-6 {
		t.Fatalf("expected kp 5.33, got %v", kp)
	}
	want := time.Date(2026, 1, 22, 3, 0, 0, 0, time.UTC)
	if !ts.Equal(want) {
		t.Fatalf("expected %v, got %v", want, ts)
	}
}
