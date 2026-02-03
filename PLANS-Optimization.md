# PLANS-Optimization.md — Plan of Record (Optimization branch)

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

### Plan v4 — Disable R2 solar override (R shown only for R3/R4)
- **Date**: 2026-02-03
- **Status**: Implemented
- **Scope Ledger snapshot**: v4
- **Owner**: Assistant
- **Approval**: Pending (user can reply "Approved v4")

1) Goals
- Remove R2 from solar weather overrides so `R` appears only for R3/R4 thresholds.

2) Non-goals
- No code changes to the cluster.
- No protocol, backpressure, timeout, shutdown, or observability contract changes.

3) Assumptions
- Solar weather configuration is loaded from `data/config/solarweather.yaml`.
- Removing R2 is acceptable for operations.

4) Requirements and edge cases
Functional:
- R overrides are eligible only for R3/R4 thresholds in the config.
- R2 events do not produce an `R` glyph.

Edge cases:
- If no R level is active, no R override applies (normal path glyphs remain).

5) Architecture (plan before code)
- N/A (configuration-only change).

6) Contracts and compatibility
- Protocol/format: No contract changes.
- Ordering: No contract changes.
- Drop/disconnect policy: No contract changes.
- Deadlines/timeouts: No contract changes.
- Observability: No contract changes.

7) User-visible behavior
- `R` glyph will no longer appear for R2-level events.

8) Config impact
- `data/config/solarweather.yaml` updated (removed `R2` level).

9) Observability plan
- None.

10) Test plan (before code)
- None (configuration-only change).

11) Performance/measurement plan
- None.

12) Rollout/ops plan
- Deploy updated `data/config/solarweather.yaml`.

13) Open questions / needs-confirmation
- Approval of Plan v4 ("Approved v4").

### Plan v3 — Add shared PLANS template and AGENTS pointer
- **Date**: 2026-02-03
- **Status**: Implemented
- **Scope Ledger snapshot**: v3
- **Owner**: Assistant (with user approval)
- **Approval**: Pending (user can reply "Approved v3")

1) Goals
- Add a durable, copy/pasteable `PLANS-TEMPLATE.md` in repo root.
- Reference the template from AGENTS.md so new branches use a consistent structure.

2) Non-goals
- No code changes to the cluster.
- No protocol, backpressure, timeout, shutdown, or observability contract changes.

3) Assumptions
- New branches will create PLANS files by copying the template.
- Agents will follow AGENTS.md and the template as the plan-of-record structure.

4) Requirements and edge cases
Functional:
- Provide a root-level template with Plan vN skeleton, Scope Ledger table, Decision Log, Plan Index, and Context for Resume.
- Ensure AGENTS.md points to the template for new branch initialization.

Non-functional:
- Concise and scannable at conversation start.
- Avoid duplication across AGENTS and PLANS files.

Edge cases:
- Template drift: update template when plan structure evolves.

5) Architecture (plan before code)
- N/A (workflow/documentation change; no runtime architecture impacted).

6) Contracts and compatibility
- Protocol/format: No contract changes.
- Ordering: No contract changes.
- Drop/disconnect policy: No contract changes.
- Deadlines/timeouts: No contract changes.
- Observability: No contract changes.

7) User-visible behavior
- No user-visible behavior changes.

8) Config impact
- None.

9) Observability plan
- None (workflow only).

10) Test plan (before code)
- Manual workflow validation in the next conversation:
  - Confirm template is referenced from AGENTS.md.
  - Confirm new branch uses template as starting plan-of-record.

11) Performance/measurement plan
- None (no hot path changes).

12) Rollout/ops plan
- Add `PLANS-TEMPLATE.md` to repo root.
- Update AGENTS.md to point to the template.

13) Open questions / needs-confirmation
- Approval of Plan v3 ("Approved v3").

---

### Plan v2 — Refine plan-of-record workflow and AGENTS contract
- **Date**: 2026-02-03
- **Status**: Implemented
- **Scope Ledger snapshot**: v2
- **Owner**: Assistant (with user approval)
- **Approval**: Pending (user can reply "Approved v2")

1) Goals
- Make PLANS-{branch}.md the explicit plan-of-record with pre/post phases and approval hooks.
- Ensure AGENTS.md explicitly requires pre-code plan refresh and post-code reconciliation.
- Add concise, durable structure to keep long chats coherent.

2) Non-goals
- No code changes to the cluster.
- No protocol, backpressure, timeout, shutdown, or observability contract changes.

3) Assumptions
- The active git branch name is encoded into the PLANS filename (PLANS-{branch}.md).
- Agents will follow AGENTS.md and treat this file as the plan-of-record for this branch.

4) Requirements and edge cases
Functional:
- PLANS must include versioned Plan vN entries with pre/post phases and approval status.
- AGENTS must reference plan refresh and post-implementation reconciliation.

Non-functional:
- Concise and scannable at conversation start.
- Durable across long (50+ turn) conversations.

Edge cases:
- Stale plans: if scope diverges or file is >7 days without update, refresh before code.
- Conflicting scope items: mark older items Superseded with impact note.

5) Architecture (plan before code)
- N/A (workflow/documentation change; no runtime architecture impacted).

6) Contracts and compatibility
- Protocol/format: No contract changes.
- Ordering: No contract changes.
- Drop/disconnect policy: No contract changes.
- Deadlines/timeouts: No contract changes.
- Observability: No contract changes.

7) User-visible behavior
- No user-visible behavior changes.

8) Config impact
- None.

9) Observability plan
- None (workflow only).

10) Test plan (before code)
- Manual workflow validation in the next conversation:
  - Confirm agents read this file first.
  - Confirm agents refresh Plan vN pre-code for Non-trivial changes.
  - Confirm agents update Scope Ledger and Decision Log after decisions.
  - Confirm "Context for Resume" is refreshed when scope changes.

11) Performance/measurement plan
- None (no hot path changes).

12) Rollout/ops plan
- Commit AGENTS.md and PLANS-Optimization.md updates to the branch.
- Use in subsequent conversations as the authoritative context state.

13) Open questions / needs-confirmation
- Approval of Plan v2 ("Approved v2").

---

### Plan v1 — Persistent context workflow for long conversations
- **Date**: 2026-02-02
- **Status**: Implemented
- **Scope Ledger snapshot**: v1
- **Owner**: Assistant (with user approval)
- **Approval**: Pending (user can reply "Approved v1")

1) Goals
- Establish a per-branch PLANS file that any agent can read first to regain context and avoid scope drift.
- Add an explicit Scope Ledger and append-only decision log to make “agreed changes” unambiguous.
- Provide a short "Context for Resume" section to re-orient after context truncation.

2) Non-goals
- No code changes to the cluster.
- No protocol, backpressure, timeout, shutdown, or observability contract changes.

3) Assumptions
- The active git branch name is encoded into the PLANS filename (PLANS-{branch}.md).
- Agents will follow AGENTS.md and treat this file as the plan-of-record for this branch.

4) Requirements and edge cases
Functional:
- A single file in repo root that: (a) tracks scope state, (b) logs decisions, (c) records what changed, (d) captures verification status.
- Clear status transitions for scope items: Agreed/Pending, Implemented, Deferred, Superseded.

Non-functional:
- Must be readable quickly at conversation start.
- Must be stable across long (50+ turn) conversations.

Edge cases:
- Context truncation: "Context for Resume" must be sufficient to re-orient.
- Conflicting scope items: mark older items Superseded with impact note.

5) Architecture (plan before code)
- N/A (workflow/documentation change; no runtime architecture impacted).

6) Contracts and compatibility
- Protocol/format: No contract changes.
- Ordering: No contract changes.
- Drop/disconnect policy: No contract changes.
- Deadlines/timeouts: No contract changes.
- Observability: No contract changes.

7) User-visible behavior
- No user-visible behavior changes.

8) Config impact
- None.

9) Observability plan
- None (workflow only).

10) Test plan (before code)
- Manual workflow validation in the next conversation:
  - Confirm agents read this file first.
  - Confirm agents update Scope Ledger and Decision Log after decisions.
  - Confirm "Context for Resume" is refreshed when scope changes.

11) Performance/measurement plan
- None (no hot path changes).

12) Rollout/ops plan
- Commit this file and AGENTS.md changes to the branch.
- Use in subsequent conversations as the authoritative context state.

13) Open questions / needs-confirmation
- Approval of Plan v1 ("Approved v1").

---

## Post-implementation notes (Plan v4)
Implementation notes:
- Removed `R2` from `data/config/solarweather.yaml` so `R` only applies to R3/R4.

Deviations:
- None.

Verification commands actually run:
- None (configuration change).

Final contract statement:
- Protocol/format: No contract changes.
- User-visible: `R` glyph no longer appears for R2-level events.

Scope Ledger status updates:
- S7 marked Implemented (see Scope Ledger v4).

---

## Post-implementation notes (Plan v3)
Implementation notes:
- Added `PLANS-TEMPLATE.md` in repo root.
- Updated AGENTS.md to point to the template for new branch initialization.

Deviations:
- None.

Verification commands actually run:
- None (documentation/workflow change).

Final contract statement:
- No contract changes.

Scope Ledger status updates:
- S5 and S6 marked Implemented (see Scope Ledger v3).

---

## Post-implementation notes (Plan v2)
Implementation notes:
- Updated AGENTS.md to require pre-code Plan vN refresh and post-code reconciliation.
- Restructured PLANS-Optimization.md into a plan-of-record format with Plan vN entries.

Deviations:
- None.

Verification commands actually run:
- None (documentation/workflow change).

Final contract statement:
- No contract changes.

Scope Ledger status updates:
- S3 and S4 marked Implemented (see Scope Ledger v2).

---

## Post-implementation notes (Plan v1)
Implementation notes:
- Implemented per-branch PLANS file pattern and updated AGENTS.md to require reading this file first.

Deviations:
- None.

Verification commands actually run:
- None (documentation/workflow change).

Final contract statement:
- No contract changes.

Scope Ledger status updates:
- S1 and S2 marked Implemented (see Scope Ledger v1).

---

## Scope Ledger v4
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S1 | Create PLANS-Optimization.md template | Implemented | This file |
| S2 | Add PERSISTENT CONTEXT section to AGENTS.md | Implemented | |
| S3 | Expand AGENTS.md to enforce plan-of-record workflow | Implemented | Pre/post plan refresh, plan-of-record section |
| S4 | Restructure PLANS-Optimization.md into Plan vN format | Implemented | Plan v2 added |
| S5 | Add PLANS-TEMPLATE.md in repo root | Implemented | Plan skeleton for new branches |
| S6 | Reference PLANS-TEMPLATE.md from AGENTS.md | Implemented | New-branch guidance |
| S7 | Remove R2 from solar weather overrides | Implemented | R only for R3/R4 in config |

Status values: Agreed/Pending, Implemented, Deferred, Superseded (per AGENTS.md)  
Completed items remain inline - never removed, only status changes.

---

## Scope Ledger v3
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S1 | Create PLANS-Optimization.md template | Implemented | This file |
| S2 | Add PERSISTENT CONTEXT section to AGENTS.md | Implemented | |
| S3 | Expand AGENTS.md to enforce plan-of-record workflow | Implemented | Pre/post plan refresh, plan-of-record section |
| S4 | Restructure PLANS-Optimization.md into Plan vN format | Implemented | Plan v2 added |
| S5 | Add PLANS-TEMPLATE.md in repo root | Implemented | Plan skeleton for new branches |
| S6 | Reference PLANS-TEMPLATE.md from AGENTS.md | Implemented | New-branch guidance |

Status values: Agreed/Pending, Implemented, Deferred, Superseded (per AGENTS.md)  
Completed items remain inline - never removed, only status changes.

---

## Scope Ledger v2
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S1 | Create PLANS-Optimization.md template | Implemented | This file |
| S2 | Add PERSISTENT CONTEXT section to AGENTS.md | Implemented | |
| S3 | Expand AGENTS.md to enforce plan-of-record workflow | Implemented | Pre/post plan refresh, plan-of-record section |
| S4 | Restructure PLANS-Optimization.md into Plan vN format | Implemented | Plan v2 added |

Status values: Agreed/Pending, Implemented, Deferred, Superseded (per AGENTS.md)  
Completed items remain inline - never removed, only status changes.

---

## Active Implementation
### Current Phase
Complete

### In Progress
(none)

### Completed This Session
- [x] Added PLANS-TEMPLATE.md
- [x] Updated AGENTS.md to reference PLANS-TEMPLATE.md

### Blocked / Questions
- [ ] Approval of Plan v1 ("Approved v1"), Plan v2 ("Approved v2"), and Plan v3 ("Approved v3")

---

## Decision Log (Append-Only)
### D1 — 2026-02-02 Per-Branch PLANS Files
- **Context**: Need persistent context for 50+ turn conversations with Codex/Claude Code
- **Chosen**: Per-branch `PLANS-{branch}.md` files in repo root, shared by all AI agents
- **Alternatives**: Single PLANS.md (rejected - branch conflicts), separate files per tool (rejected - no single source of truth)
- **Impact**: No contract changes; operational workflow improvement

### D2 — 2026-02-03 Plan-of-Record Refinements
- **Context**: Clarify pre/post plan refresh, approval hooks, and stale-plan rules
- **Chosen**: Plan vN format with approval line and explicit pre/post phases in AGENTS.md
- **Alternatives**: Keep prior lightweight PLANS (rejected - higher drift risk)
- **Impact**: No contract changes; clearer workflow enforcement

### D3 — 2026-02-03 Add PLANS Template
- **Context**: Need a copy/pasteable, durable plan skeleton for new branches
- **Chosen**: Add `PLANS-TEMPLATE.md` and reference it from AGENTS.md
- **Alternatives**: Embed template in AGENTS.md (rejected - too verbose)
- **Impact**: No contract changes; clearer and more consistent planning

### D4 — 2026-02-03 Restrict R overrides to R3/R4
- **Context**: Operator preference to suppress R2-level `R` glyphs.
- **Chosen**: Remove R2 from `data/config/solarweather.yaml`.
- **Alternatives**: Keep R2 but remap glyph; raise R2 threshold to R3 (rejected; config clarity).
- **Impact**: User-visible behavior change for R2 events only.

---

## Files Modified
- `AGENTS.md` - reference `PLANS-TEMPLATE.md` for new branch initialization
- `PLANS-Optimization.md` - added Plan v4 and Scope Ledger v4 updates
- `PLANS-TEMPLATE.md` - new plan-of-record template
- `data/config/solarweather.yaml` - removed R2 level

---

## Verification Status
- [x] Files updated as planned
- [ ] Test workflow in next conversation (record date on completion)

---

## Plan Index (history)
- Plan v4 — Disable R2 solar override (R shown only for R3/R4) — 2026-02-03 — Scope Ledger v4 — status: Implemented
- Plan v3 — Add shared PLANS template and AGENTS pointer — 2026-02-03 — Scope Ledger v3 — status: Implemented
- Plan v2 — Refine plan-of-record workflow and AGENTS contract — 2026-02-03 — Scope Ledger v2 — status: Implemented
- Plan v1 — Persistent context workflow for long conversations — 2026-02-02 — Scope Ledger v1 — status: Implemented

---

## Context for Resume
Plan-of-record workflow is now explicit and includes a reusable `PLANS-TEMPLATE.md`. R2-level solar overrides were removed from `data/config/solarweather.yaml` so `R` appears only for R3/R4. Approvals for Plan v1–v4 are pending. Scope Ledger v4 includes the R2 removal.
