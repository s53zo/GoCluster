# PLANS-console-events.md — Plan of Record (Compact)

## Current State Snapshot
- Active Plan: Plan v4 — Events Log Color Regression Hardening — Implemented
- Scope Ledger: v9 — Pending: 0, Implemented: 6, Deferred: 0, Superseded: 0
- Next up:
  - Optional manual runtime check with startup logs to confirm consistent white/plain events rendering.
  - No remaining required implementation scope.
  - Continue monitoring for future formatting regressions.
- Last updated: 2026-02-06 15:44 (local)

---

## Current Plan (ACTIVE)

### Plan v4 — Events Log Color Regression Hardening
- Date: 2026-02-06
- Status: Implemented
- Scope Ledger snapshot: v9
- Owner: Assistant (with user approval)
- Approval: Approved v4

Pre-Implementation Gate (Mandatory):
- Classification: Non-trivial
- Scope Ledger IDs approved: S6
- Approval evidence (quote + timestamp): "Approved v4" — 2026-02-06 15:41
- Ready to implement: Yes

Goals:
- Ensure Events page system log renders deterministic plain white text, even if incoming lines contain bracket/tag-like patterns.
- Preserve source-log semantics (no destructive stripping of message content).

Non-Goals:
- No changes to event routing between pages.
- No protocol/backend behavior changes.

Requirements/Edge Cases:
- Events pane must not apply tview dynamic-color parsing to system log payload.
- Bracket/tag-like content in logs must render literally and remain readable.
- Existing overview/info pane dynamic colors remain unchanged.

Architecture (bounds/backpressure/shutdown):
- UI rendering-path hardening for Events pane only.
- Keep bounded streamPanel buffering/scheduling unchanged.
- Keep existing `streamPanel` bounded buffers and scheduler unchanged.
- No new goroutines/queues/channels.

Contracts:
- No contract changes.

Tests:
- `go test ./ui`
- Add/adjust targeted test to ensure events log payload with tag-like tokens renders literally (no color parsing side effects).

Acceptance Criteria by Scope Item:
- S6 — Events pane renders startup/system logs in consistent white/plain text with literal bracket content; no unintended gray-state shifts.

Rollout/Ops:
- None.

---

## Post-code (Plan v4)
Deviations:
- None.

Verification commands actually run:
- `gofmt -w ui\dashboard_v2.go ui\dashboard_v2_test.go`
- `go test ./ui`

Final contract statement:
- No contract changes.

Scope Ledger status updates:
- S6 — Implemented.

---

## Scope Ledger v9 (LIVE)
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S1 | Add new `events` console page and navigation wiring. | Implemented | Added page construction, keybindings (F5/`e`), and page-order/config support. |
| S2 | Reuse Overview section content on Events page. | Implemented | Added events overview summary boxes using existing overview parsing helpers. |
| S3 | Render events stream in scrollable panel via existing Ingest scroll helper/primitive (`streamPanel`). | Implemented | `eventsPanel` uses `newStreamPanel` and shared `appendStream` scheduling path. |
| S4 | Restrict events panel feed to system stream only; keep drop/correction/unlicensed/harmonic/reputation on their original pages. | Implemented | Removed non-system routing into events panel; added regression test coverage. |
| S5 | Enforce cross-page event color consistency mode (A=white system stream, B=gray all event panes, or C=plain-text emission at source). | Implemented | Mode C implemented: removed source color-tag generation from event log emitters; added formatter/reporter tests. |
| S6 | Harden Events pane rendering so system logs are always plain/white and tag-like content is shown literally. | Implemented | Disabled dynamic-color parsing for events stream view and added literal bracket rendering test. |

---

## Turn Log (Append-Only, last 25 kept)
- 2026-02-06 15:08 — Initial branch plan created.
- 2026-02-06 15:13 — Corrected process state: approval reset to Pending; scope ledger presented for user review.
- 2026-02-06 15:16 — Approval received ("Approved v1"); implementation started for S1-S3.
- 2026-02-06 15:13 — Implemented events page scope S1-S3 and verified with gofmt + `go test ./ui ./config`.
- 2026-02-06 15:18 — Corrected runtime config dependency by adding `events` to `data/config/app.yaml` page list.
- 2026-02-06 15:20 — Logged regression report and prepared Plan v2 with pending approval for scoped routing fix (S4).
- 2026-02-06 15:22 — Approval received ("Approved v2"); started implementing S4.
- 2026-02-06 15:23 — Implemented S4 and verified with gofmt + `go test ./ui`.
- 2026-02-06 15:27 — Logged new color-consistency request as Plan v3 with pending scope item S5.
- 2026-02-06 15:29 — Expanded S5 options to include source-level normalization (mode C) per user feedback.
- 2026-02-06 15:31 — Clarified mode C semantics: no source colorization (not downstream stripping).
- 2026-02-06 15:34 — Approval received for Mode C (`Approved v3`); implementation started for S5.
- 2026-02-06 15:36 — Implemented S5 (Mode C) and verified with gofmt + `go test . ./ui`.
- 2026-02-06 15:40 — Logged screenshot-confirmed color regression and created Plan v4 scope item S6.
- 2026-02-06 15:42 — Approval received ("Approved v4"); started implementing S6.
- 2026-02-06 15:44 — Implemented S6 and verified with gofmt + `go test ./ui`.

---

## Decision Log (Append-Only)
### D1 — 2026-02-06 Reuse Existing Scrolling Primitive
- Context: User requested identical scrolling behavior as Ingest and no replicated code.
- Chosen: Use existing `streamPanel` helper and append scheduler path.
- Alternatives: Copy scroll logic; create new generalized helper in this change.
- Impact: Lower regression risk, immediate behavior parity, bounded memory preserved.

### D2 — 2026-02-06 Events Routing Scope Correction
- Context: Drops and related ingest/pipeline streams were incorrectly mirrored into events page.
- Chosen: Restrict events stream input to `AppendSystem`/`SystemWriter` only.
- Alternatives: Keep merged stream; add per-type filter toggle in config.
- Impact: Restores page responsibility boundaries and removes unintended cross-page duplication.

### D3 — 2026-02-06 Color Consistency Strategy (Pending)
- Context: Raw system log lines can trigger dynamic-color parsing artifacts and create inconsistent white/gray text.
- Chosen: Pending user selection.
- Alternatives:
  - A: Force system stream plain/white by escaping tag markers in system log path.
  - B: Force gray styling across all event panes.
  - C: Ensure source emission is plain text from origin (no color markup inserted).
- Impact: A is minimal/local; B changes look broadly; C preserves log semantics and avoids downstream stripping.

### D4 — 2026-02-06 Events Rendering Hardening
- Context: Despite source plain-text emission, Events pane still shows intermittent gray lines in real startup logs.
- Chosen: Pending approval.
- Alternatives:
  - Disable dynamic color parsing for events log pane and render payload literally.
  - Escape bracket/tag tokens only in system-writer path.
- Impact: Deterministic appearance and stronger isolation from tag-like log content.

---

## Plan Index (history)
- Plan v1 — Add Events Console Page — 2026-02-06 — Scope Ledger v3 — status: Implemented
- Plan v2 — Events Stream Scope Correction — 2026-02-06 — Scope Ledger v5 — status: Implemented
- Plan v3 — Cross-Page Event Color Consistency — 2026-02-06 — Scope Ledger v7 — status: Implemented
- Plan v4 — Events Log Color Regression Hardening — 2026-02-06 — Scope Ledger v9 — status: Implemented

---

## Context for Resume
**Done**:
- Identified regression source: events panel was fed by non-system append paths.
- Prepared scoped fix plan item S4.
- Implemented S4 by limiting events panel stream to `AppendSystem` path only.
- Added UI regression test for events stream routing behavior.
- Verified with `go test ./ui`.
- Implemented Mode C by removing event-source color-tag generation and emitting plain text at source.
- Added unit tests for plain-text event formatter outputs and reporter emission.
- Verified with `go test . ./ui`.
- Implemented events-pane rendering hardening (disable dynamic colors on events stream) to ensure literal bracket/log rendering.
- Added targeted UI test for literal bracket content in events stream.
- Verified with `go test ./ui`.
**In Progress**:
- None.
**Next**:
- Optional manual startup-log visual check in terminal.
**Key Decisions**:
- Reuse `streamPanel` instead of introducing duplicate scroll code.
**Files Hot**:
- `PLANS-console-events.md`
- `ui/dashboard_v2.go`
- `config/config.go`
- `config/ui_v2_test.go`
- `ui/dashboard_v2_test.go`

---

## Compaction Rule
- Keep only: Active Plan, live Scope Ledger, last 25 Turn Log entries, Decision Log, Plan Index, Current State Snapshot, Context for Resume.
- Move older plans/logs/superseded scope rows to `PLANS-ARCHIVE-console-events.md`.
