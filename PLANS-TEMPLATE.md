# PLANS-{branch}.md — Plan of Record (Compact)

## Current State Snapshot
- Active Plan: Plan vN — <title> — <status>
- Scope Ledger: vM — Pending: <N>, Implemented: <N>, Deferred: <N>, Superseded: <N>
- Next up: <1–3 bullets>
- Last updated: YYYY-MM-DD HH:MM (local)

---

## Current Plan (ACTIVE)

### Plan vN — <title>
- Date: YYYY-MM-DD
- Status: Planned | In Progress | Implemented
- Scope Ledger snapshot: vM
- Owner: Assistant (with user approval)
- Approval: Pending

Pre-Implementation Gate (Mandatory):
- Classification: Small | Non-trivial
- Scope Ledger IDs approved: <e.g., S2,S3,S4>
- Approval evidence (quote + timestamp): "<exact user approval text>" — YYYY-MM-DD HH:MM
- Ready to implement: Yes | No

Goals:
- …

Non-Goals:
- …

Requirements/Edge Cases:
- …

Architecture (bounds/backpressure/shutdown):
- …

Contracts:
- No contract changes. (or list changes only)

Tests:
- …

Rollout/Ops:
- …

---

## Post-code (Plan vN)
Deviations:
- None.

Verification commands actually run:
- None.

Final contract statement:
- …

Scope Ledger status updates:
- …

---

## Scope Ledger vM (LIVE)
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S1 | <scope item> | Agreed/Pending | |

---

## Turn Log (Append-Only, last 25 kept)
- YYYY-MM-DD HH:MM — <short description>

---

## Decision Log (Append-Only)
### D1 — YYYY-MM-DD <title>
- Context: …
- Chosen: …
- Alternatives: …
- Impact: …

---

## Plan Index (history)
- Plan vN — <title> — YYYY-MM-DD — Scope Ledger vM — status: <status>

---

## Context for Resume
**Done**:
- …
**In Progress**:
- …
**Next**:
- …
**Key Decisions**:
- …
**Files Hot**:
- …

---

## Compaction Rule
- Keep only: Active Plan, live Scope Ledger, last 25 Turn Log entries, Decision Log, Plan Index, Current State Snapshot, Context for Resume.
- Move older plans/logs/superseded scope rows to `PLANS-ARCHIVE-{branch}.md`.
