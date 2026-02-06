# PLANS-log-dedupe.md — Plan of Record (Compact)

## Current State Snapshot
- Active Plan: Plan v2 — Drop-Noise Dedupe Redesign — Implemented
- Change Classification: Non-trivial
- Scope Ledger: v2 — Pending: 0, Implemented: 6, Deferred: 0, Superseded: 0
- Next up:
  - Observe live console noise levels and tune window if operators request.
- Last updated: 2026-02-06 (local)

---

## Current Plan (ACTIVE)

### Plan v2 — Drop-Noise Dedupe Redesign
- Date: 2026-02-06
- Status: Implemented
- Scope Ledger snapshot: v2
- Owner: Assistant (with user approval)
- Approval: Approved v2

Goals:
- Reduce repetitive console noise from dropped calls related to:
  - CTY unknown checks,
  - CTY invalid calls (`>=3 leading letters`),
  - unlicensed spotter drops.
- Preserve deterministic and bounded behavior under sustained high-rate drops.
- Keep all ingest/drop correctness semantics unchanged.
- Add a YAML knob so operators can tune/disable dedupe without code changes.

Non-Goals:
- No changes to drop decisions or filtering logic.
- No changes to telnet protocol lines sent to clients.
- No new background workers/timers/channels for dedupe.

Requirements/Edge Cases:
- Dedupe must apply only to target line families above; all other drop lines remain unchanged.
- Dedupe must work for both plain text and color-tagged UI lines.
- Must periodically re-emit an event after suppression window expiry (not silent forever).
- Re-emitted line must include suppression context (count + window).
- State must remain bounded by explicit max-key cap with deterministic eviction.
- Unknown/malformed lines must bypass dedupe (fail-open to visibility).
- Must be concurrency-safe for reporter call paths.
- Must not add unbounded alloc growth or blocking behavior.
- YAML knob behavior:
  - `0` disables dedupe entirely (full raw visibility).
  - `>0` enables dedupe with that suppression window in seconds.
  - invalid/negative values must fail config load with explicit error.

Architecture (bounds/backpressure/shutdown):
- Reporter-boundary dedupe component (`dropLogDeduper`) with:
  - key extraction per line family (`cty:kind:role:call`, `unlicensed:role:call`),
  - fixed suppression window,
  - fixed key cap and oldest-seen eviction.
- YAML knob (single operator control):
  - `logging.drop_dedupe_window_seconds` (int)
  - default: `45`
  - `0` means disabled.
- Integration points:
  - `makeDroppedReporter(...)` for CTY and related drop lines.
  - `makeUnlicensedReporter(...)` for unlicensed lines.
- Resource bounds:
  - In-memory map capped at `maxKeys`.
  - No goroutines, no channels, no timers created by dedupe component.
- Failure mode:
  - if dedupe parsing fails, pass original line through (no lost visibility).

Deduping Logic (Plain Language):
- For each incoming line, try to classify it into one of the target types.
- If line is not a target type: print immediately, unchanged.
- If line is target type:
  - Build a key from reason + role + callsign.
  - If this key has not been seen recently, print immediately and open a suppression window.
  - If same key repeats before window expires, suppress it and increment hidden-count.
  - When the window expires and the key appears again, print once with summary suffix:
    - `(suppressed=N over <window>)`
  - Reset hidden-count and start a new window.

Concrete Examples:
- Example A (CTY unknown):
  - t=00s `CTY drop: unknown DX K1ABC ...` -> printed
  - t=05s same key -> suppressed
  - t=20s same key -> suppressed
  - t=50s same key (45s window) -> printed with `(suppressed=2 over 45s)`
- Example B (Unlicensed):
  - `Unlicensed US DE K1XYZ ...` and `Unlicensed US DE K1XYZ ...` -> deduped
  - `Unlicensed US DE K1XYZ ...` and `Unlicensed US DE W1XYZ ...` -> separate keys, both printed

Contracts:
- No protocol/format changes for telnet/packet streams.
- No backpressure/drop-policy contract changes.
- Observability contract change (console/UI only):
  - repeated target lines may be suppressed within a bounded window,
  - periodic re-emits include suppression counts.

Dependency Impact (Light):
- Upstream: ingest validation + license gate produce source lines.
- Shared component: reporter closures in `main.go`.
- Downstream: console UI panes and stdout logs.
- Contract changes: No protocol/backpressure changes; console observability semantics refined.

Tests:
- Unit: key extraction for CTY unknown/invalid and unlicensed (plain + color lines).
- Unit: suppression within window and re-emit with suppression count.
- Unit: bounded key cap eviction behavior.
- Boundary: non-target lines pass through unchanged.
- Config: YAML knob default/load/disable/invalid-value validation tests.
- Verification commands:
  - `gofmt -w main.go drop_log_dedupe.go drop_log_dedupe_test.go`
  - `go test ./... -run "DropLogDeduper|IngestValidator"`

Rollout/Ops:
- Default behavior: dedupe enabled for target noisy lines.
- No config migration in v2.
- Operator expectation: lower repeated drop-line noise with occasional summarized re-emits.
- Operator control:
  - `logging.drop_dedupe_window_seconds: 0` -> disable dedupe
  - `logging.drop_dedupe_window_seconds: 45` -> default behavior

Acceptance Criteria by Scope Item:
- S2: Non-target lines are never deduped (bit-for-bit pass-through).
- S3: Target duplicates within window are suppressed and later summarized.
- S4: Deduper memory stays bounded by key cap; eviction is deterministic.
- S5: Reporter integration preserves existing UI routing/log fallback behavior.
- S6: YAML knob is documented, validated, and controls enable/window as specified.

Alternatives Considered:
- Increase existing rate limits only:
  - simpler, but loses per-call suppression context.
- Dedupe earlier in ingest path:
  - tighter coupling to validation logic; harder to keep UI/log semantics isolated.
- Exact-line string dedupe:
  - too sensitive to counters/source text noise; poor grouping quality.

---

## Post-code (Plan v2)
Deviations:
- None.

Verification commands actually run:
- `gofmt -w config/config.go config/logging_dedupe_test.go main.go drop_log_dedupe.go`
- `go test ./config ./... -run "DropLogDeduper|IngestValidator|LoggingDropDedupe"`

Final contract statement:
- Implemented targeted reporter-side dedupe for CTY unknown/invalid and unlicensed dropped-call lines, with a configurable YAML window (`logging.drop_dedupe_window_seconds`, default `120`, `0` disables, negative rejected). No ingest/drop correctness or telnet protocol contract changes.

Scope Ledger status updates:
- S2 -> Implemented
- S3 -> Implemented
- S4 -> Implemented
- S5 -> Implemented
- S6 -> Implemented

---

## Scope Ledger v2 (LIVE, cumulative)
| ID | Item | Status | Notes |
|----|------|--------|-------|
| S1 | Implement initial bounded dedupe prototype for CTY-invalid, CTY-unknown, and unlicensed lines | Implemented | Prior prototype exists; superseded by v2 acceptance gates |
| S2 | Finalize target matching + bypass rules (only CTY unknown/invalid + unlicensed) | Implemented | Includes color-tagged UI compatibility |
| S3 | Enforce suppression semantics (windowed suppress + summarized re-emit) | Implemented | Re-emits include suppression count + window |
| S4 | Prove bounded state and eviction determinism | Implemented | Max keys + oldest-seen eviction covered by tests |
| S5 | Reporter integration and regression checks for UI/log routing | Implemented | UI/log fallback preserved; non-target lines pass through |
| S6 | Add single YAML knob `logging.drop_dedupe_window_seconds` with validation and tests | Implemented | `0` disables; `>0` enables windowed dedupe; default `120` |

---

## Turn Log (Append-Only, last 25 kept)
- 2026-02-06 — Plan v1 created and implemented (initial prototype).
- 2026-02-06 — Restart requested; created Plan v2 with explicit hard-gate scope and acceptance criteria.
- 2026-02-06 — Approved v2 with default window 120s; implemented S2-S6 and validated with tests.

---

## Decision Log (Append-Only)
### D1 — 2026-02-06 Reporter-boundary dedupe
- Context: repeated dropped-call lines generate high console noise.
- Chosen: dedupe at reporter boundary.
- Alternatives: ingest-path dedupe; rate-limit only.
- Impact: no ingest correctness changes; noise reduced where displayed.

### D2 — 2026-02-06 Targeted family-only dedupe
- Context: only specific drop families are noisy.
- Chosen: dedupe CTY unknown/invalid and unlicensed families only.
- Alternatives: dedupe all drop lines; exact-line dedupe.
- Impact: preserves visibility for non-target drop categories.

---

## Plan Index (history)
- Plan v1 — Console Drop Log Dedupe (prototype) — 2026-02-06 — Scope Ledger v1 — Implemented
- Plan v2 — Drop-Noise Dedupe Redesign — 2026-02-06 — Scope Ledger v2 — Implemented

---

## Context for Resume
**Done**:
- Prototype dedupe implementation exists in branch.
- Plan v2 now defines explicit non-trivial scope and acceptance criteria.
- Implemented config-driven dedupe window with default 120s and disable knob.
- Added config validation + tests and updated operator-facing config docs.
**In Progress**:
- (none)
**Next**:
- Monitor production noise and adjust window only if needed.
**Key Decisions**:
- D1, D2.
**Files Hot**:
- `PLANS-log-dedupe.md`
- `main.go`
- `drop_log_dedupe.go`
- `drop_log_dedupe_test.go`
- `config/config.go`
- `config/logging_dedupe_test.go`
- `data/config/app.yaml`

---

## Compaction Rule
- Keep only: Active Plan, live Scope Ledger, last 25 Turn Log entries, Decision Log, Plan Index, Current State Snapshot, Context for Resume.
- Move older plans/logs/superseded scope rows to `PLANS-ARCHIVE-log-dedupe.md`.
