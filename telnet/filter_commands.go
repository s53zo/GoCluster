package telnet

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"dxcluster/filter"
	"dxcluster/spot"
)

// DialectName identifies the command dialect a client is using.
type DialectName string

const (
	// DialectGoCluster is the default PASS/REJECT/SHOW FILTER syntax.
	DialectGoCluster DialectName = "gocluster"
	// DialectCC models a subset of CC Cluster aliases mapped onto existing filter atoms.
	DialectCC DialectName = "cc"
)

type filterAction int

const (
	actionAllow filterAction = iota
	actionBlock
	actionShowState
	actionShowSupported
	actionSummary
)

type parsedFilterCommand struct {
	action filterAction
	domain string
	args   []string
}

type dialectSpec struct {
	name    DialectName
	aliases []string
	parse   func(tokens []string, upper []string) (parsedFilterCommand, bool, string)
}

type domainHandler struct {
	name            string
	aliases         []string
	showSupported   bool
	apply           func(c *Client, action filterAction, args []string) (string, bool)
	showState       func(f *filter.Filter) string
	showSupportedFn func(f *filter.Filter) string
}

type filterCommandEngine struct {
	dialects       map[DialectName]*dialectSpec
	dialectAliases map[string]DialectName
	domains        map[string]*domainHandler
	defaultDialect DialectName
}

func newFilterCommandEngine() *filterCommandEngine {
	engine := &filterCommandEngine{
		dialects:       make(map[DialectName]*dialectSpec),
		dialectAliases: make(map[string]DialectName),
		domains:        make(map[string]*domainHandler),
		defaultDialect: DialectGoCluster,
	}
	engine.registerDialects()
	engine.registerDomains()
	return engine
}

func (e *filterCommandEngine) registerDialects() {
	gocluster := &dialectSpec{
		name:    DialectGoCluster,
		aliases: []string{"gocluster", "classic", "default"},
		parse:   parseClassicDialect,
	}
	cc := &dialectSpec{
		name:    DialectCC,
		aliases: []string{"cc"},
		parse:   parseCCDialect,
	}
	for _, d := range []*dialectSpec{gocluster, cc} {
		e.dialects[d.name] = d
		for _, alias := range d.aliases {
			e.dialectAliases[strings.ToLower(alias)] = d.name
		}
	}
}

func (e *filterCommandEngine) registerDomains() {
	handlers := []*domainHandler{
		newBandHandler(),
		newDXBMHandler(),
		newModeHandler(),
		newSourceHandler(),
		newCallPatternHandler("DXCALL"),
		newCallPatternHandler("DECALL"),
		newConfidenceHandler(),
		newContinentHandler("DXCONT", func(f *filter.Filter, value string, allowed bool) { f.SetDXContinent(value, allowed) },
			func(f *filter.Filter) (bool, bool, map[string]bool, map[string]bool) {
				return f.AllDXContinents, f.BlockAllDXContinents, f.DXContinents, f.BlockDXContinents
			}),
		newContinentHandler("DECONT", func(f *filter.Filter, value string, allowed bool) { f.SetDEContinent(value, allowed) },
			func(f *filter.Filter) (bool, bool, map[string]bool, map[string]bool) {
				return f.AllDEContinents, f.BlockAllDEContinents, f.DEContinents, f.BlockDEContinents
			}),
		newZoneHandler("DXZONE", func(f *filter.Filter, value int, allowed bool) { f.SetDXZone(value, allowed) },
			func(f *filter.Filter) (bool, bool, map[int]bool, map[int]bool) {
				return f.AllDXZones, f.BlockAllDXZones, f.DXZones, f.BlockDXZones
			}),
		newZoneHandler("DEZONE", func(f *filter.Filter, value int, allowed bool) { f.SetDEZone(value, allowed) },
			func(f *filter.Filter) (bool, bool, map[int]bool, map[int]bool) {
				return f.AllDEZones, f.BlockAllDEZones, f.DEZones, f.BlockDEZones
			}),
		newDXCCHandler("DXDXCC", func(f *filter.Filter, code int, allowed bool) { f.SetDXDXCC(code, allowed) },
			func(f *filter.Filter) (bool, bool, map[int]bool, map[int]bool) {
				return f.AllDXDXCC, f.BlockAllDXDXCC, f.DXDXCC, f.BlockDXDXCC
			}),
		newDXCCHandler("DEDXCC", func(f *filter.Filter, code int, allowed bool) { f.SetDEDXCC(code, allowed) },
			func(f *filter.Filter) (bool, bool, map[int]bool, map[int]bool) {
				return f.AllDEDXCC, f.BlockAllDEDXCC, f.DEDXCC, f.BlockDEDXCC
			}),
		newGrid2Handler("DXGRID2", func(f *filter.Filter, value string, allowed bool) { f.SetDXGrid2Prefix(value, allowed) },
			func(f *filter.Filter) (bool, bool, map[string]bool, map[string]bool) {
				return f.AllDXGrid2, f.BlockAllDXGrid2, f.DXGrid2Prefixes, f.BlockDXGrid2
			}),
		newGrid2Handler("DEGRID2", func(f *filter.Filter, value string, allowed bool) { f.SetDEGrid2Prefix(value, allowed) },
			func(f *filter.Filter) (bool, bool, map[string]bool, map[string]bool) {
				return f.AllDEGrid2, f.BlockAllDEGrid2, f.DEGrid2Prefixes, f.BlockDEGrid2
			}),
		newFeatureToggleHandler("BEACON", func(f *filter.Filter, enabled bool) { f.SetBeaconEnabled(enabled) }),
		newFeatureToggleHandler("WWV", func(f *filter.Filter, enabled bool) { f.SetWWVEnabled(enabled) }),
		newFeatureToggleHandler("WCY", func(f *filter.Filter, enabled bool) { f.SetWCYEnabled(enabled) }),
		newFeatureToggleHandler("ANNOUNCE", func(f *filter.Filter, enabled bool) { f.SetAnnounceEnabled(enabled) }, "PC93"),
		newAllResetHandler(),
	}
	for _, h := range handlers {
		e.registerDomain(h)
	}
}

func (e *filterCommandEngine) registerDomain(handler *domainHandler) {
	e.domains[strings.ToUpper(handler.name)] = handler
	for _, alias := range handler.aliases {
		e.domains[strings.ToUpper(alias)] = handler
	}
}

func (e *filterCommandEngine) availableDialectNames() []string {
	if e == nil {
		return nil
	}
	names := make([]string, 0, len(e.dialects))
	for name := range e.dialects {
		names = append(names, string(name))
	}
	sort.Strings(names)
	return names
}

func normalizeDialectName(name string) DialectName {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "cc":
		return DialectCC
	case "classic", "default", "gocluster", "":
		return DialectGoCluster
	default:
		return DialectGoCluster
	}
}

// Handle parses and executes a filter command for the active dialect.
// Returns the user-facing response and whether the command was handled.
func (e *filterCommandEngine) Handle(client *Client, line string) (string, bool) {
	if client == nil || e == nil {
		return "", false
	}
	tokens := strings.Fields(strings.TrimSpace(line))
	if len(tokens) == 0 {
		return "", false
	}
	upper := make([]string, len(tokens))
	for i, t := range tokens {
		upper[i] = strings.ToUpper(t)
	}

	active := client.dialect
	parsed, ok, errText := e.parse(tokens, upper, active)
	if ok {
		if errText != "" {
			return errText, true
		}
		resp, mutated := e.execute(client, parsed)
		if mutated {
			_ = client.saveFilter()
		}
		return resp, true
	}

	if hint := e.hintForOtherDialect(tokens, upper, active); hint != "" {
		return hint, true
	}

	return "", false
}

func (e *filterCommandEngine) parse(tokens, upper []string, dialect DialectName) (parsedFilterCommand, bool, string) {
	spec, ok := e.dialects[dialect]
	if !ok || spec == nil {
		return parsedFilterCommand{}, false, ""
	}
	parsed, matched, errText := spec.parse(tokens, upper)
	if !matched && dialect != DialectGoCluster {
		// Try classic as a safe fallback to preserve current behavior when the
		// client switches to an unknown dialect.
		if gc, ok := e.dialects[DialectGoCluster]; ok && gc != nil {
			return gc.parse(tokens, upper)
		}
	}
	return parsed, matched, errText
}

func (e *filterCommandEngine) hintForOtherDialect(tokens, upper []string, active DialectName) string {
	for name, spec := range e.dialects {
		if name == active || spec == nil {
			continue
		}
		_, matched, _ := spec.parse(tokens, upper)
		if matched {
			return fmt.Sprintf("Command matches the %s dialect. Switch with DIALECT %s or use the %s syntax.\n", spec.name, strings.ToUpper(string(spec.name)), strings.ToUpper(string(active)))
		}
	}
	return ""
}

func (e *filterCommandEngine) execute(client *Client, cmd parsedFilterCommand) (string, bool) {
	switch cmd.action {
	case actionSummary:
		client.filterMu.RLock()
		defer client.filterMu.RUnlock()
		return fmt.Sprintf("Current filters: %s\n", client.filter.String()), false
	}

	handler := e.domains[strings.ToUpper(strings.TrimSpace(cmd.domain))]
	if handler == nil {
		switch cmd.action {
		case actionAllow:
			if strings.EqualFold(cmd.domain, "DXBM") {
				return "Unknown DXBM band. Supported: 160, 80, 40, 30, 20, 17, 15, 12, 10, 6, 2 (and 1 if enabled). Modes ignored; use SET/NO<MODE>.\n", false
			}
			return unknownPassTypeMsg, false
		case actionBlock:
			if strings.EqualFold(cmd.domain, "DXBM") {
				return "Unknown DXBM band. Supported: 160, 80, 40, 30, 20, 17, 15, 12, 10, 6, 2 (and 1 if enabled). Modes ignored; use SET/NO<MODE>.\n", false
			}
			return unknownRejectTypeMsg, false
		default:
			return invalidFilterCommandMsg, false
		}
	}

	switch cmd.action {
	case actionShowSupported:
		if handler.showSupportedFn == nil {
			return invalidFilterCommandMsg, false
		}
		client.filterMu.RLock()
		resp := handler.showSupportedFn(client.filter)
		client.filterMu.RUnlock()
		return resp, false
	case actionShowState:
		if handler.showState == nil {
			return invalidFilterCommandMsg, false
		}
		client.filterMu.RLock()
		resp := handler.showState(client.filter)
		client.filterMu.RUnlock()
		return resp, false
	default:
		resp, mutated := handler.apply(client, cmd.action, cmd.args)
		return resp, mutated
	}
}

func parseClassicDialect(tokens, upper []string) (parsedFilterCommand, bool, string) {
	if len(upper) == 0 {
		return parsedFilterCommand{}, false, ""
	}
	switch upper[0] {
	case "PASS":
		if len(upper) < 2 {
			return parsedFilterCommand{}, true, passFilterUsageMsg
		}
		return parsedFilterCommand{action: actionAllow, domain: upper[1], args: tokens[2:]}, true, ""
	case "REJECT":
		if len(upper) < 2 {
			return parsedFilterCommand{}, true, rejectFilterUsageMsg
		}
		return parsedFilterCommand{action: actionBlock, domain: upper[1], args: tokens[2:]}, true, ""
	case "SHOW":
		if len(upper) < 2 || upper[1] != "FILTER" {
			return parsedFilterCommand{}, false, ""
		}
		if len(upper) == 2 {
			return parsedFilterCommand{action: actionSummary}, true, ""
		}
		domain := upper[2]
		action := actionShowState
		if domain == "MODES" || domain == "BANDS" || domain == "CONFIDENCE" {
			action = actionShowSupported
		}
		return parsedFilterCommand{action: action, domain: domain, args: tokens[3:]}, true, ""
	default:
		return parsedFilterCommand{}, false, ""
	}
}

func parseCCDialect(tokens, upper []string) (parsedFilterCommand, bool, string) {
	if len(upper) == 0 {
		return parsedFilterCommand{}, false, ""
	}

	if upper[0] == "SET/FILTER" && len(upper) >= 2 {
		// DXBM/PASS or DXBM/REJECT inline or separated.
		if strings.HasPrefix(strings.ToUpper(upper[1]), "DXBM/") {
			mode := strings.TrimPrefix(strings.ToUpper(upper[1]), "DXBM/")
			switch mode {
			case "PASS":
				return parsedFilterCommand{action: actionAllow, domain: "DXBM", args: tokens[2:]}, true, ""
			case "REJECT":
				return parsedFilterCommand{action: actionBlock, domain: "DXBM", args: tokens[2:]}, true, ""
			}
		} else if strings.EqualFold(upper[1], "DXBM") && len(upper) >= 3 {
			if strings.EqualFold(upper[2], "PASS") {
				return parsedFilterCommand{action: actionAllow, domain: "DXBM", args: tokens[3:]}, true, ""
			}
			if strings.EqualFold(upper[2], "REJECT") {
				return parsedFilterCommand{action: actionBlock, domain: "DXBM", args: tokens[3:]}, true, ""
			}
		}
	}

	switch upper[0] {
	case "SET/ANN":
		return parsedFilterCommand{action: actionAllow, domain: "ANNOUNCE"}, true, ""
	case "SET/NOANN":
		return parsedFilterCommand{action: actionBlock, domain: "ANNOUNCE"}, true, ""
	case "SET/BEACON":
		return parsedFilterCommand{action: actionAllow, domain: "BEACON"}, true, ""
	case "SET/NOBEACON":
		return parsedFilterCommand{action: actionBlock, domain: "BEACON"}, true, ""
	case "SET/WWV":
		return parsedFilterCommand{action: actionAllow, domain: "WWV"}, true, ""
	case "SET/NOWWV":
		return parsedFilterCommand{action: actionBlock, domain: "WWV"}, true, ""
	case "SET/WCY":
		return parsedFilterCommand{action: actionAllow, domain: "WCY"}, true, ""
	case "SET/NOWCY":
		return parsedFilterCommand{action: actionBlock, domain: "WCY"}, true, ""
	case "SET/SKIMMER":
		return parsedFilterCommand{action: actionAllow, domain: "SOURCE", args: []string{"SKIMMER"}}, true, ""
	case "SET/NOSKIMMER":
		return parsedFilterCommand{action: actionBlock, domain: "SOURCE", args: []string{"SKIMMER"}}, true, ""
	case "SET/NOFILTER":
		// CC "no filter" means permissive defaults.
		return parsedFilterCommand{action: actionBlock, domain: "ALL"}, true, ""
	case "SET/FILTER":
		if len(upper) < 2 {
			return parsedFilterCommand{}, true, passFilterUsageMsg
		}
		domain := upper[1]
		args := tokens[2:]
		// Recognize inline /OFF to disable a domain quickly.
		if strings.HasSuffix(domain, "/OFF") {
			domain = strings.TrimSuffix(domain, "/OFF")
			args = []string{"ALL"}
			return parsedFilterCommand{action: actionBlock, domain: domain, args: args}, true, ""
		}
		return parsedFilterCommand{action: actionAllow, domain: domain, args: args}, true, ""
	case "UNSET/FILTER":
		if len(upper) < 2 {
			return parsedFilterCommand{}, true, rejectFilterUsageMsg
		}
		return parsedFilterCommand{action: actionBlock, domain: upper[1], args: tokens[2:]}, true, ""
	case "SHOW/FILTER", "SH/FILTER":
		if len(upper) == 1 {
			return parsedFilterCommand{action: actionSummary}, true, ""
		}
		domain := upper[1]
		action := actionShowState
		if domain == "MODES" || domain == "BANDS" || domain == "CONFIDENCE" {
			action = actionShowSupported
		}
		return parsedFilterCommand{action: action, domain: domain, args: tokens[2:]}, true, ""
	default:
		// Mode shortcuts: SET/FT8, SET/NOFT8, etc. (CC supports CW, FT4, FT8, RTTY)
		if len(upper) == 1 && strings.HasPrefix(upper[0], "SET/NO") {
			mode := strings.TrimPrefix(upper[0], "SET/NO")
			if isCCMode(mode) {
				return parsedFilterCommand{action: actionBlock, domain: "MODE", args: []string{mode}}, true, ""
			}
		}
		if len(upper) == 1 && strings.HasPrefix(upper[0], "SET/") && !strings.Contains(upper[0], "/NO") {
			mode := strings.TrimPrefix(upper[0], "SET/")
			if isCCMode(mode) {
				return parsedFilterCommand{action: actionAllow, domain: "MODE", args: []string{mode}}, true, ""
			}
		}
		return parsedFilterCommand{}, false, ""
	}
}

func newBandHandler() *domainHandler {
	return &domainHandler{
		name:          "BAND",
		showSupported: true,
		apply: func(c *Client, action filterAction, args []string) (string, bool) {
			switch action {
			case actionAllow:
				value := strings.TrimSpace(strings.Join(args, " "))
				if value == "" {
					return passFilterUsageMsg, false
				}
				if strings.EqualFold(value, "ALL") {
					c.updateFilter(func(f *filter.Filter) { f.ResetBands() })
					return "All bands enabled\n", true
				}
				rawBands := parseBandList(value)
				if len(rawBands) == 0 {
					return passFilterUsageMsg, false
				}
				normalizedBands, invalid := normalizeBands(rawBands)
				if len(invalid) > 0 {
					return fmt.Sprintf("Unknown band: %s\nSupported bands: %s\n", strings.Join(invalid, ", "), strings.Join(spot.SupportedBandNames(), ", ")), false
				}
				if len(normalizedBands) == 0 {
					return passFilterUsageMsg, false
				}
				c.updateFilter(func(f *filter.Filter) {
					for _, band := range normalizedBands {
						f.SetBand(band, true)
					}
				})
				if len(normalizedBands) == 1 {
					return fmt.Sprintf("Filter set: Band %s\n", normalizedBands[0]), true
				}
				return fmt.Sprintf("Filter set: Bands %s\n", strings.Join(normalizedBands, ", ")), true
			case actionBlock:
				value := strings.TrimSpace(strings.Join(args, " "))
				if value == "" {
					return rejectFilterUsageMsg, false
				}
				if strings.EqualFold(value, "ALL") {
					c.updateFilter(func(f *filter.Filter) {
						f.ResetBands()
						f.BlockAllBands = true
						f.AllBands = false
					})
					return "All bands blocked\n", true
				}
				rawBands := parseBandList(value)
				if len(rawBands) == 0 {
					return rejectFilterUsageMsg, false
				}
				normalizedBands, invalid := normalizeBands(rawBands)
				if len(invalid) > 0 {
					return fmt.Sprintf("Unknown band: %s\nSupported bands: %s\n", strings.Join(invalid, ", "), strings.Join(spot.SupportedBandNames(), ", ")), false
				}
				if len(normalizedBands) == 0 {
					return rejectFilterUsageMsg, false
				}
				c.updateFilter(func(f *filter.Filter) {
					for _, band := range normalizedBands {
						f.SetBand(band, false)
					}
				})
				return fmt.Sprintf("Band filters disabled: %s\n", strings.Join(normalizedBands, ", ")), true
			default:
				return invalidFilterCommandMsg, false
			}
		},
		showState: func(f *filter.Filter) string {
			return fmt.Sprintf("Supported bands: %s\n", strings.Join(spot.SupportedBandNames(), ", "))
		},
		showSupportedFn: func(f *filter.Filter) string {
			return fmt.Sprintf("Supported bands: %s\n", strings.Join(spot.SupportedBandNames(), ", "))
		},
	}
}

func newDXBMHandler() *domainHandler {
	return &domainHandler{
		name: "DXBM",
		apply: func(c *Client, action filterAction, args []string) (string, bool) {
			return applyDXBMBands(c, action, args)
		},
	}
}

func newModeHandler() *domainHandler {
	return &domainHandler{
		name:          "MODE",
		showSupported: true,
		apply: func(c *Client, action filterAction, args []string) (string, bool) {
			switch action {
			case actionAllow:
				modeArgs := strings.TrimSpace(strings.Join(args, " "))
				if strings.EqualFold(modeArgs, "ALL") {
					c.updateFilter(func(f *filter.Filter) { f.ResetModes() })
					return "All modes enabled\n", true
				}
				modes := parseModeList(modeArgs)
				if len(modes) == 0 {
					return "Usage: PASS MODE <mode>[,<mode>...] (comma or space separated)\nType HELP for usage.\n", false
				}
				invalid := collectInvalidModes(modes)
				if len(invalid) > 0 {
					return fmt.Sprintf("Unknown mode: %s\nSupported modes: %s\n", strings.Join(invalid, ", "), strings.Join(filter.SupportedModes, ", ")), false
				}
				c.updateFilter(func(f *filter.Filter) {
					for _, mode := range modes {
						f.SetMode(mode, true)
					}
				})
				return fmt.Sprintf("Filter set: Modes %s\n", strings.Join(modes, ", ")), true
			case actionBlock:
				modeArgs := strings.TrimSpace(strings.Join(args, " "))
				if modeArgs == "" {
					return "Usage: REJECT MODE <mode>[,<mode>...] (comma or space separated, or ALL)\nType HELP for usage.\n", false
				}
				if strings.EqualFold(modeArgs, "ALL") {
					c.updateFilter(func(f *filter.Filter) {
						f.ResetModes()
						f.BlockAllModes = true
						f.AllModes = false
					})
					return "All modes blocked\n", true
				}
				modes := parseModeList(modeArgs)
				if len(modes) == 0 {
					return "Usage: REJECT MODE <mode>[,<mode>...] (comma or space separated, or ALL)\nType HELP for usage.\n", false
				}
				invalid := collectInvalidModes(modes)
				if len(invalid) > 0 {
					return fmt.Sprintf("Unknown mode: %s\nSupported modes: %s\n", strings.Join(invalid, ", "), strings.Join(filter.SupportedModes, ", ")), false
				}
				c.updateFilter(func(f *filter.Filter) {
					for _, mode := range modes {
						f.SetMode(mode, false)
					}
				})
				return fmt.Sprintf("Mode filters disabled: %s\n", strings.Join(modes, ", ")), true
			default:
				return invalidFilterCommandMsg, false
			}
		},
		showSupportedFn: func(f *filter.Filter) string {
			var b strings.Builder
			for i, mode := range filter.SupportedModes {
				enabled := "DISABLED"
				if f.AllModes || f.Modes[mode] {
					enabled = "ENABLED"
				}
				b.WriteString(fmt.Sprintf("%s=%s", mode, enabled))
				if i < len(filter.SupportedModes)-1 {
					b.WriteString(", ")
				}
			}
			return fmt.Sprintf("Supported modes: %s\n", b.String())
		},
	}
}

func newSourceHandler() *domainHandler {
	return &domainHandler{
		name: "SOURCE",
		apply: func(c *Client, action filterAction, args []string) (string, bool) {
			value := strings.ToUpper(strings.TrimSpace(strings.Join(args, " ")))
			switch action {
			case actionAllow:
				if value == "" {
					return "Usage: PASS SOURCE <HUMAN|SKIMMER|ALL>\nType HELP for usage.\n", false
				}
				if strings.EqualFold(value, "ALL") {
					c.updateFilter(func(f *filter.Filter) { f.ResetSources() })
					return "Source filtering disabled\n", true
				}
				if !filter.IsSupportedSource(value) {
					return fmt.Sprintf("Unknown source: %s\nValid sources: %s\n", value, strings.Join(filter.SupportedSources, ", ")), false
				}
				c.updateFilter(func(f *filter.Filter) { f.SetSource(value, true) })
				return fmt.Sprintf("Filter set: Source %s\n", value), true
			case actionBlock:
				if value == "" {
					return "Usage: REJECT SOURCE <HUMAN|SKIMMER>\nType HELP for usage.\n", false
				}
				if !filter.IsSupportedSource(value) {
					return fmt.Sprintf("Unknown source: %s\nValid sources: %s\n", value, strings.Join(filter.SupportedSources, ", ")), false
				}
				c.updateFilter(func(f *filter.Filter) { f.SetSource(value, false) })
				return fmt.Sprintf("Source filters disabled: %s\n", value), true
			default:
				return invalidFilterCommandMsg, false
			}
		},
		showState: func(f *filter.Filter) string {
			state := "ALL"
			if len(f.Sources) > 0 {
				state = strings.Join(keysString(f.Sources), ", ")
			}
			if f.BlockAllSources {
				return "Source: BLOCK ALL\n"
			}
			return fmt.Sprintf("Source: %s\n", state)
		},
	}
}

func newCallPatternHandler(name string) *domainHandler {
	return &domainHandler{
		name: name,
		apply: func(c *Client, action filterAction, args []string) (string, bool) {
			switch action {
			case actionAllow:
				if len(args) == 0 {
					return passFilterUsageMsg, false
				}
				value := strings.ToUpper(args[0])
				c.updateFilter(func(f *filter.Filter) {
					if name == "DXCALL" {
						f.AddDXCallsignPattern(value)
					} else {
						f.AddDECallsignPattern(value)
					}
				})
				if name == "DXCALL" {
					return fmt.Sprintf("Filter set: DX callsign %s\n", value), true
				}
				return fmt.Sprintf("Filter set: DE callsign %s\n", value), true
			case actionBlock:
				c.updateFilter(func(f *filter.Filter) {
					if name == "DXCALL" {
						f.ClearDXCallsignPatterns()
					} else {
						f.ClearDECallsignPatterns()
					}
				})
				if name == "DXCALL" {
					return "DX callsign filters cleared\n", true
				}
				return "DE callsign filters cleared\n", true
			default:
				return invalidFilterCommandMsg, false
			}
		},
		showState: func(f *filter.Filter) string {
			if name == "DXCALL" {
				return fmt.Sprintf("DX callsigns: %s\n", strings.Join(f.DXCallsigns, ", "))
			}
			return fmt.Sprintf("DE callsigns: %s\n", strings.Join(f.DECallsigns, ", "))
		},
	}
}

func newConfidenceHandler() *domainHandler {
	return &domainHandler{
		name:          "CONFIDENCE",
		showSupported: true,
		apply: func(c *Client, action filterAction, args []string) (string, bool) {
			value := strings.TrimSpace(strings.Join(args, " "))
			switch action {
			case actionAllow:
				if value == "" {
					return "Usage: PASS CONFIDENCE <symbol>[,<symbol>...] (symbols: ?,S,C,P,V,B or ALL)\nType HELP for usage.\n", false
				}
				if strings.EqualFold(value, "ALL") {
					c.updateFilter(func(f *filter.Filter) { f.ResetConfidence() })
					return "All confidence symbols enabled\n", true
				}
				symbols := parseConfidenceList(value)
				if len(symbols) == 0 {
					return "Usage: PASS CONFIDENCE <symbol>[,<symbol>...] (symbols: ?,S,C,P,V,B or ALL)\nType HELP for usage.\n", false
				}
				invalid := collectInvalidConfidenceSymbols(symbols)
				if len(invalid) > 0 {
					return fmt.Sprintf("Unknown confidence symbol: %s\nSupported symbols: %s\n", strings.Join(invalid, ", "), strings.Join(filter.SupportedConfidenceSymbols, ", ")), false
				}
				c.updateFilter(func(f *filter.Filter) {
					for _, symbol := range symbols {
						f.SetConfidenceSymbol(symbol, true)
					}
				})
				return fmt.Sprintf("Confidence symbols enabled: %s\n", strings.Join(symbols, ", ")), true
			case actionBlock:
				if value == "" {
					return "Usage: REJECT CONFIDENCE <symbol>[,<symbol>...] (comma or space separated, or ALL)\nType HELP for usage.\n", false
				}
				if strings.EqualFold(value, "ALL") {
					c.updateFilter(func(f *filter.Filter) {
						f.ResetConfidence()
						f.BlockAllConfidence = true
						f.AllConfidence = false
					})
					return "All confidence symbols blocked (non-exempt modes)\n", true
				}
				symbols := parseConfidenceList(value)
				if len(symbols) == 0 {
					return "Usage: REJECT CONFIDENCE <symbol>[,<symbol>...] (comma or space separated, or ALL)\nType HELP for usage.\n", false
				}
				invalid := collectInvalidConfidenceSymbols(symbols)
				if len(invalid) > 0 {
					return fmt.Sprintf("Unknown confidence symbol: %s\nSupported symbols: %s\n", strings.Join(invalid, ", "), strings.Join(filter.SupportedConfidenceSymbols, ", ")), false
				}
				c.updateFilter(func(f *filter.Filter) {
					for _, symbol := range symbols {
						f.SetConfidenceSymbol(symbol, false)
					}
				})
				return fmt.Sprintf("Confidence symbols disabled: %s\n", strings.Join(symbols, ", ")), true
			default:
				return invalidFilterCommandMsg, false
			}
		},
		showSupportedFn: func(f *filter.Filter) string {
			var b strings.Builder
			for i, symbol := range filter.SupportedConfidenceSymbols {
				enabled := "DISABLED"
				if f.AllConfidence || f.ConfidenceSymbolEnabled(symbol) {
					enabled = "ENABLED"
				}
				b.WriteString(fmt.Sprintf("%s=%s", symbol, enabled))
				if i < len(filter.SupportedConfidenceSymbols)-1 {
					b.WriteString(", ")
				}
			}
			return fmt.Sprintf("Confidence symbols: %s\n", b.String())
		},
	}
}

func newContinentHandler(name string, setter func(*filter.Filter, string, bool), snapshot func(*filter.Filter) (bool, bool, map[string]bool, map[string]bool)) *domainHandler {
	return &domainHandler{
		name: name,
		apply: func(c *Client, action filterAction, args []string) (string, bool) {
			value := strings.TrimSpace(strings.Join(args, " "))
			switch action {
			case actionAllow:
				if value == "" {
					return fmt.Sprintf("Usage: PASS %s <cont>[,<cont>...] (continents: AF, AN, AS, EU, NA, OC, SA, or ALL)\nType HELP for usage.\n", name), false
				}
				if strings.EqualFold(value, "ALL") {
					c.updateFilter(func(f *filter.Filter) {
						if name == "DXCONT" {
							f.ResetDXContinents()
						} else {
							f.ResetDEContinents()
						}
					})
					return fmt.Sprintf("All %s continents enabled\n", strings.ToLower(name[:2])), true
				}
				continents := parseContinentList(value)
				if len(continents) == 0 {
					return fmt.Sprintf("Usage: PASS %s <cont>[,<cont>...] (continents: AF, AN, AS, EU, NA, OC, SA, or ALL)\nType HELP for usage.\n", name), false
				}
				if invalid := collectInvalidContinents(continents); len(invalid) > 0 {
					return fmt.Sprintf("Unknown continent: %s\nSupported continents: %s\n", strings.Join(invalid, ", "), strings.Join(filter.SupportedContinents, ", ")), false
				}
				c.updateFilter(func(f *filter.Filter) {
					for _, cont := range continents {
						setter(f, cont, true)
					}
				})
				return fmt.Sprintf("Filter set: %s continents %s\n", name[:2], strings.Join(continents, ", ")), true
			case actionBlock:
				if value == "" {
					return fmt.Sprintf("Usage: REJECT %s <cont>[,<cont>...] (comma or space separated, or ALL)\nType HELP for usage.\n", name), false
				}
				if strings.EqualFold(value, "ALL") {
					c.updateFilter(func(f *filter.Filter) {
						if name == "DXCONT" {
							f.ResetDXContinents()
							f.BlockAllDXContinents = true
							f.AllDXContinents = false
						} else {
							f.ResetDEContinents()
							f.BlockAllDEContinents = true
							f.AllDEContinents = false
						}
					})
					return fmt.Sprintf("All %s continents blocked\n", strings.ToLower(name[:2])), true
				}
				continents := parseContinentList(value)
				if len(continents) == 0 {
					return fmt.Sprintf("Usage: REJECT %s <cont>[,<cont>...] (comma or space separated, or ALL)\nType HELP for usage.\n", name), false
				}
				if invalid := collectInvalidContinents(continents); len(invalid) > 0 {
					return fmt.Sprintf("Unknown continent: %s\nSupported continents: %s\n", strings.Join(invalid, ", "), strings.Join(filter.SupportedContinents, ", ")), false
				}
				c.updateFilter(func(f *filter.Filter) {
					for _, cont := range continents {
						setter(f, cont, false)
					}
				})
				return fmt.Sprintf("%s continent filters disabled: %s\n", name[:2], strings.Join(continents, ", ")), true
			default:
				return invalidFilterCommandMsg, false
			}
		},
		showState: func(f *filter.Filter) string {
			all, blockAll, allow, block := snapshot(f)
			return fmt.Sprintf("%s continents: %s\n", name[:2], formatAllowBlockStrings(all, blockAll, allow, block))
		},
	}
}

func newZoneHandler(name string, setter func(*filter.Filter, int, bool), snapshot func(*filter.Filter) (bool, bool, map[int]bool, map[int]bool)) *domainHandler {
	return &domainHandler{
		name: name,
		apply: func(c *Client, action filterAction, args []string) (string, bool) {
			value := strings.TrimSpace(strings.Join(args, " "))
			switch action {
			case actionAllow:
				if value == "" {
					return fmt.Sprintf("Usage: PASS %s <zone>[,<zone>...] (1-40, or ALL)\nType HELP for usage.\n", name), false
				}
				if strings.EqualFold(value, "ALL") {
					c.updateFilter(func(f *filter.Filter) {
						if name == "DXZONE" {
							f.ResetDXZones()
						} else {
							f.ResetDEZones()
						}
					})
					return fmt.Sprintf("All %s zones enabled\n", strings.ToLower(name[:2])), true
				}
				zones := parseZoneList(value)
				if len(zones) == 0 {
					return fmt.Sprintf("Usage: PASS %s <zone>[,<zone>...] (1-40, or ALL)\nType HELP for usage.\n", name), false
				}
				if invalid := collectInvalidZones(zones); len(invalid) > 0 {
					return fmt.Sprintf("Unknown CQ zone: %v\nValid zones: %d-%d\n", invalid, filter.MinCQZone(), filter.MaxCQZone()), false
				}
				c.updateFilter(func(f *filter.Filter) {
					for _, zone := range zones {
						setter(f, zone, true)
					}
				})
				return fmt.Sprintf("Filter set: %s zones %s\n", name[:2], joinZones(zones)), true
			case actionBlock:
				if value == "" {
					return fmt.Sprintf("Usage: REJECT %s <zone>[,<zone>...] (comma or space separated, or ALL)\nType HELP for usage.\n", name), false
				}
				if strings.EqualFold(value, "ALL") {
					c.updateFilter(func(f *filter.Filter) {
						if name == "DXZONE" {
							f.ResetDXZones()
							f.BlockAllDXZones = true
							f.AllDXZones = false
						} else {
							f.ResetDEZones()
							f.BlockAllDEZones = true
							f.AllDEZones = false
						}
					})
					return fmt.Sprintf("All %s zones blocked\n", strings.ToLower(name[:2])), true
				}
				zones := parseZoneList(value)
				if len(zones) == 0 {
					return fmt.Sprintf("Usage: REJECT %s <zone>[,<zone>...] (comma or space separated, or ALL)\nType HELP for usage.\n", name), false
				}
				if invalid := collectInvalidZones(zones); len(invalid) > 0 {
					return fmt.Sprintf("Unknown CQ zone: %v\nValid zones: %d-%d\n", invalid, filter.MinCQZone(), filter.MaxCQZone()), false
				}
				c.updateFilter(func(f *filter.Filter) {
					for _, zone := range zones {
						setter(f, zone, false)
					}
				})
				return fmt.Sprintf("%s zone filters disabled: %s\n", name[:2], joinZones(zones)), true
			default:
				return invalidFilterCommandMsg, false
			}
		},
		showState: func(f *filter.Filter) string {
			all, blockAll, allow, block := snapshot(f)
			return fmt.Sprintf("%s zones: %s\n", name[:2], formatAllowBlockInts(all, blockAll, allow, block))
		},
	}
}

func newDXCCHandler(name string, setter func(*filter.Filter, int, bool), snapshot func(*filter.Filter) (bool, bool, map[int]bool, map[int]bool)) *domainHandler {
	return &domainHandler{
		name: name,
		apply: func(c *Client, action filterAction, args []string) (string, bool) {
			value := strings.TrimSpace(strings.Join(args, " "))
			switch action {
			case actionAllow:
				if value == "" {
					return fmt.Sprintf("Usage: PASS %s <code>[,<code>...] (comma or space separated, or ALL)\nType HELP for usage.\n", name), false
				}
				if strings.EqualFold(value, "ALL") {
					c.updateFilter(func(f *filter.Filter) {
						if name == "DXDXCC" {
							f.ResetDXDXCC()
						} else {
							f.ResetDEDXCC()
						}
					})
					return fmt.Sprintf("All %s DXCCs enabled\n", strings.ToLower(name[:2])), true
				}
				codes, invalid := parseDXCCList(value)
				if len(codes) == 0 {
					return fmt.Sprintf("Usage: PASS %s <code>[,<code>...] (comma or space separated, or ALL)\nType HELP for usage.\n", name), false
				}
				if len(invalid) > 0 {
					return fmt.Sprintf("Invalid DXCC code: %s\n", strings.Join(invalid, ", ")), false
				}
				c.updateFilter(func(f *filter.Filter) {
					for _, code := range codes {
						setter(f, code, true)
					}
				})
				return fmt.Sprintf("Filter set: %s %s\n", name[:2], joinZones(codes)), true
			case actionBlock:
				if value == "" {
					return fmt.Sprintf("Usage: REJECT %s <code>[,<code>...] (comma or space separated, or ALL)\nType HELP for usage.\n", name), false
				}
				if strings.EqualFold(value, "ALL") {
					c.updateFilter(func(f *filter.Filter) {
						if name == "DXDXCC" {
							f.ResetDXDXCC()
							f.BlockAllDXDXCC = true
							f.AllDXDXCC = false
						} else {
							f.ResetDEDXCC()
							f.BlockAllDEDXCC = true
							f.AllDEDXCC = false
						}
					})
					return fmt.Sprintf("All %s DXCCs blocked\n", strings.ToLower(name[:2])), true
				}
				codes, invalid := parseDXCCList(value)
				if len(codes) == 0 {
					return fmt.Sprintf("Usage: REJECT %s <code>[,<code>...] (comma or space separated, or ALL)\nType HELP for usage.\n", name), false
				}
				if len(invalid) > 0 {
					return fmt.Sprintf("Invalid DXCC code: %s\n", strings.Join(invalid, ", ")), false
				}
				c.updateFilter(func(f *filter.Filter) {
					for _, code := range codes {
						setter(f, code, false)
					}
				})
				return fmt.Sprintf("%s DXCC filters disabled: %s\n", name[:2], joinZones(codes)), true
			default:
				return invalidFilterCommandMsg, false
			}
		},
		showState: func(f *filter.Filter) string {
			all, blockAll, allow, block := snapshot(f)
			return fmt.Sprintf("%s DXCC: %s\n", name[:2], formatAllowBlockInts(all, blockAll, allow, block))
		},
	}
}

func newGrid2Handler(name string, setter func(*filter.Filter, string, bool), snapshot func(*filter.Filter) (bool, bool, map[string]bool, map[string]bool)) *domainHandler {
	return &domainHandler{
		name: name,
		apply: func(c *Client, action filterAction, args []string) (string, bool) {
			value := strings.TrimSpace(strings.Join(args, " "))
			switch action {
			case actionAllow:
				if value == "" {
					return fmt.Sprintf("Usage: PASS %s <grid>[,<grid>...] (two characters, or ALL)\nType HELP for usage.\n", name), false
				}
				if strings.EqualFold(value, "ALL") {
					c.updateFilter(func(f *filter.Filter) {
						if name == "DXGRID2" {
							f.ResetDXGrid2()
						} else {
							f.ResetDEGrid2()
						}
					})
					return fmt.Sprintf("All %s 2-character grids enabled\n", strings.ToLower(name[:2])), true
				}
				gridList, invalidTokens := parseGrid2List(value)
				if len(gridList) == 0 {
					return fmt.Sprintf("Usage: PASS %s <grid>[,<grid>...] (two characters, or ALL)\nType HELP for usage.\n", name), false
				}
				if len(invalidTokens) > 0 {
					return fmt.Sprintf("Unknown 2-character grid: %s\n", strings.Join(invalidTokens, ", ")), false
				}
				c.updateFilter(func(f *filter.Filter) {
					for _, grid := range gridList {
						setter(f, grid, true)
					}
				})
				return fmt.Sprintf("Filter set: %s 2-character grids %s\n", name[:2], strings.Join(gridList, ", ")), true
			case actionBlock:
				if value == "" {
					return fmt.Sprintf("Usage: REJECT %s <grid>[,<grid>...] (comma or space separated, or ALL)\nType HELP for usage.\n", name), false
				}
				if strings.EqualFold(value, "ALL") {
					c.updateFilter(func(f *filter.Filter) {
						if name == "DXGRID2" {
							f.ResetDXGrid2()
							f.BlockAllDXGrid2 = true
							f.AllDXGrid2 = false
						} else {
							f.ResetDEGrid2()
							f.BlockAllDEGrid2 = true
							f.AllDEGrid2 = false
						}
					})
					return fmt.Sprintf("All %s 2-character grids blocked\n", strings.ToLower(name[:2])), true
				}
				gridList, invalidTokens := parseGrid2List(value)
				if len(gridList) == 0 {
					return fmt.Sprintf("Usage: REJECT %s <grid>[,<grid>...] (comma or space separated, or ALL)\nType HELP for usage.\n", name), false
				}
				if len(invalidTokens) > 0 {
					return fmt.Sprintf("Unknown 2-character grid: %s\n", strings.Join(invalidTokens, ", ")), false
				}
				c.updateFilter(func(f *filter.Filter) {
					for _, grid := range gridList {
						setter(f, grid, false)
					}
				})
				return fmt.Sprintf("%s 2-character grid filters disabled: %s\n", name[:2], strings.Join(gridList, ", ")), true
			default:
				return invalidFilterCommandMsg, false
			}
		},
		showState: func(f *filter.Filter) string {
			all, blockAll, allow, block := snapshot(f)
			return fmt.Sprintf("%s 2-character grids: %s\n", name[:2], formatAllowBlockStrings(all, blockAll, allow, block))
		},
	}
}

func newFeatureToggleHandler(name string, setter func(*filter.Filter, bool), aliases ...string) *domainHandler {
	enableLabel, disableLabel, statusLabel := featureLabels(name)
	return &domainHandler{
		name:    name,
		aliases: aliases,
		apply: func(c *Client, action filterAction, args []string) (string, bool) {
			switch action {
			case actionAllow:
				c.updateFilter(func(f *filter.Filter) { setter(f, true) })
				return fmt.Sprintf("%s enabled\n", enableLabel), true
			case actionBlock:
				c.updateFilter(func(f *filter.Filter) { setter(f, false) })
				return fmt.Sprintf("%s disabled\n", disableLabel), true
			default:
				return invalidFilterCommandMsg, false
			}
		},
		showState: func(f *filter.Filter) string {
			enabled := false
			switch name {
			case "BEACON":
				enabled = f.BeaconsEnabled()
			case "WWV":
				enabled = f.WWVEnabled()
			case "WCY":
				enabled = f.WCYEnabled()
			case "ANNOUNCE":
				enabled = f.AnnounceEnabled()
			}
			status := "ENABLED"
			if !enabled {
				status = "DISABLED"
			}
			return fmt.Sprintf("%s: %s\n", statusLabel, status)
		},
	}
}

func newAllResetHandler() *domainHandler {
	return &domainHandler{
		name:    "ALL",
		aliases: []string{},
		apply: func(c *Client, action filterAction, args []string) (string, bool) {
			if action != actionBlock {
				return invalidFilterCommandMsg, false
			}
			c.updateFilter(func(f *filter.Filter) { f.Reset() })
			return "All filters cleared\n", true
		},
	}
}

func applyDXBMBands(c *Client, action filterAction, args []string) (string, bool) {
	if c == nil {
		return invalidFilterCommandMsg, false
	}
	values := splitListValues(strings.Join(args, " "))
	if len(values) == 0 {
		return "Usage: SET/FILTER DXBM/PASS|REJECT <band>[,<band>...]\nType HELP for usage.\n", false
	}
	bands, invalid := normalizeDXBMBands(values)
	if len(invalid) > 0 {
		return fmt.Sprintf("Unknown DXBM band: %s\nSupported: 160, 80, 40, 30, 20, 17, 15, 12, 10, 6, 2 (and 1 if enabled). Modes ignored; use SET/NO<MODE>.\n", strings.Join(invalid, ", ")), false
	}
	if len(bands) == 0 {
		return "No valid DXBM bands provided.\n", false
	}

	switch action {
	case actionAllow:
		// Pass wins: apply allow set first and ignore block set overlaps.
		c.updateFilter(func(f *filter.Filter) {
			for _, band := range bands {
				f.SetBand(band, true)
			}
		})
		return fmt.Sprintf("DXBM mapped: allow bands %s\n", strings.Join(bands, ", ")), true
	case actionBlock:
		// Remove any overlap with allow; pass wins.
		c.updateFilter(func(f *filter.Filter) {
			for _, band := range bands {
				f.SetBand(band, false)
			}
		})
		return fmt.Sprintf("DXBM mapped: block bands %s\n", strings.Join(bands, ", ")), true
	default:
		return invalidFilterCommandMsg, false
	}
}

func normalizeDXBMBands(values []string) (bands []string, invalid []string) {
	seen := make(map[string]bool)
	for _, v := range values {
		raw := strings.ToUpper(strings.TrimSpace(v))
		if raw == "" {
			continue
		}
		switch {
		case strings.HasPrefix(raw, "160"):
			addBand("160M", &bands, seen)
		case strings.HasPrefix(raw, "80"):
			addBand("80M", &bands, seen)
		case strings.HasPrefix(raw, "40"):
			addBand("40M", &bands, seen)
		case strings.HasPrefix(raw, "30"):
			addBand("30M", &bands, seen)
		case strings.HasPrefix(raw, "20"):
			addBand("20M", &bands, seen)
		case strings.HasPrefix(raw, "17"):
			addBand("17M", &bands, seen)
		case strings.HasPrefix(raw, "15"):
			addBand("15M", &bands, seen)
		case strings.HasPrefix(raw, "12"):
			addBand("12M", &bands, seen)
		case strings.HasPrefix(raw, "10"):
			addBand("10M", &bands, seen)
		case strings.HasPrefix(raw, "6"):
			addBand("6M", &bands, seen)
		case strings.HasPrefix(raw, "2"):
			addBand("2M", &bands, seen)
		case strings.HasPrefix(raw, "1"):
			addBand("1.25M", &bands, seen)
		default:
			invalid = append(invalid, v)
		}
	}
	return bands, invalid
}

func addBand(name string, out *[]string, seen map[string]bool) {
	if seen[name] {
		return
	}
	*out = append(*out, name)
	seen[name] = true
}

func normalizeBands(values []string) ([]string, []string) {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]bool)
	invalid := make([]string, 0)
	for _, candidate := range values {
		norm := spot.NormalizeBand(candidate)
		if norm == "" || !spot.IsValidBand(norm) {
			invalid = append(invalid, candidate)
			continue
		}
		if !seen[norm] {
			normalized = append(normalized, norm)
			seen[norm] = true
		}
	}
	return normalized, invalid
}

func formatAllowBlockStrings(allowAll, blockAll bool, allow, block map[string]bool) string {
	if blockAll {
		return "BLOCK ALL"
	}
	allowStr := "ALL"
	if len(allow) > 0 {
		allowStr = strings.Join(keysString(allow), ", ")
	} else if !allowAll {
		allowStr = "NONE"
	}
	blockStr := "NONE"
	if len(block) > 0 {
		blockStr = strings.Join(keysString(block), ", ")
	}
	return fmt.Sprintf("allow=%s block=%s", allowStr, blockStr)
}

func formatAllowBlockInts(allowAll, blockAll bool, allow, block map[int]bool) string {
	if blockAll {
		return "BLOCK ALL"
	}
	allowStr := "ALL"
	if len(allow) > 0 {
		allowStr = strings.Join(keysInt(allow), ", ")
	} else if !allowAll {
		allowStr = "NONE"
	}
	blockStr := "NONE"
	if len(block) > 0 {
		blockStr = strings.Join(keysInt(block), ", ")
	}
	return fmt.Sprintf("allow=%s block=%s", allowStr, blockStr)
}

func splitListValues(arg string) []string {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return nil
	}
	cleaned := strings.ReplaceAll(arg, ",", " ")
	values := strings.Fields(cleaned)
	if len(values) == 0 {
		return nil
	}
	return values
}

func parseBandList(arg string) []string {
	return splitListValues(arg)
}

func parseModeList(arg string) []string {
	values := splitListValues(arg)
	if len(values) == 0 {
		return nil
	}
	modes := make([]string, 0, len(values))
	for _, value := range values {
		mode := strings.ToUpper(strings.TrimSpace(value))
		if mode == "" {
			continue
		}
		modes = append(modes, mode)
	}
	return modes
}

func parseConfidenceList(arg string) []string {
	values := splitListValues(arg)
	if len(values) == 0 {
		return nil
	}
	symbols := make([]string, 0, len(values))
	for _, value := range values {
		symbol := strings.ToUpper(strings.TrimSpace(value))
		if symbol == "" {
			continue
		}
		symbols = append(symbols, symbol)
	}
	return symbols
}

func parseContinentList(arg string) []string {
	values := splitListValues(arg)
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	continents := make([]string, 0, len(values))
	for _, value := range values {
		cont := strings.ToUpper(strings.TrimSpace(value))
		if cont == "" || seen[cont] {
			continue
		}
		continents = append(continents, cont)
		seen[cont] = true
	}
	return continents
}

func parseZoneList(arg string) []int {
	values := splitListValues(arg)
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int]bool)
	zones := make([]int, 0, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		zone, err := strconv.Atoi(v)
		if err != nil {
			return append(zones, -1)
		}
		if seen[zone] {
			continue
		}
		zones = append(zones, zone)
		seen[zone] = true
	}
	return zones
}

func parseDXCCList(arg string) ([]int, []string) {
	values := splitListValues(arg)
	if len(values) == 0 {
		return nil, nil
	}
	seen := make(map[int]bool)
	codes := make([]int, 0, len(values))
	invalid := make([]string, 0)
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		code, err := strconv.Atoi(v)
		if err != nil || code <= 0 {
			invalid = append(invalid, value)
			continue
		}
		if seen[code] {
			continue
		}
		codes = append(codes, code)
		seen[code] = true
	}
	return codes, invalid
}

func parseGrid2List(arg string) ([]string, []string) {
	values := splitListValues(arg)
	if len(values) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool)
	grids := make([]string, 0, len(values))
	invalid := make([]string, 0)
	for _, value := range values {
		raw := strings.ToUpper(strings.TrimSpace(value))
		if raw == "" {
			continue
		}
		if len(raw) > 2 {
			raw = raw[:2]
		}
		if len(raw) != 2 {
			invalid = append(invalid, value)
			continue
		}
		grid := raw
		if seen[grid] {
			continue
		}
		grids = append(grids, grid)
		seen[grid] = true
	}
	return grids, invalid
}

func collectInvalidConfidenceSymbols(symbols []string) []string {
	invalid := make([]string, 0)
	for _, symbol := range symbols {
		if !filter.IsSupportedConfidenceSymbol(symbol) {
			invalid = append(invalid, symbol)
		}
	}
	return invalid
}

func collectInvalidContinents(continents []string) []string {
	invalid := make([]string, 0)
	for _, cont := range continents {
		if !filter.IsSupportedContinent(cont) {
			invalid = append(invalid, cont)
		}
	}
	return invalid
}

func collectInvalidZones(zones []int) []int {
	invalid := make([]int, 0)
	for _, zone := range zones {
		if !filter.IsSupportedZone(zone) {
			invalid = append(invalid, zone)
		}
	}
	return invalid
}

func collectInvalidModes(modes []string) []string {
	invalid := make([]string, 0)
	for _, mode := range modes {
		if !filter.IsSupportedMode(mode) {
			invalid = append(invalid, mode)
		}
	}
	return invalid
}

func keysString(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func keysInt(m map[int]bool) []string {
	vals := make([]int, 0, len(m))
	for k := range m {
		vals = append(vals, k)
	}
	sort.Ints(vals)
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, fmt.Sprintf("%d", v))
	}
	return out
}

func joinZones(zones []int) string {
	if len(zones) == 0 {
		return ""
	}
	cp := append([]int(nil), zones...)
	sort.Ints(cp)
	parts := make([]string, 0, len(cp))
	for _, z := range cp {
		parts = append(parts, fmt.Sprintf("%d", z))
	}
	return strings.Join(parts, ", ")
}

func titleCase(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return ""
	}
	return strings.ToUpper(lower[:1]) + lower[1:]
}

func featureLabels(name string) (enable string, disable string, status string) {
	switch strings.ToUpper(name) {
	case "BEACON":
		return "Beacon spots", "Beacon spots", "Beacon spots"
	case "WWV":
		return "WWV bulletins", "WWV bulletins", "WWV bulletins"
	case "WCY":
		return "WCY bulletins", "WCY bulletins", "WCY bulletins"
	case "ANNOUNCE":
		return "Announcements", "Announcements", "Announcements"
	default:
		tc := titleCase(name)
		return tc, tc, tc
	}
}

func isCCMode(mode string) bool {
	mode = strings.ToUpper(strings.TrimSpace(mode))
	switch mode {
	case "CW", "FT4", "FT8", "RTTY":
		return true
	default:
		return false
	}
}
