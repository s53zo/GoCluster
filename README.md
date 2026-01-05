# DX Cluster Server

A modern Go-based DX cluster that aggregates amateur radio spots, enriches them with CTY metadata, and broadcasts them to telnet clients.

## Quickstart

1. Install Go `1.25+` (see `go.mod`).
2. Edit `data/config/` (at minimum: set your callsigns in `ingest.yaml` and `telnet.port` in `runtime.yaml`). If you plan to peer with other DXSpider nodes, populate `peering.yaml` (local callsign, peers, ACL/passwords). You can override the path with `DXC_CONFIG_PATH` if you want to point at a different directory or a legacy single-file config.
3. Run:
   ```pwsh
   go mod tidy
   go run .
   ```
4. Connect: `telnet localhost 9300` (or whatever `telnet.port` is set to in `data/config/runtime.yaml`).

## Architecture and Spot Sources

1. **Telnet Server** (`telnet/server.go`) handles client connections, commands, and spot broadcasting using worker goroutines.
2. **RBN Clients** (`rbn/client.go`) maintain connections to the CW/RTTY (port 7000) and Digital (port 7001) feeds. Each line is parsed and normalized, then sent through the shared ingest CTY/ULS gate for validation and enrichment before queuing.
   - Parsing uses a single left-to-right token walk assisted by an Aho–Corasick (AC) keyword scanner so the line only needs to be scanned once.
   - The parser supports both `DX de CALL: 14074.0 ...` and the glued form `DX de CALL:14074.0 ...` by splitting the spotter token into `DECall` + optional attached frequency.
   - Frequency is the first token that parses as a plausible dial frequency (currently `100.0`-`3,000,000.0` kHz), rather than assuming a fixed column index.
   - Mode is taken from the first explicit mode token (`CW`, `USB`, `JS8`, `SSTV`, `FT8`, `MSK144`, etc.). If mode is absent, it is inferred from `data/config/mode_allocations.yaml` (with a simple fallback: `USB` >= 10 MHz else `CW`).
   - Report/SNR is recognized in both `+5 dB` and `-13dB` forms; `HasReport` is set whenever a report is present (including a valid `0 dB`).
   - Ingest burst protection is sized per source via `rbn.slot_buffer` / `rbn_digital.slot_buffer` in `data/config/ingest.yaml`; overflow logs are tagged by source for easier diagnosis.
3. **PSKReporter MQTT** (`pskreporter/client.go`) subscribes to a single catch-all `pskr/filter/v2/+/+/#` topic and filters modes downstream according to `pskreporter.modes`. It converts JSON payloads into canonical spots and preserves locator-based grids. Set `pskreporter.append_spotter_ssid: true` if you want receiver callsigns that lack SSIDs to pick up a `-#` suffix for deduplication. PSKReporter spots no longer carry a comment string; DX/DE grids stay in metadata and are shown in the fixed tail of telnet output. Configure `pskreporter.cty_cache_size`, `pskreporter.cty_cache_ttl_seconds`, and `pskreporter.max_payload_bytes` to bound ingest CTY cache memory usage and guard against oversized payloads.
4. **CTY Database** (`cty/parser.go` + `data/cty/cty.plist`) performs longest-prefix lookups; when a callsign includes slashes, it prefers the shortest matching segment (portable/location prefix), so `N2WQ/VE3` and `VE3/N2WQ` both resolve to `VE3` (Canada) for metadata.
5. **Dedup Engine** (`dedup/deduplicator.go`) filters duplicates before they reach the ring buffer. A zero-second window effectively disables dedup, but the pipeline stays unified. A secondary, broadcast-only dedupe (configurable window, default 60s) runs after call correction/harmonic/frequency adjustments to collapse same-DX/frequency reports from spotters that share the same DE ADIF (DXCC) and DE CQ zone; it hashes kHz + DE ADIF + DE zone + DX call (time window enforced separately), so one spot per window per spotter country/zone reaches clients while the ring/history remain intact. When `secondary_prefer_stronger_snr` is true, the stronger SNR duplicate replaces the cached entry and is broadcast. Spotter SSID display is controlled at broadcast time (see `rbn.keep_ssid_suffix`).
6. **Frequency Averager** (`spot/frequency_averager.go`) merges CW/RTTY skimmer reports by averaging corroborating reports within a tolerance and rounding to 0.1 kHz once the minimum corroborators is met.
7. **Call/Harmonic/License Guards** (`spot/correction.go`, `spot/harmonics.go`, `main.go`) apply consensus-based call corrections, suppress harmonics, and finally run FCC license gating for DX right before broadcast/buffering (CTY validation runs in the ingest gate; corrected calls are re-validated against CTY before acceptance). Harmonic suppression supports a stepped minimum dB delta (configured via `harmonics.min_report_delta_step`) so higher-order harmonics must be progressively weaker. Call correction honours `call_correction.min_snr_cw` / `min_snr_rtty` (and `min_snr_voice` if set) so marginal decodes can be ignored when counting corroborators; USB/LSB uses a wider frequency tolerance and candidate search window to reflect 3 kHz SSB bandwidth. An optional cooldown gate (`call_correction.cooldown_*`) can temporarily refuse to flip away from a call that already has recent diverse support unless the alternate is decisively stronger; `cooldown_min_reporters` follows `adaptive_min_reports` per band when enabled, and cooldown rejections log as `reason=cooldown` in the decision DB. Calls ending in `/B` (standard beacon IDs) are auto-tagged and bypass correction/harmonic/license drops (only user filters can hide them). The license gate uses a license-normalized base call (e.g., `W6/UT5UF` -> `UT5UF`) to decide if FCC checks apply and which call to query, while CTY metadata still reflects the portable/location prefix (so `N2WQ/VE3` reports Canada for DXCC but uses `N2WQ` for licensing); drops appear in the "Unlicensed US Calls" pane.
8. **Skimmer Frequency Corrections** (`cmd/rbnskewfetch`, `skew/`, `rbn/client.go`, `pskreporter/client.go`) download SM7IUN's skew list, convert it to JSON, and apply per-spotter multiplicative factors before any callsign normalization for every CW/RTTY skimmer feed.

### Aho–Corasick Spot Parsing (Non-PSKReporter)

Non-PSKReporter sources (RBN CW/RTTY, RBN digital, and upstream/human telnet feeds) arrive as DX-cluster style text lines (e.g., `DX de ...`). The parser in `rbn/client.go` uses a small Aho–Corasick (AC) automaton to recognize keywords in a single pass and drive a left-to-right extraction.

High-level flow:

- **Keyword dictionary**: `DX`, `DE`, `DB`, `WPM`, plus all supported mode tokens (`CW`, `SSB` as an alias normalized to `USB`/`LSB`, `USB`, `LSB`, `JS8`, `SSTV`, `RTTY`, `FT4`, `FT8`, `MSK144`, and common variants like `FT-8`).
- **Automaton build (once)**: patterns are compiled into a trie and failure links are built with a BFS. This runs once and is reused for every line.
- **Per-line scan**:
  - Tokenize the raw line on whitespace while tracking token byte offsets.
  - Run the AC scan over the uppercased line to find keyword hits.
  - Classify each token by checking for an exact hit that spans the token (fallback: scan the token text itself when whitespace/punctuation causes slight drift).
- **Single pass extraction**: walk tokens left-to-right, consuming fields as they are discovered: spotter call (and optional `CALL:freq` attachment), frequency, DX call, mode, report (`<signed int> dB` or `<signed int>dB`), time (`HHMMZ`), then treat any remaining unconsumed tokens as the free-form comment.
- **Mode inference**: when no explicit mode token exists, infer from `data/config/mode_allocations.yaml` by frequency (fallback: `USB` ≥ 10 MHz else `CW`).
- **Report semantics**: `HasReport` is strictly “report was present in the source line”, so `0 dB` is distinct from “missing report”.

### Call-Correction Distance Tuning
- CW distance can be Morse-aware with weighted/normalized dot-dash costs (configurable via `call_correction.morse_weights`: `insert`, `delete`, `sub`, `scale`; defaults 1/1/2/2).
- RTTY distance can be ITA2-aware with similar weights (configurable via `call_correction.baudot_weights`: `insert`, `delete`, `sub`, `scale`; defaults 1/1/2/2).
- If you prefer plain rune-based Levenshtein, set `call_correction.distance_model_cw: plain` and/or `distance_model_rtty: plain`.
- You can seed call-quality anchors from your own data with `call_correction.quality_priors_file` (format: `CALL SCORE [FREQ_KHZ]`; omit/<=0 freq to apply globally). Higher scores make a call more likely to act as an anchor in that bin.
- You can down-weight noisy reporters via `call_correction.spotter_reliability_file` (format: `SPOTTER WEIGHT 0-1`) and `call_correction.min_spotter_reliability` to ignore spotters below a floor. These weights apply only to call-correction consensus; other processing is unchanged.

## UI Modes (local console)

- `ui.mode: ansi` (default) draws the lightweight ANSI console in the server's terminal when stdout is a TTY. Telnet clients do **not** see this UI.
- `ui.mode: tview` enables the framed tview dashboard (requires an interactive console).
- `ui.mode: headless` disables the local console; logs continue to stdout/stderr.
- `ui.pane_lines` controls ANSI history depth and the visible heights of tview panes.
- Config block (excerpt):
  ```yaml
  ui:
    mode: "ansi"       # ansi | tview | headless
    refresh_ms: 250    # ANSI redraw cadence; 0 disables redraws
    color: true        # ANSI coloring for marked-up lines
    clear_screen: true # Clear the screen each frame; false appends instead
    pane_lines:
      stats: 8
      calls: 20
      unlicensed: 20
      harmonics: 20
      system: 40
  ```

## Data Flow and Spot Record Format

```
[Source: RBN/PSKReporter] → Parser → Ingest CTY/ULS Gate → Dedup (window-driven) → Ring Buffer → Telnet Broadcast
```

```
┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃                         DXCluster Spot Ingestion & Delivery                         ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
	┌────────────────────┐    ┌────────────────────┐    ┌─────────────────────────┐
	│ RBN CW/RTTY client │    │ RBN FT4/FT8 client │    │ PSKReporter MQTT client │
	└──────────┬─────────┘    └──────────┬─────────┘    └──────────┬──────────────┘
		   │                         │                         │
		   ▼                         ▼                         ▼
	┌────────────────────┐    ┌────────────────────┐    ┌─────────────────────────┐
	│ RBN line parsers   │    │ RBN digital parsers│    │ PSKReporter worker pool │
	└──────────┬─────────┘    └──────────┬─────────┘    └──────────┬──────────────┘
		   │                         │                         │
		   ├─────────────────────────┴─────────────────────────┤
		   ▼                                                   ▼
	 ┌────────────────────────────────────────────────────────────────────┐
	 │ Normalize callsigns → ingest CTY/ULS checks → enrich metadata      │
	 │ (shared logic in `spot` + `cty` packages)                           │
	 └──────────────────────────────┬──────────────────────────────────────┘
					│
					▼
			  ┌──────────────────────────────┐
			  │ Dedup engine (cluster/user)  │
			  └───────────────┬──────────────┘
					  │
					  ▼
			  ┌──────────────────────────────┐
			  │ Ring buffer (`buffer/`)      │
			  └───────────────┬──────────────┘
					  │
					  ▼
			  ┌──────────────────────────────┐
			  │ Telnet server (`telnet/`)    │
			  └───────────────┬──────────────┘
					  │
					  ▼
			  Connected telnet clients + filters
```

### Telnet Spot Line Format

The telnet server broadcasts spots as fixed-width DX-cluster lines:

- Exactly **78 characters**, followed by **CRLF** (line endings are normalized in `telnet.Client.Send`).
- Column numbering below is **1-based** (column 1 is the `D` in `DX de `).
- The left side is padded so **mode always starts at column 40**.
- The displayed DX callsign uses the canonical normalized call (portable suffixes stripped) and is truncated to 10 characters to preserve fixed columns (the full normalized callsign is still stored and hashed).
- The right-side tail is fixed so clients can rely on it:
  - Grid: columns 67-70 (4 chars; blank if unknown)
  - Confidence: column 72 (1 char; blank if unknown)
  - Time: columns 74-78 (`HHMMZ`)
- Any free-form comment text is sanitized (tabs/newlines removed) and truncated so it can never push the grid/confidence/time tail; a space always separates the comment area from the fixed tail.

Report formatting:

- If a report is present: `MODE <report> dB` (e.g., `FT8 -12 dB`, `CW 27 dB`, `MSK144 +7 dB`).
- Human spots without SNR omit the report entirely and show only `MODE`.

Example:

```
DX de W3LPL:       7009.5  K1ABC       FT8 -5 dB                          FN20 S 0615Z
```

Each `spot.Spot` stores:
- **ID** - monotonic identifier
- **DXCall / DECall** - normalized callsigns (portable suffixes stripped, location prefixes like `/VE3` retained; validation accepts 3-15 characters; telnet display truncates DX to 10 as described above)
- **Frequency** (kHz), **Band**, **Mode**, **Report** (dB/SNR), **HasReport** (distinguishes missing SNR from a real 0)
- **Time** - UTC timestamp from the source
- **Comment** - free-form message (human ingest strips mode/SNR/time tokens before storing)
- **SourceType / SourceNode** - origin tags (`RBN`, `FT8`, `FT4`, `PSKREPORTER`, `UPSTREAM`, etc.)
- **TTL** - hop count preventing loops
- **IsHuman** - whether the spot was reported by a human operator (RBN/PSKReporter spots are skimmers; peer/upstream/manual spots are human)
- **IsBeacon** - true when the DX call ends with `/B` or the comment mentions `NCDXF`/`BEACON` (used to suppress beacon corrections/filtering)
- **DXMetadata / DEMetadata** - structured `CallMetadata` each containing:
	- `Continent`
	- `Country`
	- `CQZone`
	- `ITUZone`
	- `ADIF` (DXCC/ADIF country code)
	- `Grid`

All ingest sources run through a shared CTY/ULS validation gate before deduplication. Callsigns are normalized once (uppercased, dots converted to `/`, trailing slashes removed, and portable suffixes like `/P`, `/M`, `/MM`, `/AM`, `/QRP` stripped) before CTY lookup, so W1AW and W1AW/P collapse to the same canonical call for hashing and filters. Location prefixes (for example `/VE3`) are retained; CTY lookup then chooses the shortest matching slash segment so `N2WQ/VE3` and `VE3/N2WQ` both resolve to `VE3` for metadata. Validation still requires at least one digit to avoid non-amateur identifiers before malformed or unknown calls are filtered out. Automated feeds mark the `IsHuman` flag as `false` so downstream processors can tell which spots originated from telescopic inputs versus human operator submissions. Call correction re-validates suggested DX calls against CTY before accepting them; FCC license gating runs after correction using a license-normalized base call (for example, `W6/UT5UF` is evaluated as `UT5UF`) and drops unlicensed US calls (beacons bypass this drop; user filters still apply).

## Commands

Telnet clients can issue commands via the prompt once logged in. The processor, located in `commands/processor.go`, supports the following general commands:

- `HELP` / `H` – display the help text that includes short summaries and valid bands/modes at the bottom of the message.
- `SHOW DX [N]` / `SHOW/DX [N]` - stream the most recent `N` spots (`N` ranges from 1-100, default 10). When the Pebble archive is enabled, results come from the archive (newest-first); otherwise they fall back to the in-memory ring buffer. The command accepts the alias `SH DX` as well.
- `SHOW MYDX [N]` - like `SHOW DX`, but runs archived/ring-buffered spots through the logged-in user's filters (self-spots always pass). Very narrow filters may return fewer than `N` results.
- `BYE`, `QUIT`, `EXIT` - request a graceful logout; the server replies with `73!` and closes the connection.

Filter management commands use a table-driven engine in `telnet/server.go` with explicit dialect selection. The default `go` dialect uses `PASS`/`REJECT`/`SHOW FILTER`. A CC-style subset is available via `DIALECT cc` (aliases: `SET/ANN`, `SET/NOANN`, `SET/BEACON`, `SET/NOBEACON`, `SET/WWV`, `SET/NOWWV`, `SET/WCY`, `SET/NOWCY`, `SET/SKIMMER`, `SET/NOSKIMMER`, `SET/<MODE>`, `SET/NO<MODE>`, `SET/FILTER DXBM/PASS|REJECT <band>` mapping CC DXBM bands to our band filters, `SET/NOFILTER`, plus `SET/FILTER`/`UNSET/FILTER`/`SHOW/FILTER`). `DIALECT LIST` shows the available dialects, the chosen dialect is persisted per callsign along with filter state, and HELP renders the verbs for the active dialect. Classic/go commands operate on each client's `filter.Filter` and fall into `PASS`, `REJECT`, and `SHOW FILTER` groups:

- `SHOW FILTER` - prints the current filter state for bands, modes, source category, continents, CQ zones, and callsigns.
- `SHOW FILTER MODES` - lists every supported mode along with whether it is currently enabled for the session.
- `SHOW FILTER BANDS` - lists all supported bands that can be enabled.
- `SHOW FILTER DXCONT` / `DECONT` - list supported DX/spotter continents and enabled state.
- `SHOW FILTER DXZONE` / `DEZONE` - list CQ zones (1-40) and enabled state for DX/spotter.
- `SHOW FILTER DXGRID2` / `DEGRID2` - list enabled 2-character DX/DE grid prefixes or `ALL`.
- `PASS BAND <band>[,<band>...]` - enables filtering for the comma- or space-separated list (each item normalized via `spot.NormalizeBand`), or specify `ALL` to accept every band; use the band names from `spot.SupportedBandNames()`.
- `PASS MODE <mode>[,<mode>...]` - enables one or more modes (comma- or space-separated) that must exist in `filter.SupportedModes`, or specify `ALL` to accept every mode.
- `PASS SOURCE <HUMAN|SKIMMER|ALL>` - filter by spot origin: `HUMAN` passes only spots with `IsHuman=true`, `SKIMMER` passes only spots with `IsHuman=false`, and `ALL` disables source filtering.
- `PASS DXCONT <cont>[,<cont>...]` / `DECONT <cont>[,<cont>...]` - enable only the listed DX/spotter continents (AF, AN, AS, EU, NA, OC, SA), or `ALL`.
- `PASS DXZONE <zone>[,<zone>...]` / `DEZONE <zone>[,<zone>...]` - enable only the listed DX/spotter CQ zones (1-40), or `ALL`.
- `PASS DXGRID2 <grid>[,<grid>...]` - enable only the listed 2-character DX grid prefixes. Tokens longer than two characters are truncated (e.g., `FN05` -> `FN`); `ALL` resets to accept every DX 2-character grid.
- `PASS DEGRID2 <grid>[,<grid>...]` - enable only the listed 2-character DE grid prefixes (same parsing/truncation as DXGRID2); `ALL` resets to accept every DE 2-character grid.
- `PASS DXCALL <pattern>` - begins delivering only spots with DX calls matching the supplied pattern.
- `PASS DECALL <pattern>` - begins delivering only spots with DE/spotter calls matching the supplied pattern.
- `PASS CONFIDENCE <symbol>[,<symbol>...]` - enables the comma- or space-separated list of consensus glyphs (valid symbols: `?`, `S`, `C`, `P`, `V`, `B`; use `ALL` to accept every glyph).
- `PASS BEACON` - explicitly enable delivery of beacon spots (DX calls ending `/B`; enabled by default).
- `REJECT ALL` - resets every filter back to the default (no filtering).
- `REJECT BAND <band>[,<band>...]` - disables only the comma- or space-separated list of bands provided (use `ALL` to block every band).
- `REJECT MODE <mode>[,<mode>...]` - disables only the comma- or space-separated list of modes provided (specify `ALL` to block every mode).
- `REJECT SOURCE <HUMAN|SKIMMER>` - blocks one origin category (human/operator spots vs automated/skimmer spots).
- `REJECT DXCONT` / `DECONT` / `DXZONE` / `DEZONE` - block continent/zone filters (use `ALL` to block all).
- `REJECT DXGRID2 <grid>[,<grid>...]` - remove specific 2-character DX grid prefixes (tokens truncated to two characters); `ALL` blocks every DX 2-character grid.
- `REJECT DEGRID2 <grid>[,<grid>...]` - remove specific 2-character DE grid prefixes (tokens truncated to two characters); `ALL` blocks every DE 2-character grid.
- `REJECT DXCALL` - removes all DX callsign patterns.
- `REJECT DECALL` - removes all DE callsign patterns.
- `REJECT CONFIDENCE <symbol>[,<symbol>...]` - disables only the comma- or space-separated list of glyphs provided (use `ALL` to block every glyph).
- `REJECT BEACON` - drop beacon spots entirely (they remain tagged internally for future processing).
- `SHOW FILTER BEACON` - display the current beacon-filter state.
- `SHOW FILTER CONFIDENCE` - lists each glyph alongside whether it is currently enabled.

Confidence glyphs are only emitted for modes that run consensus-based correction (CW/RTTY/USB/LSB voice modes). FT8/FT4 spots carry no confidence glyphs, so confidence filters do not affect them. After correction assigns `P`/`V`/`C`/`?`, any remaining `?` is upgraded to `S` when the DX call is present in `MASTER.SCP`.

Band, mode, confidence, and DXGRID2/DEGRID2 commands share identical semantics: they accept comma- or space-separated lists, ignore duplicates/case, and treat the literal `ALL` as a shorthand to reset that filter back to "allow every band/mode/confidence glyph/2-character grid." DXGRID2 applies only to the DX grid when it is exactly two characters long; DEGRID2 applies only to the DE grid when it is exactly two characters long. 4/6-character or empty grids are unaffected, and longer tokens provided by the user are truncated to their first two characters before validation.

Confidence indicator legend in telnet output:

- `?` - Unknown/low support
- `S` - DX call is present in `MASTER.SCP` and the post-correction confidence would otherwise be `?`
- `P` - 25-50% consensus for the subject call (no correction applied)
- `V` - More than 50% consensus for the subject call (no correction applied)
- `B` - Correction was suggested but CTY validation failed (call left unchanged)
- `C` - Callsign was corrected and CTY-validated

### Telnet Reputation Gate

The passwordless reputation gate throttles telnet `DX` commands based on call history, ASN/geo consistency, and prefix pressure. It is designed to slow down new or suspicious senders while keeping known-good calls flowing. Drops are silent to clients, but surfaced in the console stats and system pane.

Core behavior:
- New calls wait an initial probation window before any spots are accepted.
- Per-band limits ramp by one each window up to a cap; total cap increases after ramp completion.
- Country mismatch (IP vs CTY) or IPinfo/Cymru disagreement adds an extra delay before ramping.
- New ASN or geo flips reset the call to probation.
- Prefix token buckets (/24, /48) shed load before per-call limits.

Data sources:
- IPinfo Lite daily snapshot (local CSV, zero-latency lookup) is the primary source.
- Team Cymru DNS is an optional fallback on snapshot misses or staleness.
  - The snapshot downloader uses `curl` with the configured token and unzips to `ipinfo_snapshot_path`.

Configuration:
- See the `reputation` section in `config.yaml` (or `data/config/*.yaml`) for all thresholds, download paths, and observability knobs.

Use `PASS CONFIDENCE` with the glyphs above to whitelist the consensus levels you want to see (for example, `PASS CONFIDENCE P,V` keeps strong/very strong reports while dropping `?`/`S`/`B` entries).

Use `REJECT BEACON` to suppress DX beacons when you only want live operator traffic; `PASS BEACON` re-enables them, and `SHOW FILTER BEACON` reports the current state. Regardless of delivery, `/B` spots are excluded from call-correction, frequency-averaging, and harmonic checks.
Errors during filter commands return a usage message (e.g., invalid bands or modes refer to the supported lists) and the `SHOW FILTER` commands help confirm the active settings.

Continent and CQ-zone filters behave like the band/mode whitelists: start permissive, tighten with `PASS`, reset with `ALL`. When a continent/zone filter is active, spots missing that metadata are rejected so the whitelist cannot be bypassed by incomplete records.

New-user filter defaults are configured in `data/config/runtime.yaml` under `filter:` and are only applied when a callsign has no saved filter file in `data/users/`:

- `filter.default_modes`: initial mode selection for `PASS/REJECT MODE`.
- `filter.default_sources`: initial SOURCE selection (`HUMAN` for `IsHuman=true`, `SKIMMER` for `IsHuman=false`). Omit the field or list both categories to disable SOURCE filtering (equivalent to `PASS SOURCE ALL`).

Existing users keep whatever is stored in their `data/users/<CALL>.yaml` file; changing these defaults only affects newly created users.

## RBN Skew Corrections

1. Enable the `skew` block in `data/config/data.yaml` (the server writes to `skew.file` after each refresh):

```yaml
skew:
  enabled: true
  url: "https://sm7iun.se/rbnskew.csv"
  file: "data/skm_correction/rbnskew.json"
```

2. (Optional) Run `go run ./cmd/rbnskewfetch -out data/skm_correction/rbnskew.json` once to pre-seed the JSON file before enabling the feature.
3. Restart the cluster. At startup, it loads the JSON file (if present) and then fetches the CSV at the next `skew.refresh_utc` boundary (default `00:30` UTC). The built-in scheduler automatically refreshes the list every day at that UTC time and rewrites `skew.file`, so no external cron job is required.

Each RBN spot uses the *raw* spotter string (SSID intact, before any normalization) to look up the correction. If found, the original frequency is multiplied by the factor before any dedup, CTY validation, call correction, or harmonic detection runs. This keeps SSID-specific skew data aligned with the broadcast nodes.

To match the 100 Hz accuracy of the underlying skimmers, the corrected frequency is rounded to the nearest 0.1 kHz before it continues through the pipeline.

## Known Calls Cache

1. Populate the `known_calls` block in `data/config/data.yaml`:

```yaml
 known_calls:
  enabled: true
  url: "https://www.supercheckpartial.com/MASTER.SCP"
  file: "data/scp/MASTER.SCP"
  refresh_utc: "01:15"
```

2. On startup the server checks `known_calls.file`. If it is missing and `known_calls.url` is set, the file is downloaded immediately before any spots are processed. The freshly written file is then parsed into the in-memory cache so consensus/confidence checks can use it right away.
3. When `known_calls.enabled` is true, the built-in scheduler refreshes the file every day at `known_calls.refresh_utc` (default `01:00` UTC). Each download writes to a temporary file, swaps it into place atomically, and updates the runtime cache without needing a restart.

You can disable the scheduler by setting `known_calls.enabled: false`. In that mode the server will still load whatever file already exists (and will fetch it once at startup if an URL is provided), but it will not refresh it automatically.

## CTY Database Refresh

1. Configure the `cty` block in `data/config/data.yaml`:

```yaml
cty:
  enabled: true
  url: "https://www.country-files.com/cty/cty.plist"
  file: "data/cty/cty.plist"
  refresh_utc: "00:45"
```

2. On startup the server downloads `cty.plist` if it is missing and a URL is configured, then loads it into the CTY trie/cache used by lookups. If the file already exists it is loaded directly.
3. When `cty.enabled` is true, the scheduler downloads the plist daily at `cty.refresh_utc`, writes it via an atomic temp-file swap, and reloads the in-memory CTY database so new prefixes are used without a restart. Failures log a warning and retry with backoff; the last-good CTY DB remains active.
4. The stats pane includes a `CTY: age ...` line that shows how long it has been since the last successful refresh (and a failure count when retries are failing), so staleness is visible at a glance.

## FCC ULS Downloads

1. Configure the `fcc_uls` block in `data/config/data.yaml`:

```yaml
fcc_uls:
  enabled: true
  url: "https://data.fcc.gov/download/pub/uls/complete/l_amat.zip"
  archive_path: "data/fcc/l_amat.zip"
  db_path: "data/fcc/fcc_uls.db"
  refresh_utc: "02:15"
```

2. On startup the cluster launches a background job that downloads the archive if it is missing or stale (using conditional requests when a metadata file exists), extracts the AM/EN/HD tables, and builds a fresh SQLite database at `fcc_uls.db_path`. Both the ZIP and DB are written via temp files and swapped atomically; metadata/status is stored at `archive_path + ".status.json"` (the previous `.meta.json` is still read for compatibility).
3. During the load, only active licenses are kept (`HD.license_status = 'A'`). HD is slimmed to a few useful fields (unique ID, call sign, status, service, grant/expire/cancel/last-action dates), and AM is reduced to just unique ID + call sign for active records. EN is not loaded. The downloaded ZIP is deleted after a successful build to save space.
4. When `fcc_uls.enabled` is true, a built-in scheduler refreshes the archive and rebuilds the database once per day at `fcc_uls.refresh_utc` (UTC). The job runs independently of spot processing, so the rest of the cluster continues handling spots while the download, unzip, and load proceed.
5. The console/TUI stats include an FCC line showing active-record counts and the DB size.

## Grid Persistence and Caching

- Grids and known-call flags are stored in Pebble at `grid_db` (default `data/grids/pebble`, a directory). Each batch is committed with `Sync` for durability; the in-memory cache continues serving while backfills rebuild on new spots.
- Writes are batched by `grid_flush_seconds` (default `60s`); a final flush runs during shutdown.
- The in-memory grid cache is a bounded LRU of size `grid_cache_size` (default `100000`). Cache misses fall back to Pebble via the async backfill path when `grid_db_check_on_miss` is true, keeping the output path non-blocking.
- Pebble tuning knobs (defaults tuned for read-heavy durability): `grid_block_cache_mb=64`, `grid_bloom_filter_bits=10`, `grid_memtable_size_mb=32`, `grid_l0_compaction_threshold=4`, `grid_l0_stop_writes_threshold=16`, `grid_write_queue_depth=64`.
- The stats line `Grids: +X / Y since start / Z in DB` counts accepted grid changes (not repeated identical reports); `Z` is the current entry count maintained in Pebble metadata.
- If you set `grid_ttl_days > 0`, the store purges rows whose `updated_at` timestamp is older than that many days right after each SCP refresh. Continuous SCP membership or live grid updates keep records fresh automatically.
- `grid_preflight_timeout_ms` is ignored for the Pebble prototype (retained for config compatibility).

## Runtime Logs and Corrections

- **Call corrections**: `2025/11/19 18:50:45 Call corrected: VE3N -> VE3NE at 7011.1 kHz (8 / 88%)`
- **Frequency averaging**: `2025/11/19 18:50:45 Frequency corrected: VE3NE 7011.3 -> 7011.1 kHz (8 / 88%)`
- **Harmonic suppression**: `2025/11/19 18:50:45 Harmonic suppressed: VE3NE 14022.0 -> 7011.0 kHz (3 / 18 dB)` plus a paired frequency-corrected line indicating the fundamental retained.
- **Stats ticker** (per `stats.display_interval_seconds`): `PSKReporter: <TOTAL> TOTAL / <CW> CW / <RTTY> RTTY / <FT8> FT8 / <FT4> FT4 / <MSK144> MSK144`

### Sample Session

Below is a hypothetical telnet session showing the documented commands in action (server replies are shown after each input):

```
telnet localhost 9300
Experimental DX Cluster
Please login with your callsign

Enter your callsign:
N1ABC
Hello N1ABC, you are now connected.
Type HELP for available commands.
HELP
Available commands:
... (supported modes/bands summary)
SHOW/DX 5
DX1 14.074 FT8 599 N1ABC>W1XYZ
DX2 14.070 FT4 26 N1ABC>W2ABC
...
PASS BAND 20M
Filter set: Band 20M
PASS MODE FT8,FT4
Filter set: Modes FT8, FT4
PASS CONFIDENCE P,V
Confidence symbols enabled: P, V
SHOW FILTER MODES
Supported modes: FT8=ENABLED, FT4=ENABLED, CW=DISABLED, ...
SHOW FILTER CONFIDENCE
Confidence symbols: ?=DISABLED, S=DISABLED, C=ENABLED, P=ENABLED, V=ENABLED, B=DISABLED
SHOW FILTER
Current filters: Bands: 20M | Modes: FT8, FT4 | Confidence: P, V
REJECT MODE FT4
Mode filters disabled: FT4
REJECT ALL
All filters cleared
BYE
73!
```

Use these commands interactively to tailor the spot stream to your operating preferences.

### Telnet Throughput Controls

The telnet server fans every post-dedup spot to every connected client. When PSKReporter or both RBN feeds spike, the broadcast queue can saturate and you'll see `Broadcast channel full, dropping spot` along with a rising `Telnet drops` metric in the stats ticker. Tune the `telnet` block in `data/config/runtime.yaml` to match your load profile:

- `broadcast_workers` keeps the existing behavior (`0` = auto at half your CPUs, minimum 2).
- `broadcast_queue_size` controls the global queue depth ahead of the worker pool (default `2048`); larger buffers smooth bursty ingest before anything is dropped.
- `worker_queue_size` controls how many per-shard jobs each worker buffers before dropping a shard assignment (default `128`).
- `client_buffer_size` defines how many spots a single telnet session can fall behind before its personal queue starts dropping (default `128`).
- `broadcast_batch_interval_ms` micro-batches outbound broadcasts to reduce mutex/IO churn (default `250`; set to `0` for immediate sends). Each shard flushes on interval or when the batch reaches its max size, preserving order per shard.
- `login_line_limit` caps how many bytes a user can enter at the login prompt (default `32`). Keep this tight to prevent hostile clients from allocating massive buffers before authentication.
- `command_line_limit` caps how long any post-login command may be (default `128`). Raise this when operators expect comma-heavy filter commands or scripted clients that send longer payloads.
- `keepalive_seconds` emits a CRLF to every connected client on a cadence (default `60`; `0` disables). Blank lines sent by clients are treated as keepalives and get an immediate CRLF reply so idle TCP sessions stay open.

Increase the queue sizes if you see the broadcast-channel drop message frequently, or raise `broadcast_workers` when you have CPU headroom and thousands of concurrent clients.

### Archive Durability (Pebble)

The optional Pebble archive is built to stay out of the hot path: enqueue is non-blocking and drops when backpressure builds. With the archive enabled, you can tune durability vs throughput:

- `archive.synchronous`: defaults to `off` for maximum throughput when the archive is disposable; `normal`, `full`, or `extra` enable fsync for stronger crash safety.
- `archive.auto_delete_corrupt_db`: when true, the server deletes the archive directory on startup if Pebble reports corruption (or the path is not a directory), then recreates an empty store.
- `archive.busy_timeout_ms` and `archive.preflight_timeout_ms` are ignored for Pebble (retained for compatibility).

Operational guidance: enable `auto_delete_corrupt_db` only if the archive is truly disposable. If you need to preserve data through crashes, leave auto-delete off and raise synchronous to `normal`/`full` (or disable the archive entirely).

## Project Structure

```
.
├─ data/config/            # Runtime configuration (split YAML files)
│  ├─ app.yaml             # Server identity, stats interval, console UI
│  ├─ ingest.yaml          # RBN/PSKReporter/human ingest + call cache
│  ├─ pipeline.yaml        # Dedup, call correction, harmonics, spot policy
│  ├─ data.yaml            # CTY/known_calls/FCC/skew + grid DB tuning
│  ├─ runtime.yaml         # Telnet server settings, buffer/filter defaults
│  └─ mode_allocations.yaml # Mode inference for RBN/human ingest
├─ config/                 # YAML loader + defaults (merges dir or single file)
├─ cmd/                    # Helper binaries (CTY lookup, skew fetch, analysis)
├─ rbn/, pskreporter/, telnet/, dedup/, filter/, spot/, stats/, gridstore/  # Core packages
├─ data/cty/cty.plist      # CTY prefix database for metadata lookups
├─ go.mod / go.sum         # Go module definition + checksums
└─ main.go                 # Entry point wiring ingest, protections, telnet server
```
## Code Walkthrough

- `main.go` glues together ingest clients (RBN/PSKReporter), protections (dedup, call correction, harmonics, frequency averaging), persistence (grid store), telnet server, dashboard, schedulers (FCC ULS, known calls, skew), and graceful shutdown. Helpers are commented so you can follow the pipeline without prior cluster context.
- `telnet/server.go` documents the connection lifecycle, broadcast sharding, filter commands, and how per-client filters interact with the shared ring buffer.
- `buffer/` explains the lock-free ring buffer used by SHOW/DX and broadcasts; it stores atomic spot pointers and IDs to avoid partial reads.
- `config/` describes the YAML schema, default normalization, and `Print` diagnostics. The “Configuration Loader Defaults” section mirrors these behaviors.
- `cty/` covers longest-prefix CTY lookups and cache metrics. `spot/` holds the canonical spot struct, formatting, hashing, validation, callsign utilities, harmonics/frequency averaging/correction helpers, and known-calls cache.
- `dedup/`, `filter/`, `gridstore/`, `skew/`, and `uls/` each have package-level docs and function comments outlining how they feed or persist data without blocking ingest.
- `rbn/` and `pskreporter/` detail how each source is parsed, normalized, skew-corrected, and routed into the ingest CTY/ULS gate before deduplication.
- `commands/` and `cmd/*` binaries include focused comments explaining the helper CLIs for SHOW/DX, CTY lookup, and skew prefetching.

## Getting Started

1. Update `data/config/ingest.yaml` with your preferred callsigns for the `rbn`, `rbn_digital`, optional `human_telnet`, and optional `pskreporter` sections. Optionally list `pskreporter.modes` (e.g., [`FT8`, `FT4`]) to accept only those modes after subscribing to the catch-all topic. If you enable `human_telnet`, review `data/config/mode_allocations.yaml` (used to infer CW vs LSB/USB when the incoming spot line does not include an explicit mode token).
2. If peering with DXSpider clusters, edit `data/config/peering.yaml`: set `local_callsign`, optional `listen_port`, hop/version fields, and add one or more peers (host/port/password/prefer_pc9x). ACLs are available for inbound (`allow_ips`/`allow_callsigns`). Topology persistence is optional (disabled by default); set `peering.topology.db_path` to enable SQLite caching (WAL mode) with the configured retention.
2. Optionally enable/tune `call_correction` (master `enabled` switch, minimum corroborating spotters, required advantage, confidence percent, recency window, max edit distance, per-mode distance models, and `invalid_action` failover). `distance_model_cw` switches CW between the baseline rune-based Levenshtein (`plain`) and a Morse-aware cost function (`morse`), `distance_model_rtty` toggles RTTY between `plain` and a Baudot/ITA2-aware scorer (`baudot`), while USB/LSB voice modes always stay on `plain` because those reports are typed by humans.
3. Optionally enable/tune `harmonics` to drop harmonic CW/USB/LSB/RTTY spots (master `enabled`, recency window, maximum harmonic multiple, frequency tolerance, and minimum report delta).
4. Set `spot_policy.max_age_seconds` to drop stale spots before they're processed further. For CW/RTTY frequency smoothing, tune `spot_policy.frequency_averaging_seconds` (window), `spot_policy.frequency_averaging_tolerance_hz` (allowed deviation), and `spot_policy.frequency_averaging_min_reports` (minimum corroborating reports).
5. (Optional) Enable `skew.enabled` after generating `skew.file` via `go run ./cmd/rbnskewfetch` (or let the server fetch it at the next 00:30 UTC window). The server applies each skimmer's multiplicative correction before normalization so SSIDs stay unique.
6. If you maintain a historical callsign list, set `known_calls.file` plus `known_calls.url` (leave `enabled: true` to keep it refreshed). On first launch the server downloads the file if missing, loads it into memory, and then refreshes it daily at `known_calls.refresh_utc`.
7. Grids/known calls are persisted in Pebble (`grid_db`, default `data/grids/pebble`). Tune `grid_flush_seconds` for batch cadence, `grid_cache_size` to bound the in-memory LRU used for grid comparisons, `grid_block_cache_mb`/`grid_bloom_filter_bits`/`grid_memtable_size_mb`/`grid_l0_stop_writes_threshold` for Pebble read/write tuning, and `grid_write_queue_depth`/`grid_ttl_days` for buffering and retention.
8. Adjust `stats.display_interval_seconds` in `data/config/app.yaml` to control how frequently runtime statistics print to the console (defaults to 30 seconds).
9. Install dependencies and run:
	 ```pwsh
	 go mod tidy
	 go run .
	 ```
10. Connect via `telnet localhost 9300` (or your configured `telnet.port`), enter your callsign, and the server will immediately stream real-time spots.

## Configuration Loader Defaults

`config.Load` accepts a directory (merging all YAML files) or a single YAML file; the server defaults to `data/config`. It normalizes missing fields and refuses to start when time strings are invalid. Key fallbacks:

- Stats tickers default to `30s` when unset. Telnet queues fall back to `broadcast_queue_size=2048`, `worker_queue_size=128`, `client_buffer_size=128`, and friendly greeting/duplicate-login messages are injected if blank.
- Call correction uses conservative baselines unless overridden: `min_consensus_reports=4`, `min_advantage=1`, `min_confidence_percent=70`, `recency_seconds=45`, `max_edit_distance=2`, `frequency_tolerance_hz=0.5`, `voice_frequency_tolerance_hz=2000`, `voice_candidate_window_khz=2`, `min_snr_voice=0`, `invalid_action=broadcast`. Empty distance models inherit from `distance_model` or default to `plain`; negative SNR floors/extras are clamped to zero.
- Harmonic suppression clamps to sane minimums (`recency_seconds=120`, `max_harmonic_multiple=4`, `frequency_tolerance_hz=20`, `min_report_delta=6`, `min_report_delta_step>=0`).
- Spot policy defaults prevent runaway averaging: `max_age_seconds=120`, `frequency_averaging_seconds=45`, `frequency_averaging_tolerance_hz=300`, `frequency_averaging_min_reports=4`.
- Archive defaults keep writes lightweight: `queue_size=10000`, `batch_size=500`, `batch_interval_ms=200`, `cleanup_interval_seconds=3600`, `synchronous=off`, `auto_delete_corrupt_db=false` (`busy_timeout_ms`/`preflight_timeout_ms` are ignored for Pebble).
- Known calls default to `data/scp/MASTER.SCP` and refresh at `01:00` UTC if unspecified. CTY falls back to `data/cty/cty.plist`.
- FCC ULS fetches use the official URL/paths (`archive_path=data/fcc/l_amat.zip`, `db_path=data/fcc/fcc_uls.db`, `temp_dir` inherits from `db_path`), and refresh times must parse as `HH:MM` or loading fails fast.
- Grid store defaults: `grid_db=data/grids/pebble`, `grid_flush_seconds=60`, `grid_cache_size=100000`, `grid_block_cache_mb=64`, `grid_bloom_filter_bits=10`, `grid_memtable_size_mb=32`, `grid_l0_compaction_threshold=4`, `grid_l0_stop_writes_threshold=16`, `grid_write_queue_depth=64`, with TTL/retention floors of zero to avoid negative durations.
- Dedup windows are coerced to zero-or-greater; `output_buffer_size` defaults to `1000` so bursts do not immediately drop spots.
- Buffer capacity defaults to `300000` spots; skew downloads default to SM7IUN's CSV (`url=https://sm7iun.se/rbnskew.csv`, `file=data/skm_correction/rbnskew.json`, `refresh_utc=00:30`) with non-negative `min_spots`.
- `config.Print` writes a concise summary of the loaded settings to stdout for easy startup diagnostics.

## Testing & Tooling

- `go test ./...` validates packages (not all directories contain tests yet).
- `gofmt -w ./...` keeps formatting consistent.
- `go run cmd/ctylookup -data data/cty/cty.plist` lets you interactively inspect CTY entries used for validation (portable calls resolve by location prefix).
- `go run cmd/rbnskewfetch -out data/skm_correction/rbnskew.json` forces an immediate skew-table refresh (the server still performs automatic downloads at 00:30 UTC daily).

Let me know if you want diagrams, sample logs, or scripted deployment steps added next.
