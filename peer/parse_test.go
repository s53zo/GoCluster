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
