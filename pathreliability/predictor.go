package pathreliability

import "time"

// Predictor coordinates store access and presentation mapping.
type Predictor struct {
	store     *Store
	cfg       Config
	neighbors map[string][]string
}

// NewPredictor builds a predictor with precomputed neighbors.
func NewPredictor(cfg Config, bands []string) *Predictor {
	cfg.normalize()
	neighbors := BuildNeighborTable()
	return &Predictor{
		store:     NewStore(cfg, bands, neighbors),
		cfg:       cfg,
		neighbors: neighbors,
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
func (p *Predictor) Update(receiverCell, senderCell CellID, receiverGrid2, senderGrid2 string, band string, ft8dB float64, weight float64, now time.Time, isBeacon bool) {
	if p == nil || p.store == nil || !p.cfg.Enabled {
		return
	}
	w := weight
	if isBeacon && p.cfg.BeaconWeightCap > 0 && w > p.cfg.BeaconWeightCap {
		w = p.cfg.BeaconWeightCap
	}
	p.store.Update(receiverCell, senderCell, receiverGrid2, senderGrid2, band, ft8dB, w, now)
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
	if p == nil || p.store == nil || !p.cfg.Enabled {
		return Result{Glyph: insufficient}
	}

	// Receive (DX->user): receiver=user, sender=dx.
	rFine, rCoarse, rNbr, _ := p.store.Lookup(userCell, dxCell, userGrid2, dxGrid2, band, now)
	receive := SelectSample(rFine, rCoarse, rNbr, p.cfg.MinFineWeight)

	// Transmit (user->DX): receiver=dx, sender=user.
	tFine, tCoarse, tNbr, _ := p.store.Lookup(dxCell, userCell, dxGrid2, userGrid2, band, now)
	transmit := SelectSample(tFine, tCoarse, tNbr, p.cfg.MinFineWeight)

	mergedDB, mergedWeight, ok := mergeSamples(receive, transmit, p.cfg, noisePenalty)
	if !ok || mergedWeight < p.cfg.MinEffectiveWeight {
		return Result{Glyph: insufficient, Value: mergedDB, Weight: mergedWeight}
	}
	return Result{
		Glyph:  GlyphForDB(mergedDB, mode, p.cfg),
		Value:  mergedDB,
		Weight: mergedWeight,
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

// PurgeStale runs a stale purge and returns removed count.
func (p *Predictor) PurgeStale(now time.Time) int {
	if p == nil || p.store == nil {
		return 0
	}
	return p.store.PurgeStale(now)
}
