# DOMAIN-CONTRACTS.md — Telnet/Packet Cluster Domain Rules

## Protocol Scope
- Line-oriented TCP protocol. Accept `\n` and `\r\n`.
- Telnet IAC policy: default is **no negotiation**. If IAC (0xFF) observed, close with reason “telnet negotiation unsupported” unless a specific minimal-support requirement is explicitly planned.
- Control characters outside policy must be rejected deterministically.

## Input Parsing
- Max line length: 1024B. Max token: 64B. Close on repeated abuse.
- Streaming parse with bounded per-conn buffer and incremental newline scan.
- Do not retain subslices of shared read buffers without copying.

## Output Buffering
- Per-conn bounded queues; single writer goroutine per conn.
- Coalesce writes; set write deadlines; disconnect on sustained stall.
- Control queue full ⇒ disconnect. Spots queue full ⇒ drop incoming spot.

## Fan-out + Backpressure
- Ingest must never block on per-client I/O.
- Broadcast pushes to per-conn queues; no goroutine-per-message.
- Overload: shed slow conns first, then global if system-wide.
- Preserve per-conn ordering within each stream; control priority must not starve.

## Drop/Disconnect Policy
- Spots queue full: drop incoming spot (no eviction).
- Control queue full: disconnect immediately.
- Sustained drops: if spot drop rate >5% over 30s (min sample threshold), treat as slow consumer.
  - strict: disconnect fast.
  - lenient: drop aggressively and keep connection.

## p99 Targets (nominal load)
- Ingest → enqueue: ≤5ms p99
- Ingest → first byte out: ≤25ms p99 (healthy clients)
- Overload: memory bounded, shedding increases, no GC thrash.

## Operational Readiness
- Bounded resources: per-conn/global queues, goroutines, buffers.
- No leak classes: blocked goroutines, orphan timers, unbounded retries/logs.
- Observability required: drops, disconnect reasons, queue depths, stalls, RSS, alloc rate, latency histograms.

## Security/Robustness
- Strict bounds; close on abuse.
- Rate-limit commands per-conn with bounded state.
- Truncate/rate-limit untrusted input in logs.
- Graceful shutdown: stop accept → cancel → stop writers → bounded drain → close.

## System Impact Checklist (for non-trivial changes)
- Upstream inputs, downstream outputs, shared deps, backpressure bounds, concurrency lifecycle, failure/shutdown, contracts/observability.
