package spot

import (
	"testing"
	"time"
)

func containsSpot(spots []*Spot, target *Spot) bool {
	for _, s := range spots {
		if s == target {
			return true
		}
	}
	return false
}

func TestCorrectionIndexCandidatesSearchWindow(t *testing.T) {
	now := time.Now()
	window := 30 * time.Second

	t.Run("default window filters beyond 0.5kHz", func(t *testing.T) {
		idx := NewCorrectionIndex()
		subject := &Spot{Frequency: 14000.0, Time: now}
		within := &Spot{Frequency: 14000.4, Time: now}
		outside := &Spot{Frequency: 14000.6, Time: now}

		idx.Add(within, now, window)
		idx.Add(outside, now, window)

		candidates := idx.Candidates(subject, now, window, 0.5)
		if !containsSpot(candidates, within) {
			t.Fatalf("expected spot within 0.5 kHz to be included")
		}
		if containsSpot(candidates, outside) {
			t.Fatalf("expected spot beyond 0.5 kHz to be excluded")
		}
	})

	t.Run("voice window includes 2kHz radius", func(t *testing.T) {
		idx := NewCorrectionIndex()
		subject := &Spot{Frequency: 14000.0, Time: now}
		within := &Spot{Frequency: 14001.9, Time: now}
		outside := &Spot{Frequency: 14002.1, Time: now}

		idx.Add(within, now, window)
		idx.Add(outside, now, window)

		candidates := idx.Candidates(subject, now, window, 2.0)
		if !containsSpot(candidates, within) {
			t.Fatalf("expected spot within 2.0 kHz to be included")
		}
		if containsSpot(candidates, outside) {
			t.Fatalf("expected spot beyond 2.0 kHz to be excluded")
		}
	})
}
