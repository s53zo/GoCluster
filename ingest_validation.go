package main

import (
	"container/list"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dxcluster/cty"
	"dxcluster/spot"
	"dxcluster/uls"
)

const (
	defaultIngestCTYLogInterval  = 30 * time.Second
	defaultIngestDropLogInterval = 30 * time.Second
)

// ingestValidator centralizes CTY/ULS validation before deduplication.
// It is intentionally single-consumer to keep CTY cache usage bounded and predictable.
type ingestValidator struct {
	input              chan *spot.Spot
	dedupInput         chan<- *spot.Spot
	ctyLookup          func() *cty.CTYDatabase
	unlicensedReporter func(source, role, call, mode string, freq float64)
	isLicensedUS       func(call string) bool
	ctyCache           *ctyInfoCache
	ctyDropDXCounter   rateCounter
	ctyDropDECounter   rateCounter
	dedupDropCounter   rateCounter
}

// newIngestValidator wires a bounded ingest gate for CTY/ULS checks.
// It relies on CTY DB caching plus an optional TTL LRU for hotspot calls.
func newIngestValidator(
	ctyLookup func() *cty.CTYDatabase,
	dedupInput chan<- *spot.Spot,
	unlicensedReporter func(source, role, call, mode string, freq float64),
	cacheSize int,
	cacheTTL time.Duration,
) *ingestValidator {
	inputBuffer := cap(dedupInput)
	if inputBuffer <= 0 {
		inputBuffer = 10000
	}
	return &ingestValidator{
		input:              make(chan *spot.Spot, inputBuffer),
		dedupInput:         dedupInput,
		ctyLookup:          ctyLookup,
		unlicensedReporter: unlicensedReporter,
		isLicensedUS:       uls.IsLicensedUS,
		ctyCache:           newCTYInfoCache(cacheSize, cacheTTL),
		ctyDropDXCounter:   newRateCounter(defaultIngestCTYLogInterval),
		ctyDropDECounter:   newRateCounter(defaultIngestCTYLogInterval),
		dedupDropCounter:   newRateCounter(defaultIngestDropLogInterval),
	}
}

// Input returns the channel ingest sources should send spots into.
func (v *ingestValidator) Input() chan<- *spot.Spot {
	if v == nil {
		return nil
	}
	return v.input
}

// Start launches the validator loop.
func (v *ingestValidator) Start() {
	if v == nil {
		return
	}
	go v.run()
}

func (v *ingestValidator) run() {
	for s := range v.input {
		if s == nil {
			continue
		}
		if !v.validateSpot(s) {
			continue
		}
		select {
		case v.dedupInput <- s:
		default:
			if count, ok := v.dedupDropCounter.Inc(); ok {
				log.Printf("Ingest: dedup input full, dropping spot (source=%s total=%d)", ingestSourceLabel(s), count)
			}
		}
	}
}

// validateSpot enforces CTY validity and DE licensing before dedup.
// It refreshes metadata from CTY while preserving any grid fields already attached.
func (v *ingestValidator) validateSpot(s *spot.Spot) bool {
	if s == nil {
		return false
	}
	s.EnsureNormalized()
	if v.ctyLookup == nil {
		return true
	}
	ctyDB := v.ctyLookup()
	if ctyDB == nil {
		return true
	}

	dxCall := s.DXCallNorm
	if dxCall == "" {
		dxCall = s.DXCall
	}
	deCall := s.DECallNorm
	if deCall == "" {
		deCall = s.DECall
	}

	now := time.Now()
	dxInfo, ok := v.lookupCTY(ctyDB, dxCall, now)
	if !ok {
		v.logCTYDrop("DX", dxCall, s)
		return false
	}
	deInfo, ok := v.lookupCTY(ctyDB, deCall, now)
	if !ok {
		v.logCTYDrop("DE", deCall, s)
		return false
	}

	dxGrid := strings.TrimSpace(s.DXMetadata.Grid)
	deGrid := strings.TrimSpace(s.DEMetadata.Grid)
	s.DXMetadata = metadataFromPrefix(dxInfo)
	s.DEMetadata = metadataFromPrefix(deInfo)
	if dxGrid != "" {
		s.DXMetadata.Grid = dxGrid
	}
	if deGrid != "" {
		s.DEMetadata.Grid = deGrid
	}

	if v.isLicensedUS != nil {
		deLicenseCall := strings.TrimSpace(uls.NormalizeForLicense(deCall))
		if deLicenseCall != "" {
			if info, ok := ctyDB.LookupCallsign(deLicenseCall); ok && info.ADIF == 291 {
				callKey := deLicenseCall
				if callKey == "" {
					callKey = deCall
				}
				if licensed, ok := licCache.get(callKey, now); ok {
					if !licensed {
						if v.unlicensedReporter != nil {
							v.unlicensedReporter(ingestSourceLabel(s), "DE", callKey, s.ModeNorm, s.Frequency)
						}
						return false
					}
				} else if !v.isLicensedUS(callKey) {
					licCache.set(callKey, false, now)
					if v.unlicensedReporter != nil {
						v.unlicensedReporter(ingestSourceLabel(s), "DE", callKey, s.ModeNorm, s.Frequency)
					}
					return false
				} else {
					licCache.set(callKey, true, now)
				}
			}
		}
	}

	return true
}

func (v *ingestValidator) lookupCTY(db *cty.CTYDatabase, call string, now time.Time) (*cty.PrefixInfo, bool) {
	if db == nil || call == "" {
		return nil, false
	}
	if v.ctyCache != nil {
		if info, ok := v.ctyCache.get(call, now); ok {
			return info, info != nil
		}
	}
	info, ok := db.LookupCallsignPortable(call)
	if v.ctyCache != nil {
		v.ctyCache.set(call, info, ok, now)
	}
	return info, ok
}

func (v *ingestValidator) logCTYDrop(role, call string, s *spot.Spot) {
	counter := &v.ctyDropDXCounter
	if role == "DE" {
		counter = &v.ctyDropDECounter
	}
	if count, ok := counter.Inc(); ok {
		log.Printf("CTY drop: unknown %s %s at %.1f kHz (source=%s total=%d)", role, call, s.Frequency, ingestSourceLabel(s), count)
	}
}

func ingestSourceLabel(s *spot.Spot) string {
	if s == nil {
		return "unknown"
	}
	label := strings.TrimSpace(s.SourceNode)
	if label != "" {
		return label
	}
	if s.SourceType != "" {
		return string(s.SourceType)
	}
	return "unknown"
}

// rateCounter throttles log emission while tracking totals.
type rateCounter struct {
	interval time.Duration
	last     atomic.Int64
	total    atomic.Uint64
}

func newRateCounter(interval time.Duration) rateCounter {
	return rateCounter{interval: interval}
}

// Inc increments the counter and returns (total, shouldLog).
func (c *rateCounter) Inc() (uint64, bool) {
	if c == nil {
		return 0, false
	}
	total := c.total.Add(1)
	if c.interval <= 0 {
		return total, true
	}
	now := time.Now().UnixNano()
	for {
		last := c.last.Load()
		if now-last < c.interval.Nanoseconds() {
			return total, false
		}
		if c.last.CompareAndSwap(last, now) {
			return total, true
		}
	}
}

// ctyInfoCache is a bounded TTL LRU for CTY lookups (hits and misses).
// It is safe for concurrent use.
type ctyInfoCache struct {
	shards []ctyInfoCacheShard
}

type ctyInfoCacheShard struct {
	mu      sync.Mutex
	max     int
	ttl     time.Duration
	order   *list.List
	entries map[string]*list.Element
}

type ctyInfoCacheEntry struct {
	key  string
	info *cty.PrefixInfo
	ok   bool
	when time.Time
}

const ctyCacheShardCount = 16

func newCTYInfoCache(maxEntries int, ttl time.Duration) *ctyInfoCache {
	if maxEntries <= 0 {
		return nil
	}
	if ttl < 0 {
		ttl = 0
	}
	shardCount := ctyCacheShardCount
	if maxEntries < shardCount {
		shardCount = maxEntries
	}
	if shardCount <= 0 {
		return nil
	}
	perShard := maxEntries / shardCount
	if perShard <= 0 {
		perShard = 1
	}
	shards := make([]ctyInfoCacheShard, shardCount)
	for i := range shards {
		shards[i] = ctyInfoCacheShard{
			max:     perShard,
			ttl:     ttl,
			order:   list.New(),
			entries: make(map[string]*list.Element, perShard),
		}
	}
	return &ctyInfoCache{shards: shards}
}

func (c *ctyInfoCache) get(key string, now time.Time) (*cty.PrefixInfo, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	shard := c.shardFor(key)
	return shard.get(key, now)
}

func (c *ctyInfoCache) set(key string, info *cty.PrefixInfo, ok bool, now time.Time) {
	if c == nil || key == "" {
		return
	}
	shard := c.shardFor(key)
	shard.set(key, info, ok, now)
}

func (c *ctyInfoCache) shardFor(key string) *ctyInfoCacheShard {
	hash := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= 16777619
	}
	return &c.shards[hash%uint32(len(c.shards))]
}

func (s *ctyInfoCacheShard) get(key string, now time.Time) (*cty.PrefixInfo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	elem, ok := s.entries[key]
	if !ok {
		return nil, false
	}
	entry := elem.Value.(ctyInfoCacheEntry)
	if s.ttl > 0 && now.Sub(entry.when) > s.ttl {
		delete(s.entries, key)
		s.order.Remove(elem)
		return nil, false
	}
	s.order.MoveToFront(elem)
	if !entry.ok {
		return nil, true
	}
	return entry.info, true
}

func (s *ctyInfoCacheShard) set(key string, info *cty.PrefixInfo, ok bool, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if elem, exists := s.entries[key]; exists {
		entry := elem.Value.(ctyInfoCacheEntry)
		entry.info = info
		entry.ok = ok
		entry.when = now
		elem.Value = entry
		s.order.MoveToFront(elem)
		return
	}
	entry := ctyInfoCacheEntry{key: key, info: info, ok: ok, when: now}
	elem := s.order.PushFront(entry)
	s.entries[key] = elem
	if s.max > 0 && len(s.entries) > s.max {
		if oldest := s.order.Back(); oldest != nil {
			evicted := oldest.Value.(ctyInfoCacheEntry)
			delete(s.entries, evicted.key)
			s.order.Remove(oldest)
		}
	}
}
