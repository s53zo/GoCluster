package ui

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestFrameSchedulerCoalescesLatestPerID(t *testing.T) {
	f := newFrameScheduler(nil, 60, 50*time.Millisecond, nil)

	var seq []string
	f.Schedule("pane-a", func() { seq = append(seq, "a1") })
	f.Schedule("pane-a", func() { seq = append(seq, "a2") })
	f.Schedule("pane-b", func() { seq = append(seq, "b1") })

	f.flush()

	if len(seq) != 2 {
		t.Fatalf("expected 2 callbacks, got %d (%v)", len(seq), seq)
	}
	if seq[0] != "a2" || seq[1] != "b1" {
		t.Fatalf("unexpected callback order/content: %v", seq)
	}

	f.flush()
	if len(seq) != 2 {
		t.Fatalf("expected no additional callbacks after empty flush, got %v", seq)
	}
}

func TestFrameSchedulerFlushesPendingOnStop(t *testing.T) {
	f := newFrameScheduler(nil, 60, 50*time.Millisecond, nil)
	var called atomic.Uint64

	f.Start()
	f.Schedule("pane", func() { called.Add(1) })
	f.Stop()

	if called.Load() != 1 {
		t.Fatalf("expected pending callback to flush on stop, got %d", called.Load())
	}
}

func TestFrameSchedulerStopIdempotent(t *testing.T) {
	f := newFrameScheduler(nil, 60, 50*time.Millisecond, nil)
	f.Start()
	f.Stop()
	f.Stop()
}
