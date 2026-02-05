# PLANS-main.md — Plan of Record

## Operating rules
- This file is the durable plan-of-record for this branch. It is the single source of truth for work state in long conversations.
- Required for any Non-trivial change or contract-touching change (see AGENTS.md).
- Each plan is versioned as Plan vN and must reference the Scope Ledger snapshot it implements (Scope Ledger vM).
- Plans have two phases:
  - Pre-code: goals/non-goals/assumptions/requirements/architecture/contracts/test plan/rollout.
  - Post-code: deviations, verification commands actually run (if any), final contract statement, and Scope Ledger status updates.
- Keep history append-only: use Plan Index and Decision Log.
- The **Current State Snapshot** is mandatory and must be updated on every plan change, scope decision, or end-of-session update.
- The **Turn Log** is mandatory and append-only; add one line per significant turn (decision, scope change, implementation step, verification run).
- **Compaction rule (mandatory)**: When this file exceeds ~1,000 lines or every 25–50 turns (whichever comes first), move implemented plan entries, old Turn Log entries, and any superseded Scope Ledger rows to `PLANS-ARCHIVE-main.md`. Keep only the current plan, live Scope Ledger, last 20 Turn Log entries, Decision Log, and Plan Index in this file. Update the Current State Snapshot and Plan Index to reference the archive.

---

## Current State Snapshot
- **Active Plan**: Plan v1 — GC p99 Windowed Console Reporting — Implemented
- **Scope Ledger**: v1 — Pending: 0, Implemented: 1, Deferred: 0, Superseded: 0
- **Next up**: (none)
- **Last updated**: 2026-02-05 08:00 (local)

---

## Current Plan (ACTIVE)

### Plan v1 — GC p99 Windowed Console Reporting
- **Date**: 2026-02-05
- **Status**: Implemented
- **Scope Ledger snapshot**: v1
- **Owner**: Assistant (with user approval)
- **Approval**: Approved v1 (2026-02-05)

1) Goals
- Report GC p99 in the console/overview line as a windowed statistic aligned with the stats refresh interval.
- Avoid stale p99 values derived from the full PauseNs ring.
- Keep the change bounded and low-overhead.

2) Non-goals
- Do not alter GC behavior or tuning.
- Do not add new log-only GC metrics.
- Do not change ingest or telnet behavior.

3) Assumptions
- Stats refresh is driven by `cfg.Stats.DisplayIntervalSeconds`.
- Console/overview lines are built once per stats tick.
- GC pause ring (PauseNs) provides the most recent 256 pauses.

4) Requirements and edge cases
Functional:
- Compute GC p99 from pauses that occurred since the last stats refresh tick.
- If no GC occurs in the window, report `n/a` for GC p99.
- Keep `Last GC` age behavior unchanged.

Non-functional:
- No new goroutines.
- Bounded allocations per tick.

Edge cases:
- If more than 256 GCs occur in a single window, use the most recent 256 pauses and note truncation internally (no crash).
- Handle startup: initial window should not include all historical pauses.

5) Architecture (plan before code)
- Add a small GC window tracker owned by the stats loop (`displayStatsWithFCC`).
- Each tick: read `runtime.MemStats`, compute windowed p99 using `NumGC` delta and PauseNs ring, then update overview lines with the windowed value.
- Maintain a previous `NumGC` baseline captured before the first tick.

6) Contracts and compatibility
- Protocol/format: No changes.
- Ordering: No changes.
- Drop/disconnect policy: No changes.
- Deadlines/timeouts: No changes.
- Observability: GC p99 semantics change to “p99 of pauses since last stats refresh.”

7) User-visible behavior
- Console/overview line label becomes `GC p99 (interval)`.
- GC p99 reflects only the most recent stats window; shows `n/a` when no GC occurs in the window.

8) Config impact
- None.

9) Observability plan
- Update console/overview GC p99 calculation to windowed.
- Update label text to `GC p99 (interval)`.

10) Test plan (before code)
- Unit tests for windowed GC p99 calculation:
  - No new GC in window → `n/a` with count=0.
  - Multiple new GCs → correct p99.
  - Delta > ring size → truncation path produces stable output.

11) Performance/measurement plan
- Not required (stats-path only, low frequency).

12) Rollout/ops plan
- Deploy normally; verify console GC p99 updates per tick and does not remain stale after a single spike.

13) Open questions / needs-confirmation
- None.

---

## Pending Work Summary
- (none)

---

## Scope Ledger v1 (LIVE)
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S1 | Compute GC p99 over the stats refresh window and report it in the console/overview line (not whole-process ring). | Implemented | Display `n/a` when no GC; label `GC p99 (interval)` |

---

## Active Implementation
### Current Phase
Complete

### In Progress
(none)

### Completed This Session
- [x] S1 — Windowed GC p99 reporting in console/overview line

### Blocked / Questions
- [ ] (none)

---

## Turn Log (Append-Only)
- 2026-02-05 07:53 — Created Plan v1 and Scope Ledger v1 for windowed GC p99 console reporting
- 2026-02-05 08:00 — Implemented windowed GC p99 reporting; updated UI label; added tests; ran go test ./...

---

## Decision Log (Append-Only)
### D1 — 2026-02-05 Windowed GC p99
- **Context**: Console GC p99 currently computed over entire PauseNs ring and can be stale.
- **Chosen**: Compute p99 over GCs since the last stats refresh tick.
- **Alternatives**: Keep ring-based p99; compute time-windowed via runtime/metrics.
- **Impact**: Observability semantics change; no protocol/runtime behavior change.

---

## Post-implementation notes (Plan v1)
Implementation notes:
- Added a GC pause window tracker keyed to the stats refresh loop.
- Updated console/overview GC p99 label to `GC p99 (interval)` and show `n/a` when no GC occurs in the window.
- Added unit tests for windowed p99 and truncation behavior.

Deviations:
- None.

Verification commands actually run:
- `go test ./...`

Final contract statement:
- Observability contract updated: GC p99 now reflects pauses since the last stats refresh tick.

Scope Ledger status updates:
- S1 → Implemented

---

## Files Modified
- gc_window.go
- gc_window_test.go
- main.go
- ui/dashboard_v2.go
- PLANS-main.md

---

## Verification Status
- [x] go test ./...

---

## Plan Index (history)
- Plan v1 — GC p99 Windowed Console Reporting — 2026-02-05 — Scope Ledger v1 — status: Implemented

---

## Context for Resume
**Done**:
- S1 — Implement windowed GC p99 reporting in console/overview line
**In Progress**:
- (none)
**Next**:
- (none)
**Key Decisions**:
- D1 — Windowed GC p99 per stats refresh tick
**Files Hot**:
- main.go
- gc_window.go
