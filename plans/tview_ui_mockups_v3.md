# tview UI Mockups v3 (Keybindings + Snapshot Model)

These mockups reflect the v3 proposal: F1 Help, F2 Overview (default), F3–F7 for pages, and the consolidated footer.

---

## Overview Page (Default, F2)

```
+----------------------------------------------------------------------------------+
| DX Cluster v1.2.3  Uptime: 48:32  Nodes: 3  Mem: 45MB  [F1:Help] [F2:Overview]   |
+----------------------------------------------------------------------------------+
| MEMORY                                                                          |
| Exec: 45MB  Ring: 12MB  Dedup: 8MB (23%)  Meta: 4MB (89%)                      |
+----------------------------------------------------------------------------------+
| INGEST RATES (spots/min)                          PIPELINE EFFICIENCY            |
| RBN: 1,234   PSK: 5,678   Human: 45   Peer: 123   Primary: 98.5%  F:95 M:92 S:88 |
+----------------------------------------------------------------------------------+
| TELNET                                                    GRID DB                |
| Clients: 42/100   Drops: Q:0 C:0 W:0   Send fails: 3      Records: 1.2M  Hit: 94%|
+----------------------------------------------------------------------------------+
| DATA SOURCES                                                                     |
| CTY: 2026-01-31 08:00Z    FCC ULS: 2026-01-30 22:00Z                             |
+----------------------------------------------------------------------------------+
| [F1]Help  [F2]Overview  [F3]Ingest  [F4]Pipeline  [F5]Network  [F6]Events  [F7]Debug  [Q]Quit |
+----------------------------------------------------------------------------------+
```

---

## Events Page (F6)

```
+----------------------------------------------------------------------------------+
| EVENT LOG                                                              [F6] 5/6 |
+----------------------------------------------------------------------------------+
| [All] [Drops] [Corrections] [Unlicensed] [Harmonics] [System]                   |
+----------------------------------------------------------------------------------+
| 15:04:32 [DROP] CTY validation failed: XX1ABC from RBN CW                        |
| 15:04:28 [CORR] K1ABC -> KL1ABC at 14100.0 kHz (3/75%)                           |
| 15:04:15 [UNLC] US Tech KF5XYZ dropped from PSK FT8                              |
| 15:03:59 [DROP] Reputation: IP 192.168.1.1 throttled                             |
| 15:03:45 [HARM] Suppressed harmonic at 28200.0 kHz                               |
| 15:03:12 [DROP] PC61 parse error: malformed spot                                 |
| 15:02:58 [CORR] W1AW -> WW1AW at 7050.0 kHz (5/83%)                              |
| 15:02:44 [DROP] CTY validation failed: YY2ABC from PSK RTTY                      |
| 15:02:31 [SYS ] Grid checkpoint completed in 45ms                                |
| 15:02:15 [UNLC] US Tech N3XYZ dropped from RBN CW                                |
| ... (virtual scroll: 1,234 events in buffer)                                     |
+----------------------------------------------------------------------------------+
| Showing: All  Buffer: 1,000/1,000  Drops: 12  [↑/↓]Scroll  [/]Search  [1-6]Filter|
+----------------------------------------------------------------------------------+
| [F1]Help  [F2]Overview  [F3]Ingest  [F4]Pipeline  [F5]Network  [F7]Debug  [Q]Quit |
+----------------------------------------------------------------------------------+
```

---

## Help Overlay (F1)

```
+----------------------------------------------------------------------------------+
|                                KEYBOARD HELP                                   |
+----------------------------------------------------------------------------------+
| NAVIGATION                                                                      |
|  F1  Help   F2 Overview   F3 Ingest   F4 Pipeline   F5 Network   F6 Events  F7 Debug |
|  Tab Next page   Shift+Tab Previous page   q / Ctrl+C Quit                       |
|                                                                                |
| EVENTS/DEBUG                                                                     |
|  ↑/↓ or k/j Scroll   PageUp/Down Fast scroll   Home/End Top/Bottom               |
|  1-6 Filter tabs (Events)   / Search   Esc Clear search / close                  |
+----------------------------------------------------------------------------------+
```

---

*Mockup Version: 3.0*
*Date: 2026-01-31*
