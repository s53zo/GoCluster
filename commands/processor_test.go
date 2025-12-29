package commands

import (
	"math"
	"strings"
	"testing"
	"time"

	"dxcluster/cty"
	"dxcluster/spot"
)

func TestDXCommandQueuesSpot(t *testing.T) {
	input := make(chan *spot.Spot, 1)
	p := NewProcessor(nil, nil, input, nil)

	resp := p.ProcessCommandForClient("DX 7001 K8ZB Testing...1...2...3", "N2WQ", "203.0.113.5")
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

	resp := p.ProcessCommandForClient("DX 7001 K8ZB", "N2WQ", "")
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

	resp = p.ProcessCommandForClient("DX 7001 K8ZB Test", "", "")
	if !strings.Contains(resp, "logged-in") {
		t.Fatalf("expected callsign error, got %q", resp)
	}
}

func TestDXCommandCTYValidation(t *testing.T) {
	ctyDB := loadTestCTY(t)
	ctyLookup := func() *cty.CTYDatabase { return ctyDB }
	input := make(chan *spot.Spot, 1)
	p := NewProcessor(nil, nil, input, ctyLookup)

	resp := p.ProcessCommandForClient("DX 7001 K8ZB Test", "N2WQ", "")
	if strings.Contains(resp, "Unknown DX callsign") {
		t.Fatalf("expected known DX to queue, got %q", resp)
	}
	select {
	case <-input:
	default:
		t.Fatalf("expected spot queued for known CTY call")
	}

	resp = p.ProcessCommandForClient("DX 7001 K4ZZZ Test", "N2WQ", "")
	if !strings.Contains(resp, "Unknown DX callsign") {
		t.Fatalf("expected CTY validation error, got %q", resp)
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
		<key>ExactCallsign</key>
		<true/>
	</dict>
<key>K1</key>
	<dict>
		<key>Country</key>
		<string>Alpha</string>
		<key>Prefix</key>
		<string>K1</string>
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
