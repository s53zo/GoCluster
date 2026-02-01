package ui

import (
	"sync"
	"sync/atomic"
	"time"
)

// EventKind identifies a UI event category.
type EventKind int

const (
	EventDrop EventKind = iota
	EventCorrection
	EventUnlicensed
	EventHarmonic
	EventReputation
	EventSystem
)

func (k EventKind) Label() string {
	switch k {
	case EventDrop:
		return "DROP"
	case EventCorrection:
		return "CORR"
	case EventUnlicensed:
		return "UNLC"
	case EventHarmonic:
		return "HARM"
	case EventReputation:
		return "REPUT"
	case EventSystem:
		return "SYS"
	default:
		return "UNK"
	}
}

// StyledEvent represents a single UI event line.
type StyledEvent struct {
	Timestamp time.Time
	Kind      EventKind
	Message   string
}

// DropPolicy defines behavior when buffer limits are exceeded.
type DropPolicy struct {
	MaxMessageBytes  int
	EvictOnByteLimit bool
	LogDrops         bool
}

// DropReason enumerates why an event was dropped.
type DropReason int

const (
	DropOversized DropReason = iota
	DropEvicted
	DropByteLimit
)

// DropMetrics holds counters for drop events.
type DropMetrics struct {
	Oversized uint64
	Evicted   uint64
	ByteLimit uint64
}

// EventSnapshot provides an immutable snapshot and sequence.
type EventSnapshot struct {
	Events []StyledEvent
	Seq    uint64
}

// BoundedEventBuffer stores events in a bounded ring with byte limits.
// Concurrency: Append() may be called by multiple goroutines; SnapshotInto()
// is typically called by a single render goroutine.
type BoundedEventBuffer struct {
	mu       sync.RWMutex
	events   []StyledEvent
	head     int
	count    int
	maxCount int
	maxBytes int64
	curBytes int64
	policy   DropPolicy
	name     string
	seq      atomic.Uint64

	dropOversized atomic.Uint64
	dropEvicted   atomic.Uint64
	dropByteLimit atomic.Uint64

	logMu       sync.Mutex
	lastDropLog time.Time
	logf        func(format string, args ...interface{})
}

// NewBoundedEventBuffer creates a bounded event buffer.
func NewBoundedEventBuffer(name string, maxCount int, maxBytes int64, policy DropPolicy, logf func(format string, args ...interface{})) *BoundedEventBuffer {
	if maxCount <= 0 {
		maxCount = 1
	}
	if maxBytes < 0 {
		maxBytes = 0
	}
	return &BoundedEventBuffer{
		events:   make([]StyledEvent, maxCount),
		maxCount: maxCount,
		maxBytes: maxBytes,
		policy:   policy,
		name:     name,
		logf:     logf,
	}
}

// Append inserts an event, enforcing count and byte limits.
// Returns false if the event was dropped.
func (b *BoundedEventBuffer) Append(e StyledEvent) bool {
	if b == nil {
		return false
	}
	msgSize := int64(len(e.Message))
	if b.policy.MaxMessageBytes > 0 && len(e.Message) > b.policy.MaxMessageBytes {
		b.dropOversized.Add(1)
		if b.policy.LogDrops {
			b.logDrop("oversized", len(e.Message), b.policy.MaxMessageBytes)
		}
		return false
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Enforce count limit (always evict oldest).
	for b.count >= b.maxCount && b.count > 0 {
		b.evictOldestLocked()
	}

	// Enforce byte limit.
	if b.maxBytes > 0 && b.curBytes+msgSize > b.maxBytes {
		if !b.policy.EvictOnByteLimit {
			b.dropByteLimit.Add(1)
			if b.policy.LogDrops {
				b.logDrop("byte_limit", len(e.Message), int(b.maxBytes))
			}
			return false
		}
		for b.count > 0 && b.curBytes+msgSize > b.maxBytes {
			b.evictOldestLocked()
		}
		if b.curBytes+msgSize > b.maxBytes {
			b.dropByteLimit.Add(1)
			if b.policy.LogDrops {
				b.logDrop("byte_limit", len(e.Message), int(b.maxBytes))
			}
			return false
		}
	}

	pos := (b.head + b.count) % len(b.events)
	b.events[pos] = e
	b.curBytes += msgSize
	b.count++
	b.seq.Add(1)
	return true
}

// SnapshotInto copies events into dst and returns an immutable snapshot.
// The caller owns dst and the returned slice.
func (b *BoundedEventBuffer) SnapshotInto(dst []StyledEvent) EventSnapshot {
	if b == nil {
		return EventSnapshot{Events: dst[:0]}
	}
	b.mu.RLock()
	defer b.mu.RUnlock()

	if cap(dst) < b.count {
		dst = make([]StyledEvent, b.count)
	} else {
		dst = dst[:b.count]
	}
	for i := 0; i < b.count; i++ {
		pos := (b.head + i) % len(b.events)
		dst[i] = b.events[pos]
	}
	return EventSnapshot{Events: dst, Seq: b.seq.Load()}
}

// DropSnapshot returns current drop counters.
func (b *BoundedEventBuffer) DropSnapshot() DropMetrics {
	if b == nil {
		return DropMetrics{}
	}
	return DropMetrics{
		Oversized: b.dropOversized.Load(),
		Evicted:   b.dropEvicted.Load(),
		ByteLimit: b.dropByteLimit.Load(),
	}
}

// BufferUsage returns current counts and bytes.
func (b *BoundedEventBuffer) BufferUsage() (count int, maxCount int, bytes int64, maxBytes int64) {
	if b == nil {
		return 0, 0, 0, 0
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.count, b.maxCount, b.curBytes, b.maxBytes
}

func (b *BoundedEventBuffer) evictOldestLocked() {
	if b.count == 0 {
		return
	}
	old := b.events[b.head]
	b.curBytes -= int64(len(old.Message))
	b.head = (b.head + 1) % len(b.events)
	b.count--
	b.dropEvicted.Add(1)
}

func (b *BoundedEventBuffer) logDrop(reason string, size int, limit int) {
	if b.logf == nil {
		return
	}
	now := time.Now().UTC()
	b.logMu.Lock()
	if !b.lastDropLog.IsZero() && now.Sub(b.lastDropLog) < 30*time.Second {
		b.logMu.Unlock()
		return
	}
	b.lastDropLog = now
	b.logMu.Unlock()
	b.logf("UI: dropped %s event (%d bytes > %d) buffer=%s", reason, size, limit, b.name)
}
