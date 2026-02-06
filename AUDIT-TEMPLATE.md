# AUDIT-TEMPLATE.md — Self-Audit + PR Summary

## Self-Audit (pass/fail)
- First code edit occurred after approval (`Approval: Approved vN`): Yes/No (if `No`, audit fails)
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

## Process Compliance (required)
- Plan approved before first code edit: Yes/No
- If `No`: stop and remediate before continuing implementation.
- Change classification recorded (`Small` with justification or `Non-trivial`): Yes/No
- Pre-code plan completeness verified (goals/non-goals/requirements/architecture/contracts/tests/rollout): Yes/No
- Scope Ledger items atomic + testable + acceptance criteria present: Yes/No
- Instruction precedence respected (`AGENTS.md` workflow over global defaults when conflicting): Yes/No

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
