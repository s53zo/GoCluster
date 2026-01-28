package pathreliability

import (
	"math"
	"sync"
	"time"
)

const (
	defaultShards = 64
	ln2           = 0.6931471805599453
)

// bucket holds decaying power stats for a directional path.
type bucket struct {
	sumPower float64
	weight   float64
	// lastUpdate stores Unix seconds.
	lastUpdate int64
}

type shard struct {
	mu      sync.RWMutex
	buckets map[uint64]*bucket
}

// Store aggregates decaying FT8-equiv path stats.
type Store struct {
	shards    []shard
	cfg       Config
	bandIndex BandIndex
}

// NewStore constructs a path store with normalized config.
func NewStore(cfg Config, bands []string) *Store {
	cfg.normalize()
	if len(bands) == 0 {
		bands = []string{"160m", "80m", "60m", "40m", "30m", "20m", "17m", "15m", "12m", "10m", "6m", "4m", "2m", "1m"}
	}
	idx := NewBandIndex(bands)
	s := &Store{
		shards:    make([]shard, defaultShards),
		cfg:       cfg,
		bandIndex: idx,
	}
	for i := range s.shards {
		s.shards[i].buckets = make(map[uint64]*bucket)
	}
	return s
}

// Update applies a new FT8-equiv reading to the directional path.
// weight should normally be 1.0; beacons may be clamped by caller.
func (s *Store) Update(receiverCell, senderCell CellID, receiverCoarse, senderCoarse CellID, band string, power float64, weight float64, now time.Time) {
	if s == nil || !s.cfg.Enabled {
		return
	}
	idx, ok := s.bandIndex.Lookup(band)
	if !ok {
		return
	}
	halfLife := s.bandIndex.HalfLifeSeconds(band, s.cfg)
	if receiverCell == InvalidCell || senderCell == InvalidCell {
		// Still allow coarse update when fine cells are missing.
	} else {
		s.updateBucket(packKey(receiverCell, senderCell, idx), power, weight, now, halfLife)
	}
	if receiverCoarse != InvalidCell && senderCoarse != InvalidCell {
		s.updateBucket(packCoarseKey(receiverCoarse, senderCoarse, idx), power, weight, now, halfLife)
	}
}

func (s *Store) updateBucket(key uint64, power float64, weight float64, now time.Time, halfLifeSec int) {
	if key == 0 {
		return
	}
	sh := &s.shards[key%uint64(len(s.shards))]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	nowSec := now.Unix()
	b, ok := sh.buckets[key]
	if !ok {
		sh.buckets[key] = &bucket{
			sumPower:   power * weight,
			weight:     weight,
			lastUpdate: nowSec,
		}
		return
	}
	elapsed := nowSec - b.lastUpdate
	decay := decayFactor(elapsed, halfLifeSec)
	oldWeight := b.weight * decay
	oldSumPower := b.sumPower * decay
	newWeight := oldWeight + weight
	if newWeight <= 0 {
		b.weight = 0
		b.sumPower = 0
		b.lastUpdate = nowSec
		return
	}
	newSumPower := oldSumPower + power*weight
	b.sumPower = newSumPower
	b.weight = newWeight
	b.lastUpdate = nowSec
}

// Sample represents a decayed power reading with weight.
type Sample struct {
	Value  float64
	Weight float64
	AgeSec int64
}

type bandCounts struct {
	fine   int
	coarse int
}

type weightHistogram struct {
	total int
	bins  []int
}

// Lookup returns the decayed samples for the given keys.
func (s *Store) Lookup(receiverCell, senderCell CellID, receiverCoarse, senderCoarse CellID, band string, now time.Time) (fine Sample, coarse Sample) {
	if s == nil || !s.cfg.Enabled {
		return
	}
	idx, ok := s.bandIndex.Lookup(band)
	if !ok {
		return
	}
	halfLife := s.bandIndex.HalfLifeSeconds(band, s.cfg)
	if receiverCell != InvalidCell && senderCell != InvalidCell {
		fine = s.sample(packKey(receiverCell, senderCell, idx), halfLife, now)
	}
	if receiverCoarse != InvalidCell && senderCoarse != InvalidCell {
		coarse = s.sample(packCoarseKey(receiverCoarse, senderCoarse, idx), halfLife, now)
	}
	return
}

func (s *Store) sample(key uint64, halfLife int, now time.Time) Sample {
	if key == 0 {
		return Sample{}
	}
	sh := &s.shards[key%uint64(len(s.shards))]
	sh.mu.RLock()
	b := sh.buckets[key]
	sh.mu.RUnlock()
	if b == nil {
		return Sample{}
	}
	nowSec := now.Unix()
	age := nowSec - b.lastUpdate
	if age < 0 {
		age = 0
	}
	staleAfter := s.staleAfterSeconds(halfLife)
	if staleAfter > 0 && age > staleAfter {
		return Sample{}
	}
	decay := decayFactor(age, halfLife)
	decayedWeight := b.weight * decay
	if decayedWeight <= 0 {
		return Sample{}
	}
	decayedSumPower := b.sumPower * decay
	if decayedSumPower <= 0 {
		return Sample{}
	}
	return Sample{
		Value:  decayedSumPower / decayedWeight,
		Weight: decayedWeight,
		AgeSec: age,
	}
}

// PurgeStale removes buckets older than stale-after.
func (s *Store) PurgeStale(now time.Time) int {
	if s == nil {
		return 0
	}
	removed := 0
	bands := s.bandIndex.Bands()
	staleAfterByBand := make([]int64, len(bands))
	for i, band := range bands {
		halfLife := s.bandIndex.HalfLifeSeconds(band, s.cfg)
		staleAfterByBand[i] = s.staleAfterSeconds(halfLife)
	}
	nowSec := now.Unix()
	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.Lock()
		for k, b := range sh.buckets {
			if b == nil {
				delete(sh.buckets, k)
				removed++
				continue
			}
			age := nowSec - b.lastUpdate
			if age < 0 {
				age = 0
			}
			isCoarse := k&0xFFFF != 0
			var idx uint16
			if isCoarse {
				idx = uint16((k >> 32) & 0xFFFF)
			} else {
				idx = uint16((k >> 48) & 0xFFFF)
			}
			if int(idx) >= len(staleAfterByBand) {
				continue
			}
			staleAfter := staleAfterByBand[idx]
			if staleAfter > 0 && age > staleAfter {
				delete(sh.buckets, k)
				removed++
			}
		}
		sh.mu.Unlock()
	}
	return removed
}

// Stats returns counts of active fine/coarse buckets (non-stale).
func (s *Store) Stats(now time.Time) (fine int, coarse int) {
	if s == nil || !s.cfg.Enabled {
		return 0, 0
	}
	bands := s.bandIndex.Bands()
	staleAfterByBand := make([]int64, len(bands))
	for i, band := range bands {
		halfLife := s.bandIndex.HalfLifeSeconds(band, s.cfg)
		staleAfterByBand[i] = s.staleAfterSeconds(halfLife)
	}
	nowSec := now.Unix()
	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.RLock()
		for key, b := range sh.buckets {
			if b == nil {
				continue
			}
			age := nowSec - b.lastUpdate
			if age < 0 {
				age = 0
			}
			isCoarse := key&0xFFFF != 0
			var idx uint16
			if isCoarse {
				idx = uint16((key >> 32) & 0xFFFF)
			} else {
				idx = uint16((key >> 48) & 0xFFFF)
			}
			if int(idx) >= len(staleAfterByBand) {
				continue
			}
			staleAfter := staleAfterByBand[idx]
			if staleAfter > 0 && age > staleAfter {
				continue
			}
			if isCoarse {
				coarse++
			} else {
				fine++
			}
		}
		sh.mu.RUnlock()
	}
	return fine, coarse
}

// StatsByBand returns counts of active fine/coarse buckets per band.
func (s *Store) StatsByBand(now time.Time) []bandCounts {
	if s == nil || !s.cfg.Enabled {
		return nil
	}
	bands := s.bandIndex.Bands()
	if len(bands) == 0 {
		return nil
	}
	counts := make([]bandCounts, len(bands))
	staleAfterByBand := make([]int64, len(bands))
	for i, band := range bands {
		halfLife := s.bandIndex.HalfLifeSeconds(band, s.cfg)
		staleAfterByBand[i] = s.staleAfterSeconds(halfLife)
	}
	nowSec := now.Unix()
	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.RLock()
		for key, b := range sh.buckets {
			if b == nil {
				continue
			}
			age := nowSec - b.lastUpdate
			if age < 0 {
				age = 0
			}
			isCoarse := key&0xFFFF != 0
			var idx uint16
			if isCoarse {
				idx = uint16((key >> 32) & 0xFFFF)
			} else {
				idx = uint16((key >> 48) & 0xFFFF)
			}
			if int(idx) >= len(counts) {
				continue
			}
			staleAfter := staleAfterByBand[idx]
			if staleAfter > 0 && age > staleAfter {
				continue
			}
			if isCoarse {
				counts[idx].coarse++
			} else {
				counts[idx].fine++
			}
		}
		sh.mu.RUnlock()
	}
	return counts
}

// WeightHistogramByBand returns per-band bucket weight histograms for active buckets.
// edges defines the ascending bin boundaries (len+1 bins, last bin is >= last edge).
func (s *Store) WeightHistogramByBand(now time.Time, edges []float64) []weightHistogram {
	if s == nil || !s.cfg.Enabled {
		return nil
	}
	bands := s.bandIndex.Bands()
	if len(bands) == 0 {
		return nil
	}
	if len(edges) == 0 {
		return nil
	}
	binCount := len(edges) + 1
	counts := make([]weightHistogram, len(bands))
	for i := range counts {
		counts[i].bins = make([]int, binCount)
	}
	halfLives := make([]int, len(bands))
	for i, band := range bands {
		halfLives[i] = s.bandIndex.HalfLifeSeconds(band, s.cfg)
	}
	staleAfterByBand := make([]int64, len(bands))
	for i, halfLife := range halfLives {
		staleAfterByBand[i] = s.staleAfterSeconds(halfLife)
	}
	nowSec := now.Unix()
	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.RLock()
		for key, b := range sh.buckets {
			if b == nil {
				continue
			}
			age := nowSec - b.lastUpdate
			if age < 0 {
				age = 0
			}
			isCoarse := key&0xFFFF != 0
			var idx uint16
			if isCoarse {
				idx = uint16((key >> 32) & 0xFFFF)
			} else {
				idx = uint16((key >> 48) & 0xFFFF)
			}
			if int(idx) >= len(counts) {
				continue
			}
			staleAfter := staleAfterByBand[idx]
			if staleAfter > 0 && age > staleAfter {
				continue
			}
			halfLife := halfLives[idx]
			decay := decayFactor(age, halfLife)
			decayedWeight := float64(b.weight) * decay
			if decayedWeight <= 0 {
				continue
			}
			bin := weightBinIndex(decayedWeight, edges)
			counts[idx].total++
			counts[idx].bins[bin]++
		}
		sh.mu.RUnlock()
	}
	return counts
}

func weightBinIndex(weight float64, edges []float64) int {
	for i, edge := range edges {
		if weight < edge {
			return i
		}
	}
	return len(edges)
}

func decayFactor(ageSec int64, halfLifeSec int) float64 {
	if halfLifeSec <= 0 || ageSec <= 0 {
		return 1
	}
	return math.Exp(-ln2 * float64(ageSec) / float64(halfLifeSec))
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func (s *Store) staleAfterSeconds(halfLifeSec int) int64 {
	if s == nil {
		return 0
	}
	if s.cfg.StaleAfterHalfLifeMultiplier > 0 && halfLifeSec > 0 {
		seconds := math.Ceil(float64(halfLifeSec) * s.cfg.StaleAfterHalfLifeMultiplier)
		if seconds < 1 {
			seconds = 1
		}
		return int64(seconds)
	}
	if s.cfg.StaleAfterSeconds <= 0 {
		return 0
	}
	return int64(s.cfg.StaleAfterSeconds)
}
