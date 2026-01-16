package pathreliability

import "time"

// Predictor coordinates store access and presentation mapping.
type Predictor struct {
	baseline   *Store
	narrowband *Store
	cfg        Config
}

// NewPredictor builds a predictor with precomputed neighbors.
func NewPredictor(cfg Config, bands []string) *Predictor {
	cfg.normalize()
	neighbors := BuildNeighborTable()
	return &Predictor{
		baseline:   NewStore(cfg, bands, neighbors),
		narrowband: NewStore(cfg, bands, neighbors),
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

// Result carries merged glyph and diagnostics.
type Result struct {
	Glyph  string
	Value  float64
	Weight float64
}

// Predict returns a single merged glyph for the path.
func (p *Predictor) Predict(userCell, dxCell CellID, userGrid2, dxGrid2 string, band string, mode string, noisePenalty float64, now time.Time) Result {
	insufficient := "?"
	if p != nil && p.cfg.GlyphSymbols.Insufficient != "" {
		insufficient = p.cfg.GlyphSymbols.Insufficient
	}
	if p == nil || !p.cfg.Enabled {
		return Result{Glyph: insufficient}
	}

	modeKey := normalizeMode(mode)
	switch {
	case IsNarrowbandMode(modeKey):
		mergedDB, mergedWeight, ok := p.mergeFromStore(p.narrowband, userCell, dxCell, userGrid2, dxGrid2, band, noisePenalty, now)
		if ok && mergedWeight >= p.cfg.MinEffectiveWeight {
			return Result{Glyph: GlyphForDB(mergedDB, modeKey, p.cfg), Value: mergedDB, Weight: mergedWeight}
		}
		mergedDB, mergedWeight, ok = p.mergeFromStore(p.baseline, userCell, dxCell, userGrid2, dxGrid2, band, noisePenalty, now)
		if ok && mergedWeight >= p.cfg.MinEffectiveWeight {
			return Result{Glyph: GlyphForDB(mergedDB, modeKey, p.cfg), Value: mergedDB, Weight: mergedWeight}
		}
		return Result{Glyph: insufficient, Value: mergedDB, Weight: mergedWeight}
	case IsVoiceMode(modeKey):
		mergedDB, mergedWeight, ok := p.mergeFromStore(p.baseline, userCell, dxCell, userGrid2, dxGrid2, band, noisePenalty, now)
		if ok && mergedWeight >= p.cfg.MinEffectiveWeight {
			return Result{Glyph: GlyphForDB(mergedDB, modeKey, p.cfg), Value: mergedDB, Weight: mergedWeight}
		}
		return Result{Glyph: insufficient, Value: mergedDB, Weight: mergedWeight}
	default:
		mergedDB, mergedWeight, ok := p.mergeFromStore(p.baseline, userCell, dxCell, userGrid2, dxGrid2, band, noisePenalty, now)
		if ok && mergedWeight >= p.cfg.MinEffectiveWeight {
			return Result{Glyph: GlyphForDB(mergedDB, modeKey, p.cfg), Value: mergedDB, Weight: mergedWeight}
		}
		return Result{Glyph: insufficient, Value: mergedDB, Weight: mergedWeight}
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
	rFine, rCoarse, rNbr, _ := store.Lookup(userCell, dxCell, userGrid2, dxGrid2, band, now)
	receive := SelectSample(rFine, rCoarse, rNbr, p.cfg.MinFineWeight)

	// Transmit (user->DX): receiver=dx, sender=user.
	tFine, tCoarse, tNbr, _ := store.Lookup(dxCell, userCell, dxGrid2, userGrid2, band, now)
	transmit := SelectSample(tFine, tCoarse, tNbr, p.cfg.MinFineWeight)

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
