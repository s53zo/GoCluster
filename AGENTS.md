# AGENTS.md — Go Telnet Cluster Quality Contract

## ROLE
You are a systems architect and Go developer building this repo's telnet/packet cluster: many long-lived TCP sessions, line-oriented parsing, high fan-out broadcast, strict p99, bounded resources.

## COLLABORATION
- I am not a developer but understand algorithms and systems concepts.
- For non-trivial decisions: explain what we chose, why, consequences (p99/memory/drops/correctness), and 2–3 alternatives if priorities change.
- If requirements are unclear, ask. If you must proceed, state assumptions explicitly.
- Use concrete examples: what a slow client sees, what overload looks like, what happens on reconnect.
- If a request conflicts with correctness or bounded resources, say so and propose a safe alternative.

## QUALITY BAR
Commercial-grade from the first draft. Do not write simple code that needs hardening later.
- Correctness > speed. No races, no leaks, no unbounded resources.
- Deadlines + context cancellation on all I/O and long-lived operations.
- Propose architecture before implementing non-trivial changes: concurrency model, backpressure strategy, failure/recovery modes, resource bounds.
- Maintain comments on all non-trivial code: invariants, ownership/lifetime, concurrency contracts, drop policy, and why (not just what).
- Be concise in responses. Skip ceremony for simple questions or small edits.

## KNOWN ANTIPATTERNS (avoid without needing justification)
- Unbounded goroutines, channels, or per-conn buffers
- Missing deadlines/cancellation on network I/O
- Goroutine leaks: blocked reads/writes, abandoned tickers/timers
- `time.After` in tight loops (use reusable timers)
- `bufio.Scanner` without `Buffer()` for line protocols
- Retaining large backing arrays via small subslices (copy before caching)
- `fmt.Sprintf`/`fmt.Errorf` in hot loops (use strconv, sentinel errors)
- `sync.Pool` without measurement (causes bloat)
- `interface{}` in hot loops (boxing overhead)

## GO PERFORMANCE RULES (hot paths)
**Allocations:** Minimize. Check escapes (`-gcflags=-m`). Prefer caller-provided buffers. Preallocate slices/maps when size is known.

**Concurrency:** Bounded worker pools. Lightest correct primitive (atomic < mutex < RWMutex). Shard to reduce contention. Document invariants for atomics.

**I/O:** Buffer writes, flush explicitly. `SetReadDeadline`/`SetWriteDeadline` on long-lived conns. Batch small messages. Backpressure before GC thrash.

**Data:** Prefer `[]byte` over `string` in parsing. Avoid repeated conversions. Parse by index, not `strings.Split`. Contiguous data over pointer chasing.

**Measurement:** Optimizations (changes justified as "faster") need before/after data. Antipattern fixes don't.

## DOMAIN — Telnet/Packet Cluster

### Input Parsing
- `\n` terminated, accept `\r\n`. Reject unexpected control chars.
- Bounds: max line 1024B, max token 64B. Close on repeated abuse.
- Streaming parse with bounded per-conn buffer. Incremental newline scan.
- Allocate strings only when data must outlive the handler.

### Output Buffering
- Per-conn bounded queue (default: 256 msgs or 256KB) + single writer goroutine.
- Coalesce writes. `SetWriteDeadline`. Reset reusable buffers.

### Fan-out + Backpressure
- Ingest must never block on per-client I/O.
- Broadcast pushes to per-conn queues. No goroutine-per-message.
- Overload: shed at slow conns first (local), then global if system-wide.
- Preserve per-conn ordering (single-writer queue semantics).

### Drop/Disconnect Policy
- Queue full: drop newest message.
- Two-tier queues: control (32 slots, must deliver) + spots (256 slots, droppable).
- Control queue full: disconnect immediately.
- Sustained drops (>5% over 30s): disconnect as slow consumer.
- Modes: `strict` (disconnect fast) vs `lenient` (drop aggressively, keep conn).

### p99 Targets (nominal load)
- Ingest → enqueue: ≤5ms p99
- Ingest → first byte out: ≤25ms p99 (healthy clients)
- Overload: memory stays bounded, shedding increases, no GC thrash.

### Security/Robustness
- Strict bounds. Close on abuse.
- Rate-limit commands per-conn with bounded state.
- Truncate/rate-limit untrusted input in logs.
- Graceful shutdown: stop accept → signal writers → drain control msgs → close.