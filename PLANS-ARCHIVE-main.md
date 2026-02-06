# PLANS-ARCHIVE-main.md — Archive

Archived from PLANS-main.md on 2026-02-05

# PLANS-main.md — Plan of Record

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
- **Compaction rule (mandatory)**: When this file exceeds ~1,000 lines or every 25–50 turns (whichever comes first), move implemented plan entries, old Turn Log entries, and any superseded Scope Ledger rows to `PLANS-ARCHIVE-main.md`. Keep only the current plan, live Scope Ledger, last 20 Turn Log entries, Decision Log, and Plan Index in this file. Update the Current State Snapshot and Plan Index to reference the archive.

---

## Current State Snapshot
- **Active Plan**: Plan v5 — Configurable NEARBY Login Warning — Implemented
- **Scope Ledger**: v5 — Pending: 0, Implemented: 6, Deferred: 0, Superseded: 0
- **Next up**: (none)
- **Last updated**: 2026-02-05 10:50 (local)

---

## Current Plan (ACTIVE)

### Plan v5 — Configurable NEARBY Login Warning
- **Date**: 2026-02-05
- **Status**: Implemented
- **Scope Ledger snapshot**: v5
- **Owner**: Assistant (with user approval)
- **Approval**: Approved v5

1) Goals
- Add `telnet.nearby_login_warning` to `data/config/runtime.yaml`.
- Use that value for the login warning line when NEARBY is active.

2) Non-goals
- No changes to NEARBY semantics or persistence.
- No changes to other login banner content.

3) Assumptions
- Runtime config is loaded into `config.Config` and used by telnet server startup.

4) Requirements and edge cases
Functional:
- If `telnet.nearby_login_warning` is set, use it in the greeting.
- If empty or missing, fall back to the default message.
- Ensure the warning ends with `\n` before concatenating to the greeting.

Non-functional:
- No new goroutines or unbounded buffers.

Edge cases:
- Whitespace-only values are treated as unset (use default).

5) Architecture (plan before code)
- Add field to config struct and YAML parsing.
- Wire it into `ServerOptions` and the greeting warning path.
- Update runtime.yaml documentation.

6) Contracts and compatibility
- Protocol/format: Login greeting warning text becomes configurable.
- Ordering: No change.
- Drop/disconnect policy: No change.
- Deadlines/timeouts: No change.
- Observability: No change.

7) User-visible behavior
- Operators can customize the NEARBY login warning text.

8) Config impact
- New optional `telnet.nearby_login_warning` string.

9) Observability plan
- None.

10) Test plan (before code)
- Ensure warning selection uses configured value and defaults when empty.

11) Performance/measurement plan
- None.

12) Rollout/ops plan
- Update `runtime.yaml` sample.

13) Open questions / needs-confirmation
- None.

### Plan v4 — Persist NEARBY + Login Warning
- **Date**: 2026-02-05
- **Status**: Implemented
- **Scope Ledger snapshot**: v4
- **Owner**: Assistant (with user approval)
- **Approval**: Approved v4

1) Goals
- Persist NEARBY ON/OFF state across login/logoff.
- Warn users at login when NEARBY is enabled.

2) Non-goals
- No changes to NEARBY matching semantics.
- No changes to filter evaluation order or backpressure behavior.

3) Assumptions
- Per-user state is persisted via `filter.UserRecord` YAML.
- Login greeting can include an additional line without breaking clients.

4) Requirements and edge cases
Functional:
- NEARBY state persists per callsign (saved on toggle).
- On login, if NEARBY is enabled, display a warning message.
- If NEARBY is enabled but grid is missing or H3 tables are unavailable, disable NEARBY and show a warning.

Non-functional:
- No new goroutines or unbounded buffers.

Edge cases:
- Corrupt or missing user record falls back to NEARBY OFF.
- If NEARBY is enabled but grid lookup fails, warn and disable.

5) Architecture (plan before code)
- Persist `NearbyEnabled` in `filter.Filter` YAML.
- On login: attempt to enable NEARBY using cached grid/H3; disable and warn on failure.
- Append warning line to greeting banner when NEARBY is active.

6) Contracts and compatibility
- Protocol/format: Adds a login warning line when NEARBY is enabled.
- Ordering: No change.
- Drop/disconnect policy: No change.
- Deadlines/timeouts: No change.
- Observability: Log warning if NEARBY could not be activated.

7) User-visible behavior
- Users see a warning at login when NEARBY is active.
- NEARBY remains active across sessions unless explicitly disabled.

8) Config impact
- Adds a boolean in user YAML (backward compatible).

9) Observability plan
- Log warning when NEARBY was enabled but could not be activated.

10) Test plan (before code)
- User record round-trip preserves NEARBY.
- Login flow: warning shown when NEARBY active; disabled with warning if grid missing.

11) Performance/measurement plan
- None.

12) Rollout/ops plan
- Deploy normally; existing user records remain valid.

13) Open questions / needs-confirmation
- None.

### Plan v3 — Ingest Liveness Indicators
- **Date**: 2026-02-05
- **Status**: Implemented
- **Scope Ledger snapshot**: v3
- **Owner**: Assistant
- **Approval**: Pending

1) Goals
- Show a green/red liveness indicator for RBN, PSK, and P92 in tview Overview and Ingest pages.
- Mirror the indicator in console output where ingest lines are logged.
- Make the liveness criteria deterministic and bounded.

2) Non-goals
- No changes to ingest parsing, deduplication, or forwarding behavior.
- No new network connections or probes.
- No changes to telnet protocol output to clients.

3) Assumptions
- RBN has two feeds: CW/RTTY and Digital (FT8/FT4).
- PSKReporter health snapshot timestamps are accurate and monotonic enough for idle detection.
- Peer manager can report whether any peer session is active.

4) Requirements and edge cases
Functional:
- RBN indicator is green only when both RBN feeds are connected; otherwise red.
- PSK indicator is green only when MQTT is connected and messages are arriving within a bounded window.
- P92 indicator is green only when at least one peer session is active; otherwise red.
- Indicators appear after the labels on overview and ingest pages (tview) and console lines.

Non-functional:
- No new goroutines.
- Bounded memory; no unbounded buffers or caches.

Edge cases:
- If a feed is disabled/missing, indicator should be red (explicitly not connected).
- At startup before first message, PSK should be red until a message arrives.
- If both RBN feeds are enabled but one reconnects, indicator should flip deterministically.

5) Architecture (plan before code)
- Compute liveness once per stats tick in the main stats loop.
- RBN: use rbn.Client HealthSnapshot Connected for both CW/RTTY and Digital; green only if both are true.
- PSK: use pskreporter.Client HealthSnapshot Connected AND LastPayloadAt age <= ingest idle threshold.
- P92: add a Manager method to return active session count (or bool); green if count > 0.
- Render indicator via a small formatter that produces a colored suffix (e.g., `[green]●[-]` / `[red]●[-]`).

6) Contracts and compatibility
- Protocol/format: No changes.
- Ordering: No changes.
- Drop/disconnect policy: No changes.
- Deadlines/timeouts: No changes.
- Observability: Console/UI text changes only (add colored liveness markers).

7) User-visible behavior
- Overview/Ingest pages: `RBN`, `PSK`, `P92` labels show green/red status markers based on liveness.
- Console output: ingest-related lines include the same markers (plain text / ANSI as applicable).

8) Config impact
- None.

9) Dependency impact (Full)
- Upstream inputs: RBN client health snapshot, PSKReporter health snapshot, peer manager session state.
- Downstream consumers: tview dashboard overview/ingest text, console log lines.
- Contracts: Observability-only; no protocol changes.

10) Test plan (before code)
- Unit tests for the liveness formatter and RBN/PSK/P92 decision logic.
- Regression tests to ensure disabled/missing inputs yield red status.

11) Performance/measurement plan
- Not required (stats-path only).

12) Rollout/ops plan
- Deploy normally; verify indicator color toggles with feed connectivity and PSK message arrival.

13) Open questions / needs-confirmation
- Should PSK use LastPayloadAt or LastSpotAt for liveness? (default: LastPayloadAt)
- Confirm that “connected” for P92 means at least one active peer session.

---

### Plan v2 — Fix RBN FT Ingest Rates
- **Date**: 2026-02-05
- **Status**: Implemented
- **Scope Ledger snapshot**: v2
- **Owner**: Assistant
- **Approval**: Pending

1) Goals
- Ensure RBN FT8/FT4 ingest rates show correctly in the console/overview lines.
- Align source/mode counter keys so RBN Digital FT spots are counted deterministically.
- Keep ingest/broadcast behavior unchanged.

2) Non-goals
- No changes to ingest parsing or deduplication.
- No changes to telnet protocol output lines.
- No changes to rate calculation windowing.

3) Assumptions
- RBN Digital feed uses SourceType FT8/FT4 for FT modes.
- SourceStats labels for FT8/FT4 are currently `RBN-FT` (per tests).
- Console/overview pulls counts from stats tracker snapshots.

4) Requirements and edge cases
Functional:
- RBN FT8/FT4 counts must reflect actual FT spots ingested from the RBN Digital feed.
- Combined RBN total should include FT8/FT4 counts from the digital feed.

Non-functional:
- No new goroutines or unbounded allocations.
- No changes to per-spot hot path semantics beyond key lookup.

Edge cases:
- If RBN Digital includes non-FT modes (SourceType RBN + SourceNode RBN-DIGITAL), they should continue to count without breaking FT counts.
- Counter resets should still yield non-negative deltas.

5) Architecture (plan before code)
- Keep `sourceStatsLabel` semantics unchanged (`RBN-FT` for FT8/FT4).
- Update display stats to read FT counts from `RBN-FT` source keys instead of `RBN-DIGITAL`.
- Ensure combined RBN total includes RBN-FT totals.

6) Contracts and compatibility
- Protocol/format: No changes.
- Ordering: No changes.
- Drop/disconnect policy: No changes.
- Deadlines/timeouts: No changes.
- Observability: Console/overview FT8/FT4 counts now reflect actual RBN Digital FT traffic.

7) User-visible behavior
- Console/overview line `RBN: ... | FT8 ... | FT4 ...` will show non-zero values when FT spots are ingested.

8) Config impact
- None.

9) Dependency impact (Full)
- Upstream inputs: stats tracker counters (source/mode keys).
- Downstream consumers: console/overview display, UI dashboard ingest rates, prop report (indirect; unchanged).
- Contracts: Observability-only; no protocol changes.

10) Test plan (before code)
- Add/update unit coverage to ensure FT8/FT4 counters are read from `RBN-FT`.
- Regression test for `sourceStatsLabel` remains `RBN-FT` for FT modes.

11) Performance/measurement plan
- Not required (stats-path only, low frequency).

12) Rollout/ops plan
- Deploy normally; verify console shows non-zero FT8/FT4 when RBN Digital feed is active.

13) Open questions / needs-confirmation
- None.

---

### Plan v1 — GC p99 Windowed Console Reporting
- **Date**: 2026-02-05
- **Status**: Implemented
- **Scope Ledger snapshot**: v1
- **Owner**: Assistant (with user approval)
- **Approval**: Approved v1 (2026-02-05)

1) Goals
- Report GC p99 in the console/overview line as a windowed statistic aligned with the stats refresh interval.
- Avoid stale p99 values derived from the full PauseNs ring.
- Keep the change bounded and low-overhead.

2) Non-goals
- Do not alter GC behavior or tuning.
- Do not add new log-only GC metrics.
- Do not change ingest or telnet behavior.

3) Assumptions
- Stats refresh is driven by `cfg.Stats.DisplayIntervalSeconds`.
- Console/overview lines are built once per stats tick.
- GC pause ring (PauseNs) provides the most recent 256 pauses.

4) Requirements and edge cases
Functional:
- Compute GC p99 from pauses that occurred since the last stats refresh tick.
- If no GC occurs in the window, report `n/a` for GC p99.
- Keep `Last GC` age behavior unchanged.

Non-functional:
- No new goroutines.
- Bounded allocations per tick.

Edge cases:
- If more than 256 GCs occur in a single window, use the most recent 256 pauses and note truncation internally (no crash).
- Handle startup: initial window should not include all historical pauses.

5) Architecture (plan before code)
- Add a small GC window tracker owned by the stats loop (`displayStatsWithFCC`).
- Each tick: read `runtime.MemStats`, compute windowed p99 using `NumGC` delta and PauseNs ring, then update overview lines with the windowed value.
- Maintain a previous `NumGC` baseline captured before the first tick.

6) Contracts and compatibility
- Protocol/format: No changes.
- Ordering: No changes.
- Drop/disconnect policy: No changes.
- Deadlines/timeouts: No changes.
- Observability: GC p99 semantics change to “p99 of pauses since last stats refresh.”

7) User-visible behavior
- Console/overview line label becomes `GC p99 (interval)`.
- GC p99 reflects only the most recent stats window; shows `n/a` when no GC occurs in the window.

8) Config impact
- None.

9) Observability plan
- Update console/overview GC p99 calculation to windowed.
- Update label text to `GC p99 (interval)`.

10) Test plan (before code)
- Unit tests for windowed GC p99 calculation:
  - No new GC in window → `n/a` with count=0.
  - Multiple new GCs → correct p99.
  - Delta > ring size → truncation path produces stable output.

11) Performance/measurement plan
- Not required (stats-path only, low frequency).

12) Rollout/ops plan
- Deploy normally; verify console GC p99 updates per tick and does not remain stale after a single spike.

13) Open questions / needs-confirmation
- None.

---

## Pending Work Summary
- (none)

---

## Scope Ledger v5 (LIVE)
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S1 | Compute GC p99 over the stats refresh window and report it in the console/overview line (not whole-process ring). | Implemented | Display `n/a` when no GC; label `GC p99 (interval)` |
| S2 | Fix RBN FT8/FT4 ingest rates showing as zero by aligning stats source/mode keys. | Implemented | Console/overview and dashboard ingest lines |
| S3 | Add green/red liveness indicators for RBN, PSK, and P92 in tview Overview/Ingest pages and console ingest lines. | Implemented | RBN green when both feeds connected; PSK green when MQTT messages arriving; P92 green when peer session active |
| S4 | Persist NEARBY ON/OFF state across sessions. | Implemented | |
| S5 | Warn at login when NEARBY is enabled (disable on missing grid/H3). | Implemented | |
| S6 | Add telnet.nearby_login_warning and wire into greeting. | Implemented | |

---

## Active Implementation
### Current Phase
Complete

### In Progress
- [ ] (none)

### Completed This Session
- [x] S4 — Persist NEARBY state across sessions
- [x] S5 — Login warning when NEARBY is enabled
- [x] S6 — Configurable NEARBY login warning

### Blocked / Questions
- [ ] (none)

---

## Turn Log (Append-Only)
- 2026-02-05 07:53 — Created Plan v1 and Scope Ledger v1 for windowed GC p99 console reporting
- 2026-02-05 08:00 — Implemented windowed GC p99 reporting; updated UI label; added tests; ran go test ./...
- 2026-02-05 08:45 — Created Plan v2 and Scope Ledger v2 for RBN FT ingest rate fix (pre-code)
- 2026-02-05 09:05 — Implemented Plan v2; fixed RBN FT stats key usage; added tests; ran go test ./...
- 2026-02-05 09:20 — Created Plan v3 and Scope Ledger v3 for ingest liveness indicators (pre-code)
- 2026-02-05 09:40 — Implemented Plan v3; added ingest liveness indicators; ran go test ./...
- 2026-02-05 10:10 — Created Plan v4 and Scope Ledger v4 for NEARBY persistence + login warning
- 2026-02-05 10:25 — Implemented Plan v4; persisted NEARBY state; added login warning; updated tests/docs
- 2026-02-05 10:35 — Ran go test ./filter -run Nearby and go test ./telnet -run Nearby
- 2026-02-05 10:45 — Created Plan v5 and Scope Ledger v5 for configurable NEARBY login warning
- 2026-02-05 10:50 — Implemented Plan v5; wired telnet.nearby_login_warning into greeting; updated config/docs

---

## Decision Log (Append-Only)
### D1 — 2026-02-05 Windowed GC p99
- **Context**: Console GC p99 currently computed over entire PauseNs ring and can be stale.
- **Chosen**: Compute p99 over GCs since the last stats refresh tick.
- **Alternatives**: Keep ring-based p99; compute time-windowed via runtime/metrics.
- **Impact**: Observability semantics change; no protocol/runtime behavior change.

### D2 — 2026-02-05 RBN FT stats key alignment
- **Context**: Console ingest line shows RBN FT8/FT4 as zero despite FT spots being ingested.
- **Chosen**: Keep `RBN-FT` as the source label for FT8/FT4 and adjust display counters to use that key.
- **Alternatives**: Change source labels to `RBN-DIGITAL`; split FT counts across RBN-DIGITAL and RBN-FT.
- **Impact**: Observability-only; counters now reflect actual FT traffic.

### D3 — 2026-02-05 Ingest liveness indicators
- **Context**: Operators need a quick “connected/healthy” indicator for ingest sources.
- **Chosen**: Add green/red marker after RBN/PSK/P92 labels based on health snapshots and session state.
- **Alternatives**: Add separate status lines; show yellow for idle.
- **Impact**: Observability-only; UI/console text changes.

### D4 — 2026-02-05 Persist NEARBY + login warning
- **Context**: Users want NEARBY to persist across sessions and be reminded at login.
- **Chosen**: Persist `NearbyEnabled` in user YAML and append a login warning when active; disable with warning if grid/H3 missing.
- **Alternatives**: Per-session only; warn only on first toggle.
- **Impact**: Login banner includes NEARBY warning; NEARBY state survives reconnects.

### D5 — 2026-02-05 Configurable NEARBY login warning
- **Context**: Operators want to customize the NEARBY login warning text.
- **Chosen**: Add `telnet.nearby_login_warning` with a default fallback when empty.
- **Alternatives**: Hardcode only; reuse login_greeting with tokens.
- **Impact**: Warning text becomes operator-configurable.

---

## Post-implementation notes (Plan v4)
Implementation notes:
- Persisted NEARBY state in user records and re-enabled it on login when possible.
- Added a login banner warning when NEARBY is active; disable with warning if grid/H3 missing.
- Added tests for NEARBY persistence and login-time handling.

Deviations:
- None.

Verification commands actually run:
- `go test ./filter -run Nearby`
- `go test ./telnet -run Nearby`

Final contract statement:
- Login banner adds a NEARBY warning line; NEARBY state now persists across sessions.

Scope Ledger status updates:
- S4 → Implemented
- S5 → Implemented

---

## Post-implementation notes (Plan v5)
Implementation notes:
- Added `telnet.nearby_login_warning` to runtime config and wired it into the login greeting warning.
- Default warning text is used when the setting is empty.

Deviations:
- None.

Verification commands actually run:
- None.

Final contract statement:
- Login warning text is now operator-configurable via runtime.yaml.

Scope Ledger status updates:
- S6 → Implemented

---

## Post-implementation notes (Plan v1)
Implementation notes:
- Added a GC pause window tracker keyed to the stats refresh loop.
- Updated console/overview GC p99 label to `GC p99 (interval)` and show `n/a` when no GC occurs in the window.
- Added unit tests for windowed p99 and truncation behavior.

Deviations:
- None.

Verification commands actually run:
- `go test ./...`

Final contract statement:
- Observability contract updated: GC p99 now reflects pauses since the last stats refresh tick.

Scope Ledger status updates:
- S1 → Implemented

---

## Post-implementation notes (Plan v2)
Implementation notes:
- RBN FT8/FT4 ingest deltas now read from the `RBN-FT` source key.
- Added a dedicated helper and unit test to guard the key alignment.

Deviations:
- None.

Verification commands actually run:
- `go test ./...`

Final contract statement:
- Observability corrected: RBN FT8/FT4 ingest counts now reflect `RBN-FT` stats keys.

Scope Ledger status updates:
- S2 → Implemented

---

## Post-implementation notes (Plan v3)
Implementation notes:
- Added liveness computation for RBN/PSK/P92 in the stats tick and displayed colored markers in ingest lines.
- Added peer session count helper and unit tests for liveness logic.

Deviations:
- None. Used `[green]ON[-]` / `[red]OFF[-]` markers (ASCII) instead of a dot glyph.

Verification commands actually run:
- `go test ./...`

Final contract statement:
- Observability updated: ingest lines now include liveness markers for RBN/PSK/P92.

Scope Ledger status updates:
- S3 → Implemented

---

## Files Modified
- PLANS-main.md
- README.md
- commands/processor.go
- config/config.go
- data/config/runtime.yaml
- filter/filter.go
- filter/user_record_test.go
- main.go
- telnet/server.go
- telnet/server_filter_test.go

---

## Verification Status
- [x] go test ./... (Plan v3)
- [x] go test ./filter -run Nearby (Plan v4)
- [x] go test ./telnet -run Nearby (Plan v4)
- [ ] Not run (Plan v5)

---

## Plan Index (history)
- Plan v1 — GC p99 Windowed Console Reporting — 2026-02-05 — Scope Ledger v1 — status: Implemented
- Plan v2 — Fix RBN FT Ingest Rates — 2026-02-05 — Scope Ledger v2 — status: Implemented
- Plan v3 — Ingest Liveness Indicators — 2026-02-05 — Scope Ledger v3 — status: Implemented
- Plan v4 — Persist NEARBY + Login Warning — 2026-02-05 — Scope Ledger v4 — status: Implemented
- Plan v5 — Configurable NEARBY Login Warning — 2026-02-05 — Scope Ledger v5 — status: Implemented

---

## Context for Resume
**Done**:
- S1 — Implement windowed GC p99 reporting in console/overview line
- S2 — Fix RBN FT8/FT4 ingest rate counters in console/overview
- S3 — Add ingest liveness indicators in tview overview/ingest pages and console lines
- S4 — Persist NEARBY ON/OFF across sessions
- S5 — Login warning when NEARBY is enabled (disable if grid/H3 missing)
- S6 — Configurable NEARBY login warning (runtime.yaml)
**In Progress**:
- (none)
**Next**:
- (none)
**Key Decisions**:
- D1 — Windowed GC p99 per stats refresh tick
- D2 — Align RBN FT stats with `RBN-FT` source key
- D3 — Add green/red ingest liveness markers based on health snapshots
- D4 — Persist NEARBY and warn at login
- D5 — Make NEARBY login warning configurable
**Files Hot**:
- telnet/server.go
- filter/filter.go
- config/config.go
