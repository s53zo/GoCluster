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

// bucket holds decaying FT8-equiv stats for a directional path.
type bucket struct {
	avg    float32
	weight float32
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
	neighbors map[string][]string
}

// NewStore constructs a path store with normalized config.
func NewStore(cfg Config, bands []string, neighbors map[string][]string) *Store {
	cfg.normalize()
	if len(bands) == 0 {
		bands = []string{"160m", "80m", "60m", "40m", "30m", "20m", "17m", "15m", "12m", "10m", "6m", "4m", "2m", "1m"}
	}
	idx := NewBandIndex(bands)
	s := &Store{
		shards:    make([]shard, defaultShards),
		cfg:       cfg,
		bandIndex: idx,
		neighbors: neighbors,
	}
	for i := range s.shards {
		s.shards[i].buckets = make(map[uint64]*bucket)
	}
	return s
}

// Update applies a new FT8-equiv reading to the directional path.
// weight should normally be 1.0; beacons may be clamped by caller.
func (s *Store) Update(receiverCell, senderCell CellID, receiverGrid2, senderGrid2 string, band string, value float64, weight float64, now time.Time) {
	if s == nil || !s.cfg.Enabled {
		return
	}
	idx, ok := s.bandIndex.Lookup(band)
	if !ok {
		return
	}
	halfLife := s.bandIndex.HalfLifeSeconds(band, s.cfg)
	if receiverCell == InvalidCell || senderCell == InvalidCell {
		// Still allow coarse update if grids are valid.
	} else {
		s.updateBucket(packKey(receiverCell, senderCell, idx), value, weight, now, halfLife)
	}
	if receiverGrid2 != "" && senderGrid2 != "" {
		s.updateBucket(packGrid2Key(receiverGrid2, senderGrid2, idx), value, weight, now, halfLife)
	}
}

func (s *Store) updateBucket(key uint64, value float64, weight float64, now time.Time, halfLifeSec int) {
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
			avg:        float32(clamp(value, s.cfg.ClampMin, s.cfg.ClampMax)),
			weight:     float32(weight),
			lastUpdate: nowSec,
		}
		return
	}
	elapsed := nowSec - b.lastUpdate
	decay := decayFactor(elapsed, halfLifeSec)
	oldWeight := float64(b.weight) * decay
	newWeight := oldWeight + weight
	if newWeight <= 0 {
		b.weight = 0
		b.avg = 0
		b.lastUpdate = nowSec
		return
	}
	oldAvg := float64(b.avg)
	newAvg := (oldAvg*oldWeight + value*weight) / newWeight
	b.avg = float32(clamp(newAvg, s.cfg.ClampMin, s.cfg.ClampMax))
	b.weight = float32(newWeight)
	b.lastUpdate = nowSec
}

// Sample represents a decayed reading with weight.
type Sample struct {
	Value  float64
	Weight float64
	AgeSec int64
}

// Lookup returns the decayed sample for the given key.
func (s *Store) Lookup(receiverCell, senderCell CellID, receiverGrid2, senderGrid2 string, band string, now time.Time) (fine Sample, coarse Sample, neighbors []Sample, reverse Sample) {
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
	if receiverGrid2 != "" && senderGrid2 != "" {
		coarse = s.sample(packGrid2Key(receiverGrid2, senderGrid2, idx), halfLife, now)
		if s.cfg.NeighborRadius > 0 {
			for _, nbr := range s.neighbors[receiverGrid2] {
				neighbors = append(neighbors, s.sample(packGrid2Key(nbr, senderGrid2, idx), halfLife, now))
			}
		}
		// Reverse hint uses sender as receiver.
		reverse = s.sample(packGrid2Key(senderGrid2, receiverGrid2, idx), halfLife, now)
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
	if age > int64(s.cfg.StaleAfterSeconds) {
		return Sample{}
	}
	decay := decayFactor(age, halfLife)
	decayedWeight := float64(b.weight) * decay
	if decayedWeight <= 0 {
		return Sample{}
	}
	return Sample{
		Value:  float64(b.avg),
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
	cutoff := now.Add(-time.Duration(s.cfg.StaleAfterSeconds) * time.Second).Unix()
	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.Lock()
		for k, b := range sh.buckets {
			if b == nil || b.lastUpdate <= cutoff {
				delete(sh.buckets, k)
				removed++
			}
		}
		sh.mu.Unlock()
	}
	return removed
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
