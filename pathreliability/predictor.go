package pathreliability

import "time"

// Predictor coordinates store access and presentation mapping.
type Predictor struct {
	baseline   *Store
	narrowband *Store
	cfg        Config
}

// NewPredictor builds a predictor with normalized configuration.
func NewPredictor(cfg Config, bands []string) *Predictor {
	cfg.normalize()
	return &Predictor{
		baseline:   NewStore(cfg, bands),
		narrowband: NewStore(cfg, bands),
		cfg:        cfg,
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
	switch bucket {
	case BucketBaseline:
		if p.baseline != nil {
			p.baseline.Update(receiverCell, senderCell, receiverGrid2, senderGrid2, band, ft8dB, w, now)
		}
	case BucketNarrowband:
		if p.narrowband != nil {
			p.narrowband.Update(receiverCell, senderCell, receiverGrid2, senderGrid2, band, ft8dB, w, now)
		}
	default:
		return
	}
}

// PredictionSource describes which bucket family produced the glyph.
type PredictionSource uint8

const (
	SourceInsufficient PredictionSource = iota
	SourceBaseline
	SourceNarrowband
)

// Result carries merged glyph and diagnostics.
type Result struct {
	Glyph  string
	Value  float64
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
	makeResult := func(db, weight float64, source PredictionSource) Result {
		return Result{Glyph: GlyphForDB(db, modeKey, p.cfg), Value: db, Weight: weight, Source: source}
	}
	makeInsufficient := func(db, weight float64) Result {
		return Result{Glyph: insufficient, Value: db, Weight: weight, Source: SourceInsufficient}
	}

	switch {
	case IsNarrowbandMode(modeKey):
		nbDB, nbWeight, nbOK := p.mergeFromStore(p.narrowband, userCell, dxCell, userGrid2, dxGrid2, band, noisePenalty, now)
		baseDB, baseWeight, baseOK := p.mergeFromStore(p.baseline, userCell, dxCell, userGrid2, dxGrid2, band, noisePenalty, now)

		nbSufficient := nbOK && nbWeight >= p.cfg.MinEffectiveWeightNarrowband
		baseSufficient := baseOK && baseWeight >= p.cfg.MinEffectiveWeight

		if nbSufficient && baseSufficient {
			ratio := p.cfg.NarrowbandOverrideMinWeightRatio
			if ratio <= 0 || nbWeight >= baseWeight*ratio {
				return makeResult(nbDB, nbWeight, SourceNarrowband)
			}
			return makeResult(baseDB, baseWeight, SourceBaseline)
		}
		if nbSufficient {
			return makeResult(nbDB, nbWeight, SourceNarrowband)
		}
		if baseSufficient {
			return makeResult(baseDB, baseWeight, SourceBaseline)
		}
		if baseOK && (!nbOK || baseWeight >= nbWeight) {
			return makeInsufficient(baseDB, baseWeight)
		}
		if nbOK {
			return makeInsufficient(nbDB, nbWeight)
		}
		return makeInsufficient(0, 0)
	case IsVoiceMode(modeKey):
		mergedDB, mergedWeight, ok := p.mergeFromStore(p.baseline, userCell, dxCell, userGrid2, dxGrid2, band, noisePenalty, now)
		if ok && mergedWeight >= p.cfg.MinEffectiveWeight {
			return makeResult(mergedDB, mergedWeight, SourceBaseline)
		}
		if ok {
			return makeInsufficient(mergedDB, mergedWeight)
		}
		return makeInsufficient(0, 0)
	default:
		mergedDB, mergedWeight, ok := p.mergeFromStore(p.baseline, userCell, dxCell, userGrid2, dxGrid2, band, noisePenalty, now)
		if ok && mergedWeight >= p.cfg.MinEffectiveWeight {
			return makeResult(mergedDB, mergedWeight, SourceBaseline)
		}
		if ok {
			return makeInsufficient(mergedDB, mergedWeight)
		}
		return makeInsufficient(0, 0)
	}
}

func mergeSamples(receive Sample, transmit Sample, cfg Config, noisePenalty float64) (float64, float64, bool) {
	hasReceive := receive.Weight > 0
	hasTransmit := transmit.Weight > 0
	if !hasReceive && !hasTransmit {
		return 0, 0, false
	}
	receiveDB := receive.Value
	if hasReceive && noisePenalty > 0 {
		receiveDB = ApplyNoise(receiveDB, noisePenalty, cfg.ClampMin, cfg.ClampMax)
	}
	if hasReceive && hasTransmit {
		mergedDB := cfg.MergeReceiveWeight*receiveDB + cfg.MergeTransmitWeight*transmit.Value
		mergedWeight := cfg.MergeReceiveWeight*receive.Weight + cfg.MergeTransmitWeight*transmit.Weight
		return mergedDB, mergedWeight, true
	}
	if hasReceive {
		return receiveDB, receive.Weight * cfg.ReverseHintDiscount, true
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
	if p.baseline != nil {
		removed += p.baseline.PurgeStale(now)
	}
	if p.narrowband != nil {
		removed += p.narrowband.PurgeStale(now)
	}
	return removed
}

// PredictorStats returns counts of active fine/coarse buckets (non-stale).
type PredictorStats struct {
	BaselineFine   int
	BaselineCoarse int
	NarrowFine     int
	NarrowCoarse   int
}

// BandBucketStats reports fine/coarse bucket counts per band.
type BandBucketStats struct {
	Band           string
	BaselineFine   int
	BaselineCoarse int
	NarrowFine     int
	NarrowCoarse   int
}

// Stats returns counts of active fine/coarse buckets for each bucket class.
func (p *Predictor) Stats(now time.Time) PredictorStats {
	if p == nil || !p.cfg.Enabled {
		return PredictorStats{}
	}
	stats := PredictorStats{}
	if p.baseline != nil {
		stats.BaselineFine, stats.BaselineCoarse = p.baseline.Stats(now)
	}
	if p.narrowband != nil {
		stats.NarrowFine, stats.NarrowCoarse = p.narrowband.Stats(now)
	}
	return stats
}

// StatsByBand returns per-band bucket counts for baseline and narrowband stores.
func (p *Predictor) StatsByBand(now time.Time) []BandBucketStats {
	if p == nil || !p.cfg.Enabled {
		return nil
	}
	var bands []string
	if p.baseline != nil {
		bands = p.baseline.bandIndex.Bands()
	} else if p.narrowband != nil {
		bands = p.narrowband.bandIndex.Bands()
	}
	if len(bands) == 0 {
		return nil
	}
	baselineCounts := []bandCounts{}
	if p.baseline != nil {
		baselineCounts = p.baseline.StatsByBand(now)
	}
	narrowCounts := []bandCounts{}
	if p.narrowband != nil {
		narrowCounts = p.narrowband.StatsByBand(now)
	}
	stats := make([]BandBucketStats, len(bands))
	for i, band := range bands {
		entry := BandBucketStats{Band: band}
		if i < len(baselineCounts) {
			entry.BaselineFine = baselineCounts[i].fine
			entry.BaselineCoarse = baselineCounts[i].coarse
		}
		if i < len(narrowCounts) {
			entry.NarrowFine = narrowCounts[i].fine
			entry.NarrowCoarse = narrowCounts[i].coarse
		}
		stats[i] = entry
	}
	return stats
}
