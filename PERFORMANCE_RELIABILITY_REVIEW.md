# Performance and Reliability Review - DX Cluster Server

**Date:** December 5, 2025  
**Reviewer:** Kiro AI  
**Repository:** gocluster (DX Cluster Server)

## Executive Summary

This review focuses on **observed and measurable** performance/reliability issues in the DX Cluster Server. The codebase demonstrates solid concurrent design with proper safeguards (LRU caches, window-bounded dedup, sharded locks, non-blocking drops with metrics). 

**Key Principle:** Recommendations are based on actual bottlenecks visible in the code (broadcast drops, buffer overflows) and profiling opportunities, not theoretical concerns.

**Overall Assessment:** Production-ready architecture with good defensive patterns. Focus optimization efforts on areas where the code already logs drops/warnings.

---

## Actionable Issues (Priority by Evidence)

### 1. **Broadcast Queue Drops Under Load** ⚠️ HIGH

**Location:** `telnet/server.go:320-340`  
**Evidence:** Code already logs `"Broadcast channel full, dropping spot"` with metrics

**Issue:** The broadcast channel (default 2048) can fill during PSKReporter FT8 decode bursts, causing drops.

**Current Safeguards:**
- Non-blocking send with drop metrics ✓
- Per-worker queues (sharded delivery) ✓
- Per-client buffers with drop tracking ✓

**Recommendation:**
```yaml
# data/config/runtime.yaml tuning based on client count and ingest rate
telnet:
  broadcast_queue_size: 8192  # 4x default for FT8 bursts
  worker_queue_size: 256      # 2x default
  client_buffer_size: 256     # 2x default
```

**Why:** Sizing should be based on observed drops in logs. If you're seeing frequent "Broadcast channel full" messages, increase these values. The architecture already handles backpressure correctly; this is just tuning.

---

### 2. **RBN Slot Buffer Configuration** ⚠️ MEDIUM

**Location:** `rbn/client.go:60-70`, `data/config/ingest.yaml`  
**Evidence:** Code logs `"Spot channel full, dropping spot"` when buffer overflows

**Issue:** The 100-slot fallback is too small for FT8/FT4 decode cycles, but operators may not know to configure `slot_buffer`.

**Current Safeguards:**
- Configurable via `rbn.slot_buffer` and `rbn_digital.slot_buffer` ✓
- Drop logging with capacity info ✓

**Recommendation:**
```yaml
# data/config/ingest.yaml - document recommended values
rbn:
  slot_buffer: 500  # CW/RTTY: lower burst rate
rbn_digital:
  slot_buffer: 2000  # FT8/FT4: synchronized decode bursts
```

**Why:** The architecture is correct (configurable buffers, drop metrics). Just need better defaults and documentation for operators.

---

### 3. **Missing Panic Recovery in Some Goroutines** ⚠️ MEDIUM

**Location:** Various goroutines  
**Evidence:** `processOutputSpots` has recovery, but not all goroutines do

**Audit Needed:**
- `telnet/server.go:handleClient` - ❌ no recovery
- `telnet/server.go:broadcastWorker` - ❌ no recovery  
- `rbn/client.go:readLoop` - ❌ no recovery
- `pskreporter/client.go` workers - needs check
- `main.go:processOutputSpots` - ✓ has recovery

**Recommendation:**
```go
// Pattern to apply to long-running goroutines
func (s *Server) handleClient(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Client handler panic: %v\n%s", r, debug.Stack())
		}
		conn.Close()
	}()
	// ... existing code ...
}
```

**Why:** A panic in a client handler shouldn't crash the server. This is defensive programming for production stability.

---

### 4. **Profiling Opportunities** 📊 PROFILING NEEDED

**Location:** Hot paths that need measurement before optimization

**Areas to Profile:**
1. **Call correction** (`spot/correction.go`) - Complex consensus algorithm, runs on every CW/RTTY spot
2. **String operations** (`rbn/client.go:parseSpot`) - Runs thousands of times/second
3. **Cache lookups** (call normalization, CTY, known calls) - High frequency operations
4. **Frequency averaging** (`spot/frequency_averager.go`) - Global mutex, could be sharded

**Recommendation:**
```go
// Add pprof endpoints to main.go
import _ "net/http/pprof"

go func() {
	log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

Then profile under load:
```bash
# CPU profile during FT8 burst
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Memory profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutine blocking
go tool pprof http://localhost:6060/debug/pprof/block
```

**Why:** Don't optimize without data. Profile first, then optimize the actual bottlenecks

---

## Potential Optimizations (Profile First)

### 5. **String Operations in parseSpot()** 🔍 PROFILE FIRST

**Location:** `rbn/client.go:parseSpot()`  
**Status:** Needs profiling to confirm if this is actually hot

**Observations:**
- Regex is pre-compiled (good) ✓
- Multiple string operations per spot
- Runs thousands of times/second

**Profile First, Then Consider:**
```go
// Only if profiling shows string ops are hot
// Fast path for well-formed lines
if !strings.Contains(line, "  ") {
	parts = strings.Fields(line)  // Skip regex
} else {
	parts = whitespaceRE.Split(line, -1)
}
```

**Why:** The regex is already compiled. Don't optimize without evidence it's a bottleneck.

---

### 6. **Call Normalization Cache Sharding** 🔍 PROFILE FIRST

**Location:** `rbn/client.go:280-290` (rbnNormalizeCache)  
**Status:** Single mutex, but needs profiling to confirm contention

**Current Design:**
- Global cache with single mutex
- Size and TTL configurable
- High hit rate expected (same skimmers repeat)

**If Profiling Shows Contention:**
```go
// Shard similar to deduplicator
type shardedCallCache struct {
	shards [64]struct {
		mu    sync.RWMutex
		cache map[string]string
	}
}

func (c *shardedCallCache) Get(call string) (string, bool) {
	shard := &c.shards[hash(call)%64]
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	val, ok := shard.cache[call]
	return val, ok
}
```

**Why:** Sharding is a proven pattern in this codebase (deduplicator), but only apply if profiling shows lock contention.

---

### 7. **Frequency Averager Sharding** 🔍 PROFILE FIRST

**Location:** `spot/frequency_averager.go:30-60`  
**Status:** Global mutex, but only runs on CW/RTTY (not FT8/FT4)

**Current Design:**
- Single mutex for all callsigns
- Only processes CW/RTTY spots
- Prunes on every call

**If Profiling Shows Contention:**
- Shard by callsign hash (64 shards)
- Or use sync.Map with per-callsign locks

**Why:** CW/RTTY volume is lower than FT8. May not be a bottleneck. Profile first

---

## Observability Gaps

### 8. **Limited Metrics Exposure** 📊 NICE-TO-HAVE

**Current State:**
- Stats ticker prints to console ✓
- Drop metrics logged ✓
- Dashboard shows key metrics ✓

**Missing:**
- Prometheus/metrics endpoint
- Per-source drop rates
- Cache hit rates
- Goroutine count tracking

**Recommendation:**
```go
// Add metrics endpoint
import "github.com/prometheus/client_golang/prometheus/promhttp"

http.Handle("/metrics", promhttp.Handler())
go http.ListenAndServe(":9090", nil)
```

**Why:** Current logging is adequate for operations, but structured metrics would enable better alerting and trending.

---

### 9. **Profiling Endpoints** 📊 NICE-TO-HAVE

**Current State:** No pprof endpoints

**Recommendation:**
```go
import _ "net/http/pprof"

// Add to main.go
go func() {
	log.Println("pprof server on :6060")
	log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

**Why:** Essential for performance investigation. Should be added before any optimization work

---

## What's Already Good ✓

**Defensive Patterns:**
- Deduplicator: 64-shard design, window-bounded cleanup, no leak risk
- Grid cache: Hard LRU cap, evicts immediately when full
- Telnet broadcast: Non-blocking drops, per-worker queues, per-client buffers
- RBN/PSK clients: Configurable buffers, reconnect with exponential backoff
- Ring buffer: Lock-free atomic operations
- Error handling: Drops logged with context and metrics

**Configuration:**
- Buffer sizes tunable via data/config/runtime.yaml
- Cache sizes/TTLs configurable
- Reconnect behavior configurable
- All key parameters exposed

---

## Action Plan (Evidence-Based)

### Immediate (Do Now)
1. **Add pprof endpoints** - Required for any optimization work
2. **Audit panic recovery** - Add to goroutines that lack it (targeted list above)
3. **Document buffer tuning** - Add recommended values to data/config/runtime.yaml comments

### Monitor First (Gather Data)
4. **Watch for drop messages** in logs:
   - "Broadcast channel full" → increase broadcast_queue_size
   - "Spot channel full" → increase slot_buffer
   - "Worker queue full" → increase worker_queue_size
5. **Profile under load** - Run pprof during FT8 bursts to find actual hotspots
6. **Track metrics** - Monitor existing drop counters in stats ticker

### Optimize Only If Needed (Based on Profiling)
7. **If call cache shows contention** → shard it
8. **If string ops show up hot** → optimize parseSpot()
9. **If frequency averager blocks** → shard it

### Nice-to-Have (Not Critical)
10. **Prometheus metrics** - Better than console logging for trending
11. **Structured logging** - Easier to parse/analyze
12. **Load testing** - Validate capacity before production scaling

---

## Key Takeaway

**Don't optimize speculatively.** The architecture is sound with proper safeguards. Focus on:

1. **Tune existing knobs** (buffer sizes) based on observed drops
2. **Add profiling** to find actual bottlenecks
3. **Add panic recovery** where missing (defensive programming)
4. **Monitor metrics** that already exist

The code already handles backpressure correctly (non-blocking drops with metrics). The main operational task is tuning buffer sizes based on your specific load profile (client count, ingest rate, burst patterns).
