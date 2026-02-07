# ADR-0002: UI v2 Render Pipeline Optimization

- Status: Accepted
- Date: 2026-02-07

## Context

The existing `tview-v2` UI used `TextView.SetText()` rebuilds for high-rate event streams, per-render snapshot copies, and per-frame scheduler map churn.

Under sustained ingress, that design increased CPU and allocation pressure in the UI hot path while providing no additional correctness guarantees.

This repo needs UI behavior that remains bounded and deterministic under load:

- Bounded stream memory per pane.
- Coalesced UI updates that do not block ingest paths.
- Predictable shutdown and no background leak classes.
- Lower heap churn in steady-state rendering.

## Decision

Adopt a new internal render pipeline for `ui.mode=tview-v2` while keeping the external `ui.Surface` contract and existing page/key semantics unchanged.

Implemented decisions:

1. Use a virtualized custom stream primitive (`virtualLogView`) with a bounded ring and viewport-only drawing.
2. Replace render-time snapshot copying with immutable snapshots stored via `atomic.Pointer`.
3. Replace scheduler per-frame map drain with stable slot + dirty-bit coalescing.
4. Keep static layout heights in update hot paths (remove dynamic `ResizeItem` churn).
5. Add focused tests and benchmarks for scheduler coalescing, bounded stream behavior, and render hot paths.

## Alternatives Considered

1. Keep `TextView`-based streams and only tune FPS.
- Rejected: lowers draw frequency but does not address full-text rebuild allocations.

2. Migrate to ANSI-only renderer for performance.
- Rejected: would remove interactive page UX and is a broader product/UI shift than requested.

3. Build a full web UI for modern visuals.
- Rejected: adds network/security/deployment complexity and is outside current local-console scope.

## Consequences

### Benefits

- Render hot path now scales with visible rows, not total buffered lines.
- Per-frame scheduler overhead is bounded without map clear/rebuild churn.
- Snapshot read path avoids repeated per-render copies.
- Existing `Surface` integration points in `main.go` remain unchanged.

### Risks

- Custom primitive complexity is higher than `TextView` usage.
- Any primitive behavior drift (focus/scroll semantics) requires explicit tests.

### Operational Impact

- No protocol, drop/disconnect, telnet timeout, or ingest contract changes.
- No new operator configuration required.
- UI latency/alloc behavior is now directly benchmarked in `./ui` tests.

## Links

- Code: `ui/dashboard_v2.go`, `ui/panels.go`, `ui/virtual_log_view.go`, `ui/scheduler.go`
- Tests: `ui/dashboard_v2_test.go`, `ui/virtual_log_view_test.go`, `ui/scheduler_test.go`
- Benchmarks: `ui/perf_bench_test.go`
- Decision index: `docs/decision-log.md`
