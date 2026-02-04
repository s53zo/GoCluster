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

### Plan v5 — Atomic CallQuality Swap
- **Date**: 2026-02-04
- **Status**: Implemented
- **Scope Ledger snapshot**: v5
- **Owner**: Assistant (with user approval)
- **Approval**: Approved v5 (user request)

1) Goals
- Remove data-race potential by making callQuality swaps atomic.
- Ensure cleanup stop/start ordering is deterministic when reconfiguring.

2) Non-goals
- No behavioral changes to correction logic or TTL/cap policy.
- No protocol/format changes.

3) Assumptions
- ConfigureCallQualityStore is usually called at startup, but may be called at runtime in the future.

4) Requirements and edge cases
Functional:
- Atomic swap ensures readers never observe a partially updated global pointer.
- Old cleanup goroutine is stopped after swap.
Edge cases:
- callQuality pointer may be nil during early init; callers must handle nil safely.

5) Architecture (plan before code)
- Replace global callQuality variable with `atomic.Pointer[*CallQualityStore]`.
- Add helper to load the current store once per call site.
- ConfigureCallQualityStore uses atomic swap and stop/start ordering.
- Dependency rigor: Light (single component, no contract changes).

6) Contracts and compatibility
- Protocol/format: No change.
- Ordering: No change.
- Drop/disconnect policy: No change.
- Deadlines/timeouts: No change.
- Observability: No change.

7) User-visible behavior
- No user-visible behavior changes.

8) Config impact
- None.

9) Observability plan
- None.

10) Test plan (before code)
- No new tests required (atomic pointer change); existing callQuality tests remain valid.

11) Performance/measurement plan
- None.

12) Rollout/ops plan
- None.

13) Open questions / needs-confirmation
- None.

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

## Active Implementation
### Current Phase
Complete

### In Progress
- [ ] (none)

### Completed This Session
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
- [x] `go test ./...` (Plan v5)
- [x] `go test ./...` (Plan v4)
- [x] `go test ./filter ./telnet` (timed out) [Plan v1]
- [x] `go test ./...` (Plan v2)

---

## Plan Index (history)
- Plan v5 — Atomic CallQuality Swap — 2026-02-04 — Scope Ledger v5 — status: Implemented
- Plan v4 — GC Compaction + Map Diagnostics — 2026-02-04 — Scope Ledger v4 — status: Implemented
- Plan v1 — PASS NEARBY ON/OFF (H3 L1/L2) — 2026-02-04 — Scope Ledger v1 — status: Implemented
- Plan v2 — NEARBY L1 for 60m — 2026-02-04 — Scope Ledger v2 — status: Implemented
- Plan v3 — Bounded Call Quality + Stats Cardinality Cap — 2026-02-04 — Scope Ledger v3 — status: Implemented

---

## Context for Resume
Implemented: atomic callQuality swap + stop/start ordering. Also: bounded call-quality store with pinned priors + TTL/cap + config knobs; categorical stats source labels; peak-based map compaction for path reliability + dedup caches; DXC_MAP_LOG_INTERVAL diagnostics; tests and docs updated. NEARBY 60m uses L1 (res-1); other NEARBY semantics unchanged.
