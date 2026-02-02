package telnet

import (
	"sort"
	"testing"
	"time"
)

func TestLatencyTrackerSnapshot(t *testing.T) {
	tracker := newLatencyTracker(8)
	samples := []time.Duration{
		5 * time.Millisecond,
		1 * time.Millisecond,
		3 * time.Millisecond,
		2 * time.Millisecond,
		4 * time.Millisecond,
	}
	for _, d := range samples {
		tracker.Observe(d)
	}
	snap := tracker.Snapshot()
	if snap.N != uint64(len(samples)) {
		t.Fatalf("expected N=%d, got %d", len(samples), snap.N)
	}

	expected := make([]int64, len(samples))
	for i, d := range samples {
		expected[i] = d.Nanoseconds()
	}
	sort.Slice(expected, func(i, j int) bool { return expected[i] < expected[j] })
	p50 := expected[len(expected)/2]
	p99 := expected[int(float64(len(expected)-1)*0.99)]

	if snap.P50 != time.Duration(p50) {
		t.Fatalf("expected p50=%s, got %s", time.Duration(p50), snap.P50)
	}
	if snap.P99 != time.Duration(p99) {
		t.Fatalf("expected p99=%s, got %s", time.Duration(p99), snap.P99)
	}
}

func TestLatencyTrackerClampsNegative(t *testing.T) {
	tracker := newLatencyTracker(4)
	tracker.Observe(-5 * time.Millisecond)
	snap := tracker.Snapshot()
	if snap.N != 1 {
		t.Fatalf("expected N=1, got %d", snap.N)
	}
	if snap.P50 != 0 {
		t.Fatalf("expected p50=0, got %s", snap.P50)
	}
	if snap.P99 != 0 {
		t.Fatalf("expected p99=0, got %s", snap.P99)
	}
}
