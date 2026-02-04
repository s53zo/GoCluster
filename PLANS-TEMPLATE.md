# PLANS-{branch}.md — Plan of Record

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

---

## Current State Snapshot
- **Active Plan**: Plan vN — <short title> — <status>
- **Scope Ledger**: vM — Pending: <N>, Implemented: <N>, Deferred: <N>, Superseded: <N>
- **Next up**: <1–3 bullets>
- **Last updated**: YYYY-MM-DD HH:MM (local)

---

## Current Plan (ACTIVE)

### Plan vN — <short title>
- **Date**: YYYY-MM-DD
- **Status**: Planned | In Progress | Implemented
- **Scope Ledger snapshot**: vM
- **Owner**: Assistant (with user approval)
- **Approval**: Pending (user can reply "Approved vN")

1) Goals
- Describe the intended outcomes.

2) Non-goals
- Explicitly list what is not in scope.

3) Assumptions
- State key assumptions that the plan relies on.

4) Requirements and edge cases
Functional:
- List required behaviors and interfaces.

Non-functional:
- List performance, reliability, or resource constraints.

Edge cases:
- List notable edge cases and handling.

5) Architecture (plan before code)
- Summarize concurrency model, ownership, bounds, backpressure, and failure modes.

6) Contracts and compatibility
- Protocol/format:
- Ordering:
- Drop/disconnect policy:
- Deadlines/timeouts:
- Observability:

7) User-visible behavior
- Describe user-visible behavior changes or state none.

8) Config impact
- List config changes or state none.

9) Observability plan
- List metrics/logs changes or state none.

10) Test plan (before code)
- List boundary and regression tests to add/update.

11) Performance/measurement plan
- List benchmarks or profiling steps if applicable.

12) Rollout/ops plan
- Describe rollout steps and risk mitigations.

13) Open questions / needs-confirmation
- List items requiring user confirmation.

---

## Post-implementation notes (Plan vN)
Implementation notes:
- Summarize what was implemented.

Deviations:
- State deviations from the plan or "None."

Verification commands actually run:
- List commands actually executed, or "None."

Final contract statement:
- State whether any contracts changed.

Scope Ledger status updates:
- Update item statuses for this plan.

---

## Pending Work Summary
- S# — <short description> — <blocking detail or next step>

---

## Scope Ledger vM
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S1 | <scope item> | Agreed/Pending | |

Status values: Agreed/Pending, Implemented, Deferred, Superseded (per AGENTS.md)  
Completed items remain inline - never removed, only status changes.

---

## Active Implementation
### Current Phase
Planned | In Progress | Complete

### In Progress
(none)

### Completed This Session
- [ ] <item>

### Blocked / Questions
- [ ] <item>

---

## Turn Log (Append-Only)
- REQUIRED: add one line per significant turn (decision, scope change, implementation step, verification run).
- YYYY-MM-DD HH:MM — <short description of change or decision>

---

## Decision Log (Append-Only)
### D1 — YYYY-MM-DD <title>
- **Context**: <why>
- **Chosen**: <decision>
- **Alternatives**: <alternatives considered>
- **Impact**: <impact on contracts/ops>

---

## Files Modified
- <file>

---

## Verification Status
- [ ] <status>

---

## Plan Index (history)
- Plan vN — <title> — YYYY-MM-DD — Scope Ledger vM — status: <status>

---

## Context for Resume
Summarize the current plan, scope state, and any critical decisions.
