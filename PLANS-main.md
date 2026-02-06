# PLANS-main.md — Plan of Record (Compact)

## Current State Snapshot
- Active Plan: None (all implemented)
- Scope Ledger: v5 — Pending: 0, Implemented: 6, Deferred: 0, Superseded: 0
- Next up: (none)
- Last updated: 2026-02-05 11:25 (local)

---

## Current Plan (MOST RECENT)

### Plan v5 — Configurable NEARBY Login Warning
- Date: 2026-02-05
- Status: Implemented
- Scope Ledger snapshot: v5
- Owner: Assistant (with user approval)
- Approval: Approved v5

Goals:
- Add `telnet.nearby_login_warning` to `data/config/runtime.yaml`.
- Use it for the login warning line when NEARBY is active.

Non-Goals:
- No changes to NEARBY semantics or other login banner content.

Requirements/Edge Cases:
- Empty/whitespace value ⇒ default warning; ensure warning ends with `\n`.

Architecture (bounds/backpressure/shutdown):
- Config field → server options → greeting warning path.

Contracts:
- Protocol/format: login warning text configurable. No other contract changes.

Tests:
- Verify configured value and default fallback.

Rollout/Ops:
- Update `runtime.yaml` sample.

---

## Post-code (Plan v5)
Deviations:
- None.

Verification commands actually run:
- None.

Final contract statement:
- Login warning text is operator-configurable via `runtime.yaml`.

Scope Ledger status updates:
- S6 → Implemented

---

## Scope Ledger v5 (LIVE)
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S1 | Windowed GC p99 in console/overview | Implemented | `GC p99 (interval)`; `n/a` if none |
| S2 | Fix RBN FT8/FT4 ingest rates (stats key) | Implemented | Use `RBN-FT` counters |
| S3 | Liveness indicators for RBN/PSK/P92 | Implemented | Green/red status markers |
| S4 | Persist NEARBY ON/OFF across sessions | Implemented | |
| S5 | Login warning when NEARBY enabled | Implemented | Disable if grid/H3 missing |
| S6 | Configurable NEARBY login warning | Implemented | `telnet.nearby_login_warning` |

---

## Turn Log (Append-Only, last 25 kept)
- 2026-02-05 07:53 — Created Plan v1 and Scope Ledger v1 for windowed GC p99
- 2026-02-05 08:00 — Implemented Plan v1; added tests; ran `go test ./...`
- 2026-02-05 08:45 — Created Plan v2 and Scope Ledger v2 for RBN FT fix
- 2026-02-05 09:05 — Implemented Plan v2; added tests; ran `go test ./...`
- 2026-02-05 09:20 — Created Plan v3 and Scope Ledger v3 for ingest liveness
- 2026-02-05 09:40 — Implemented Plan v3; ran `go test ./...`
- 2026-02-05 10:10 — Created Plan v4 and Scope Ledger v4 for NEARBY persistence + warning
- 2026-02-05 10:25 — Implemented Plan v4; updated tests/docs
- 2026-02-05 10:35 — Ran `go test ./filter -run Nearby` and `go test ./telnet -run Nearby`
- 2026-02-05 10:45 — Created Plan v5 and Scope Ledger v5 for configurable warning
- 2026-02-05 10:50 — Implemented Plan v5; updated config/docs

---

## Decision Log (Append-Only)
### D1 — 2026-02-05 Windowed GC p99
- Context: GC p99 was stale when computed over full ring.
- Chosen: windowed p99 per stats refresh tick.
- Alternatives: keep ring-based; use runtime/metrics.
- Impact: observability semantics only.

### D2 — 2026-02-05 RBN FT stats key alignment
- Context: FT8/FT4 counters showed zero.
- Chosen: keep `RBN-FT` labels; display reads that key.
- Alternatives: rename labels; split counts.
- Impact: observability-only.

### D3 — 2026-02-05 Ingest liveness indicators
- Context: need quick health status in UI/console.
- Chosen: green/red markers from health snapshots.
- Alternatives: separate status lines; add yellow idle.
- Impact: UI/console text only.

### D4 — 2026-02-05 Persist NEARBY + login warning
- Context: users want NEARBY persistence + reminder.
- Chosen: persist per-user; warn on login; disable if grid/H3 missing.
- Alternatives: session-only; warn only on toggle.
- Impact: login banner includes warning.

### D5 — 2026-02-05 Configurable NEARBY warning
- Context: operators want custom warning text.
- Chosen: add `telnet.nearby_login_warning`.
- Alternatives: hardcode; reuse login_greeting tokens.
- Impact: warning text configurable.

---

## Plan Index (history)
- Plan v1 — GC p99 Windowed Console Reporting — 2026-02-05 — Scope Ledger v1 — Implemented (archived)
- Plan v2 — Fix RBN FT Ingest Rates — 2026-02-05 — Scope Ledger v2 — Implemented (archived)
- Plan v3 — Ingest Liveness Indicators — 2026-02-05 — Scope Ledger v3 — Implemented (archived)
- Plan v4 — Persist NEARBY + Login Warning — 2026-02-05 — Scope Ledger v4 — Implemented (archived)
- Plan v5 — Configurable NEARBY Login Warning — 2026-02-05 — Scope Ledger v5 — Implemented

---

## Context for Resume
**Done**:
- S1–S6 — All items implemented
**In Progress**:
- (none)
**Next**:
- (none)
**Key Decisions**:
- D1–D5 — See Decision Log
**Files Hot**:
- (none)

---

## Compaction Rule
- Keep only: most recent plan, live Scope Ledger, last 25 Turn Log entries, Decision Log, Plan Index, Current State Snapshot, Context for Resume.
- Move older plans/logs/superseded scope rows to `PLANS-ARCHIVE-main.md`.
