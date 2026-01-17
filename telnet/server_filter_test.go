package telnet

import (
	"reflect"
	"strings"
	"testing"

	"dxcluster/cty"
	"dxcluster/filter"
	"dxcluster/spot"
)

func newTestClient() *Client {
	return &Client{
		filter:  filter.NewFilter(),
		dialect: DialectGo,
	}
}

const sampleCTYPLIST = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
<key>K1</key>
	<dict>
		<key>Country</key>
		<string>Alpha</string>
		<key>Prefix</key>
		<string>K1</string>
		<key>ADIF</key>
		<integer>1</integer>
		<key>CQZone</key>
		<integer>5</integer>
		<key>ITUZone</key>
		<integer>8</integer>
		<key>Continent</key>
		<string>NA</string>
		<key>ExactCallsign</key>
		<false/>
	</dict>
<key>W6</key>
	<dict>
		<key>Country</key>
		<string>United States</string>
		<key>Prefix</key>
		<string>W6</string>
		<key>ADIF</key>
		<integer>291</integer>
		<key>CQZone</key>
		<integer>3</integer>
		<key>ITUZone</key>
		<integer>6</integer>
		<key>Continent</key>
		<string>NA</string>
		<key>ExactCallsign</key>
		<false/>
	</dict>
</dict>
</plist>`

func loadTestCTY(t *testing.T) *cty.CTYDatabase {
	t.Helper()
	db, err := cty.LoadCTYDatabaseFromReader(strings.NewReader(sampleCTYPLIST))
	if err != nil {
		t.Fatalf("failed to load CTY sample: %v", err)
	}
	return db
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
			name: "pass dxcall pattern list",
			cmd:  "PASS DXCALL K1*,W1*",
			check: func(t *testing.T, f *filter.Filter) {
				if len(f.DXCallsigns) != 2 {
					t.Fatalf("expected two DX callsign patterns, got %d", len(f.DXCallsigns))
				}
				if f.DXCallsigns[0] != "K1*" || f.DXCallsigns[1] != "W1*" {
					t.Fatalf("expected DX callsign patterns K1*, W1*, got %v", f.DXCallsigns)
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
			name: "pass decall pattern list",
			cmd:  "PASS DECALL W1*,K1*",
			check: func(t *testing.T, f *filter.Filter) {
				if len(f.DECallsigns) != 2 {
					t.Fatalf("expected two DE callsign patterns, got %d", len(f.DECallsigns))
				}
				if f.DECallsigns[0] != "W1*" || f.DECallsigns[1] != "K1*" {
					t.Fatalf("expected DE callsign patterns W1*, K1*, got %v", f.DECallsigns)
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
			name: "reject source all blocks",
			cmd:  "REJECT SOURCE ALL",
			check: func(t *testing.T, f *filter.Filter) {
				if !f.BlockAllSources || f.AllSources {
					t.Fatalf("expected all sources blocked after REJECT SOURCE ALL")
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
			name: "reject dxcall requires args",
			cmd:  "REJECT DXCALL",
			setup: func(c *Client) {
				c.filter.AddDXCallsignPattern("K1*")
			},
			check: func(t *testing.T, f *filter.Filter) {
				if len(f.DXCallsigns) != 1 || f.DXCallsigns[0] != "K1*" {
					t.Fatalf("expected DX callsign patterns unchanged")
				}
			},
		},
		{
			name: "reject decall requires args",
			cmd:  "REJECT DECALL",
			setup: func(c *Client) {
				c.filter.AddDECallsignPattern("W1*")
			},
			check: func(t *testing.T, f *filter.Filter) {
				if len(f.DECallsigns) != 1 || f.DECallsigns[0] != "W1*" {
					t.Fatalf("expected DE callsign patterns unchanged")
				}
			},
		},
		{
			name: "reject all invalid",
			cmd:  "REJECT ALL",
			check: func(t *testing.T, f *filter.Filter) {
				assertFilterMatchesDefaults(t, f)
			},
		},
		{
			name: "pass all invalid",
			cmd:  "PASS ALL",
			check: func(t *testing.T, f *filter.Filter) {
				assertFilterMatchesDefaults(t, f)
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

func TestShowFilterSnapshotDefault(t *testing.T) {
	client := newTestClient()
	engine := newFilterCommandEngine()

	resp, handled := engine.Handle(client, "SHOW FILTER")
	if !handled {
		t.Fatalf("expected SHOW FILTER to be handled")
	}
	if !strings.HasPrefix(resp, "Current filters: ") {
		t.Fatalf("expected summary line, got: %q", resp)
	}
	if strings.Contains(resp, "MODE: allow=ALL") {
		t.Fatalf("expected default modes to be listed explicitly, got: %q", resp)
	}
	if !strings.Contains(resp, "MODE: allow=CW, LSB, USB, RTTY") {
		t.Fatalf("expected default mode list in snapshot, got: %q", resp)
	}
	if !strings.Contains(resp, "DXCALL: allow=ALL block=NONE") {
		t.Fatalf("expected DXCALL line to show allow/block defaults, got: %q", resp)
	}
}

func TestShowFilterNormalization(t *testing.T) {
	client := newTestClient()
	engine := newFilterCommandEngine()

	if _, handled := engine.Handle(client, "PASS SOURCE HUMAN"); !handled {
		t.Fatalf("expected PASS SOURCE HUMAN to be handled")
	}
	if _, handled := engine.Handle(client, "PASS SOURCE SKIMMER"); !handled {
		t.Fatalf("expected PASS SOURCE SKIMMER to be handled")
	}
	resp, _ := engine.Handle(client, "SHOW FILTER")
	if !strings.Contains(resp, "SOURCE: allow=ALL") {
		t.Fatalf("expected SOURCE allow list normalized to ALL, got: %q", resp)
	}

	allContinents := strings.Join(filter.SupportedContinents, ", ")
	if _, handled := engine.Handle(client, "PASS DXCONT "+allContinents); !handled {
		t.Fatalf("expected PASS DXCONT to be handled")
	}
	resp, _ = engine.Handle(client, "SHOW FILTER")
	if !strings.Contains(resp, "DXCONT: allow=ALL") {
		t.Fatalf("expected DXCONT allow list normalized to ALL, got: %q", resp)
	}
}

func TestShowFilterShowsBlockList(t *testing.T) {
	client := newTestClient()
	engine := newFilterCommandEngine()

	if _, handled := engine.Handle(client, "REJECT BAND 20m"); !handled {
		t.Fatalf("expected REJECT BAND to be handled")
	}
	resp, _ := engine.Handle(client, "SHOW FILTER")
	if !strings.Contains(resp, "BAND: allow=ALL block=20m") {
		t.Fatalf("expected band block list in snapshot, got: %q", resp)
	}
	if !strings.Contains(resp, "effective: all except: 20m") {
		t.Fatalf("expected effective label in snapshot, got: %q", resp)
	}
}

func TestShowFilterDeprecatedForms(t *testing.T) {
	engine := newFilterCommandEngine()
	client := newTestClient()

	resp, handled := engine.Handle(client, "SHOW FILTER MODES")
	if !handled {
		t.Fatalf("expected SHOW FILTER MODES to be handled")
	}
	if !strings.HasPrefix(resp, "Current filters: ") || !strings.Contains(strings.ToLower(resp), "deprecated") {
		t.Fatalf("expected snapshot with deprecation warning, got: %q", resp)
	}

	client.dialect = DialectCC
	resp, handled = engine.Handle(client, "SHOW/FILTER BAND")
	if !handled {
		t.Fatalf("expected SHOW/FILTER BAND to be handled")
	}
	if !strings.HasPrefix(resp, "Current filters: ") || !strings.Contains(strings.ToLower(resp), "deprecated") {
		t.Fatalf("expected cc snapshot with deprecation warning, got: %q", resp)
	}
}

func TestRejectCallsignUsesBlocklist(t *testing.T) {
	client := newTestClient()
	engine := newFilterCommandEngine()
	client.filter.AddDXCallsignPattern("K1*")

	resp, handled := engine.Handle(client, "REJECT DXCALL W1*")
	if !handled {
		t.Fatalf("expected REJECT DXCALL with args to be handled")
	}
	if strings.Contains(strings.ToLower(resp), "arguments ignored") {
		t.Fatalf("did not expect ignored-argument warning, got: %q", resp)
	}
	if len(client.filter.DXCallsigns) != 1 || client.filter.DXCallsigns[0] != "K1*" {
		t.Fatalf("expected DX allowlist to remain intact")
	}
	if len(client.filter.BlockDXCallsigns) != 1 || client.filter.BlockDXCallsigns[0] != "W1*" {
		t.Fatalf("expected DX blocklist to contain W1*")
	}

	resp, handled = engine.Handle(client, "REJECT DXCALL")
	if !handled {
		t.Fatalf("expected REJECT DXCALL to be handled")
	}
	if len(client.filter.DXCallsigns) != 1 || client.filter.DXCallsigns[0] != "K1*" {
		t.Fatalf("expected DX allowlist unchanged without args")
	}
	if len(client.filter.BlockDXCallsigns) != 1 || client.filter.BlockDXCallsigns[0] != "W1*" {
		t.Fatalf("expected DX blocklist unchanged without args")
	}
}

func TestShowFilterDXCCNormalizationUsesCTY(t *testing.T) {
	ctyDB := loadTestCTY(t)
	engine := newFilterCommandEngineWithCTY(func() *cty.CTYDatabase { return ctyDB })
	client := newTestClient()

	if _, handled := engine.Handle(client, "PASS DXDXCC 1,291"); !handled {
		t.Fatalf("expected PASS DXDXCC to be handled")
	}
	resp, _ := engine.Handle(client, "SHOW FILTER")
	if !strings.Contains(resp, "DXDXCC: allow=ALL") {
		t.Fatalf("expected DXDXCC allow list normalized to ALL, got: %q", resp)
	}
}

func TestPassZoneRejectsInvalidToken(t *testing.T) {
	client := newTestClient()
	engine := newFilterCommandEngine()

	resp, handled := engine.Handle(client, "PASS DXZONE 10 ABC")
	if !handled {
		t.Fatalf("expected PASS DXZONE to be handled")
	}
	if !strings.Contains(resp, "Unknown CQ zone: ABC") {
		t.Fatalf("expected invalid token in response, got: %q", resp)
	}
	if len(client.filter.DXZones) != 0 || !client.filter.AllDXZones {
		t.Fatalf("expected zone filter unchanged after invalid input")
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

func TestCCDialectCallsignList(t *testing.T) {
	engine := newFilterCommandEngine()
	client := newTestClient()
	client.dialect = DialectCC

	resp, handled := engine.Handle(client, "SET/FILTER DXCALL K1*,W1*")
	if !handled || resp == "" {
		t.Fatalf("expected SET/FILTER DXCALL handled, got handled=%v resp=%q", handled, resp)
	}
	if len(client.filter.DXCallsigns) != 2 {
		t.Fatalf("expected two DX callsign patterns, got %d", len(client.filter.DXCallsigns))
	}

	resp, handled = engine.Handle(client, "UNSET/FILTER DXCALL W1*")
	if !handled || resp == "" {
		t.Fatalf("expected UNSET/FILTER DXCALL handled, got handled=%v resp=%q", handled, resp)
	}
	if len(client.filter.BlockDXCallsigns) != 1 || client.filter.BlockDXCallsigns[0] != "W1*" {
		t.Fatalf("expected DX blocklist to contain W1*")
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

func TestCCDialectOffSpecialCases(t *testing.T) {
	engine := newFilterCommandEngine()
	client := newTestClient()
	client.dialect = DialectCC

	resp, handled := engine.Handle(client, "SET/FILTER SOURCE/OFF")
	if !handled || !strings.Contains(resp, "sources") {
		t.Fatalf("expected SOURCE/OFF handled, got handled=%v resp=%q", handled, resp)
	}
	if !client.filter.BlockAllSources || client.filter.AllSources {
		t.Fatalf("expected SOURCE/OFF to block all sources")
	}

	client.filter.AddDXCallsignPattern("K1*")
	resp, handled = engine.Handle(client, "SET/FILTER DXCALL/OFF")
	if !handled || !strings.Contains(strings.ToLower(resp), "blocked") {
		t.Fatalf("expected DXCALL/OFF to block all callsigns, got handled=%v resp=%q", handled, resp)
	}
	if len(client.filter.BlockDXCallsigns) != 1 || client.filter.BlockDXCallsigns[0] != "*" {
		t.Fatalf("expected DXCALL/OFF to block all callsigns")
	}

	client.filter.AddDECallsignPattern("W1*")
	resp, handled = engine.Handle(client, "SET/FILTER DECALL/OFF")
	if !handled || !strings.Contains(strings.ToLower(resp), "blocked") {
		t.Fatalf("expected DECALL/OFF to block all callsigns, got handled=%v resp=%q", handled, resp)
	}
	if len(client.filter.BlockDECallsigns) != 1 || client.filter.BlockDECallsigns[0] != "*" {
		t.Fatalf("expected DECALL/OFF to block all callsigns")
	}

	resp, handled = engine.Handle(client, "SET/FILTER DECALL/ON")
	if !handled || !strings.Contains(strings.ToLower(resp), "enabled") {
		t.Fatalf("expected DECALL/ON to allow all callsigns, got handled=%v resp=%q", handled, resp)
	}
	if len(client.filter.BlockDECallsigns) != 0 {
		t.Fatalf("expected DECALL/ON to clear blocklist")
	}
}

func TestResetFilterDefaultsGoDialect(t *testing.T) {
	engine := newFilterCommandEngine()
	client := newTestClient()
	client.dialect = DialectGo

	client.filter.SetBand("20M", false)
	client.filter.SetMode("FT8", true)
	client.filter.SetSource("HUMAN", true)
	client.filter.AddDXCallsignPattern("K1*")
	client.filter.SetWWVEnabled(false)
	client.filter.BlockAllBands = true
	client.filter.BlockAllModes = true
	client.filter.BlockAllSources = true

	resp, handled := engine.Handle(client, "RESET FILTER")
	if !handled || resp == "" {
		t.Fatalf("expected RESET FILTER handled with response, got handled=%v resp=%q", handled, resp)
	}

	assertFilterMatchesDefaults(t, client.filter)
}

func TestResetFilterDefaultsCCDialect(t *testing.T) {
	engine := newFilterCommandEngine()
	client := newTestClient()
	client.dialect = DialectCC

	client.filter.SetBand("20M", false)
	client.filter.SetMode("FT8", true)
	client.filter.SetSource("HUMAN", true)
	client.filter.AddDECallsignPattern("W1*")
	client.filter.SetAnnounceEnabled(false)
	client.filter.BlockAllBands = true
	client.filter.BlockAllModes = true
	client.filter.BlockAllSources = true

	resp, handled := engine.Handle(client, "RESET FILTER")
	if !handled || resp == "" {
		t.Fatalf("expected RESET FILTER handled with response, got handled=%v resp=%q", handled, resp)
	}

	assertFilterMatchesDefaults(t, client.filter)
}

func assertFilterMatchesDefaults(t *testing.T, got *filter.Filter) {
	t.Helper()
	expected := filter.NewFilter()

	if got == nil {
		t.Fatalf("expected filter instance, got nil")
	}
	if got.AllBands != expected.AllBands || got.BlockAllBands != expected.BlockAllBands {
		t.Fatalf("band defaults mismatch: got AllBands=%v BlockAllBands=%v", got.AllBands, got.BlockAllBands)
	}
	if got.AllModes != expected.AllModes || got.BlockAllModes != expected.BlockAllModes {
		t.Fatalf("mode defaults mismatch: got AllModes=%v BlockAllModes=%v", got.AllModes, got.BlockAllModes)
	}
	if !reflect.DeepEqual(got.Modes, expected.Modes) {
		t.Fatalf("mode defaults mismatch: got=%v expected=%v", got.Modes, expected.Modes)
	}
	if got.AllSources != expected.AllSources || got.BlockAllSources != expected.BlockAllSources {
		t.Fatalf("source defaults mismatch: got AllSources=%v BlockAllSources=%v", got.AllSources, got.BlockAllSources)
	}
	if !reflect.DeepEqual(got.Sources, expected.Sources) {
		t.Fatalf("source defaults mismatch: got=%v expected=%v", got.Sources, expected.Sources)
	}
	if len(got.DXCallsigns) != 0 || len(got.DECallsigns) != 0 {
		t.Fatalf("expected callsign patterns cleared, got DX=%v DE=%v", got.DXCallsigns, got.DECallsigns)
	}
	if got.AllConfidence != expected.AllConfidence || got.BlockAllConfidence != expected.BlockAllConfidence {
		t.Fatalf("confidence defaults mismatch: got AllConfidence=%v BlockAllConfidence=%v", got.AllConfidence, got.BlockAllConfidence)
	}
	if got.BeaconsEnabled() != expected.BeaconsEnabled() || got.WWVEnabled() != expected.WWVEnabled() || got.WCYEnabled() != expected.WCYEnabled() || got.AnnounceEnabled() != expected.AnnounceEnabled() {
		t.Fatalf("feature defaults mismatch: got beacon=%v wwv=%v wcy=%v announce=%v", got.BeaconsEnabled(), got.WWVEnabled(), got.WCYEnabled(), got.AnnounceEnabled())
	}
	if got.AllDXContinents != expected.AllDXContinents || got.AllDEContinents != expected.AllDEContinents {
		t.Fatalf("continent defaults mismatch: got DX=%v DE=%v", got.AllDXContinents, got.AllDEContinents)
	}
	if got.AllDXZones != expected.AllDXZones || got.AllDEZones != expected.AllDEZones {
		t.Fatalf("zone defaults mismatch: got DX=%v DE=%v", got.AllDXZones, got.AllDEZones)
	}
	if got.AllDXGrid2 != expected.AllDXGrid2 || got.AllDEGrid2 != expected.AllDEGrid2 {
		t.Fatalf("grid defaults mismatch: got DX=%v DE=%v", got.AllDXGrid2, got.AllDEGrid2)
	}
	if got.AllDXDXCC != expected.AllDXDXCC || got.AllDEDXCC != expected.AllDEDXCC {
		t.Fatalf("dxcc defaults mismatch: got DX=%v DE=%v", got.AllDXDXCC, got.AllDEDXCC)
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
	server := &Server{
		dialectSourceDef:  "default",
		dialectSourcePers: "persisted",
	}
	template := "Current dialect: <DIALECT> (<DIALECT_SOURCE>). Use DIALECT LIST or DIALECT <DIALECT_DEFAULT> to switch. Type HELP for commands in this dialect.\n"
	line := formatDialectWelcome(template, dialectTemplateData{
		dialect:        strings.ToUpper(string(DialectGo)),
		source:         server.dialectSourceLabel(DialectGo, true, nil, defaultDialect),
		defaultDialect: strings.ToUpper(string(defaultDialect)),
	})
	if !strings.Contains(line, "GO") || !strings.Contains(strings.ToLower(line), "default") {
		t.Fatalf("expected default dialect welcome line, got %q", line)
	}
	if !strings.Contains(strings.ToUpper(line), "HELP") {
		t.Fatalf("expected welcome line to mention HELP, got %q", line)
	}

	line = formatDialectWelcome(template, dialectTemplateData{
		dialect:        strings.ToUpper(string(DialectCC)),
		source:         server.dialectSourceLabel(DialectCC, false, nil, defaultDialect),
		defaultDialect: strings.ToUpper(string(defaultDialect)),
	})
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
