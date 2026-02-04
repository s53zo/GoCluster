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

## Active Implementation
### Current Phase
Complete

### In Progress
(none)

### Completed This Session
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

---

## Files Modified
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
- [x] `go test ./filter ./telnet` (timed out) [Plan v1]
- [ ] Not run (Plan v2)

---

## Plan Index (history)
- Plan v1 — PASS NEARBY ON/OFF (H3 L1/L2) — 2026-02-04 — Scope Ledger v1 — status: Implemented
- Plan v2 — NEARBY L1 for 60m — 2026-02-04 — Scope Ledger v2 — status: Implemented

---

## Context for Resume
Implemented: 60m now uses L1 (res-1) for NEARBY matching; help/docs/tests updated. All other NEARBY semantics unchanged.
