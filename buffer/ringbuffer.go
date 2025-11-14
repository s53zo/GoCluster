package buffer

import (
	"sync"

	"dxcluster/spot"
)

// RingBuffer holds a fixed number of recent spots
type RingBuffer struct {
	spots    []*spot.Spot
	capacity int
	head     int // Next write position
	size     int // Current number of spots
	nextID   uint64
	mu       sync.RWMutex
}

// NewRingBuffer creates a new ring buffer with the specified capacity
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		spots:    make([]*spot.Spot, capacity),
		capacity: capacity,
		head:     0,
		size:     0,
		nextID:   1,
	}
}

// Add inserts a new spot into the buffer
func (rb *RingBuffer) Add(s *spot.Spot) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Assign unique ID
	s.ID = rb.nextID
	rb.nextID++

	// Add to buffer
	rb.spots[rb.head] = s
	rb.head = (rb.head + 1) % rb.capacity

	// Update size
	if rb.size < rb.capacity {
		rb.size++
	}
}

// GetRecent returns the N most recent spots
func (rb *RingBuffer) GetRecent(n int) []*spot.Spot {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if n > rb.size {
		n = rb.size
	}

	result := make([]*spot.Spot, 0, n)

	// Start from most recent and go backwards
	for i := 0; i < n; i++ {
		idx := (rb.head - 1 - i + rb.capacity) % rb.capacity
		if rb.spots[idx] != nil {
			result = append(result, rb.spots[idx])
		}
	}

	return result
}

// GetAll returns all spots in the buffer (most recent first)
func (rb *RingBuffer) GetAll() []*spot.Spot {
	return rb.GetRecent(rb.size)
}

// Size returns the current number of spots in the buffer
func (rb *RingBuffer) Size() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.size
}

// Clear removes all spots from the buffer
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.spots = make([]*spot.Spot, rb.capacity)
	rb.head = 0
	rb.size = 0
}
