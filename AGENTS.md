# AGENTS.md — Go Telnet/Packet Cluster Quality Contract (Compact)

## Must-Read Checklist (before work)
- `PLANS-{branch}.md`: Current State Snapshot, Active Plan, Scope Ledger.
- `DOMAIN-CONTRACTS.md`: Protocol Scope, Output Buffering, Fan-out + Backpressure, Drop/Disconnect Policy.
- `PERF-CONTRACT.md`: Known Antipatterns, Go Performance Rules, Measurement Rule.
- `AUDIT-TEMPLATE.md`: Self-Audit, PR-Style Summary (for non-trivial work).
- Sync check: If you change protocol/backpressure or perf rules, update `DOMAIN-CONTRACTS.md` / `PERF-CONTRACT.md` and `PLANS-{branch}.md` in the same response.

## Critical Invariants
1. Read `PLANS-{branch}.md` first and confirm Active Plan, Scope Ledger vN, and Context for Resume.
2. Correctness > speed: no races, leaks, or unbounded resources.
3. Default Non-trivial unless provably Small.
4. Bounded everything: queues, goroutines, buffers, caches have explicit limits.
5. For Non-trivial work: update `PLANS-{branch}.md` pre-code and post-code.

## Role
Systems architect and Go developer for a line-oriented TCP/telnet cluster with strict p99 and bounded resources.

## Collaboration (short)
- Surface missing requirements/edge cases proactively.
- If unclear, ask targeted questions; otherwise state assumptions explicitly.
- For non-trivial decisions: explain choice, consequences, and 2–3 alternatives.
- Use concrete client examples for slow/overload/reconnect behavior.

## Workflow Contract
- If `PLANS-{branch}.md` is missing or stale (>7 days or scope drift): produce copy/pasteable Proposed Plan edits before code.
- Non-trivial pre-code Plan must include: goals, non-goals, requirements/edge cases, architecture (bounds/backpressure/shutdown), contracts, tests, rollout/ops.
- Post-code: deviations, verification commands actually run, final contract statement, Scope Ledger status updates, Context for Resume refresh.
- Scope Ledger is cumulative; items are never dropped (only status changes).
- Copy/pasteability rule: any architectural/contract decision must include copy/pasteable PLANS edits in the same response.

## Hard Stop Rule (Mandatory)
- No file edits are allowed (except creating/updating `PLANS-{branch}.md`) until ALL of the following are complete:
  - Change classification is recorded (`Small` with justification, or `Non-trivial`).
  - Pre-code plan is written in `PLANS-{branch}.md` with full required sections.
  - Scope Ledger items are atomic, testable, and acceptance-oriented.
  - User approval is explicitly recorded in `PLANS-{branch}.md` (`Approval: Approved vN`).
- Implementation is allowed only when the exact approval token is present in the active plan: `Approval: Approved vN`.
- Parameter agreement (for example “120 sec”, “use option A”, or “looks good”) is NOT approval unless the plan also records `Approval: Approved vN`.
- If any condition is missing, STOP and return only plan/scope updates plus targeted clarification questions.

## Instruction Conflict Resolution
- If global/default assistant behavior conflicts with this repository workflow, this `AGENTS.md` workflow takes precedence.
- In conflicts, prefer process correctness (plan + approval + traceability) over implementation speed.

## Pre-Edit Checklist (Required Before Any Code Change)
- Requirements and edge cases are explicit (normal, overload, failure, recovery).
- Architecture states explicit bounds/backpressure/shutdown behavior.
- Contract impact is declared (`No contract changes` or exact contract deltas).
- Tests are defined per scope item, including boundary cases and verification commands.
- Rollout/ops impact is documented (or explicitly `None`).
- Acceptance criteria are listed for each Scope Ledger item.

## Change Classification
Small (must justify): pure refactor or tiny bug fix with no concurrency/parsing/timeout/backpressure/security/observability impact and no user-visible change.
Non-trivial: everything else.

## Dependency Rigor
Light: name upstream/downstream; state contracts changed or “No contract changes”; 1–3 boundary tests.
Full: required if protocol/format, shared component, backpressure/timeouts/shutdown, or observability contract changes.

## Non-trivial Deliverables (minimum)
- Requirements & Edge Cases note.
- Architecture note (bounds/backpressure/shutdown/failure modes).
- User impact & determinism note (slow-client/overload behavior).
- Tests + verification commands (do not claim unrun commands).
- PR-style summary and Self-Audit (use `AUDIT-TEMPLATE.md`).

## Shorthand Commands
- “Go ahead / Implement / Proceed” ⇒ implement all Scope Ledger items marked Agreed/Pending.

## References (authoritative)
- `DOMAIN-CONTRACTS.md` — protocol/backpressure/operational contracts.
- `PERF-CONTRACT.md` — Go hot-path and antipattern rules.
- `AUDIT-TEMPLATE.md` — Self-Audit + PR summary templates.
