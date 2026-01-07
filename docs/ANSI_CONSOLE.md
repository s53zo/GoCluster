# ANSI Console (Fixed 90x68)

## Overview
The ANSI console is the lightweight local UI used when `ui.mode: ansi` is active. It targets the process console only (not telnet clients) and is optimized for low CPU by rendering row diffs instead of full-screen clears.

## Layout (Fixed)
- Size: 90 columns x 68 rows (minimum terminal size).
- Rows: 12 stats lines, 1 blank line, then 5 panes of 1 header + 10 lines each.
- Pane headers (exact): `<<<<<<<<<< Dropped >>>>>>>>>>`, `<<<<<<<<<< Corrected >>>>>>>>>>`, `<<<<<<<<<< Unlicensed >>>>>>>>>>`, `<<<<<<<<<< Harmonics >>>>>>>>>>`, `<<<<<<<<<< System Log >>>>>>>>>>`.

```
Rows 01-12: stats
Row 13:     blank
Rows 14-24: Dropped (header + 10 lines)
Rows 25-35: Corrected (header + 10 lines)
Rows 36-46: Unlicensed (header + 10 lines)
Rows 47-57: Harmonics (header + 10 lines)
Rows 58-68: System Log (header + 10 lines)
```

## Rendering Strategy
- Event-driven: pane updates render on events with a minimum spacing of `ui.refresh_ms`.
- Diff-based updates: only rows that change are rewritten; a full redraw is triggered on terminal resize.
- Output is single-writer (one goroutine owns stdout).

## Input Handling
- Control characters are stripped (`\r`, `\n`, `\t`, `\x1b`, and other C0 controls).
- Simple markup tokens are supported: `[red] [green] [yellow] [blue] [magenta] [cyan] [white] [-]`.
- Visible content is clamped to 90 columns and padded to fixed width.

## Dropped Pane Sources
- CTY validation failures (DE/DX).
- PC61 parse drops (standardized `reason + summary` format).
- Reputation drops.
- These events are removed from the System Log pane to avoid duplication.

## Platform Notes
- Windows: target Windows Terminal; PowerShell launch is supported.
- The renderer cannot set terminal font/size; the terminal must be configured to fit 90x68.

## Config Notes
- Use `data/config/app.yaml` and `data/config/runtime.yaml` only.
- `ui.pane_lines` is for `tview` only; ANSI uses the fixed layout above.
