package dedup

import (
	"testing"
	"time"
)

func TestDeduplicatorCleanupCompactsShard(t *testing.T) {
	d := NewDeduplicator(time.Second, false, 10)
	shard := &d.shards[0]
	now := time.Now().UTC()

	shard.mu.Lock()
	for i := 0; i < dedupCompactMinPeak; i++ {
		shard.cache[uint32(i)] = cachedEntry{when: now.Add(-2 * time.Second)}
	}
	keepKey := uint32(dedupCompactMinPeak + 1)
	shard.cache[keepKey] = cachedEntry{when: now}
	shard.peak = len(shard.cache)
	shard.mu.Unlock()

	d.cleanup()

	shard.mu.Lock()
	defer shard.mu.Unlock()
	if got := len(shard.cache); got != 1 {
		t.Fatalf("expected 1 entry after cleanup, got %d", got)
	}
	if _, ok := shard.cache[keepKey]; !ok {
		t.Fatalf("expected keepKey to remain after cleanup")
	}
	if shard.peak != 1 {
		t.Fatalf("expected peak reset to 1, got %d", shard.peak)
	}
}

func TestSecondaryCleanupCompactsShard(t *testing.T) {
	d := NewSecondaryDeduper(time.Second, false)
	shard := &d.shards[0]
	now := time.Now().UTC()

	shard.mu.Lock()
	for i := 0; i < secondaryCompactMinPeak; i++ {
		shard.cache[uint32(i)] = secondaryEntry{when: now.Add(-2 * time.Second)}
	}
	keepKey := uint32(secondaryCompactMinPeak + 1)
	shard.cache[keepKey] = secondaryEntry{when: now}
	shard.peak = len(shard.cache)
	shard.mu.Unlock()

	d.cleanup()

	shard.mu.Lock()
	defer shard.mu.Unlock()
	if got := len(shard.cache); got != 1 {
		t.Fatalf("expected 1 entry after cleanup, got %d", got)
	}
	if _, ok := shard.cache[keepKey]; !ok {
		t.Fatalf("expected keepKey to remain after cleanup")
	}
	if shard.peak != 1 {
		t.Fatalf("expected peak reset to 1, got %d", shard.peak)
	}
}
