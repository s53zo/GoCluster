package bandmap

import (
	"math"
	"strings"
	"sync"
	"time"
)

// ShardCount determines how many shards guard the frequency bins per mode.
const ShardCount = 64

// MaxBinCapacity bounds the slice length per 100 Hz bin to avoid unbounded memory growth.
const MaxBinCapacity = 50

// SpotEntry is a lightweight copy of spot data needed for correction logic.
// All frequencies are stored in integer Hertz; callers must convert from kHz upstream.
type SpotEntry struct {
	Call    string // DX callsign
	Spotter string // DE callsign
	Mode    string // Mode (e.g., CW, RTTY)
	FreqHz  uint32 // Frequency in Hz (not kHz)
	Time    int64  // Unix timestamp (seconds)
	SNR     int    // Signal-to-noise ratio (dB)
}

// BandMap is a mode-partitioned spatial index over 100 Hz bins.
// It performs no validation; callers must enforce admission policy.
type BandMap struct {
	mu    sync.RWMutex
	grids map[string]*ShardedGrid
}

// ShardedGrid holds shards for a single mode.
type ShardedGrid struct {
	shards [ShardCount]*Shard
}

// Shard guards a map of binID->entries for a subset of the band.
type Shard struct {
	mu   sync.RWMutex
	bins map[uint32][]SpotEntry
}

// New constructs an empty BandMap.
func New() *BandMap {
	return &BandMap{grids: make(map[string]*ShardedGrid)}
}

// Add inserts an entry into the map. The caller must pass frequencies in Hz and
// perform any validation (CTY, SNR) beforehand.
func (bm *BandMap) Add(mode string, entry SpotEntry) {
	modeKey := strings.ToUpper(mode)
	grid := bm.getOrCreateGrid(modeKey)

	binID := entry.FreqHz / 100
	shardIdx := binID % ShardCount
	shard := grid.shards[shardIdx]

	shard.mu.Lock()
	defer shard.mu.Unlock()
	if shard.bins == nil {
		shard.bins = make(map[uint32][]SpotEntry)
	}
	list := shard.bins[binID]
	list = append(list, entry)
	if len(list) > MaxBinCapacity {
		list = list[1:]
	}
	shard.bins[binID] = list
}

// Get returns entries within center±window (Hz) newer than now-maxAgeSeconds.
// If maxAgeSeconds<=0, no time filtering is applied.
func (bm *BandMap) Get(mode string, centerFreqHz uint32, windowHz uint32, maxAgeSeconds int64) []SpotEntry {
	modeKey := strings.ToUpper(mode)

	bm.mu.RLock()
	grid, ok := bm.grids[modeKey]
	bm.mu.RUnlock()
	if !ok {
		return nil
	}

	var minFreq, maxFreq uint32
	if windowHz >= centerFreqHz {
		minFreq = 0
	} else {
		minFreq = centerFreqHz - windowHz
	}
	if math.MaxUint32-windowHz < centerFreqHz {
		maxFreq = math.MaxUint32
	} else {
		maxFreq = centerFreqHz + windowHz
	}
	minBin := minFreq / 100
	maxBin := maxFreq / 100

	cutoff := int64(0)
	if maxAgeSeconds > 0 {
		cutoff = time.Now().Unix() - maxAgeSeconds
	}

	results := make([]SpotEntry, 0, 64)
	for b := minBin; b <= maxBin; b++ {
		shardIdx := b % ShardCount
		shard := grid.shards[shardIdx]
		shard.mu.RLock()
		if shard.bins != nil {
			if entries, exists := shard.bins[b]; exists {
				for _, e := range entries {
					if e.Time >= cutoff {
						results = append(results, e)
					}
				}
			}
		}
		shard.mu.RUnlock()
		if b == math.MaxUint32 {
			break
		}
	}
	return results
}

// Prune removes entries older than now-maxAgeSeconds. If maxAgeSeconds<=0, it does nothing.
func (bm *BandMap) Prune(maxAgeSeconds int64) {
	if maxAgeSeconds <= 0 {
		return
	}
	cutoff := time.Now().Unix() - maxAgeSeconds

	bm.mu.RLock()
	grids := make([]*ShardedGrid, 0, len(bm.grids))
	for _, g := range bm.grids {
		grids = append(grids, g)
	}
	bm.mu.RUnlock()

	for _, grid := range grids {
		for i := 0; i < ShardCount; i++ {
			shard := grid.shards[i]
			shard.mu.Lock()
			if shard.bins != nil {
				for binID, entries := range shard.bins {
					n := 0
					for _, e := range entries {
						if e.Time >= cutoff {
							entries[n] = e
							n++
						}
					}
					entries = entries[:n]
					if len(entries) == 0 {
						delete(shard.bins, binID)
					} else {
						shard.bins[binID] = entries
					}
				}
			}
			shard.mu.Unlock()
		}
	}
}

func (bm *BandMap) getOrCreateGrid(mode string) *ShardedGrid {
	bm.mu.RLock()
	grid, ok := bm.grids[mode]
	bm.mu.RUnlock()
	if ok {
		return grid
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()
	if grid, ok = bm.grids[mode]; ok {
		return grid
	}
	grid = &ShardedGrid{}
	for i := 0; i < ShardCount; i++ {
		grid.shards[i] = &Shard{}
	}
	bm.grids[mode] = grid
	return grid
}
