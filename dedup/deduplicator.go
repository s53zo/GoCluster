package dedup

import (
	"fmt"
	"log"
	"sync"
	"time"

	"dxcluster/spot"
)

// Deduplicator removes duplicate spots using 32-bit hashes
// Modified to support unified architecture: ALL sources feed into inputChan
type Deduplicator struct {
	window time.Duration
	cache  map[uint32]time.Time // hash -> expiry time
	mu     sync.RWMutex
	ticker *time.Ticker
	stopCh chan struct{}

	// Unified architecture: ALL sources feed spots here
	inputChan  chan *spot.Spot // Input from ALL sources (RBN, PSKReporter, clusters, MQTT)
	outputChan chan *spot.Spot // Output to ring buffer (deduplicated spots)

	// Statistics
	totalSpots     uint64
	duplicateSpots uint64
	keptNewest     uint64 // Count of times we kept a newer duplicate
}

// NewDeduplicator creates a new hash-based deduplicator
func NewDeduplicator(window time.Duration) *Deduplicator {
	d := &Deduplicator{
		window:     window,
		cache:      make(map[uint32]time.Time),
		ticker:     time.NewTicker(30 * time.Second),
		stopCh:     make(chan struct{}),
		inputChan:  make(chan *spot.Spot, 1000), // Buffer 1000 spots
		outputChan: make(chan *spot.Spot, 1000), // Buffer 1000 spots
	}

	// Start cleanup goroutine
	go d.cleanupLoop()

	log.Printf("Deduplicator created with %v window", window)

	return d
}

// Start begins the unified deduplication processing loop
// This runs in its own goroutine and processes ALL incoming spots
func (d *Deduplicator) Start() {
	log.Println("Deduplicator: Starting unified processing loop for ALL sources")
	go d.processLoop()
}

// processLoop is the main loop that processes spots from ALL sources
// It consumes from inputChan and outputs to outputChan
func (d *Deduplicator) processLoop() {
	for {
		select {
		case <-d.stopCh:
			log.Println("Deduplicator: Processing loop shutting down")
			close(d.outputChan)
			return

		case spot := <-d.inputChan:
			// Use the existing Process() method to check for duplicates
			shouldBroadcast, _ := d.Process(spot)

			if shouldBroadcast {
				// Not a duplicate - send to output channel
				d.outputChan <- spot
			}
			// If duplicate, just drop it (don't send to output)
		}
	}
}

// GetInputChannel returns the input channel where ALL sources should send spots
// RBN, PSKReporter, cluster peers, MQTT all feed into this channel
func (d *Deduplicator) GetInputChannel() chan<- *spot.Spot {
	return d.inputChan
}

// GetOutputChannel returns the output channel that feeds the ring buffer
// Only deduplicated spots come out of this channel
func (d *Deduplicator) GetOutputChannel() <-chan *spot.Spot {
	return d.outputChan
}

// Process checks if a spot is a duplicate using hash-based detection
// Returns: (shouldBroadcast bool, isDuplicate bool)
// This method is kept for backward compatibility and is used internally by processLoop
func (d *Deduplicator) Process(s *spot.Spot) (bool, bool) {
	hash := s.Hash32()

	d.mu.Lock()
	defer d.mu.Unlock()

	d.totalSpots++
	now := time.Now()

	// Debug: log first few hashes
	if d.totalSpots <= 5 {
		log.Printf("DEBUG: Spot #%d: %s on %.1f kHz -> hash 0x%08x",
			d.totalSpots, s.DXCall, s.Frequency, hash)
	}

	// Check if we've seen this hash recently
	if expiry, exists := d.cache[hash]; exists {
		// Check if it's still within the deduplication window
		if now.Before(expiry) {
			// It's a duplicate
			d.duplicateSpots++

			log.Printf("DUPLICATE: %s on %.1f kHz (hash 0x%08x) - %d dupes so far",
				s.DXCall, s.Frequency, hash, d.duplicateSpots)

			// Log stats periodically
			if d.duplicateSpots%10 == 0 {
				log.Printf("Dedup stats: %d spots, %d duplicates (%.1f%%), cache size=%d, window=%v",
					d.totalSpots, d.duplicateSpots,
					float64(d.duplicateSpots)/float64(d.totalSpots)*100,
					len(d.cache), d.window)
			}

			return false, true // Don't broadcast, it's a duplicate
		}
	}

	// New spot or expired - add to cache
	d.cache[hash] = now.Add(d.window)

	// Debug: log cache additions for first few spots
	if d.totalSpots <= 5 {
		log.Printf("DEBUG: Added hash 0x%08x to cache, expires at %v",
			hash, now.Add(d.window).Format("15:04:05"))
	}

	return true, false // Broadcast it, not a duplicate
}

// cleanupLoop periodically removes expired hashes
func (d *Deduplicator) cleanupLoop() {
	for {
		select {
		case <-d.stopCh:
			return
		case <-d.ticker.C:
			d.cleanup()
		}
	}
}

// cleanup removes expired hashes from cache
func (d *Deduplicator) cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	removed := 0

	for hash, expiry := range d.cache {
		if now.After(expiry) {
			delete(d.cache, hash)
			removed++
		}
	}

	if removed > 0 {
		log.Printf("Dedup cleanup: removed %d expired hashes, %d remain in cache",
			removed, len(d.cache))
	}
}

// GetStats returns deduplication statistics
func (d *Deduplicator) GetStats() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	dupRate := 0.0
	if d.totalSpots > 0 {
		dupRate = float64(d.duplicateSpots) / float64(d.totalSpots) * 100
	}

	return map[string]interface{}{
		"cache_size":      len(d.cache),
		"total_spots":     d.totalSpots,
		"duplicate_spots": d.duplicateSpots,
		"duplicate_rate":  fmt.Sprintf("%.1f%%", dupRate),
		"window_seconds":  d.window.Seconds(),
		"kept_newest":     d.keptNewest,
	}
}

// Stop stops the deduplicator cleanup goroutine and processing loop
func (d *Deduplicator) Stop() {
	log.Println("Deduplicator: Stopping...")
	d.ticker.Stop()
	close(d.stopCh)
	close(d.inputChan) // Close input to stop processLoop

	// Log final stats
	stats := d.GetStats()
	log.Printf("Deduplicator stopped. Final stats: %+v", stats)
}
