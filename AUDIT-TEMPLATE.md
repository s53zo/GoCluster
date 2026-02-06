# AUDIT-TEMPLATE.md — Self-Audit + PR Summary

## Self-Audit (pass/fail)
- Scope completeness vs Scope Ledger and Definition of Done
- Dependency impact identified; contract changes explicit; tests cover boundaries
- PLANS consistency (pre/post updates, contracts/tests/rollout match)
- Correctness and protocol semantics (incl. telnet/IAC policy)
- Concurrency/cancellation/shutdown safety (no leaks)
- Backpressure/queues/drop semantics are precise + tested
- Resource bounds (per-conn + global)
- Performance evidence when applicable
- Security/robustness (input validation, log hygiene)
- Testing adequacy (unit + race relevance + fuzz if applicable)
- Documentation quality (invariants, ownership, operator-visible behavior)

Include verification commands and what “good” looks like. Do not claim unrun commands.

---

## PR-Style Summary
- Summary (what changed + why)
- Plan-of-record reference (Plan vN + Scope Ledger vM)
- Tradeoffs (latency vs delivery vs memory; strict vs lenient)
- Risks and mitigations
- Contracts/compatibility (explicit list or “No contract changes”)
- User impact & determinism (slow-client/overload behavior)
- Observability impact (metrics/log fields)
- Verification commands (exact)
- Scope-to-Code Traceability
