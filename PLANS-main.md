# PLANS-main.md — Plan of Record (Compact)

## Current State Snapshot
- Active Plan: None (all implemented)
- Scope Ledger: v7 — Pending: 0, Implemented: 8, Deferred: 0, Superseded: 0
- Next up: (none)
- Last updated: 2026-02-06  (local)

---

## Current Plan (MOST RECENT)

### Plan v7 — NEARBY Persistence Until Explicit OFF
- Date: 2026-02-06
- Status: Implemented
- Scope Ledger snapshot: v7
- Owner: Assistant (with user approval)
- Approval: Approved v7

Goals:
- Ensure `NEARBY=ON` persists until user issues `PASS NEARBY OFF` (or other explicit reset commands).

Non-Goals:
- No change to NEARBY matching semantics or H3 resolution.
- No change to backpressure, queueing, or shutdown behavior.

Requirements/Edge Cases:
- Persisted `nearby_enabled` must not be toggled off during login bootstrap failures.
- Session-only data (`NearbySnapshot`, cached cells) remains non-persistent.
- If grid/H3 cannot bootstrap at login, keep NEARBY flag ON and emit warning; do not persist OFF.
- Explicit user actions (`PASS NEARBY OFF`, `RESET FILTER`, `RESET DEFAULT`) still disable NEARBY.
- Determinism: while ON but not bootstrapped, spot matching remains deterministic (nearby path yields no matches until cells become valid).

Architecture (bounds/backpressure/shutdown):
- Modify login bootstrap path to avoid calling `DisableNearby()` for transient bootstrap failures.
- Introduce warning path that reports NEARBY ON but inactive when bootstrap prerequisites are missing.
- Keep existing in-memory bounds and no additional goroutines/channels.
- No concurrency or buffering changes.

Contracts:
- No protocol/format changes. No drop/backpressure changes.
- User-visible behavior change: login no longer auto-turns NEARBY OFF on missing grid/H3; NEARBY remains ON until explicit user disable.

Dependency Impact (Full):
- Upstream: login record load + optional grid lookup.
- Shared component: telnet login bootstrap (`applyNearbyLoginState`) and filter state transitions.
- Downstream: spot matching path under `NearbyEnabled`, greeting/warning text, persisted user record.
- Contract statement: location-filter state persistence semantics changed (auto-disable removed from login bootstrap failures).

Tests:
- Add/adjust login tests:
  - Missing grid while `NearbyEnabled=true` must keep NEARBY ON and return warning.
  - Invalid/failed H3 bootstrap while `NearbyEnabled=true` must keep NEARBY ON and return warning.
  - Valid bootstrap keeps existing warning and ON state.
- Keep/confirm persistence round-trip test for `nearby_enabled`.
- Verification:
  - `go test ./telnet -run Nearby`
  - `go test ./filter -run Nearby`

Rollout/Ops:
- No config schema changes.
- Operator-visible behavior: warning may indicate ON-but-inactive state instead of forced disable.

---

## Post-code (Plan v7)
Deviations:
- None.

Verification commands actually run:
- `go test ./telnet -run Nearby`
- `go test ./filter -run Nearby`

Final contract statement:
- Login bootstrap no longer persists `NEARBY=OFF` on temporary grid/H3 bootstrap failures; `NEARBY` remains ON until explicit user disable, with an ON-but-inactive warning when prerequisites are missing.

Scope Ledger status updates:
- S8 → Implemented

---

## Scope Ledger v7 (LIVE)
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S1 | Windowed GC p99 in console/overview | Implemented | `GC p99 (interval)`; `n/a` if none |
| S2 | Fix RBN FT8/FT4 ingest rates (stats key) | Implemented | Use `RBN-FT` counters |
| S3 | Liveness indicators for RBN/PSK/P92 | Implemented | Green/red status markers |
| S4 | Persist NEARBY ON/OFF across sessions | Implemented | |
| S5 | Login warning when NEARBY enabled | Implemented | Disable if grid/H3 missing |
| S6 | Configurable NEARBY login warning | Implemented | `telnet.nearby_login_warning` |
| S7 | Fix NEARBY persistence regression on load | Implemented | Remove normalization reset |
| S8 | Remove login auto-disable persistence path for NEARBY bootstrap failures | Implemented | Keep ON until explicit user OFF |

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
- 2026-02-06 — Implemented Plan v6; fix NEARBY persistence reset
- 2026-02-06 — Implemented Plan v7; removed login auto-disable persistence path and validated NEARBY tests

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
- Plan v6 — Fix NEARBY Persistence Regression — 2026-02-06 — Scope Ledger v6 — Implemented
- Plan v7 — NEARBY Persistence Until Explicit OFF — 2026-02-06 — Scope Ledger v7 — Implemented

---

## Context for Resume
**Done**:
- S1–S8 — All items implemented
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
