## Profiling Guide

This document describes how to profile gocluster for CPU and memory analysis, and how to generate profiles for Profile-Guided Optimization (PGO).

### Purpose

1. **Understand resource usage** - Identify where CPU time and memory allocations are spent
2. **Enable compiler optimization** - Generate profiles that Go's PGO uses to produce faster binaries

### Prerequisites

Before capturing profiles, configure the environment:

```bash
# Environment variables
set DXC_PPROF_ADDR=localhost:6061

# Config settings (data/config/app.yaml)
ui:
  mode: headless    # Eliminates UI overhead from profiles

# Optional: tighten GC for memory analysis
set GOGC=50
```

### Capture Profiles

Run gocluster under steady load for at least 2 minutes before capturing.

**CPU Profile (120 seconds)**
```bash
curl "http://localhost:6061/debug/pprof/profile?seconds=120" -o logs/cpu-$(date +%Y%m%d-%H%M%S).pprof
```

**Heap Snapshot**
```bash
curl "http://localhost:6061/debug/pprof/heap" -o logs/heap-$(date +%Y%m%d-%H%M%S).pprof
```

### Storage Convention

Store profiles in `logs/` with descriptive prefixes:

```
logs/
  baseline-20251228-120000.cpu.pprof    # Before changes
  baseline-20251228-120000.heap.pprof
  sqlite-opt-20251228-140000.cpu.pprof  # After SQLite optimization
  sqlite-opt-20251228-140000.heap.pprof
```

### Analyze Results

**Top CPU consumers (flat time)**
```bash
go tool pprof -top logs/cpu-*.pprof
```

**Top CPU consumers (cumulative/call tree)**
```bash
go tool pprof -cum logs/cpu-*.pprof
```

**Top memory allocators**
```bash
go tool pprof -top logs/heap-*.pprof
```

**Interactive exploration**
```bash
go tool pprof logs/cpu-*.pprof
# Then use: top, top -cum, list <function>, web
```

### Summary Template

After analysis, document findings in plain English:

```
## Profile Summary: {label}
Date: {YYYY-MM-DD}
Duration: {seconds}s
Load: {spots/min}

### Top 5 CPU Consumers
1. {function} - {%} flat - {what it does}
2. ...

### Top 3 Memory Allocators
1. {function} - {MB} - {why it allocates}

### Assessment
{One paragraph: where is time spent, is it expected, what could improve}
```

### Profile-Guided Optimization (PGO)

Go 1.21+ uses CPU profiles to optimize the generated binary.

**Merge multiple profiles for representative coverage**
```bash
go tool pprof -proto logs/*.cpu.pprof > default.pgo
```

**Build with PGO**
```bash
go build -pgo=default.pgo
```

Or simply place `default.pgo` in the main package directory - `go build` detects it automatically.

**Expected improvement**: 2-7% CPU reduction through better inlining and devirtualization decisions.

### Known Hotspots

These areas typically dominate profiles under high PSKReporter load:

| Component | Why | Mitigation |
|-----------|-----|------------|
| `memeqbody` | String comparisons in maps/dedup | Pre-normalize strings, use integer keys |
| `runtime.cgocall` | SQLite via modernc (pure Go) | `synchronous=OFF`, batch writes |
| `cty.lookupCallsignNoCache` | CTY prefix matching | Bounded TTL cache |
| `pskreporter.*` | JSON parsing, spot conversion | jsoniter ConfigFastest |
| `gridstore.Get` | Grid lookups on cache miss | Increase cache size, disable async backfill |

### Quick Reference

| Task | Command |
|------|---------|
| Capture CPU | `curl "http://localhost:6061/debug/pprof/profile?seconds=120" -o logs/cpu.pprof` |
| Capture heap | `curl "http://localhost:6061/debug/pprof/heap" -o logs/heap.pprof` |
| View top CPU | `go tool pprof -top logs/cpu.pprof` |
| View cumulative | `go tool pprof -cum logs/cpu.pprof` |
| Generate PGO | `go tool pprof -proto logs/*.cpu.pprof > default.pgo` |
| Build with PGO | `go build -pgo=default.pgo` |
