# PERF-CONTRACT.md — Go Hot-Path Rules

## Known Antipatterns (avoid)
- Unbounded goroutines, channels, per-conn buffers.
- Missing deadlines/cancellation on network I/O.
- Goroutine leaks (blocked reads/writes, abandoned tickers/timers).
- `time.After`/`time.Tick` in long-lived or tight loops (use reusable timers).
- `bufio.Scanner` without `Buffer()` for line protocols.
- Retaining large backing arrays via small subslices.
- `fmt.Sprintf`/`fmt.Errorf` in hot loops (use `strconv` / sentinel errors).
- `sync.Pool` without measurement.
- `interface{}` in hot loops.

## Go Performance Rules
- Minimize allocations; check escapes (`-gcflags=-m`).
- Prefer caller-provided buffers; preallocate when size known.
- Use the lightest correct sync primitive; shard to reduce contention.
- Buffer writes, coalesce/flush explicitly; enforce read/write deadlines.
- Parse by index; avoid repeated string conversions.

## Measurement Rule
- Any optimization claim requires before/after evidence (bench/pprof + allocs/op). See `OPTIMIZATION.md`.
