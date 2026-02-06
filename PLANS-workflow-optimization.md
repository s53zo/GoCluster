# PLANS-workflow-optimization.md — Plan of Record (Compact)

## Current State Snapshot
- Active Plan: Plan v1 — Workflow Docs Token Optimization — Implemented
- Scope Ledger: v1 — Pending: 0, Implemented: 6, Deferred: 0, Superseded: 0
- Next up: (none)
- Last updated: 2026-02-05 15:56 (local)

---

## Current Plan (ACTIVE)

### Plan v1 — Workflow Docs Token Optimization
- Date: 2026-02-05
- Status: Implemented
- Scope Ledger snapshot: v1
- Owner: Assistant (with user approval)
- Approval: Approved v1

Goals:
- Reduce AGENTS.md verbosity while preserving workflow rigor.
- Split domain/perf/audit detail into referenced files.
- Make PLANS template concise and enforce compaction at 25 turns.
- Compact PLANS-main.md and add archive.

Non-Goals:
- No changes to runtime behavior, configs, or tests.
- No weakening of correctness, bounded-resource, or audit requirements.

Requirements/Edge Cases:
- Critical invariants remain explicit and enforceable.
- References are stable and discoverable.
- Copy/pasteability preserved for plans and audit templates.

Architecture (bounds/backpressure/shutdown):
- Docs-only refactor: AGENTS.md as workflow contract + pointers.
- Domain/perf/audit details moved to dedicated files.
- PLANS template compacted; PLANS-main archived and slimmed.

Contracts:
- Documentation/workflow contract only; no runtime contract changes.

Tests:
- None (docs-only change).

Rollout/Ops:
- N/A.

---

## Post-code (Plan v1)
Deviations:
- None.

Verification commands actually run:
- None.

Final contract statement:
- Workflow docs reorganized for brevity; domain/perf/audit contracts moved to separate files.

Scope Ledger status updates:
- S1 → Implemented
- S2 → Implemented
- S3 → Implemented
- S4 → Implemented
- S5 → Implemented
- S6 → Implemented

---

## Scope Ledger v1 (LIVE)
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S1 | Compact AGENTS.md and reference new contract files | Implemented | |
| S2 | Add DOMAIN-CONTRACTS.md (protocol/backpressure/domain rules) | Implemented | |
| S3 | Add PERF-CONTRACT.md (antipatterns/perf rules) | Implemented | |
| S4 | Add AUDIT-TEMPLATE.md (self-audit + PR summary templates) | Implemented | |
| S5 | Replace PLANS-TEMPLATE.md with minimal template | Implemented | |
| S6 | Compact PLANS-main.md and create PLANS-ARCHIVE-main.md | Implemented | |

---

## Turn Log (Append-Only, last 25 kept)
- 2026-02-05 11:15 — Created Plan v1 for workflow docs optimization
- 2026-02-05 11:35 — Implemented Plan v1; updated AGENTS/PLANS and added contract templates
- 2026-02-05 15:54 — Added must-read checklist to AGENTS.md
- 2026-02-05 15:55 — Added sync-check line to AGENTS.md checklist
- 2026-02-05 15:56 — Expanded sync-check to include perf contract

---

## Decision Log (Append-Only)
- (none)

---

## Plan Index (history)
- Plan v1 — Workflow Docs Token Optimization — 2026-02-05 — Scope Ledger v1 — Implemented

---

## Context for Resume
**Done**:
- S1–S6 — Workflow docs optimization complete
**In Progress**:
- (none)
**Next**:
- (none)
**Key Decisions**:
- (none)
**Files Hot**:
- (none)

---

## Compaction Rule
- Keep only: most recent plan, live Scope Ledger, last 25 Turn Log entries, Decision Log, Plan Index, Current State Snapshot, Context for Resume.
- Move older plans/logs/superseded scope rows to `PLANS-ARCHIVE-workflow-optimization.md`.
