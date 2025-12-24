package peer

import (
	"strings"
	"testing"
)

func TestBuildPC92ConfigIncludesConfigTypeAndEntry(t *testing.T) {
	s := &session{
		localCall:   "N0CALL",
		pc92Bitmap:  5,
		nodeVersion: "5457",
		nodeBuild:   "build",
		hopCount:    99,
		tsGen:       &timestampGenerator{},
		pc9x:        true,
	}
	line := s.buildPC92Config()
	if !strings.HasPrefix(line, "PC92^N0CALL^") {
		t.Fatalf("unexpected prefix: %s", line)
	}
	if !strings.Contains(line, "^C^5N0CALL:5457:build^H99^") {
		t.Fatalf("config frame missing expected payload: %s", line)
	}
}
