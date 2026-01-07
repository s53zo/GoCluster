package peer

import (
	"errors"
	"strings"
	"testing"
)

func TestFormatPC61DropLine(t *testing.T) {
	frame := &Frame{
		Type: "PC61",
		Fields: []string{
			"14025.0",
			"DX1AAA",
			"01-Jan-2025",
			"1200Z",
			"comment",
			"DE1BBB",
			"ORIGIN",
			"1.2.3.4",
		},
	}
	line := formatPC61DropLine(frame, nil, errors.New("pc61: invalid DX callsign"))
	wantParts := []string{
		"PC61 drop: reason=invalid_dx",
		"de=DE1BBB",
		"dx=DX1AAA",
		"band=20m",
		"freq=14025.0",
		"source=ORIGIN",
	}
	for _, part := range wantParts {
		if !strings.Contains(line, part) {
			t.Fatalf("expected %q in drop line, got %q", part, line)
		}
	}
}
