package main

import (
	"runtime"
	"testing"
	"time"
)

func TestGCPauseWindowSnapshotNoGC(t *testing.T) {
	var mem runtime.MemStats
	mem.NumGC = 5
	mem.PauseNs[0] = 10
	mem.PauseNs[1] = 20

	var window gcPauseWindow
	if p99, count, truncated := window.snapshot(&mem); count != 0 || truncated || p99 != 0 {
		t.Fatalf("expected init snapshot to return no data; got p99=%v count=%d truncated=%v", p99, count, truncated)
	}
	if p99, count, truncated := window.snapshot(&mem); count != 0 || truncated || p99 != 0 {
		t.Fatalf("expected no new GCs; got p99=%v count=%d truncated=%v", p99, count, truncated)
	}
}

func TestGCPauseWindowSnapshotDeltaP99(t *testing.T) {
	var mem runtime.MemStats
	mem.NumGC = 2
	mem.PauseNs[0] = 5
	mem.PauseNs[1] = 7

	var window gcPauseWindow
	_, _, _ = window.snapshot(&mem)

	mem.NumGC = 5
	mem.PauseNs[2] = 10
	mem.PauseNs[3] = 20
	mem.PauseNs[4] = 30

	p99, count, truncated := window.snapshot(&mem)
	if truncated {
		t.Fatalf("unexpected truncation")
	}
	if count != 3 {
		t.Fatalf("expected 3 pauses; got %d", count)
	}
	if want := 20 * time.Nanosecond; p99 != want {
		t.Fatalf("expected p99 %v; got %v", want, p99)
	}
}

func TestGCPauseWindowSnapshotTruncates(t *testing.T) {
	var mem runtime.MemStats
	mem.NumGC = 0

	var window gcPauseWindow
	_, _, _ = window.snapshot(&mem)

	mem.NumGC = 300
	for i := range mem.PauseNs {
		mem.PauseNs[i] = 50
	}

	p99, count, truncated := window.snapshot(&mem)
	if !truncated {
		t.Fatalf("expected truncation")
	}
	if count != len(mem.PauseNs) {
		t.Fatalf("expected %d pauses; got %d", len(mem.PauseNs), count)
	}
	if want := 50 * time.Nanosecond; p99 != want {
		t.Fatalf("expected p99 %v; got %v", want, p99)
	}
}
