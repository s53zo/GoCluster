package telnet

import (
	"strings"
	"testing"

	"dxcluster/filter"
	"dxcluster/spot"
)

func newTestClient() *Client {
	return &Client{
		filter:  filter.NewFilter(),
		dialect: DialectGo,
	}
}

func TestPassCommands(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		setup func(*Client)
		check func(*testing.T, *filter.Filter)
	}{
		{
			name: "pass band list",
			cmd:  "PASS BAND 20M,40M",
			check: func(t *testing.T, f *filter.Filter) {
				b20 := spot.NormalizeBand("20M")
				b40 := spot.NormalizeBand("40M")
				if !f.Bands[b20] || !f.Bands[b40] {
					t.Fatalf("expected bands 20M and 40M to be enabled")
				}
				if f.AllBands {
					t.Fatalf("AllBands should be false when specific bands are set")
				}
			},
		},
		{
			name: "pass band all",
			cmd:  "PASS BAND ALL",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.AllBands || f.BlockAllBands {
					t.Fatalf("expected AllBands=true and BlockAllBands=false after PASS BAND ALL")
				}
			},
		},
		{
			name: "pass mode list",
			cmd:  "PASS MODE FT8",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.Modes["FT8"] {
					t.Fatalf("expected FT8 mode to be enabled")
				}
				if f.AllModes {
					t.Fatalf("AllModes should be false when a specific mode is set")
				}
			},
		},
		{
			name: "pass source human",
			cmd:  "PASS SOURCE HUMAN",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.Sources["HUMAN"] {
					t.Fatalf("expected HUMAN source to be enabled")
				}
				if f.AllSources {
					t.Fatalf("AllSources should be false when a specific source is set")
				}
			},
		},
		{
			name: "pass source all resets",
			cmd:  "PASS SOURCE ALL",
			setup: func(c *Client) {
				c.filter.SetSource("HUMAN", true)
				c.filter.SetSource("SKIMMER", false)
			},
			check: func(t *testing.T, f *filter.Filter) {
				if !f.AllSources || f.BlockAllSources {
					t.Fatalf("expected AllSources=true and BlockAllSources=false after PASS SOURCE ALL")
				}
				if len(f.Sources) != 0 || len(f.BlockSources) != 0 {
					t.Fatalf("expected PASS SOURCE ALL to clear allow and block sets")
				}
			},
		},
		{
			name: "pass dxcall pattern",
			cmd:  "PASS DXCALL K1*",
			check: func(t *testing.T, f *filter.Filter) {
				if len(f.DXCallsigns) != 1 || f.DXCallsigns[0] != "K1*" {
					t.Fatalf("expected DX callsign pattern K1* to be stored")
				}
			},
		},
		{
			name: "pass decall pattern",
			cmd:  "PASS DECALL W1*",
			check: func(t *testing.T, f *filter.Filter) {
				if len(f.DECallsigns) != 1 || f.DECallsigns[0] != "W1*" {
					t.Fatalf("expected DE callsign pattern W1* to be stored")
				}
			},
		},
		{
			name: "pass confidence list",
			cmd:  "PASS CONFIDENCE P,V",
			check: func(t *testing.T, f *filter.Filter) {
				if f.AllConfidence {
					t.Fatalf("AllConfidence should be false when specific symbols are set")
				}
				if !f.Confidence["P"] || !f.Confidence["V"] {
					t.Fatalf("expected confidence symbols P and V to be enabled")
				}
			},
		},
		{
			name: "pass dx continent",
			cmd:  "PASS DXCONT EU",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.DXContinents["EU"] {
					t.Fatalf("expected DX continent EU to be enabled")
				}
			},
		},
		{
			name: "pass de zone",
			cmd:  "PASS DEZONE 5",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.DEZones[5] {
					t.Fatalf("expected DE zone 5 to be enabled")
				}
			},
		},
		{
			name: "pass dx dxcc",
			cmd:  "PASS DXDXCC 291",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.DXDXCC[291] {
					t.Fatalf("expected DX DXCC 291 to be enabled")
				}
			},
		},
		{
			name: "pass dx grid2 truncates",
			cmd:  "PASS DXGRID2 FN05",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.DXGrid2Prefixes["FN"] {
					t.Fatalf("expected DX grid2 prefix FN to be enabled")
				}
			},
		},
		{
			name: "pass beacon enables",
			cmd:  "PASS BEACON",
			setup: func(c *Client) {
				c.filter.SetBeaconEnabled(false)
			},
			check: func(t *testing.T, f *filter.Filter) {
				if !f.BeaconsEnabled() {
					t.Fatalf("expected beacon delivery to be enabled")
				}
			},
		},
		{
			name: "pass wwv enables",
			cmd:  "PASS WWV",
			setup: func(c *Client) {
				c.filter.SetWWVEnabled(false)
			},
			check: func(t *testing.T, f *filter.Filter) {
				if !f.WWVEnabled() {
					t.Fatalf("expected WWV bulletins to be enabled")
				}
			},
		},
		{
			name: "pass wcy enables",
			cmd:  "PASS WCY",
			setup: func(c *Client) {
				c.filter.SetWCYEnabled(false)
			},
			check: func(t *testing.T, f *filter.Filter) {
				if !f.WCYEnabled() {
					t.Fatalf("expected WCY bulletins to be enabled")
				}
			},
		},
		{
			name: "pass announce enables",
			cmd:  "PASS ANNOUNCE",
			setup: func(c *Client) {
				c.filter.SetAnnounceEnabled(false)
			},
			check: func(t *testing.T, f *filter.Filter) {
				if !f.AnnounceEnabled() {
					t.Fatalf("expected announcements to be enabled")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient()
			engine := newFilterCommandEngine()
			if tt.setup != nil {
				tt.setup(client)
			}
			resp, handled := engine.Handle(client, tt.cmd)
			if !handled {
				t.Fatalf("command %q was not handled", tt.cmd)
			}
			if resp == "" {
				t.Fatalf("expected response for command %q", tt.cmd)
			}
			tt.check(t, client.filter)
		})
	}
}

func TestRejectCommands(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		setup func(*Client)
		check func(*testing.T, *filter.Filter)
	}{
		{
			name: "reject band all blocks all bands",
			cmd:  "REJECT BAND ALL",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.BlockAllBands || f.AllBands {
					t.Fatalf("expected BlockAllBands=true and AllBands=false after REJECT BAND ALL")
				}
			},
		},
		{
			name: "reject mode list",
			cmd:  "REJECT MODE FT8",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.BlockModes["FT8"] {
					t.Fatalf("expected FT8 mode to be blocked")
				}
			},
		},
		{
			name: "reject source human",
			cmd:  "REJECT SOURCE HUMAN",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.BlockSources["HUMAN"] {
					t.Fatalf("expected HUMAN source to be blocked")
				}
			},
		},
		{
			name: "reject source skimmer",
			cmd:  "REJECT SOURCE SKIMMER",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.BlockSources["SKIMMER"] {
					t.Fatalf("expected SKIMMER source to be blocked")
				}
			},
		},
		{
			name: "reject confidence all",
			cmd:  "REJECT CONFIDENCE ALL",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.BlockAllConfidence || f.AllConfidence {
					t.Fatalf("expected BlockAllConfidence=true and AllConfidence=false after REJECT CONFIDENCE ALL")
				}
			},
		},
		{
			name: "reject degrid2 list",
			cmd:  "REJECT DEGRID2 FN",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.BlockDEGrid2["FN"] {
					t.Fatalf("expected DE grid2 prefix FN to be blocked")
				}
			},
		},
		{
			name: "reject beacon disables",
			cmd:  "REJECT BEACON",
			check: func(t *testing.T, f *filter.Filter) {
				if f.BeaconsEnabled() {
					t.Fatalf("expected beacon delivery to be disabled")
				}
			},
		},
		{
			name: "reject wwv disables",
			cmd:  "REJECT WWV",
			check: func(t *testing.T, f *filter.Filter) {
				if f.WWVEnabled() {
					t.Fatalf("expected WWV bulletins to be disabled")
				}
			},
		},
		{
			name: "reject wcy disables",
			cmd:  "REJECT WCY",
			check: func(t *testing.T, f *filter.Filter) {
				if f.WCYEnabled() {
					t.Fatalf("expected WCY bulletins to be disabled")
				}
			},
		},
		{
			name: "reject announce disables",
			cmd:  "REJECT ANNOUNCE",
			check: func(t *testing.T, f *filter.Filter) {
				if f.AnnounceEnabled() {
					t.Fatalf("expected announcements to be disabled")
				}
			},
		},
		{
			name: "reject dxcall clears patterns",
			cmd:  "REJECT DXCALL",
			setup: func(c *Client) {
				c.filter.AddDXCallsignPattern("K1*")
			},
			check: func(t *testing.T, f *filter.Filter) {
				if len(f.DXCallsigns) != 0 {
					t.Fatalf("expected DX callsign patterns cleared")
				}
			},
		},
		{
			name: "reject decall clears patterns",
			cmd:  "REJECT DECALL",
			setup: func(c *Client) {
				c.filter.AddDECallsignPattern("W1*")
			},
			check: func(t *testing.T, f *filter.Filter) {
				if len(f.DECallsigns) != 0 {
					t.Fatalf("expected DE callsign patterns cleared")
				}
			},
		},
		{
			name: "reject all resets filters",
			cmd:  "REJECT ALL",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.AllBands || !f.AllModes || !f.AllConfidence || !f.AllSources {
					t.Fatalf("expected REJECT ALL to reset filter to defaults")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient()
			engine := newFilterCommandEngine()
			if tt.setup != nil {
				tt.setup(client)
			}
			resp, handled := engine.Handle(client, tt.cmd)
			if !handled {
				t.Fatalf("command %q was not handled", tt.cmd)
			}
			if resp == "" {
				t.Fatalf("expected response for command %q", tt.cmd)
			}
			tt.check(t, client.filter)
		})
	}
}

func TestCCSyntaxReturnsHint(t *testing.T) {
	client := newTestClient()
	engine := newFilterCommandEngine()
	resp, handled := engine.Handle(client, "SET/FILTER BAND 20M")
	if !handled {
		t.Fatalf("expected cc-style command to be handled")
	}
	if !strings.Contains(strings.ToLower(resp), "dialect") || !strings.Contains(strings.ToUpper(resp), "CC") {
		t.Fatalf("expected cc syntax hint mentioning dialect, got: %q", resp)
	}
}

func TestInvalidFilterCommandShowsHelpHint(t *testing.T) {
	client := newTestClient()
	engine := newFilterCommandEngine()
	resp, handled := engine.Handle(client, "PASS")
	if !handled {
		t.Fatalf("expected PASS command to be handled")
	}
	if !strings.Contains(strings.ToLower(resp), "help") {
		t.Fatalf("expected help hint in response, got: %q", resp)
	}
}

func TestDialectSwitchCommand(t *testing.T) {
	server := &Server{filterEngine: newFilterCommandEngine()}
	client := newTestClient()

	resp, handled := server.handleDialectCommand(client, "DIALECT")
	if !handled || !strings.Contains(strings.ToLower(resp), "dialect") {
		t.Fatalf("expected current dialect response, got handled=%v resp=%q", handled, resp)
	}

	resp, handled = server.handleDialectCommand(client, "DIALECT LIST")
	if !handled || !strings.Contains(resp, "GO") || !strings.Contains(resp, "CC") || strings.Contains(strings.ToUpper(resp), "LEGACY") {
		t.Fatalf("expected dialect list, got handled=%v resp=%q", handled, resp)
	}

	resp, handled = server.handleDialectCommand(client, "DIALECT cc")
	if !handled {
		t.Fatalf("expected dialect switch to be handled")
	}
	if client.dialect != DialectCC {
		t.Fatalf("expected dialect set to cc, got %s", client.dialect)
	}
	if !strings.Contains(strings.ToUpper(resp), "CC") {
		t.Fatalf("expected confirmation of cc dialect, got %q", resp)
	}
}

func TestCCDialectSetFilterExecutes(t *testing.T) {
	engine := newFilterCommandEngine()
	client := newTestClient()
	client.dialect = DialectCC

	resp, handled := engine.Handle(client, "SET/FILTER BAND 20M")
	if !handled {
		t.Fatalf("expected cc command to be handled")
	}
	if resp == "" {
		t.Fatalf("expected response for cc command")
	}
	band := spot.NormalizeBand("20m")
	if !client.filter.Bands[band] {
		t.Fatalf("expected band %s enabled under cc dialect", band)
	}
}

func TestCCDialectAliases(t *testing.T) {
	engine := newFilterCommandEngine()
	client := newTestClient()
	client.dialect = DialectCC

	resp, handled := engine.Handle(client, "SET/ANN")
	if !handled || resp == "" {
		t.Fatalf("expected SET/ANN handled with response, got handled=%v resp=%q", handled, resp)
	}
	if !client.filter.AnnounceEnabled() {
		t.Fatalf("expected announcements enabled via CC dialect")
	}

	resp, handled = engine.Handle(client, "SET/NOBEACON")
	if !handled || resp == "" {
		t.Fatalf("expected SET/NOBEACON handled with response, got handled=%v resp=%q", handled, resp)
	}
	if client.filter.BeaconsEnabled() {
		t.Fatalf("expected beacons disabled via CC dialect")
	}

	resp, handled = engine.Handle(client, "SET/SKIMMER")
	if !handled || resp == "" {
		t.Fatalf("expected SET/SKIMMER handled with response, got handled=%v resp=%q", handled, resp)
	}
	if !client.filter.Sources["SKIMMER"] || client.filter.BlockSources["SKIMMER"] {
		t.Fatalf("expected SKIMMER allowed via CC dialect")
	}

	resp, handled = engine.Handle(client, "SET/NOSKIMMER")
	if !handled || resp == "" {
		t.Fatalf("expected SET/NOSKIMMER handled with response, got handled=%v resp=%q", handled, resp)
	}
	if !client.filter.BlockSources["SKIMMER"] {
		t.Fatalf("expected SKIMMER blocked via CC dialect")
	}

	resp, handled = engine.Handle(client, "SET/FT8")
	if !handled || resp == "" {
		t.Fatalf("expected SET/FT8 handled with response, got handled=%v resp=%q", handled, resp)
	}
	if !client.filter.Modes["FT8"] {
		t.Fatalf("expected FT8 enabled via CC dialect")
	}

	resp, handled = engine.Handle(client, "SET/NOFT8")
	if !handled || resp == "" {
		t.Fatalf("expected SET/NOFT8 handled with response, got handled=%v resp=%q", handled, resp)
	}
	if !client.filter.BlockModes["FT8"] {
		t.Fatalf("expected FT8 blocked via CC dialect")
	}
}

func TestCCDialectDXBMMapping(t *testing.T) {
	engine := newFilterCommandEngine()
	client := newTestClient()
	client.dialect = DialectCC

	resp, handled := engine.Handle(client, "SET/FILTER DXBM/PASS 160,20")
	if !handled || resp == "" {
		t.Fatalf("expected DXBM PASS handled, got handled=%v resp=%q", handled, resp)
	}
	if !client.filter.Bands[spot.NormalizeBand("160m")] || !client.filter.Bands[spot.NormalizeBand("20m")] {
		t.Fatalf("expected 160m and 20m allowed via DXBM mapping")
	}

	resp, handled = engine.Handle(client, "SET/FILTER DXBM/REJECT 160")
	if !handled || resp == "" {
		t.Fatalf("expected DXBM REJECT handled, got handled=%v resp=%q", handled, resp)
	}
	if client.filter.Bands[spot.NormalizeBand("160m")] {
		t.Fatalf("expected 160m blocked via DXBM REJECT")
	}

	resp, handled = engine.Handle(client, "SET/FILTER DXBM/PASS MW-MW")
	if !handled || !strings.Contains(resp, "Unknown DXBM band") {
		t.Fatalf("expected unknown DXBM band error, got handled=%v resp=%q", handled, resp)
	}
}

func TestCCDialectNoFilterReset(t *testing.T) {
	engine := newFilterCommandEngine()
	client := newTestClient()
	client.dialect = DialectCC

	// Bias the filter to ensure reset occurs.
	client.filter.SetBand("20M", false)
	client.filter.BlockAllBands = true

	resp, handled := engine.Handle(client, "SET/NOFILTER")
	if !handled || resp == "" {
		t.Fatalf("expected SET/NOFILTER handled with response, got handled=%v resp=%q", handled, resp)
	}
	if !client.filter.AllBands || client.filter.BlockAllBands {
		t.Fatalf("expected filters reset to permissive defaults")
	}
}

func TestCCDialectFilterOffSuffix(t *testing.T) {
	engine := newFilterCommandEngine()
	client := newTestClient()
	client.dialect = DialectCC

	resp, handled := engine.Handle(client, "SET/FILTER BAND/OFF")
	if !handled || resp == "" {
		t.Fatalf("expected BAND/OFF handled, got handled=%v resp=%q", handled, resp)
	}
	if !client.filter.BlockAllBands || client.filter.AllBands {
		t.Fatalf("expected BAND/OFF to block all bands")
	}
}

func TestDialectPersistence(t *testing.T) {
	origDir := filter.UserDataDir
	tmp := t.TempDir()
	filter.UserDataDir = tmp
	defer func() { filter.UserDataDir = origDir }()

	client := &Client{
		filter:   filter.NewFilter(),
		dialect:  DialectCC,
		callsign: "TEST1",
	}
	client.recentIPs = filter.UpdateRecentIPs(nil, "1.2.3.4")

	if err := client.saveFilter(); err != nil {
		t.Fatalf("saveFilter failed: %v", err)
	}

	record, err := filter.LoadUserRecord("TEST1")
	if err != nil {
		t.Fatalf("load user record failed: %v", err)
	}
	if got := normalizeDialectName(record.Dialect); got != DialectCC {
		t.Fatalf("expected dialect cc persisted, got %s", got)
	}
}

func TestLegacyRecordFallsBackToGoCluster(t *testing.T) {
	if got := normalizeDialectName("legacy"); got != DialectGo {
		t.Fatalf("expected legacy token to normalize to go, got %s", got)
	}
}

func TestDialectWelcomeLine(t *testing.T) {
	defaultDialect := DialectGo
	line := dialectWelcomeLine(DialectGo, true, nil, defaultDialect)
	if !strings.Contains(line, "GO") || !strings.Contains(strings.ToLower(line), "default") {
		t.Fatalf("expected default dialect welcome line, got %q", line)
	}
	if !strings.Contains(strings.ToUpper(line), "HELP") {
		t.Fatalf("expected welcome line to mention HELP, got %q", line)
	}

	line = dialectWelcomeLine(DialectCC, false, nil, defaultDialect)
	if !strings.Contains(line, "CC") || !strings.Contains(strings.ToLower(line), "persisted") {
		t.Fatalf("expected persisted cc dialect welcome line, got %q", line)
	}
	if !strings.Contains(strings.ToUpper(line), "HELP") {
		t.Fatalf("expected welcome line to mention HELP, got %q", line)
	}
}

func TestBroadcastWWVRespectsFilter(t *testing.T) {
	server := &Server{
		clients: make(map[string]*Client),
	}

	allow := &Client{
		callsign:     "ALLOW",
		bulletinChan: make(chan bulletin, 1),
		filter:       filter.NewFilter(),
	}
	block := &Client{
		callsign:     "BLOCK",
		bulletinChan: make(chan bulletin, 1),
		filter:       filter.NewFilter(),
	}
	block.filter.SetWWVEnabled(false)

	server.clients["ALLOW"] = allow
	server.clients["BLOCK"] = block

	server.BroadcastWWV("WWV", "WWV de TEST <00> : SFI=1 A=1 K=1")

	select {
	case <-allow.bulletinChan:
	default:
		t.Fatalf("expected bulletin delivered to allowed client")
	}
	select {
	case <-block.bulletinChan:
		t.Fatalf("did not expect bulletin delivered to blocked client")
	default:
	}
}

func TestBroadcastAnnouncementRespectsFilter(t *testing.T) {
	server := &Server{
		clients: make(map[string]*Client),
	}

	allow := &Client{
		callsign:     "ALLOW",
		bulletinChan: make(chan bulletin, 1),
		filter:       filter.NewFilter(),
	}
	block := &Client{
		callsign:     "BLOCK",
		bulletinChan: make(chan bulletin, 1),
		filter:       filter.NewFilter(),
	}
	block.filter.SetAnnounceEnabled(false)

	server.clients["ALLOW"] = allow
	server.clients["BLOCK"] = block

	server.BroadcastAnnouncement("To ALL de TEST: hello")

	select {
	case <-allow.bulletinChan:
	default:
		t.Fatalf("expected announcement delivered to allowed client")
	}
	select {
	case <-block.bulletinChan:
		t.Fatalf("did not expect announcement delivered to blocked client")
	default:
	}
}
