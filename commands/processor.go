// Package commands implements the minimal command processor used by telnet
// sessions. It focuses on HELP/SHOW/DX/SHOW MYDX and defers filter manipulation
// to the telnet package so both layers stay small and easy to reason about.
package commands

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"dxcluster/buffer"
	"dxcluster/cty"
	"dxcluster/filter"
	"dxcluster/reputation"
	"dxcluster/spot"
)

// archiveReader is the minimal interface the archive layer exposes for read paths.
type archiveReader interface {
	Recent(limit int) ([]*spot.Spot, error)
	RecentFiltered(limit int, match func(*spot.Spot) bool) ([]*spot.Spot, error)
}

// Processor handles telnet command parsing and replies that rely on shared state
// (recent spots in the ring buffer).
type Processor struct {
	spotBuffer *buffer.RingBuffer
	archive    archiveReader
	spotInput  chan<- *spot.Spot
	ctyLookup  func() *cty.CTYDatabase
	prefixIdx  *prefixIndex
	repGate    *reputation.Gate
	repReport  func(reputation.DropEvent)
}

// Purpose: Construct a command processor bound to shared spot state.
// Key aspects: SHOW/DX prefers archive when present; DX commands can enqueue spots.
// Upstream: Telnet server initialization.
// Downstream: Processor methods (ProcessCommand, handleShowDX, handleDX).
func NewProcessor(buf *buffer.RingBuffer, archive archiveReader, spotInput chan<- *spot.Spot, ctyLookup func() *cty.CTYDatabase, repGate *reputation.Gate, repReport func(reputation.DropEvent)) *Processor {
	return &Processor{
		spotBuffer: buf,
		archive:    archive,
		spotInput:  spotInput,
		ctyLookup:  ctyLookup,
		prefixIdx:  &prefixIndex{},
		repGate:    repGate,
		repReport:  repReport,
	}
}

// Purpose: Parse a command and return the response text.
// Key aspects: "BYE" signals the caller to close the session.
// Upstream: Telnet client command loop.
// Downstream: ProcessCommandForClient.
func (p *Processor) ProcessCommand(cmd string) string {
	return p.ProcessCommandForClient(cmd, "", "", nil, "go")
}

// Purpose: Parse a command with client context for DX posting and filtered history.
// Key aspects: Routes DX commands, SHOW/DX, and SHOW/MYDX with optional filter.
// Upstream: Telnet client command loop with callsign context.
// Downstream: handleDX, handleHelp, handleShow.
func (p *Processor) ProcessCommandForClient(cmd string, spotter string, spotterIP string, filterFn func(*spot.Spot) bool, dialect string) string {
	cmd = strings.TrimSpace(cmd)

	// Empty command
	if cmd == "" {
		return ""
	}

	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return ""
	}
	if strings.EqualFold(fields[0], "DX") {
		return p.handleDX(fields, spotter, spotterIP)
	}

	// Split into parts
	parts := strings.Fields(strings.ToUpper(cmd))
	command := parts[0]

	switch command {
	case "HELP", "H":
		return p.handleHelp(dialect)
	case "SH", "SHOW":
		if len(parts) < 2 {
			return "Usage: SHOW/DX [count]\n"
		}
		return p.handleShow(parts[1:], filterFn)
	case "BYE", "QUIT", "EXIT":
		return "BYE"
	default:
		return fmt.Sprintf("Unknown command: %s\nType HELP for available commands.\n", command)
	}
}

// Purpose: Render the HELP text for users.
// Key aspects: Includes filter command guidance and supported bands/modes; tailored per dialect.
// Upstream: ProcessCommandForClient (HELP/H).
// Downstream: filter.SupportedModes, spot.SupportedBandNames.
func (p *Processor) handleHelp(dialect string) string {
	dialect = strings.ToLower(strings.TrimSpace(dialect))
	filterHelp := filterHelpText(dialect)
	return fmt.Sprintf(`Available commands:
HELP                 - Show this help
DX <freq> <call> [comment] - Post a spot (frequency in kHz)
SHOW/DX [count]      - Show last N DX spots (default: 10)
SHOW MYDX [count]    - Show last N DX spots that match your active filters
SHOW DXCC <prefix|callsign> - Look up DXCC/ADIF and zones for a prefix or callsign
SET GRID <grid>      - Set your grid (4-6 chars) for path reliability glyphs
SET NOISE <class>    - Set noise class (QUIET|RURAL|SUBURBAN|URBAN) for glyph penalties
BYE                  - Disconnect
DIALECT [name|LIST]  - Show or set the filter command dialect (go, cc)

Current dialect: %s

%s
Supported modes: %s
Supported bands: %s

Examples:
	SHOW/DX            - Show last 10 spots
	%s
`, strings.ToUpper(normalizeDialectString(dialect)), filterHelp, strings.Join(filter.SupportedModes, ", "), strings.Join(spot.SupportedBandNames(), ", "), exampleForDialect(dialect))
}

func filterHelpText(dialect string) string {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "cc":
		return `Filter commands (CC subset):
	SET/ANN | SET/NOANN      - Enable/disable announcements
	SET/BEACON | SET/NOBEACON - Enable/disable beacon spots
	SET/WWV | SET/NOWWV      - Enable/disable WWV bulletins
	SET/WCY | SET/NOWCY      - Enable/disable WCY bulletins
	SET/SKIMMER | SET/NOSKIMMER - Allow/block skimmer spots
	SET/<MODE> | SET/NO<MODE> - Enable/disable mode (CW, FT4, FT8, RTTY)
	SET/FILTER DXBM/PASS <band>[,...]   - Map CC DXBM bands to allow (bands: 160, 80, 40, 30, 20, 17, 15, 12, 10, 6, 2, 1). Mode suffix ignored; use SET/NO<MODE>. Approximate: per-band only, not band-mode.
	SET/FILTER DXBM/REJECT <band>[,...] - Map CC DXBM bands to block (same mapping; PASS takes precedence over REJECT in overlaps)
	SET/NOFILTER             - Reset filters to permissive defaults
	SET/FILTER <type> [...]  - Allow (BAND, MODE, SOURCE, DXCALL, DECALL, CONFIDENCE, DXGRID2, DEGRID2, DXCONT, DECONT, DXZONE, DEZONE, DXDXCC, DEDXCC, BEACON, WWV, WCY, ANNOUNCE)
	SET/FILTER <type>/OFF    - Block the specified type (ALL)
	UNSET/FILTER <type> [...] - Block/clear for the specified type
	SHOW/FILTER              - Show the full filter snapshot (tokenized forms deprecated)`
	default:
		return `Filter commands (allow + block, deny wins):
	PASS BAND <band>[,<band>...] - Allow specific bands (comma/space). ALL clears blocklist and allows all.
	PASS MODE <mode>[,<mode>...] - Allow specific modes. ALL clears blocklist and allows all.
	PASS SOURCE <HUMAN|SKIMMER|ALL> - Filter by spot origin: HUMAN=IsHuman true, SKIMMER=IsHuman false. ALL disables SOURCE filtering.
	PASS DXCONT <cont>[,<cont>...] - Allow DX continents (AF, AN, AS, EU, NA, OC, SA). ALL clears blocklist.
	PASS DECONT <cont>[,<cont>...] - Allow DE continents. ALL clears blocklist.
	PASS DXZONE <zone>[,<zone>...] - Allow DX CQ zones (1-40). ALL clears blocklist.
	PASS DEZONE <zone>[,<zone>...] - Allow DE CQ zones. ALL clears blocklist.
	PASS DXDXCC <code>[,<code>...] - Allow DX ADIF/DXCC codes. ALL clears blocklist.
	PASS DEDXCC <code>[,<code>...] - Allow DE ADIF/DXCC codes. ALL clears blocklist.
	PASS DXGRID2 <grid>[,<grid>...] - Allow 2-char DX grids (truncates longer tokens); ALL clears blocklist.
	PASS DEGRID2 <grid>[,<grid>...] - Allow 2-char DE grids; ALL clears blocklist.
	PASS DXCALL <pattern> - Allow DX calls matching the pattern (supports prefix/suffix * wildcard).
	PASS DECALL <pattern> - Allow DE/spotter calls matching the pattern (supports prefix/suffix * wildcard).
	PASS CONFIDENCE <symbol>[,<symbol>...] - Allow confidence glyphs (?,S,C,P,V,B or ALL). FT8/FT4 ignore confidence filtering.
	PASS BEACON - Deliver DX beacons (/B)
	PASS WWV - Deliver WWV bulletins
	PASS WCY - Deliver WCY bulletins
	PASS ANNOUNCE - Deliver PC93 announcements
	REJECT BAND <band>[,<band>...]      - Block listed bands; ALL blocks all bands.
	REJECT MODE <mode>[,<mode>...]      - Block listed modes; ALL blocks all modes.
	REJECT SOURCE <HUMAN|SKIMMER>       - Block human or automated spots.
	REJECT DXCONT <cont>[,<cont>...]    - Block DX continents; ALL blocks all DX continents.
	REJECT DECONT <cont>[,<cont>...]    - Block DE continents; ALL blocks all DE continents.
	REJECT DXZONE <zone>[,<zone>...]    - Block DX CQ zones; ALL blocks all DX zones.
	REJECT DEZONE <zone>[,<zone>...]    - Block DE CQ zones; ALL blocks all DE zones.
	REJECT DXDXCC <code>[,<code>...]    - Block DX ADIF/DXCC codes; ALL blocks all DX DXCCs.
	REJECT DEDXCC <code>[,<code>...]    - Block DE ADIF/DXCC codes; ALL blocks all DE DXCCs.
	REJECT DXGRID2 <grid>[,<grid>...]    - Block 2-char DX grids; ALL blocks all DX grids.
	REJECT DEGRID2 <grid>[,<grid>...]    - Block 2-char DE grids; ALL blocks all DE grids.
	REJECT DXCALL - Clear all DX callsign patterns (arguments ignored; allows any DX call, subject to other filters).
	REJECT DECALL - Clear all DE callsign patterns (arguments ignored; allows any DE call, subject to other filters).
	REJECT CONFIDENCE <symbol>[,<symbol>...] - Block glyphs; ALL blocks all glyphs (non-exempt modes).
	REJECT BEACON - Suppress DX beacons
	REJECT WWV - Suppress WWV bulletins
	REJECT WCY - Suppress WCY bulletins
	REJECT ANNOUNCE - Suppress PC93 announcements
	SHOW FILTER                 - Show the full filter snapshot (tokenized forms deprecated)`
	}
}

func normalizeDialectString(dialect string) string {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "cc":
		return "cc"
	default:
		return "go"
	}
}

func exampleForDialect(dialect string) string {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "cc":
		return "SET/FILTER MODE FT8"
	default:
		return "PASS MODE FT8"
	}
}

// Purpose: Handle the DX command and enqueue a human spot.
// Key aspects: Validates callsign/frequency; parses comment for mode/report.
// Upstream: ProcessCommandForClient (DX).
// Downstream: spot.ParseSpotComment, spot.NewSpot, spotInput channel.
func (p *Processor) handleDX(fields []string, spotter string, spotterIP string) string {
	spotterRaw := strings.TrimSpace(spotter)
	if spotterRaw == "" {
		return "DX command requires a logged-in callsign.\n"
	}
	spotterNorm := spot.NormalizeCallsign(spotterRaw)
	if !spot.IsValidNormalizedCallsign(spotterNorm) {
		return "DX command requires a valid callsign.\n"
	}
	if len(fields) < 3 {
		return "Usage: DX <frequency> <callsign> [comment]\n"
	}
	freq, err := strconv.ParseFloat(fields[1], 64)
	if err != nil || freq <= 0 {
		return "Invalid frequency. Use a kHz value like 7001.0.\n"
	}
	dxRaw := strings.TrimSpace(fields[2])
	dx := spot.NormalizeCallsign(dxRaw)
	if !spot.IsValidNormalizedCallsign(dx) {
		return "Invalid DX callsign.\n"
	}
	if p.ctyLookup != nil {
		if db := p.ctyLookup(); db != nil {
			if _, ok := db.LookupCallsignPortable(dx); !ok {
				return "Unknown DX callsign (not in CTY database).\n"
			}
		}
	}
	if p.repGate != nil {
		now := time.Now().UTC()
		band := spot.FreqToBand(freq)
		decision := p.repGate.Check(reputation.Request{
			Call: spotterNorm,
			Band: band,
			IP:   spotterIP,
			Now:  now,
		})
		if decision.Drop {
			if p.repReport != nil {
				p.repReport(reputation.DropEvent{
					Call:        spotterNorm,
					Band:        band,
					IP:          spotterIP,
					Prefix:      decision.Prefix,
					Reason:      decision.Reason,
					Flags:       decision.Flags,
					ASN:         decision.ASN,
					CountryCode: decision.CountryCode,
					CountryName: decision.CountryName,
					Source:      decision.Source,
					When:        now,
				})
			}
			return ""
		}
	}
	comment := ""
	if len(fields) > 3 {
		comment = strings.TrimSpace(strings.Join(fields[3:], " "))
	}
	parsed := spot.ParseSpotComment(comment, freq)
	s := spot.NewSpotNormalized(dx, spotterNorm, freq, parsed.Mode)
	s.Comment = parsed.Comment
	s.Report = parsed.Report
	s.HasReport = parsed.HasReport
	s.SourceNode = spotterNorm
	s.SpotterIP = strings.TrimSpace(spotterIP)

	if p.spotInput == nil {
		return "Spot input is not configured on this cluster.\n"
	}
	select {
	case p.spotInput <- s:
		return "Spot queued.\n"
	default:
		log.Printf("DX command: dedup input full, dropping spot from %s", spotter)
		return "Spot queue full; try again.\n"
	}
}

// Purpose: Route SHOW subcommands with optional filter predicate.
// Key aspects: Supports SHOW/DX, SHOW/MYDX, and SHOW DXCC lookups.
// Upstream: ProcessCommandForClient (SHOW/SH).
// Downstream: handleShowDX, handleShowMYDX, handleShowDXCC.
func (p *Processor) handleShow(args []string, filterFn func(*spot.Spot) bool) string {
	if len(args) == 0 {
		return "Usage: SHOW/DX [count]\n"
	}

	subCmd := args[0]

	switch subCmd {
	case "DX":
		return p.handleShowDX(args[1:])
	case "MYDX":
		return p.handleShowMYDX(args[1:], filterFn)
	case "DXCC":
		return p.handleShowDXCC(args[1:])
	default:
		return fmt.Sprintf("Unknown SHOW subcommand: %s\n", subCmd)
	}
}

// Purpose: Render the most recent N spots for SHOW/DX.
// Key aspects: Prefers archive; falls back to ring buffer; outputs oldest-first.
// Upstream: handleShow.
// Downstream: archive.Recent, ring buffer, reverseSpotsInPlace.
func (p *Processor) handleShowDX(args []string) string {
	count := 10 // Default count

	// Parse count if provided
	if len(args) > 0 {
		var err error
		_, err = fmt.Sscanf(args[0], "%d", &count)
		if err != nil || count < 1 || count > 100 {
			return "Invalid count. Use 1-100.\n"
		}
	}

	// Prefer archive for history; fall back to ring buffer.
	var spots []*spot.Spot
	if p.archive != nil {
		if rows, err := p.archive.Recent(count); err != nil {
			log.Printf("SHOW DX: archive query failed, falling back to ring buffer: %v", err)
		} else {
			spots = rows
		}
	}
	if len(spots) == 0 && p.spotBuffer != nil {
		spots = p.spotBuffer.GetRecent(count)
	}

	if len(spots) == 0 {
		return "No spots available.\n"
	}

	// Display oldest first so the most recent spot is last in the list.
	reverseSpotsInPlace(spots)

	// Build response
	var result strings.Builder
	for _, spot := range spots {
		result.WriteString(spot.FormatDXCluster())
		result.WriteString("\r\n")
	}

	return result.String()
}

// Purpose: Render the most recent N spots that match the provided filter.
// Key aspects: Prefers archive; falls back to ring buffer; outputs oldest-first.
// Upstream: handleShow (SHOW MYDX).
// Downstream: archive.RecentFiltered, ring buffer filtered read.
func (p *Processor) handleShowMYDX(args []string, filterFn func(*spot.Spot) bool) string {
	if filterFn == nil {
		return "SHOW MYDX requires a logged-in session.\n"
	}
	count := 10 // Default count

	if len(args) > 0 {
		var err error
		_, err = fmt.Sscanf(args[0], "%d", &count)
		if err != nil || count < 1 || count > 100 {
			return "Invalid count. Use 1-100.\n"
		}
	}

	var spots []*spot.Spot
	if p.archive != nil {
		if rows, err := p.archive.RecentFiltered(count, filterFn); err != nil {
			log.Printf("SHOW MYDX: archive query failed, falling back to ring buffer: %v", err)
		} else {
			spots = rows
		}
	}
	if len(spots) == 0 && p.spotBuffer != nil {
		spots = p.spotBuffer.GetRecentFiltered(count, filterFn)
	}

	if len(spots) == 0 {
		return "No spots available.\n"
	}

	reverseSpotsInPlace(spots)

	var result strings.Builder
	for _, spot := range spots {
		result.WriteString(spot.FormatDXCluster())
		result.WriteString("\r\n")
	}

	return result.String()
}

// Purpose: Reverse a slice of spots in place.
// Key aspects: Used to present chronological output.
// Upstream: handleShowDX.
// Downstream: None.
func reverseSpotsInPlace(spots []*spot.Spot) {
	for i, j := 0, len(spots)-1; i < j; i, j = i+1, j-1 {
		spots[i], spots[j] = spots[j], spots[i]
	}
}

// Purpose: Resolve CTY metadata for a prefix or callsign and render DXCC details.
// Key aspects: Uses CTY portable lookup, reports ADIF/country/zones, and lists sibling prefixes for the same ADIF.
// Upstream: handleShow (SHOW DXCC).
// Downstream: CTY lookup, prefixIdx for sibling retrieval.
func (p *Processor) handleShowDXCC(args []string) string {
	if len(args) == 0 {
		return "Usage: SHOW DXCC <prefix|callsign>\n"
	}
	if p.ctyLookup == nil {
		return "CTY database is not available.\n"
	}
	db := p.ctyLookup()
	if db == nil {
		return "CTY database is not loaded.\n"
	}

	queryRaw := strings.TrimSpace(args[0])
	if queryRaw == "" {
		return "Usage: SHOW DXCC <prefix|callsign>\n"
	}
	lookup := spot.NormalizeCallsign(queryRaw)
	if lookup == "" {
		return "Unknown DXCC/prefix.\n"
	}

	info, ok := db.LookupCallsignPortable(lookup)
	if !ok || info == nil {
		return "Unknown DXCC/prefix.\n"
	}

	prefix := strings.ToUpper(strings.TrimSpace(info.Prefix))
	country := strings.TrimSpace(info.Country)
	continent := strings.ToUpper(strings.TrimSpace(info.Continent))
	others := p.prefixIdx.siblings(db, info.ADIF, prefix)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s -> ADIF %d | %s (%s) | Prefix: %s | CQ %d | ITU %d",
		lookup, info.ADIF, country, continent, prefix, info.CQZone, info.ITUZone))
	if len(others) > 0 {
		b.WriteString(" | Other: ")
		b.WriteString(strings.Join(others, ", "))
	}
	b.WriteByte('\n')
	return b.String()
}

// prefixIndex caches ADIF->prefix list mappings for the current CTY DB pointer.
// It builds a fresh map when the DB pointer changes (e.g., after a CTY refresh).
type prefixIndex struct {
	mu             sync.Mutex
	db             *cty.CTYDatabase
	adifToPrefixes map[int][]string
}

func (p *prefixIndex) siblings(db *cty.CTYDatabase, adif int, current string) []string {
	if p == nil || db == nil {
		return nil
	}
	p.mu.Lock()
	if p.db != db {
		p.adifToPrefixes = buildPrefixMap(db)
		p.db = db
	}
	prefixes := p.adifToPrefixes[adif]
	p.mu.Unlock()

	current = strings.ToUpper(strings.TrimSpace(current))
	out := make([]string, 0, len(prefixes))
	for _, pref := range prefixes {
		if pref == "" || pref == current {
			continue
		}
		out = append(out, pref)
	}
	return out
}

func buildPrefixMap(db *cty.CTYDatabase) map[int][]string {
	if db == nil {
		return nil
	}
	tmp := make(map[int]map[string]struct{}, len(db.Data))
	for _, info := range db.Data {
		pref := strings.ToUpper(strings.TrimSpace(info.Prefix))
		if pref == "" {
			continue
		}
		set, ok := tmp[info.ADIF]
		if !ok {
			set = make(map[string]struct{})
			tmp[info.ADIF] = set
		}
		set[pref] = struct{}{}
	}
	result := make(map[int][]string, len(tmp))
	for adif, set := range tmp {
		prefixes := make([]string, 0, len(set))
		for pref := range set {
			prefixes = append(prefixes, pref)
		}
		sort.Strings(prefixes)
		result[adif] = prefixes
	}
	return result
}
