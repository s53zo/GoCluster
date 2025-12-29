package peer

import (
	"strings"
	"testing"
)

func TestParsePC11IgnoresCommentTimeToken(t *testing.T) {
	line := "PC11^14074.0^K1ABC^23-Dec-2025^2001Z^CQ 1800Z TEST^W1XYZ^ORIGIN"
	frame, err := ParseFrame(line)
	if err != nil {
		t.Fatalf("ParseFrame error: %v", err)
	}
	spot, err := parseSpotFromFrame(frame, "FALLBACK")
	if err != nil {
		t.Fatalf("parseSpotFromFrame error: %v", err)
	}
	if got := spot.Time.UTC().Format("1504Z"); got != "2001Z" {
		t.Fatalf("expected PC11 time 2001Z, got %q", got)
	}
	if strings.Contains(spot.Comment, "1800Z") {
		t.Fatalf("expected comment time token stripped, got %q", spot.Comment)
	}
	if spot.Comment != "CQ TEST" {
		t.Fatalf("expected cleaned comment 'CQ TEST', got %q", spot.Comment)
	}
}

func TestParsePC26UsesCorrectFieldsAndIgnoresPlaceholder(t *testing.T) {
	line := "PC26^7074.0^DX1ABC^24-Dec-2025^1501Z^TEST COMMENT^SPOT1^ORIGIN^ ^H5^"
	frame, err := ParseFrame(line)
	if err != nil {
		t.Fatalf("ParseFrame error: %v", err)
	}
	spotEntry, err := parseSpotFromFrame(frame, "FALLBACK")
	if err != nil {
		t.Fatalf("parseSpotFromFrame error: %v", err)
	}
	if spotEntry.DXCall != "DX1ABC" || spotEntry.DECall != "SPOT1" {
		t.Fatalf("unexpected calls: dx=%s de=%s", spotEntry.DXCall, spotEntry.DECall)
	}
	if got := spotEntry.Time.UTC().Format("1504Z"); got != "1501Z" {
		t.Fatalf("expected time 1501Z, got %s", got)
	}
	if spotEntry.SourceNode != "ORIGIN" {
		t.Fatalf("expected origin ORIGIN, got %s", spotEntry.SourceNode)
	}
}
