package spot

import (
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

var callsignPattern = regexp.MustCompile(`^[A-Z0-9]+(?:[/-][A-Z0-9#]+)*$`)

// CallCache is a tiny concurrency-safe cache to avoid repeating the same normalization
// work for hot call/spotter strings during bursts.
type CallCache struct {
	mu       sync.Mutex
	entries  map[string]cacheEntry
	order    []string
	capacity int
	ttl      time.Duration
}

type cacheEntry struct {
	value   string
	expires time.Time
}

// NewCallCache creates a bounded FIFO cache sized for short-lived reuse.
func NewCallCache(capacity int, ttl time.Duration) *CallCache {
	if capacity <= 0 {
		capacity = 1024
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &CallCache{
		entries:  make(map[string]cacheEntry, capacity),
		order:    make([]string, 0, capacity),
		capacity: capacity,
		ttl:      ttl,
	}
}

// Get returns a cached value for the raw key.
func (c *CallCache) Get(key string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if c.ttl > 0 && time.Now().After(entry.expires) {
		delete(c.entries, key)
		return "", false
	}
	return entry.value, true
}

// Add inserts or updates a cached value, evicting the oldest item when full.
func (c *CallCache) Add(key, val string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]cacheEntry, c.capacity)
		c.order = c.order[:0]
	}
	if _, exists := c.entries[key]; exists {
		c.entries[key] = cacheEntry{value: val, expires: time.Now().Add(c.ttl)}
		return
	}
	c.entries[key] = cacheEntry{value: val, expires: time.Now().Add(c.ttl)}
	c.order = append(c.order, key)
	if len(c.entries) > c.capacity && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
		// Bound runaway order growth if many keys were evicted.
		if cap(c.order) > c.capacity*2 && len(c.order) < c.capacity {
			c.order = append([]string(nil), c.order...)
		}
	}
}

var normalizeCallCache = NewCallCache(4096, 10*time.Minute)

// ConfigureNormalizeCallCache rebuilds the global normalization cache with the
// provided limits so operators can tune size/TTL via config.
func ConfigureNormalizeCallCache(size int, ttl time.Duration) {
	normalizeCallCache = NewCallCache(size, ttl)
}

// NormalizeCallsign uppercases the string, trims whitespace, and removes trailing dots or slashes.
func NormalizeCallsign(call string) string {
	if cached, ok := normalizeCallCache.Get(call); ok {
		return cached
	}
	normalized := strings.ToUpper(strings.TrimSpace(call))
	normalized = strings.ReplaceAll(normalized, ".", "/")
	normalized = strings.TrimSuffix(normalized, "/")
	normalized = strings.TrimSpace(normalized)
	normalizeCallCache.Add(call, normalized)
	return normalized
}

func validateNormalizedCallsign(call string) bool {
	if call == "" {
		return false
	}
	if len(call) < 3 || len(call) > 10 {
		return false
	}
	if strings.IndexFunc(call, unicode.IsDigit) < 0 {
		return false
	}
	return callsignPattern.MatchString(call)
}

// IsValidCallsign applies format checks to make sure it looks like a valid amateur call.
func IsValidCallsign(call string) bool {
	normalized := NormalizeCallsign(call)
	return validateNormalizedCallsign(normalized)
}

// IsBeaconCall reports whether the normalized callsign ends with /B.
func IsBeaconCall(call string) bool {
	normalized := NormalizeCallsign(call)
	return strings.HasSuffix(normalized, "/B")
}
