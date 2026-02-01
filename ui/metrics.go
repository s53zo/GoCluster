package ui

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// LatencyTracker keeps a bounded ring of durations for percentile estimates.
type LatencyTracker struct {
	mu      sync.Mutex
	samples []time.Duration
	count   int
	idx     int
}

func NewLatencyTracker(size int) *LatencyTracker {
	if size <= 0 {
		size = 256
	}
	return &LatencyTracker{samples: make([]time.Duration, size)}
}

func (t *LatencyTracker) Observe(d time.Duration) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.samples[t.idx] = d
	t.idx = (t.idx + 1) % len(t.samples)
	if t.count < len(t.samples) {
		t.count++
	}
	t.mu.Unlock()
}

type LatencySnapshot struct {
	P50 time.Duration
	P99 time.Duration
	N   int
}

func (t *LatencyTracker) Snapshot() LatencySnapshot {
	if t == nil {
		return LatencySnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.count == 0 {
		return LatencySnapshot{}
	}
	values := make([]time.Duration, t.count)
	copy(values, t.samples[:t.count])
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	p50 := values[t.count/2]
	p99 := values[int(float64(t.count-1)*0.99)]
	return LatencySnapshot{P50: p50, P99: p99, N: t.count}
}

// Metrics tracks UI-level counters and latency distributions.
type Metrics struct {
	renderLatency *LatencyTracker
	searchLatency *LatencyTracker
	pageSwitches  atomic.Uint64
}

func NewMetrics() *Metrics {
	return &Metrics{
		renderLatency: NewLatencyTracker(512),
		searchLatency: NewLatencyTracker(512),
	}
}

func (m *Metrics) ObserveRender(d time.Duration) {
	if m == nil {
		return
	}
	m.renderLatency.Observe(d)
}

func (m *Metrics) ObserveSearch(d time.Duration) {
	if m == nil {
		return
	}
	m.searchLatency.Observe(d)
}

func (m *Metrics) PageSwitch() {
	if m == nil {
		return
	}
	m.pageSwitches.Add(1)
}

func (m *Metrics) RenderSnapshot() LatencySnapshot {
	if m == nil {
		return LatencySnapshot{}
	}
	return m.renderLatency.Snapshot()
}

func (m *Metrics) SearchSnapshot() LatencySnapshot {
	if m == nil {
		return LatencySnapshot{}
	}
	return m.searchLatency.Snapshot()
}

func (m *Metrics) PageSwitches() uint64 {
	if m == nil {
		return 0
	}
	return m.pageSwitches.Load()
}
