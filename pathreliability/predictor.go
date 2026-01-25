package pathreliability

import "time"

// Predictor coordinates store access and presentation mapping.
type Predictor struct {
	combined *Store
	cfg      Config
}

// NewPredictor builds a predictor with normalized configuration.
func NewPredictor(cfg Config, bands []string) *Predictor {
	cfg.normalize()
	return &Predictor{
		combined: NewStore(cfg, bands),
		cfg:      cfg,
	}
}

// Config returns the normalized configuration in use.
func (p *Predictor) Config() Config {
	if p == nil {
		return Config{}
	}
	return p.cfg
}

// Update ingests a spot contribution (FT8-equiv) with optional beacon weight cap.
func (p *Predictor) Update(bucket BucketClass, receiverCell, senderCell CellID, receiverGrid2, senderGrid2 string, band string, ft8dB float64, weight float64, now time.Time, isBeacon bool) {
	if p == nil || !p.cfg.Enabled {
		return
	}
	w := weight
	if isBeacon && p.cfg.BeaconWeightCap > 0 && w > p.cfg.BeaconWeightCap {
		w = p.cfg.BeaconWeightCap
	}
	power := p.cfg.powerFromDB(ft8dB)
	switch bucket {
	case BucketCombined:
		if p.combined != nil {
			p.combined.Update(receiverCell, senderCell, receiverGrid2, senderGrid2, band, power, w, now)
		}
	default:
		return
	}
}

// PredictionSource describes which bucket family produced the glyph.
type PredictionSource uint8

const (
	SourceInsufficient PredictionSource = iota
	SourceCombined
)

// Result carries merged glyph and diagnostics.
type Result struct {
	Glyph  string
	Value  float64 // power
	Weight float64
	Source PredictionSource
}

// Predict returns a single merged glyph for the path.
func (p *Predictor) Predict(userCell, dxCell CellID, userGrid2, dxGrid2 string, band string, mode string, noisePenalty float64, now time.Time) Result {
	insufficient := "?"
	if p != nil && p.cfg.GlyphSymbols.Insufficient != "" {
		insufficient = p.cfg.GlyphSymbols.Insufficient
	}
	if p == nil || !p.cfg.Enabled {
		return Result{Glyph: insufficient, Source: SourceInsufficient}
	}

	modeKey := normalizeMode(mode)
	makeResult := func(power, weight float64, source PredictionSource) Result {
		return Result{Glyph: GlyphForPower(power, modeKey, p.cfg), Value: power, Weight: weight, Source: source}
	}
	makeInsufficient := func(power, weight float64) Result {
		return Result{Glyph: insufficient, Value: power, Weight: weight, Source: SourceInsufficient}
	}

	mergedPower, mergedWeight, ok := p.mergeFromStore(p.combined, userCell, dxCell, userGrid2, dxGrid2, band, noisePenalty, now)
	if ok && mergedWeight >= p.cfg.MinEffectiveWeight {
		return makeResult(mergedPower, mergedWeight, SourceCombined)
	}
	if ok {
		return makeInsufficient(mergedPower, mergedWeight)
	}
	return makeInsufficient(0, 0)
}

func mergeSamples(receive Sample, transmit Sample, cfg Config, noisePenalty float64) (float64, float64, bool) {
	hasReceive := receive.Weight > 0
	hasTransmit := transmit.Weight > 0
	if !hasReceive && !hasTransmit {
		return 0, 0, false
	}
	receivePower := receive.Value
	if hasReceive && noisePenalty > 0 {
		receivePower = ApplyNoisePower(receivePower, noisePenalty, cfg)
	}
	if hasReceive && hasTransmit {
		mergedPower := cfg.MergeReceiveWeight*receivePower + cfg.MergeTransmitWeight*transmit.Value
		mergedWeight := cfg.MergeReceiveWeight*receive.Weight + cfg.MergeTransmitWeight*transmit.Weight
		return mergedPower, mergedWeight, true
	}
	if hasReceive {
		return receivePower, receive.Weight * cfg.ReverseHintDiscount, true
	}
	return transmit.Value, transmit.Weight * cfg.ReverseHintDiscount, true
}

func (p *Predictor) mergeFromStore(store *Store, userCell, dxCell CellID, userGrid2, dxGrid2, band string, noisePenalty float64, now time.Time) (float64, float64, bool) {
	if store == nil {
		return 0, 0, false
	}
	// Receive (DX->user): receiver=user, sender=dx.
	rFine, rCoarse := store.Lookup(userCell, dxCell, userGrid2, dxGrid2, band, now)
	receive := SelectSample(rFine, rCoarse, p.cfg.MinFineWeight)

	// Transmit (user->DX): receiver=dx, sender=user.
	tFine, tCoarse := store.Lookup(dxCell, userCell, dxGrid2, userGrid2, band, now)
	transmit := SelectSample(tFine, tCoarse, p.cfg.MinFineWeight)

	return mergeSamples(receive, transmit, p.cfg, noisePenalty)
}

// PurgeStale runs a stale purge and returns removed count.
func (p *Predictor) PurgeStale(now time.Time) int {
	if p == nil {
		return 0
	}
	removed := 0
	if p.combined != nil {
		removed += p.combined.PurgeStale(now)
	}
	return removed
}

// PredictorStats returns counts of active fine/coarse buckets (non-stale).
type PredictorStats struct {
	CombinedFine   int
	CombinedCoarse int
}

// BandBucketStats reports fine/coarse bucket counts per band.
type BandBucketStats struct {
	Band   string
	Fine   int
	Coarse int
}

// BandWeightHistogram reports per-band bucket weight distributions.
type BandWeightHistogram struct {
	Band  string
	Total int
	Bins  []int
}

// WeightHistogram carries histogram edges and per-band counts.
type WeightHistogram struct {
	Edges []float64
	Bands []BandWeightHistogram
}

// weightHistogramEdges defines decayed weight bins for log-only histograms.
var weightHistogramEdges = []float64{1, 2, 3, 5, 10}

// Stats returns counts of active fine/coarse buckets for the combined store.
func (p *Predictor) Stats(now time.Time) PredictorStats {
	if p == nil || !p.cfg.Enabled {
		return PredictorStats{}
	}
	stats := PredictorStats{}
	if p.combined != nil {
		stats.CombinedFine, stats.CombinedCoarse = p.combined.Stats(now)
	}
	return stats
}

// StatsByBand returns per-band bucket counts for the combined store.
func (p *Predictor) StatsByBand(now time.Time) []BandBucketStats {
	if p == nil || !p.cfg.Enabled {
		return nil
	}
	if p.combined == nil {
		return nil
	}
	bands := p.combined.bandIndex.Bands()
	if len(bands) == 0 {
		return nil
	}
	counts := p.combined.StatsByBand(now)
	stats := make([]BandBucketStats, len(bands))
	for i, band := range bands {
		entry := BandBucketStats{Band: band}
		if i < len(counts) {
			entry.Fine = counts[i].fine
			entry.Coarse = counts[i].coarse
		}
		stats[i] = entry
	}
	return stats
}

// WeightHistogramByBand returns per-band bucket weight histograms for the combined store.
func (p *Predictor) WeightHistogramByBand(now time.Time) WeightHistogram {
	if p == nil || !p.cfg.Enabled || p.combined == nil {
		return WeightHistogram{}
	}
	bands := p.combined.bandIndex.Bands()
	if len(bands) == 0 {
		return WeightHistogram{}
	}
	counts := p.combined.WeightHistogramByBand(now, weightHistogramEdges)
	if len(counts) == 0 {
		return WeightHistogram{}
	}
	entries := make([]BandWeightHistogram, len(bands))
	for i, band := range bands {
		entry := BandWeightHistogram{Band: band}
		if i < len(counts) {
			entry.Total = counts[i].total
			entry.Bins = append([]int(nil), counts[i].bins...)
		}
		entries[i] = entry
	}
	return WeightHistogram{
		Edges: append([]float64(nil), weightHistogramEdges...),
		Bands: entries,
	}
}
