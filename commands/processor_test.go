package commands

import (
	"math"
	"strings"
	"testing"
	"time"

	"dxcluster/buffer"
	"dxcluster/cty"
	"dxcluster/spot"
)

func TestDXCommandQueuesSpot(t *testing.T) {
	input := make(chan *spot.Spot, 1)
	p := NewProcessor(nil, nil, input, nil)

	resp := p.ProcessCommandForClient("DX 7001 K8ZB Testing...1...2...3", "N2WQ", "203.0.113.5", nil, "classic")
	if !strings.Contains(resp, "Spot queued") {
		t.Fatalf("expected queue response, got %q", resp)
	}

	select {
	case s := <-input:
		if s.DXCall != "K8ZB" {
			t.Fatalf("DXCall mismatch: %s", s.DXCall)
		}
		if s.DECall != "N2WQ" {
			t.Fatalf("DECall mismatch: %s", s.DECall)
		}
		if math.Abs(s.Frequency-7001.0) > 0.0001 {
			t.Fatalf("Frequency mismatch: %.4f", s.Frequency)
		}
		if s.Comment != "Testing...1...2...3" {
			t.Fatalf("Comment mismatch: %q", s.Comment)
		}
		if s.SourceType != spot.SourceManual || !s.IsHuman {
			t.Fatalf("unexpected source flags: %s human=%t", s.SourceType, s.IsHuman)
		}
		if s.SpotterIP != "203.0.113.5" {
			t.Fatalf("SpotterIP mismatch: %q", s.SpotterIP)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for spot")
	}
}

func TestDXCommandValidation(t *testing.T) {
	input := make(chan *spot.Spot, 1)
	p := NewProcessor(nil, nil, input, nil)

	resp := p.ProcessCommandForClient("DX 7001 K8ZB", "N2WQ", "", nil, "classic")
	if strings.Contains(resp, "Usage: DX") {
		t.Fatalf("expected optional comment to queue, got %q", resp)
	}
	select {
	case s := <-input:
		if s.Comment != "" {
			t.Fatalf("expected empty comment, got %q", s.Comment)
		}
	default:
		t.Fatal("expected spot to be queued without comment")
	}

	resp = p.ProcessCommandForClient("DX 7001 K8ZB Test", "", "", nil, "classic")
	if !strings.Contains(resp, "logged-in") {
		t.Fatalf("expected callsign error, got %q", resp)
	}
}

func TestDXCommandCTYValidation(t *testing.T) {
	ctyDB := loadTestCTY(t)
	ctyLookup := func() *cty.CTYDatabase { return ctyDB }
	input := make(chan *spot.Spot, 1)
	p := NewProcessor(nil, nil, input, ctyLookup)

	resp := p.ProcessCommandForClient("DX 7001 K8ZB Test", "N2WQ", "", nil, "classic")
	if strings.Contains(resp, "Unknown DX callsign") {
		t.Fatalf("expected known DX to queue, got %q", resp)
	}
	select {
	case <-input:
	default:
		t.Fatalf("expected spot queued for known CTY call")
	}

	resp = p.ProcessCommandForClient("DX 7001 K4ZZZ Test", "N2WQ", "", nil, "classic")
	if !strings.Contains(resp, "Unknown DX callsign") {
		t.Fatalf("expected CTY validation error, got %q", resp)
	}
}

func TestShowDXCCPrefixAndSiblings(t *testing.T) {
	ctyDB := loadTestCTY(t)
	ctyLookup := func() *cty.CTYDatabase { return ctyDB }
	p := NewProcessor(nil, nil, nil, ctyLookup)

	resp := p.ProcessCommand("SHOW DXCC IT9")
	expected := "IT9 -> ADIF 248 | Sicily (EU) | Prefix: IT9 | CQ 15 | ITU 28 | Other: I"
	if !strings.Contains(resp, expected) {
		t.Fatalf("expected %q in response, got %q", expected, resp)
	}
}

func TestShowDXCCPortableCall(t *testing.T) {
	ctyDB := loadTestCTY(t)
	ctyLookup := func() *cty.CTYDatabase { return ctyDB }
	p := NewProcessor(nil, nil, nil, ctyLookup)

	resp := p.ProcessCommand("SHOW DXCC W6/LZ5VV")
	expected := "W6/LZ5VV -> ADIF 291 | United States (NA) | Prefix: K | CQ 3 | ITU 6"
	if !strings.Contains(resp, expected) {
		t.Fatalf("expected %q in response, got %q", expected, resp)
	}
	if strings.Contains(resp, "Other:") {
		t.Fatalf("did not expect Other list for single-prefix ADIF: %q", resp)
	}
}

func TestShowDXCCMobileSuffix(t *testing.T) {
	ctyDB := loadTestCTY(t)
	ctyLookup := func() *cty.CTYDatabase { return ctyDB }
	p := NewProcessor(nil, nil, nil, ctyLookup)

	resp := p.ProcessCommand("SHOW DXCC W6/LZ5VV/M")
	expected := "W6/LZ5VV -> ADIF 291 | United States (NA) | Prefix: K | CQ 3 | ITU 6"
	if !strings.Contains(resp, expected) {
		t.Fatalf("expected %q in response, got %q", expected, resp)
	}
}

func TestShowMYDXRequiresFilter(t *testing.T) {
	p := NewProcessor(nil, nil, nil, nil)
	resp := p.ProcessCommandForClient("SHOW MYDX 5", "N2WQ", "", nil, "classic")
	if !strings.Contains(resp, "SHOW MYDX requires a logged-in session") {
		t.Fatalf("expected filter requirement message, got %q", resp)
	}
}

func TestShowMYDXFiltersResults(t *testing.T) {
	rb := buffer.NewRingBuffer(10)
	spotOld := spot.NewSpot("DXAAA", "DE1AA", 14074.0, "FT8")
	spotOld.Time = time.Now().UTC().Add(-2 * time.Minute)
	spotNew := spot.NewSpot("DXBBB", "DE2BB", 14030.0, "CW")
	spotNew.Time = time.Now().UTC().Add(-1 * time.Minute)
	rb.Add(spotOld)
	rb.Add(spotNew)

	p := NewProcessor(rb, nil, nil, nil)
	filterFn := func(s *spot.Spot) bool {
		return s != nil && s.DXCall == "DXBBB"
	}
	resp := p.ProcessCommandForClient("SHOW MYDX 5", "N2WQ", "", filterFn, "classic")
	if !strings.Contains(resp, "DXBBB") {
		t.Fatalf("expected filtered spot DXBBB, got %q", resp)
	}
	if strings.Contains(resp, "DXAAA") {
		t.Fatalf("unexpected filtered spot DXAAA in response: %q", resp)
	}
}

func TestHelpPerDialect(t *testing.T) {
	p := NewProcessor(nil, nil, nil, nil)

	classic := p.ProcessCommandForClient("HELP", "", "", nil, "classic")
	if !strings.Contains(classic, "PASS BAND") || !strings.Contains(classic, "Current dialect: GOCLUSTER") {
		t.Fatalf("classic help missing expected content: %q", classic)
	}

	cc := p.ProcessCommandForClient("HELP", "", "", nil, "cc")
	if !strings.Contains(strings.ToUpper(cc), "SET/ANN") || !strings.Contains(strings.ToUpper(cc), "SET/NOFILTER") {
		t.Fatalf("cc help missing cc aliases: %q", cc)
	}
}

const sampleCTYPLIST = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
<key>K8ZB</key>
	<dict>
		<key>Country</key>
		<string>Alpha</string>
		<key>Prefix</key>
		<string>K8ZB</string>
		<key>ADIF</key>
		<integer>1</integer>
		<key>CQZone</key>
		<integer>5</integer>
		<key>ITUZone</key>
		<integer>8</integer>
		<key>Continent</key>
		<string>NA</string>
		<key>ExactCallsign</key>
		<true/>
	</dict>
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
		<string>K</string>
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
<key>I</key>
	<dict>
		<key>Country</key>
		<string>Italy</string>
		<key>Prefix</key>
		<string>I</string>
		<key>ADIF</key>
		<integer>248</integer>
		<key>CQZone</key>
		<integer>15</integer>
		<key>ITUZone</key>
		<integer>28</integer>
		<key>Continent</key>
		<string>EU</string>
		<key>ExactCallsign</key>
		<false/>
	</dict>
<key>IT9</key>
	<dict>
		<key>Country</key>
		<string>Sicily</string>
		<key>Prefix</key>
		<string>IT9</string>
		<key>ADIF</key>
		<integer>248</integer>
		<key>CQZone</key>
		<integer>15</integer>
		<key>ITUZone</key>
		<integer>28</integer>
		<key>Continent</key>
		<string>EU</string>
		<key>ExactCallsign</key>
		<false/>
	</dict>
</dict>
</plist>`

func loadTestCTY(t *testing.T) *cty.CTYDatabase {
	t.Helper()
	db, err := cty.LoadCTYDatabaseFromReader(strings.NewReader(sampleCTYPLIST))
	if err != nil {
		t.Fatalf("load test CTY: %v", err)
	}
	return db
}
