package spot

import (
	"container/list"
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
	lru      *list.List
	entries  map[string]*list.Element
	capacity int
	ttl      time.Duration
}

type cacheEntry struct {
	key     string
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
		lru:      list.New(),
		entries:  make(map[string]*list.Element, capacity),
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
	elem, ok := c.entries[key]
	if !ok || elem == nil {
		return "", false
	}
	entry := elem.Value.(cacheEntry)
	if c.ttl > 0 && time.Now().After(entry.expires) {
		c.lru.Remove(elem)
		delete(c.entries, key)
		return "", false
	}
	// Move to front (most recently used).
	c.lru.MoveToFront(elem)
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
		c.lru = list.New()
		c.entries = make(map[string]*list.Element, c.capacity)
	}
	if elem, exists := c.entries[key]; exists && elem != nil {
		elem.Value = cacheEntry{key: key, value: val, expires: time.Now().Add(c.ttl)}
		c.lru.MoveToFront(elem)
		return
	}
	elem := c.lru.PushFront(cacheEntry{key: key, value: val, expires: time.Now().Add(c.ttl)})
	c.entries[key] = elem
	if c.lru.Len() > c.capacity {
		oldest := c.lru.Back()
		if oldest != nil {
			entry := oldest.Value.(cacheEntry)
			c.lru.Remove(oldest)
			delete(c.entries, entry.key)
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
