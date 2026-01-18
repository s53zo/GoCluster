package aggregate

import (
	"strings"
	"testing"
	"time"

	"dxcluster/config"
	"dxcluster/spot"
)

func TestSkimmerAggregationEmitsConsolidatedSpot(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := config.SkimmerAggregateConfig{
		Enabled:          true,
		DwellSeconds:     10,
		LimboSeconds:     300,
		RespotSeconds:    180,
		CacheSeconds:     3600,
		MinQuality:       2,
		MaxQuality:       9,
		SearchKHz:        5,
		MaxDeviants:      5,
		MaxEntries:       100,
		MaxRecordsPerKey: 8,
		CollapseSSID:     true,
	}
	agg := newSkimmerAggregator(cfg, func() time.Time { return now })
	var outputs []*spot.Spot
	agg.emitFn = func(s *spot.Spot) {
		outputs = append(outputs, s)
	}

	spot1 := newSkimmerSpot("DX1AAA", "SKIM1-1", 14074.0, "FT8", now)
	spot2 := newSkimmerSpot("DX1AAA", "SKIM2", 14074.0, "FT8", now.Add(2*time.Second))

	agg.ingest(spot1, now)
	agg.ingest(spot2, now.Add(2*time.Second))
	agg.processQueue(now.Add(11 * time.Second))

	if len(outputs) != 1 {
		t.Fatalf("expected 1 aggregated spot, got %d", len(outputs))
	}
	comment := outputs[0].Comment
	if !strings.Contains(comment, "Q:2*") {
		t.Fatalf("expected comment to include Q tag, got %q", comment)
	}
	if strings.Contains(comment, "+") {
		t.Fatalf("expected no respot marker, got %q", comment)
	}
}

func TestSkimmerAggregationRespotSuppression(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := config.SkimmerAggregateConfig{
		Enabled:          true,
		DwellSeconds:     1,
		LimboSeconds:     60,
		RespotSeconds:    30,
		CacheSeconds:     3600,
		MinQuality:       1,
		MaxQuality:       9,
		SearchKHz:        1,
		MaxDeviants:      3,
		MaxEntries:       100,
		MaxRecordsPerKey: 8,
		CollapseSSID:     true,
	}
	agg := newSkimmerAggregator(cfg, func() time.Time { return now })
	var outputs []*spot.Spot
	agg.emitFn = func(s *spot.Spot) {
		outputs = append(outputs, s)
	}

	spot1 := newSkimmerSpot("DX2BBB", "SKIM3", 7010.0, "CW", now)
	agg.ingest(spot1, now)
	agg.processQueue(now.Add(2 * time.Second))
	if len(outputs) != 1 {
		t.Fatalf("expected initial aggregation, got %d", len(outputs))
	}

	spot2 := newSkimmerSpot("DX2BBB", "SKIM3-2", 7010.0, "CW", now.Add(10*time.Second))
	agg.ingest(spot2, now.Add(10*time.Second))
	agg.processQueue(now.Add(12 * time.Second))
	if len(outputs) != 1 {
		t.Fatalf("expected respot suppression, got %d outputs", len(outputs))
	}

	spot3 := newSkimmerSpot("DX2BBB", "SKIM3-3", 7010.0, "CW", now.Add(40*time.Second))
	agg.ingest(spot3, now.Add(40*time.Second))
	agg.processQueue(now.Add(42 * time.Second))
	if len(outputs) != 2 {
		t.Fatalf("expected respot to emit after window, got %d", len(outputs))
	}
	if !strings.Contains(outputs[1].Comment, "+") {
		t.Fatalf("expected respot marker, got %q", outputs[1].Comment)
	}
}

func TestSkimmerAggregationSSIDCollapse(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := config.SkimmerAggregateConfig{
		Enabled:          true,
		DwellSeconds:     1,
		LimboSeconds:     2,
		RespotSeconds:    30,
		CacheSeconds:     3600,
		MinQuality:       2,
		MaxQuality:       9,
		SearchKHz:        1,
		MaxDeviants:      3,
		MaxEntries:       100,
		MaxRecordsPerKey: 8,
		CollapseSSID:     true,
	}
	agg := newSkimmerAggregator(cfg, func() time.Time { return now })
	var outputs []*spot.Spot
	agg.emitFn = func(s *spot.Spot) {
		outputs = append(outputs, s)
	}

	spot1 := newSkimmerSpot("DX3CCC", "SKIM4-1", 14030.0, "CW", now)
	spot2 := newSkimmerSpot("DX3CCC", "SKIM4-2", 14030.0, "CW", now.Add(1*time.Second))

	agg.ingest(spot1, now)
	agg.ingest(spot2, now.Add(1*time.Second))
	agg.processQueue(now.Add(3 * time.Second))

	if len(outputs) != 0 {
		t.Fatalf("expected no output with collapsed SSIDs, got %d", len(outputs))
	}
	_, _, _, _, limboDrops, _, _, _ := agg.Stats()
	if limboDrops != 1 {
		t.Fatalf("expected limbo drop, got %d", limboDrops)
	}
}

func newSkimmerSpot(dx, de string, freq float64, mode string, when time.Time) *spot.Spot {
	s := spot.NewSpot(dx, de, freq, mode)
	s.Time = when
	s.Report = 10
	s.HasReport = true
	s.SourceType = spot.SourceRBN
	s.IsHuman = false
	return s
}
