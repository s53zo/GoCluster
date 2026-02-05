package main

import (
	"runtime"
	"sort"
	"time"
)

// gcPauseWindow tracks GC pauses between stats refresh ticks.
// Ownership: displayStatsWithFCC owns the instance and calls snapshot serially.
// Invariant: snapshot is called at most once per stats interval.
type gcPauseWindow struct {
	lastNumGC   uint32
	initialized bool
}

// snapshot returns the p99 pause duration for GCs that occurred since the last
// snapshot, along with the number of pauses considered. If the number of new
// GCs exceeds the pause ring size, the snapshot is truncated to the most recent
// pauses and truncated is true.
func (w *gcPauseWindow) snapshot(mem *runtime.MemStats) (p99 time.Duration, count int, truncated bool) {
	if mem == nil {
		return 0, 0, false
	}
	if !w.initialized {
		w.lastNumGC = mem.NumGC
		w.initialized = true
		return 0, 0, false
	}
	if mem.NumGC <= w.lastNumGC {
		return 0, 0, false
	}
	delta := mem.NumGC - w.lastNumGC
	w.lastNumGC = mem.NumGC

	ringLen := len(mem.PauseNs)
	if ringLen == 0 {
		return 0, 0, false
	}

	needed := int(delta)
	if needed > ringLen {
		needed = ringLen
		truncated = true
	}

	pauses := make([]uint64, 0, needed)
	idx := int((mem.NumGC - 1) % uint32(ringLen))
	for i := 0; i < needed; i++ {
		if v := mem.PauseNs[idx]; v > 0 {
			pauses = append(pauses, v)
		}
		idx--
		if idx < 0 {
			idx = ringLen - 1
		}
	}
	if len(pauses) == 0 {
		return 0, 0, truncated
	}
	sort.Slice(pauses, func(i, j int) bool { return pauses[i] < pauses[j] })
	return time.Duration(p99Index(pauses)), len(pauses), truncated
}

func p99Index(pauses []uint64) uint64 {
	if len(pauses) == 0 {
		return 0
	}
	idx := int(float64(len(pauses)-1) * 0.99)
	if idx < 0 {
		idx = 0
	}
	return pauses[idx]
}
