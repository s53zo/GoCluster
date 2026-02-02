package telnet

import (
	"sort"
	"sync/atomic"
	"time"
)

type LatencySnapshot struct {
	P50 time.Duration
	P99 time.Duration
	N   uint64
}

type latencyTracker struct {
	samples []int64
	next    uint64
}

func newLatencyTracker(size int) *latencyTracker {
	if size <= 0 {
		size = 256
	}
	samples := make([]int64, size)
	for i := range samples {
		samples[i] = -1
	}
	return &latencyTracker{samples: samples}
}

func (t *latencyTracker) Observe(d time.Duration) {
	if t == nil || len(t.samples) == 0 {
		return
	}
	if d < 0 {
		d = 0
	}
	idx := atomic.AddUint64(&t.next, 1) - 1
	atomic.StoreInt64(&t.samples[idx%uint64(len(t.samples))], d.Nanoseconds())
}

func (t *latencyTracker) Snapshot() LatencySnapshot {
	if t == nil || len(t.samples) == 0 {
		return LatencySnapshot{}
	}
	values := make([]int64, 0, len(t.samples))
	for i := range t.samples {
		v := atomic.LoadInt64(&t.samples[i])
		if v < 0 {
			continue
		}
		values = append(values, v)
	}
	if len(values) == 0 {
		return LatencySnapshot{}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	p50 := values[len(values)/2]
	p99 := values[int(float64(len(values)-1)*0.99)]
	return LatencySnapshot{
		P50: time.Duration(p50),
		P99: time.Duration(p99),
		N:   uint64(len(values)),
	}
}

type latencyMetrics struct {
	enqueue    *latencyTracker
	firstByte  *latencyTracker
	writeStall *latencyTracker
}

func newLatencyMetrics() latencyMetrics {
	return latencyMetrics{
		enqueue:    newLatencyTracker(512),
		firstByte:  newLatencyTracker(512),
		writeStall: newLatencyTracker(512),
	}
}

func (m *latencyMetrics) snapshot() (enqueue, firstByte, writeStall LatencySnapshot) {
	if m == nil {
		return LatencySnapshot{}, LatencySnapshot{}, LatencySnapshot{}
	}
	return m.enqueue.Snapshot(), m.firstByte.Snapshot(), m.writeStall.Snapshot()
}
