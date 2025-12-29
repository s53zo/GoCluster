package commands

import (
	"math"
	"strings"
	"testing"
	"time"

	"dxcluster/spot"
)

func TestDXCommandQueuesSpot(t *testing.T) {
	input := make(chan *spot.Spot, 1)
	p := NewProcessor(nil, nil, input)

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
	p := NewProcessor(nil, nil, make(chan *spot.Spot, 1))

	resp := p.ProcessCommandForClient("DX 7001 K8ZB", "N2WQ", "")
	if !strings.Contains(resp, "Usage: DX") {
		t.Fatalf("expected usage error, got %q", resp)
	}

	resp = p.ProcessCommandForClient("DX 7001 K8ZB Test", "", "")
	if !strings.Contains(resp, "logged-in") {
		t.Fatalf("expected callsign error, got %q", resp)
	}
}
