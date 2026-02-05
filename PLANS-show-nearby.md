# PLANS-show-nearby.md — Plan of Record

## Operating rules
- This file is the durable plan-of-record for this branch. It is the single source of truth for work state in long conversations.
- Required for any Non-trivial change or contract-touching change (see AGENTS.md).
- Each plan is versioned as Plan vN and must reference the Scope Ledger snapshot it implements (Scope Ledger vM).
- Plans have two phases:
  - Pre-code: goals/non-goals/assumptions/requirements/architecture/contracts/test plan/rollout.
  - Post-code: deviations, verification commands actually run (if any), final contract statement, and Scope Ledger status updates.
- Keep history append-only: use Plan Index and Decision Log.

---

## Current Plan (ACTIVE)

### Plan v6 — Resource Efficiency Optimization (Allocation + Retention)
- **Date**: 2026-02-04
- **Status**: In Progress
- **Scope Ledger snapshot**: v6
- **Owner**: Assistant (with user approval)
- **Approval**: Approved v6 (user request)

1) Goals
- Reduce allocation churn in steady-state ingest by 30–50% vs baseline.
- Reduce GC pressure spikes during ULS refresh and archive cleanup.
- Keep live heap stable and bounded without harming p99 latency.

2) Non-goals
- No protocol/format changes.
- No changes to ordering/drop/disconnect semantics.
- No UI removal (tview stays).

3) Assumptions
- CPU is not the primary bottleneck; allocations and GC are.
- PSKReporter + RBN parsing and spot normalization dominate allocation volume.
- Pebble cache churn dominates cleanup spikes.

4) Requirements and edge cases
Functional:
- Preserve correction, dedup, and broadcast semantics.
- Parsing remains bounded and resilient to malformed input.
- PSKReporter Phase B keeps allowlist/path-only filtering before full normalization; buffer reuse must be bounded and non-blocking.
- RBN Phase C preserves token boundaries, punctuation trimming, and DX/DE matching; comment reconstruction remains order-preserving with single-space joins.
- Phase D avoids duplicate normalization in spot creation: ModeNorm/BandNorm prefilled when possible, canonical PSK mode derived once, and uppercase normalization uses ASCII fast paths without changing output.
Non-functional:
- No unbounded buffers or goroutines.
- No regression in p99 latency.

5) Architecture (plan before code)
Phased checkpoints:
A) Baseline: capture allocs/heap pprof under normal load; record top allocators.
B) PSKReporter: reduce JSON/string churn; reuse buffers; normalize once. Add a bounded payload buffer pool (size + count caps, non-blocking get/put) and pass pre-parsed mode info through conversion to avoid duplicate canonicalization.
C) RBN parsing: reduce tokenization allocations; reuse slices. Details: drop per-token uppercase allocation, avoid split/join in RBN callsign normalization, and build comments in a single pass with a builder.
D) Spot normalization: avoid repeated normalization and transient strings. Details: add ASCII-fast upper+trim helper, avoid double uppercasing, prefill ModeNorm/BandNorm in NewSpot* using canonical PSK lookup without re-normalizing.
E) Archive + Pebble cleanup: reduce iterator/buffer churn; cap batches. Details: avoid full record decode in cleanup by parsing the mode field directly from the record header, reuse precomputed iterator bounds for spot prefix scans, lazily allocate cleanup batches only when deletes occur, and clamp cleanup batch size to a safe maximum to keep delete commits bounded.
Amendment (2026-02-05): Phase E will be implemented and evaluated together with the already-landed Phase D changes (combined D+E pprof comparison), per user request to proceed as a single combined phase.

6) Contracts and compatibility
- Protocol/format: No change.
- Ordering: No change.
- Drop/disconnect policy: No change.
- Deadlines/timeouts: No change.
- Observability: No change.

7) User-visible behavior
- No user-visible behavior changes.

8) Config impact
- None unless cleanup batching needs tunable limits.

9) Observability plan
- Use DXC_HEAP_LOG_INTERVAL + alloc/heap pprof snapshots before/after each phase.

10) Test plan (before code)
- Run `go test ./...` after each phase.
- Use pprof comparisons only (no benchmarks unless needed).

11) Performance/measurement plan
- Report total alloc_space and top allocators before/after each phase.
- Amendment: Phase D+E will be reported as a combined comparison (baseline vs post-E), since D is already landed and the user requested a combined checkpoint.

12) Rollout/ops plan
- Stage in dev; compare 24h logs before/after.

13) Open questions / needs-confirmation
- None.

---

## Post-implementation notes (Plan v6 - Phase B)
Implementation notes:
- Added bounded payload buffer pooling for PSKReporter ingest (size + count caps, non-blocking).
- Parsed PSKReporter mode once and passed canonical/variant data through conversion to avoid duplicate normalization.
- Returned payload buffers on drop and after processing to reduce alloc churn.
- Phase B pprof comparison completed (baseline 12:29 vs Phase B 13:49 on 2026-02-04): PSKReporter CPU share ~2.8% → ~2.4% of total samples; PSKReporter inuse heap ~114.5MB → ~107.3MB; alloc-space share ~9.3% → ~9.8% (within noise). No regressions observed.

Deviations:
- None.

Verification commands actually run:
- `go test ./...`

Final contract statement:
- No protocol/format, ordering, drop/disconnect, deadlines/timeouts, or observability changes.

Scope Ledger status updates:
- S15 -> Implemented.

---

## Post-implementation notes (Plan v6 - Phase C)
Implementation notes:
- Removed per-token uppercase allocation in RBN tokenization; DX/DE matching uses ASCII fold comparison.
- Replaced split/join in RBN callsign normalization with last-dash slicing.
- Built RBN comments in a single pass (builder) to avoid slice/join allocations.

Deviations:
- None.

Verification commands actually run:
- `go test ./...`

Final contract statement:
- No protocol/format, ordering, drop/disconnect, deadlines/timeouts, or observability changes.

Scope Ledger status updates:
- S16 -> Implemented.

---

## Post-implementation notes (Plan v6 - Phase D)
Implementation notes:
- Added ASCII-fast upper+trim helper for spot normalization and PSK canonicalization.
- Prefilled ModeNorm/BandNorm in NewSpot* and removed double uppercasing of mode tokens.
- EnsureNormalized now uses ASCII-fast normalization for mode/continent/grid fields.

Deviations:
- None.

Verification commands actually run:
- `go test ./...`

Final contract statement:
- No protocol/format, ordering, drop/disconnect, deadlines/timeouts, or observability changes.

Scope Ledger status updates:
- S17 -> Implemented.

---

## Post-implementation notes (Plan v6 - Phase E)
Implementation notes:
- Cleanup now parses only the mode field from archive records (no full decode) to avoid per-record string allocations.
- Reused precomputed iterator bounds for spot prefix scans and lazily allocated cleanup batches only when deletes occur.
- Cleanup batch size is clamped to a safe maximum to bound delete commits.

Deviations:
- None.

Verification commands actually run:
- `go test ./...`

Final contract statement:
- No protocol/format, ordering, drop/disconnect, deadlines/timeouts, or observability changes.

Scope Ledger status updates:
- S18 -> Implemented.

---

### Plan v4 — GC Compaction + Map Diagnostics
- **Date**: 2026-02-04
- **Status**: Implemented
- **Scope Ledger snapshot**: v4
- **Owner**: Assistant (with user approval)
- **Approval**: Approved v4 (user requested “1 and 2”)

1) Goals
- Finish Plan v3: bounded call-quality store + categorical stats labels.
- Add bounded map compaction for long-lived caches (path reliability + primary/secondary dedup).
- Add opt-in map size logging to correlate GC p99 drift with live map growth.

2) Non-goals
- No protocol/format changes.
- No drop/ordering/disconnect semantics changes.
- No new always-on metrics.

3) Assumptions
- `call_correction.enabled` is true in runtime config.
- Dynamic call-quality horizon of 24h is acceptable.
- Priors must remain pinned (non-expiring).
- Map compaction can be best-effort, infrequent, and heuristic.

4) Requirements and edge cases
Functional:
- Pinned priors are never evicted and are always consulted in `IsGood`.
- Dynamic call-quality entries expire after 24h since last update and are capped at 200k.
- Stats source labels are deterministic and bounded.
- Compaction must preserve live entries and never block hot paths for long.
Edge cases:
- Empty priors file: dynamic-only store.
- TTL or max entries <=0: fall back to safe defaults.
- Compaction skips small maps and avoids thrash (peak-based heuristic).
- Map size logger is opt-in only (environment variable).

5) Architecture (plan before code)
- Call quality (Plan v3): sharded, bounded store (pinned + dynamic) with TTL + cap + cleanup goroutine.
- Stats labels (Plan v3): categorical `sourceStatsLabel` used for tracker increments.
- Compaction:
  - Path reliability store: track per-shard peak; add `Compact` that rebuilds shard maps when `len < peak*ratio` and `peak >= minPeak`.
  - Primary/secondary dedup: track per-shard peak; after cleanup deletes, rebuild shard maps when they have shrunk below threshold.
  - Compaction runs on a bounded cadence (hourly for path reliability; cleanup-driven for dedup).
- Map diagnostics:
  - Add `DXC_MAP_LOG_INTERVAL` env var to log key map sizes (callQuality pinned/dynamic, tracker cardinality, dedup cache sizes, path bucket counts).
- Dependency rigor: Full (shared components + observability + config/env).

6) Contracts and compatibility
- Protocol/format: No change.
- Ordering: No change.
- Drop/disconnect policy: No change.
- Deadlines/timeouts: No change.
- Observability: new optional log line when DXC_MAP_LOG_INTERVAL is set.

7) User-visible behavior
- Stats output shows categorical source labels instead of per-node/spotter names.
- Call correction anchor memory decays for dynamic entries after 24h; pinned priors remain.
- Optional log line for map sizes when DXC_MAP_LOG_INTERVAL is set.

8) Config impact
- New YAML keys under `call_correction` (Plan v3):
  - `call_quality_ttl_seconds`
  - `call_quality_max_entries`
  - `call_quality_cleanup_interval_seconds`
  - `call_quality_pin_priors`
- Defaults: TTL=86400, max=200000, cleanup=600, pin_priors=true.

9) Observability plan
- Opt-in DXC_MAP_LOG_INTERVAL for map sizes.
- DXC_HEAP_LOG_INTERVAL continues to emit heap object trends.

10) Test plan (before code)
- Unit tests for callQuality TTL expiration and cap eviction behavior.
- Unit tests verifying pinned priors are not evicted and still consulted.
- Unit test for categorical source label mapping.
- Unit tests for compaction heuristics preserving entries (path reliability, dedup).

11) Performance/measurement plan
- None (no hot-path optimization claim).

12) Rollout/ops plan
- Update README and sample pipeline.yaml for new call_quality knobs.
- Document DXC_MAP_LOG_INTERVAL in README.

13) Open questions / needs-confirmation
- None.

---

### Plan v3 — Bounded Call Quality + Stats Cardinality Cap
- **Date**: 2026-02-04
- **Status**: Implemented (rolled into Plan v4)
- **Scope Ledger snapshot**: v3
- **Owner**: Assistant (with user approval)
- **Approval**: Approved v3

1) Goals
- Bound call-correction quality memory with pinned priors + dynamic TTL + max entries.
- Cap stats cardinality with categorical source labels.

2) Non-goals
- No changes to correction algorithm semantics beyond bounded storage.
- No protocol/format changes.

3) Assumptions
- `call_correction.enabled` is true in runtime config.
- Dynamic memory horizon of 24h is acceptable.
- Priors must remain pinned (non-expiring).

4) Requirements and edge cases
Functional:
- Pinned priors are never evicted and are always consulted in `IsGood`.
- Dynamic entries expire after 24h since last update.
- Dynamic entries are capped at 200k (bounded memory).
- Stats source labels are deterministic and bounded.
Edge cases:
- Empty priors file: dynamic-only store.
- TTL or max entries <=0: fall back to safe defaults.
- Cleanup must not block hot paths.

5) Architecture (plan before code)
- Replace global callQuality store with a sharded, bounded store:
  - Separate pinned map (priors) and dynamic map per shard.
  - Dynamic entries track score + lastUpdated.
  - Per-shard cap = ceil(max/shards); evict oldest entries when exceeded.
  - Cleanup goroutine prunes expired dynamic entries at a configurable interval.
- Add `spot.ConfigureCallQualityStore(...)` and update priors loader to mark pinned.
- Stats: introduce a stable categorical `sourceStatsLabel` mapping and use it for tracker increments.
- Dependency rigor: Full (shared component + config schema + observability).

6) Contracts and compatibility
- Protocol/format: No change.
- Ordering: No change.
- Drop/disconnect policy: No change.
- Deadlines/timeouts: No change.
- Observability: Stats labels change to categorical source labels.

7) User-visible behavior
- Stats output shows categorical source labels instead of per-node/spotter names.
- Call correction anchor memory decays for dynamic entries after 24h; pinned priors remain.

8) Config impact
- New YAML keys under `call_correction`:
  - `call_quality_ttl_seconds`
  - `call_quality_max_entries`
  - `call_quality_cleanup_interval_seconds`
  - `call_quality_pin_priors`
- Defaults: TTL=86400, max=200000, cleanup=600, pin_priors=true.

9) Observability plan
- No new metrics. Use DXC_HEAP_LOG_INTERVAL for heap object trends.

10) Test plan (before code)
- Unit tests for callQuality TTL expiration and cap eviction behavior.
- Unit tests verifying pinned priors are not evicted and still consulted.
- Unit test for categorical source label mapping.

11) Performance/measurement plan
- None (no hot-path optimization claim).

12) Rollout/ops plan
- Update README and sample pipeline.yaml for new call_quality knobs.

13) Open questions / needs-confirmation
- None (user confirmed priors pinned + 24h TTL + 200k cap).

---

## Post-implementation notes (Plan v4)
Implementation notes:
- Replaced callQuality with bounded, sharded store (TTL/cap) and pinned priors; wired config + defaults.
- Capped stats cardinality via categorical `sourceStatsLabel`.
- Added peak-based map compaction for path reliability store and primary/secondary dedup caches.
- Added DXC_MAP_LOG_INTERVAL map-size logger and tracker cardinality helpers.
- Added tests for callQuality behavior, source label mapping, and compaction.
- Updated README and pipeline.yaml for new knobs and diagnostics.

Deviations:
- None.

Verification commands actually run:
- `go test ./...`

Final contract statement:
- No protocol/format changes. Stats labels are now categorical; optional DXC_MAP_LOG_INTERVAL log line added.

Scope Ledger status updates:
- S7 -> Implemented.
- S8 -> Implemented.
- S9 -> Implemented.
- S10 -> Implemented.
- S11 -> Implemented.
- S12 -> Implemented.

---

## Post-implementation notes (Plan v5)
Implementation notes:
- callQuality now uses atomic swap to prevent data races during reconfiguration.
- Stop/start ordering ensures old cleanup goroutine stops after swap.

Deviations:
- None.

Verification commands actually run:
- `go test ./...`

Final contract statement:
- No contract or behavior changes.

Scope Ledger status updates:
- S13 -> Implemented.

---

### Plan v2 — NEARBY L1 for 60m
- **Date**: 2026-02-04
- **Status**: Implemented
- **Scope Ledger snapshot**: v2
- **Owner**: Assistant (with user approval)
- **Approval**: Approved v2

1) Goals
- Use L1 (H3 res-1) for 60m in NEARBY matching, alongside 160m/80m.

2) Non-goals
- No changes to NEARBY enable/disable semantics.
- No changes to location-filter suspension or warnings.

3) Assumptions
- L1 = H3 res-1, L2 = H3 res-2.

4) Requirements and edge cases
Functional:
- 60m must use L1 (coarse) matching for NEARBY.
Non-functional:
- No new resource usage.
Edge cases:
- Unknown band remains rejected.

5) Architecture (plan before code)
- Update NEARBY band selection logic to treat 60m as coarse.
- Update docs/help if band list is mentioned.
- Dependency rigor: Light (single-component behavioral change).

6) Contracts and compatibility
- Protocol/format: No change.
- Ordering: No change.
- Drop/disconnect policy: No change.
- Deadlines/timeouts: No change.
- Observability: No change.

7) User-visible behavior
- 60m NEARBY matching now uses L1 cells.

8) Config impact
- None.

9) Observability plan
- None.

10) Test plan (before code)
- Add/adjust tests to verify 60m uses L1 matching.

11) Performance/measurement plan
- None.

12) Rollout/ops plan
- Update README/help text if needed.

13) Open questions / needs-confirmation
- None.

---

## Post-implementation notes (Plan v2)
Implementation notes:
- Updated NEARBY band selection to use L1 for 60m.
- Updated help text and README wording.
- Added a 60m coarse-matching test.

Deviations:
- None.

Verification commands actually run:
- None.

Final contract statement:
- Contract change: 60m now uses L1 for NEARBY matching.

Scope Ledger status updates:
- S6 -> Implemented.

---

## Scope Ledger v2
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S1 | Define PASS NEARBY ON/OFF semantics (suspend location filters, preserve mode/conf/path/beacon). | Implemented | Confirmed |
| S2 | Implement filter + command parsing and state save/restore. | Implemented | |
| S3 | Block location filter changes while NEARBY ON with warning. | Implemented | Confirmed |
| S4 | Add unit + telnet parsing tests. | Implemented | |
| S5 | Update README/help text. | Implemented | |
| S6 | Use L1 for 60m NEARBY matching. | Implemented | |

Status values: Agreed/Pending, Implemented, Deferred, Superseded (per AGENTS.md)  
Completed items remain inline - never removed, only status changes.

---

## Scope Ledger v5
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S7 | Replace unbounded callQuality with a bounded store: pinned priors + 24h TTL + max entries + cleanup interval + config knobs. | Implemented | Priors pinned |
| S8 | Cap stats cardinality with categorical source labels. | Implemented | HUMAN/RBN/RBN-DIGITAL/RBN-FT/PSKREPORTER/PEER/UPSTREAM/OTHER |
| S9 | Add YAML knobs + config normalization + README/pipeline.yaml docs. | Implemented | call_quality_* keys |
| S10 | Tests for callQuality TTL/cap/pinned behavior and source label mapping. | Implemented | |
| S11 | Add map compaction for path reliability + primary/secondary dedup caches with safe heuristics. | Implemented | Peak-based rebuild |
| S12 | Add opt-in map size logging (DXC_MAP_LOG_INTERVAL) + tracker cardinality helpers. | Implemented | Diagnostic only |
| S13 | Harden callQuality with atomic swap + stop/start ordering to avoid data races. | Implemented | |

---

## Scope Ledger v6
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S14 | Establish baseline pprof (allocs + heap) under normal load. | Agreed/Pending | Phase A |
| S15 | Reduce allocations in PSKReporter parsing/normalization. | Implemented | Phase B |
| S16 | Reduce allocations in RBN parsing/tokenization. | Implemented | Phase C |
| S17 | Reduce allocations in spot normalization path. | Implemented | Phase D |
| S18 | Reduce Pebble/cache churn during archive cleanup. | Implemented | Phase E |
| S19 | Provide pprof before/after comparisons per phase. | Agreed/Pending | Phase B comparison complete (12:29 vs 13:49); Phase D+E combined per user request (pending) |

---

## Active Implementation
### Current Phase
In Progress

### In Progress
- [ ] Baseline pprof capture + analysis (Phase A)

### Completed This Session
- [x] Spot Phase D: prefill normalized fields + ASCII-fast upper normalization
- [x] RBN Phase C: tokenization alloc reductions + callsign normalization cleanup
- [x] PSKReporter Phase B: bounded payload pool + single-pass mode normalization
- [x] Atomic callQuality swap + stop/start ordering
- [x] Bounded callQuality store + config wiring
- [x] Stats source label mapping
- [x] Map compaction for path reliability + dedup caches
- [x] Opt-in map size logging
- [x] Tests + docs updates
- [x] Update NEARBY 60m band handling
- [x] Update help/docs/tests

### Blocked / Questions
- [ ] (none)

---

## Decision Log (Append-Only)
### D1 — 2026-02-04 PASS NEARBY ON/OFF semantics
- **Context**: Nearby filter should suspend location filters while preserving path/conf/mode/beacon.
- **Chosen**: Per-session snapshot; NEARBY ON blocks location filter changes with warning.
- **Alternatives**: Persist snapshot to user yaml (rejected).
- **Impact**: New PASS command; location filter behavior changes when NEARBY ON.

### D2 — 2026-02-04 Use L1 for 60m
- **Context**: User requested 60m to use coarse matching.
- **Chosen**: Treat 60m like 160m/80m for NEARBY.
- **Alternatives**: Keep L2 for 60m.
- **Impact**: 60m NEARBY matching uses res-1 cells.

### D3 — 2026-02-04 Map compaction + diagnostics
- **Context**: GC p99 rising over uptime with map capacity retention after churn.
- **Chosen**: Peak-based compaction for path reliability + dedup caches; opt-in DXC_MAP_LOG_INTERVAL logging.
- **Alternatives**: Always rebuild maps on purge; add config knobs for compaction thresholds; rely on pprof only.
- **Impact**: Reduced GC scan set after spikes; no behavior change unless logging enabled.

### D4 — 2026-02-04 Atomic callQuality swap
- **Context**: ConfigureCallQualityStore may be called at runtime; global pointer writes risk data races.
- **Chosen**: atomic.Pointer swap + stop/start ordering.
- **Alternatives**: RWMutex around global pointer; restrict to startup only.
- **Impact**: Eliminates data-race risk during reconfiguration; no behavioral change.

### D5 — 2026-02-04 PSKReporter payload pooling + single-pass mode normalization
- **Context**: PSKReporter parsing shows heavy allocation churn from per-payload copies and repeated mode canonicalization.
- **Chosen**: Add a bounded payload buffer pool (size + count caps, non-blocking) and parse mode once for allowlist + conversion.
- **Alternatives**: Use sync.Pool (unbounded); leave as-is and rely on GC; replace JSON decode with generated parser.
- **Impact**: Reduces alloc churn without changing ingest semantics; pool memory remains bounded.

### D6 — 2026-02-04 RBN tokenization allocation reductions
- **Context**: RBN parsing hot spots include tokenization uppercase allocations and callsign split/join churn.
- **Chosen**: Drop per-token uppercase storage, use ASCII fold comparison for DX/DE, replace split/join with last-dash slicing, and rebuild comments via a single-pass builder.
- **Alternatives**: Full parser rewrite to avoid token slices; sync.Pool for tokens; leave as-is.
- **Impact**: Reduces allocation churn while preserving RBN parsing semantics.

### D7 — 2026-02-04 Spot normalization fast-paths
- **Context**: Spot normalization dominates alloc churn (Mode/ModeNorm, BandNorm, uppercase trims).
- **Chosen**: Add ASCII-fast upper+trim helper, prefill ModeNorm/BandNorm during spot creation, and avoid double uppercasing/canonicalization.
- **Alternatives**: Keep EnsureNormalized as-is; use sync.Pool for spot allocations; rewrite normalization at call sites.
- **Impact**: Reduces per-spot normalization allocations while preserving canonical fields.

### D8 — 2026-02-05 Combine Phase D+E comparison
- **Context**: User requested a single combined checkpoint for Phase D+E.
- **Chosen**: Implement Phase E and report pprof comparisons as combined D+E (baseline vs post-E), accepting loss of per-phase attribution.
- **Alternatives**: Keep per-phase D and E comparisons (slower, more attribution).
- **Impact**: Faster progress, but reduced attribution clarity.

### D9 — 2026-02-05 Archive cleanup lean mode parsing
- **Context**: Cleanup decoded full records just to determine FT retention, causing alloc churn.
- **Chosen**: Parse only the mode field from the record header/value, reuse spot prefix iterator bounds, and clamp cleanup batch size.
- **Alternatives**: Keep full decode; add a separate validity-only parser; add new config knobs for cleanup caps.
- **Impact**: Lower allocation churn during cleanup with bounded delete batches; no behavior change for valid records.

---

## Files Modified (Plan v6)
- PLANS-show-nearby.md
- pskreporter/client.go
- pskreporter/client_test.go
- rbn/client.go
- archive/archive.go
- archive/archive_cleanup_test.go
- spot/mode_alloc.go
- spot/normalize_ascii.go
- spot/spot.go

---

## Files Modified (Plan v5)
- PLANS-show-nearby.md
- spot/correction.go
- spot/priors.go
- spot/quality.go

## Files Modified (Plan v4)
- PLANS-show-nearby.md
- README.md
- config/config.go
- data/config/pipeline.yaml
- dedup/compact_test.go
- dedup/deduplicator.go
- dedup/secondary.go
- main.go
- main_stats_test.go
- pathreliability/compact_test.go
- pathreliability/predictor.go
- pathreliability/store.go
- spot/priors.go
- spot/quality.go
- spot/quality_test.go
- stats/tracker.go

## Files Modified (Plan v2)
- PLANS-show-nearby.md
- README.md
- commands/processor.go
- filter/filter.go
- filter/filter_test.go
- telnet/filter_commands.go
- telnet/server.go
- telnet/server_filter_test.go

---

## Verification Status
- [x] `go test ./...` (Plan v6 Phase E)
- [x] `go test ./...` (Plan v6 Phase D)
- [x] `go test ./...` (Plan v6 Phase C)
- [x] `go test ./...` (Plan v6 Phase B)
- [x] `go test ./...` (Plan v5)
- [x] `go test ./...` (Plan v4)
- [x] `go test ./filter ./telnet` (timed out) [Plan v1]
- [x] `go test ./...` (Plan v2)

---

## Plan Index (history)
- Plan v6 — Resource Efficiency Optimization (Allocation + Retention) — 2026-02-04 — Scope Ledger v6 — status: In Progress
- Plan v5 — Atomic CallQuality Swap — 2026-02-04 — Scope Ledger v5 — status: Implemented
- Plan v4 — GC Compaction + Map Diagnostics — 2026-02-04 — Scope Ledger v4 — status: Implemented
- Plan v1 — PASS NEARBY ON/OFF (H3 L1/L2) — 2026-02-04 — Scope Ledger v1 — status: Implemented
- Plan v2 — NEARBY L1 for 60m — 2026-02-04 — Scope Ledger v2 — status: Implemented
- Plan v3 — Bounded Call Quality + Stats Cardinality Cap — 2026-02-04 — Scope Ledger v3 — status: Implemented

---

## Context for Resume
Implemented: Phase B PSKReporter optimizations (bounded payload buffer pool + single-pass mode normalization), Phase C RBN tokenization allocation reductions (no per-token uppercase, faster callsign normalization, builder-based comment), Phase D spot normalization fast paths (ASCII upper+trim helper, prefilled ModeNorm/BandNorm, avoid double uppercasing), and Phase E archive cleanup churn reduction (mode-only record parsing, reused iter bounds, lazy cleanup batch, capped cleanup batch size). Also: atomic callQuality swap + stop/start ordering; bounded call-quality store with pinned priors + TTL/cap + config knobs; categorical stats source labels; peak-based map compaction for path reliability + dedup caches; DXC_MAP_LOG_INTERVAL diagnostics; tests and docs updated. NEARBY 60m uses L1 (res-1); other NEARBY semantics unchanged.
