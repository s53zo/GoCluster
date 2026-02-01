# tview UI Mockups - Text-Based Console Layouts

These mockups show the actual text rendering that tview would display in a terminal. All layouts use tview's Flex and Grid primitives with TextView, Table, and List widgets.

---

## Page 1: Overview (Default View)

```
+------------------------------------------------------------------------------+
| DX Cluster v1.2.3    Uptime: 48:32    Nodes: 3    [F1:Help] [F2:Ingest]     |
+------------------------------------------------------------------------------+
|                                                                              |
|  MEMORY USAGE                                                                |
|  +----------------------------------------------------------+               |
|  | Exec: 45MB  |  Ring: 12MB  |  Dedup: 8MB (23%)  |  Meta: 4MB (89%)       |               |
|  +----------------------------------------------------------+               |
|                                                                              |
|  INGEST RATES (spots/min)              PIPELINE EFFICIENCY                   |
|  +---------------------------+         +---------------------------+        |
|  | RBN:     1,234            |         | Primary:   98.5%          |        |
|  | PSK:     5,678            |         | Fast:      95%            |        |
|  | Human:      45            |         | Med:       92%            |        |
|  | Peer:      123            |         | Slow:      88%            |        |
|  +---------------------------+         +---------------------------+        |
|                                                                              |
|  TELNET SERVER                         GRID DATABASE                         |
|  +---------------------------+         +---------------------------+        |
|  | Clients: 42/100           |         | Records: 1.2M             |        |
|  | Queue drops: 0            |         | Cache hit: 94%            |        |
|  | Client drops: 12          |         | Lookups: 45K/min          |        |
|  | Send fails: 3             |         | Async drops: 0            |        |
|  +---------------------------+         +---------------------------+        |
|                                                                              |
|  DATA SOURCES                                                                |
|  +----------------------------------------------------------+               |
|  | CTY: 01-31-2026 08:00Z  |  FCC ULS: 01-30-2026 22:00Z  |               |
|  +----------------------------------------------------------+               |
|                                                                              |
+------------------------------------------------------------------------------+
| [F2]Ingest  [F3]Pipeline  [F4]Network  [F5]Events  [F6]Debug  [Q]Quit       |
+------------------------------------------------------------------------------+
```

**tview Implementation Notes:**
- Header: `tview.NewTextView()` with dynamic color tags `[blue]DX Cluster[-]`
- Memory bar: `tview.NewTable()` with 4 columns, borders disabled
- Ingest/Pipeline: Side-by-side `tview.NewFlex().AddItem(left, 0, 1, false).AddItem(right, 0, 1, false)`
- Footer: `tview.NewTextView()` with hotkey hints

---

## Page 2: Ingest Details

```
+------------------------------------------------------------------------------+
| INGEST SOURCES                                                    [F2] 2/6  |
+------------------------------------------------------------------------------+
|                                                                              |
|  RBN (TELNET)              |  PSKREPORTER (MQTT)                           |
|  ==========================+=============================================== |
|  Host: telnet.reversebeacon.net:7000 | Broker: pskreporter.info:1883        |
|  Status: CONNECTED         |  Status: CONNECTED                            |
|  Uptime: 2h 15m            |  Uptime: 48h 32m                              |
|  Reconnects: 0             |  Reconnects: 1                                |
|                            |                                               |
|  SPOTS/MINUTE              |  SPOTS/MINUTE                                 |
|  +----------------------+  |  +----------------------+                     |
|  | CW      |      234   |  |  | CW      |      123   |                     |
|  | RTTY    |      456   |  |  | RTTY    |      789   |                     |
|  | FT8     |      890   |  |  | FT8     |    2,345   |                     |
|  | FT4     |      123   |  |  | FT4     |      567   |                     |
|  |         |            |  |  | MSK144  |       89   |                     |
|  +----------------------+  |  +----------------------+                     |
|  | TOTAL   |    1,703   |  |  | TOTAL   |    3,913   |                     |
|  +----------------------+  |  +----------------------+                     |
|                            |                                               |
|  HEALTH                    |  HEALTH                                       |
|  Keepalive: OK             |  Queue drops: 0                               |
|  Parse errors: 12          |  Stale drops: 45                              |
|                            |  No-SNR drops: 12                             |
|                            |  No-grid drops: 8                             |
+------------------------------------------------------------------------------+
| [F1]Overview  [F3]Pipeline  [F4]Network  [F5]Events  [F6]Debug  [Q]Quit      |
+------------------------------------------------------------------------------+
```

**tview Implementation Notes:**
- Split pane: `tview.NewFlex().SetDirection(tview.FlexColumn)`
- Each side: `tview.NewFlex().SetDirection(tview.FlexRow)` with TextViews
- Tables use `tview.NewTable().SetBorders(true)` for grid lines
- Status indicators use color: `[green]CONNECTED[-]` or `[red]DISCONNECTED[-]`

---

## Page 3: Pipeline

```
+------------------------------------------------------------------------------+
| PIPELINE PROCESSING                                               [F3] 3/6  |
+------------------------------------------------------------------------------+
|                                                                              |
|  PRIMARY DEDUPLICATION                                                       |
|  +----------------------------------------------------------+               |
|  | Processed:  1,234,567    |    Duplicates: 456,789 (37%)  |               |
|  | Cache size: 12,345       |    Entry size: 32 bytes       |               |
|  | Memory:     0.4MB                                      |               |
|  +----------------------------------------------------------+               |
|                                                                              |
|  SECONDARY DEDUPLICATION (SNR-based)                                         |
|  +------------------+------------------+------------------+                  |
|  | FAST (5s)        | MED (30s)        | SLOW (60s)       |                  |
|  +------------------+------------------+------------------+                  |
|  | Processed: 100K  | Processed: 200K  | Processed: 150K  |                  |
|  | Duplicates: 30K  | Duplicates: 80K  | Duplicates: 45K  |                  |
|  | Forwarded:  70K  | Forwarded:  120K | Forwarded:  105K |                  |
|  | Cache:      2MB  | Cache:      4MB  | Cache:      3MB  |                  |
|  | Efficiency: 70%  | Efficiency: 60%  | Efficiency: 70%  |                  |
|  +------------------+------------------+------------------+                  |
|                                                                              |
|  CALL CORRECTION                                                             |
|  +----------------------------------------------------------+               |
|  | Corrections: 1,234  |  Rejected: 56  |  Suppressed: 12  |               |
|  | Avg confidence: 78% |  Avg supporters: 4.2             |               |
|  +----------------------------------------------------------+               |
|                                                                              |
|  HARMONIC SUPPRESSION                                                        |
|  +----------------------------------------------------------+               |
|  | Suppressed: 890  |  Patterns detected: 23  |  Memory: 0.1MB |               |
|  +----------------------------------------------------------+               |
|                                                                              |
+------------------------------------------------------------------------------+
| [F1]Overview  [F2]Ingest  [F4]Network  [F5]Events  [F6]Debug  [Q]Quit        |
+------------------------------------------------------------------------------+
```

**tview Implementation Notes:**
- Secondary dedup: `tview.NewFlex().SetDirection(tview.FlexColumn)` with 3 equal columns
- Each column: `tview.NewTextView()` with formatted stats
- Could use `tview.NewTable()` for aligned numeric columns
- Efficiency percentages color-coded: `[green]70%[-]` `[yellow]60%[-]` `[red]<50%[-]`

---

## Page 4: Network

```
+------------------------------------------------------------------------------+
| NETWORK STATUS                                                    [F4] 4/6  |
+------------------------------------------------------------------------------+
|                                                                              |
|  TELNET SERVER                          PEERING                              |
|  =================                      =================                   |
|  Port: 7300                             Local node: N2WQ-22                 |
|  TLS: Enabled                           Status: 3 active / 5 configured      |
|                                          |                                   |
|  CLIENTS (42/100)                        ACTIVE PEERS                        |
|  +-------------------+                   +-----------+--------+-------------+|
|  | Authenticated: 38 |                   | Node      | Uptime | Last PC92  ||
|  | Anonymous:     4  |                   +-----------+--------+-------------+|
|  | Reputation:    0  |                   | N1ABC-1   | 2h15m  | 45s ago    ||
|  +-------------------+                   | W2XYZ-3   | 48h32m | 12s ago    ||
|                                          | K3DEF-2   | 15m    | 3s ago     ||
|  BROADCAST STATS                         +-----------+--------+-------------+|
|  Queue drops:    0                       | K4GHI-5   | [red]DOWN[-] | --         ||
|  Client drops:   12                      | N5JKL-9   | [red]DOWN[-] | --         ||
|  Send failures:  3                       +-----------+--------+-------------+|
|                                                                          |   |
|                                          TOPOLOGY                          |
|                                          Nodes known: 45                    |
|                                          Users known: 1,234                 |
|                                                                          |   |
+------------------------------------------------------------------------------+
| [F1]Overview  [F2]Ingest  [F3]Pipeline  [F5]Events  [F6]Debug  [Q]Quit       |
+------------------------------------------------------------------------------+
```

**tview Implementation Notes:**
- Two-column layout using `tview.NewFlex().SetDirection(tview.FlexColumn)`
- Peer table: `tview.NewTable()` with 3 columns
- Table headers: `table.SetCell(0, 0, tview.NewTableCell("Node").SetAttributes(tcell.AttrBold))`
- Status colors: `[green]` for active, `[red]` for down, `[yellow]` for warning

---

## Page 5: Events

```
+------------------------------------------------------------------------------+
| EVENT LOG                                                         [F5] 5/6  |
+------------------------------------------------------------------------------+
| [All] [Drops] [Corrections] [Unlicensed] [Harmonics] [System]                |
+------------------------------------------------------------------------------+
|                                                                              |
| 15:04:32  [red][DROP][-]     CTY validation failed: XX1ABC from RBN CW      |
| 15:04:28  [green][CORRECT][-] K1ABC -> KL1ABC at 14100.0 kHz (3/75%)        |
| 15:04:15  [red][UNLIC][-]    US Tech KF5XYZ dropped from PSK FT8            |
| 15:03:59  [red][DROP][-]     Reputation: IP 192.168.1.1 throttled           |
| 15:03:45  [yellow][HARM][-]  Suppressed harmonic at 28200.0 kHz             |
| 15:03:12  [red][DROP][-]     PC61 parse error: malformed spot               |
| 15:02:58  [green][CORRECT][-] W1AW -> WW1AW at 7050.0 kHz (5/83%)           |
| 15:02:44  [red][DROP][-]     CTY validation failed: YY2ABC from PSK RTTY    |
| 15:02:31  [white][SYS][-]    Grid checkpoint completed in 45ms              |
| 15:02:15  [red][UNLIC][-]    US Tech N3XYZ dropped from RBN CW              |
| 15:01:59  [red][DROP][-]     Reputation: rapid connect from 10.0.0.1        |
| 15:01:42  [yellow][HARM][-]  Suppressed harmonic at 14150.0 kHz             |
| 15:01:28  [green][CORRECT][-] KH6ABC -> KH7ABC at 18100.0 kHz (4/80%)       |
| ... (scroll for 1,234 total events)                                         |
|                                                                              |
+------------------------------------------------------------------------------+
| Showing: All events  |  Buffer: 1,234/1,000  |  [↑/↓]Scroll  [Tab]Filter   |
+------------------------------------------------------------------------------+
| [F1]Overview  [F2]Ingest  [F3]Pipeline  [F4]Network  [F6]Debug  [Q]Quit      |
+------------------------------------------------------------------------------+
```

**tview Implementation Notes:**
- Filter tabs: `tview.NewTextView()` with clickable regions or `tview.NewList()`
- Event list: Custom `VirtualList` component (only renders visible rows)
- Each row formatted with timestamp + colored tag + message
- Virtual scrolling: Track `scrollOffset`, render only `visibleRows` count
- Footer shows buffer utilization

**Virtual Scrolling Implementation:**
```go
func (v *VirtualList) Draw(screen tcell.Screen) {
    x, y, width, height := v.GetRect()
    visibleCount := height
    
    for i := 0; i < visibleCount && v.scrollOffset+i < len(v.events); i++ {
        event := v.events[v.scrollOffset+i]
        rowY := y + i
        // Draw timestamp
        drawString(screen, x, rowY, event.Timestamp.Format("15:04:05"), tcell.StyleDefault)
        // Draw colored tag
        drawColoredTag(screen, x+10, rowY, event.Tag, event.Severity)
        // Draw message
        drawString(screen, x+25, rowY, truncate(event.Message, width-25), tcell.StyleDefault)
    }
}
```

---

## Page 6: Debug

```
+------------------------------------------------------------------------------+
| SYSTEM LOG                                                        [F6] 6/6  |
+------------------------------------------------------------------------------+
| Level: [INFO ▼]  Search: [checkpoint______]  [X] Clear                       |
+------------------------------------------------------------------------------+
|                                                                              |
| 2026-01-31 15:04:32 INF gridstore checkpoint completed duration=45ms        |
| 2026-01-31 15:04:15 WRN CTY refresh failed, retry in 5m err=timeout         |
| 2026-01-31 15:03:59 INF telnet client connected addr=192.168.1.1:54321      |
| 2026-01-31 15:03:45 DBG harmonic detector: checking 14100.0 kHz             |
| 2026-01-31 15:03:12 INF RBN reconnect successful host=telnet.reversebeacon..|
| 2026-01-31 15:02:58 INF PSKReporter connected broker=pskreporter.info       |
| 2026-01-31 15:02:44 DBG dedup: checking hash=0x12345678 age=2s              |
| 2026-01-31 15:02:31 INF gridstore batch write: 234 records in 12ms          |
| 2026-01-31 15:02:15 WRN reputation: throttling IP 192.168.1.1 (rate limit)  |
| 2026-01-31 15:01:59 INF call correction: consensus reached for K1ABC        |
| 2026-01-31 15:01:42 DBG buffer: ring capacity 8192, used 1234               |
| 2026-01-31 15:01:28 INF peering: PC92 received from N1ABC-1                 |
| ... (scroll for 4,567 total lines)                                          |
|                                                                              |
+------------------------------------------------------------------------------+
| Buffer: 4,567/5,000 lines  |  Filter: "checkpoint"  |  [↑/↓]Scroll  [/]Search|
+------------------------------------------------------------------------------+
| [F1]Overview  [F2]Ingest  [F3]Pipeline  [F4]Network  [F5]Events  [Q]Quit     |
+------------------------------------------------------------------------------+
```

**tview Implementation Notes:**
- Filter controls: `tview.NewFlex()` with `tview.NewDropDown()` + `tview.NewInputField()`
- Log lines: `tview.NewTextView()` with `SetDynamicColors(false)` (no color tags in logs)
- Search: Filter buffer and re-render
- Virtual scrolling same as Events page

---

## Help Overlay (F1)

```
+------------------------------------------------------------------------------+
|                                                                              |
|  +----------------------------------------------------------+               |
|  |                      KEYBOARD HELP                       |               |
|  +----------------------------------------------------------+               |
|  |                                                          |               |
|  |  NAVIGATION                                              |               |
|  |    F1          Show this help                            |               |
|  |    F2          Overview page                             |               |
|  |    F3          Ingest page                               |               |
|  |    F4          Pipeline page                             |               |
|  |    F5          Events page                               |               |
|  |    F6          Debug page                                |               |
|  |    Tab         Next page                                 |               |
|  |    Shift+Tab   Previous page                             |               |
|  |                                                          |               |
|  |  SCROLLING (Events/Debug pages)                          |               |
|  |    ↑ / k       Scroll up one line                        |               |
|  |    ↓ / j       Scroll down one line                      |               |
|  |    PageUp      Scroll up one page                        |               |
|  |    PageDown    Scroll down one page                      |               |
|  |    Home        Jump to top                               |               |
|  |    End         Jump to bottom                            |               |
|  |                                                          |               |
|  |  FILTERING                                               |               |
|  |    1-6         Select event filter tab (Events page)     |               |
|  |    /           Start search                              |               |
|  |    Esc         Clear search / close overlay              |               |
|  |                                                          |               |
|  |  GENERAL                                                 |               |
|  |    q / Ctrl+C  Quit application                          |               |
|  |                                                          |               |
|  +----------------------------------------------------------+               |
|                                                                              |
+------------------------------------------------------------------------------+
```

**tview Implementation Notes:**
- Modal overlay: `tview.NewModal()` or custom `tview.NewFlex()` centered
- Semi-transparent background using `tcell.StyleDefault.Background(tcell.ColorDarkGray)`
- Dismiss with Esc, F1, or click outside

---

## Color Scheme

```go
// tview color theme
tview.Styles = tview.Theme{
    PrimitiveBackgroundColor:    tcell.ColorBlack,
    ContrastBackgroundColor:     tcell.ColorBlue,
    MoreContrastBackgroundColor: tcell.ColorGreen,
    BorderColor:                 tcell.ColorGray,
    TitleColor:                  tcell.ColorWhite,
    GraphicsColor:               tcell.ColorGray,
    PrimaryTextColor:            tcell.ColorWhite,
    SecondaryTextColor:          tcell.ColorYellow,
    TertiaryTextColor:           tcell.ColorGreen,
    InverseTextColor:            tcell.ColorBlack,
    ContrastSecondaryTextColor:  tcell.ColorCyan,
}

// Severity colors for events
var severityColors = map[Severity]tcell.Color{
    SeverityDebug:   tcell.ColorGray,
    SeverityInfo:    tcell.ColorWhite,
    SeverityWarning: tcell.ColorYellow,
    SeverityError:   tcell.ColorRed,
    SeverityDrop:    tcell.ColorRed,
    SeverityCorrect: tcell.ColorGreen,
    SeverityHarm:    tcell.ColorOrange,
}
```

---

## Terminal Requirements

- Minimum size: 80 columns × 24 rows
- Recommended size: 100 columns × 30 rows
- Color support: 256 colors preferred, 16 colors minimum
- Unicode: Basic box-drawing characters supported

---

## Performance Characteristics

| Aspect | Current tview | Proposed v2 | Improvement |
|--------|---------------|-------------|-------------|
| Lines in memory | 10,000 | 6,000 | -40% |
| Render calls/sec | 10+ | 60 max | Bounded |
| Scroll latency | O(n) | O(1) | Constant |
| Memory overhead | ~15MB | ~5MB | -67% |
| First paint | 200ms | 50ms | -75% |

---

*Mockup Version: 1.0*
*Date: 2026-01-31*
