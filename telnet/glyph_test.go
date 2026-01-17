package telnet

import (
	"strings"
	"testing"
)

func TestInjectGlyphsNonASCIIReplaced(t *testing.T) {
	base := strings.Repeat("X", 78)
	got := injectGlyphs(base, "\u2605")

	if len(got) != len(base) {
		t.Fatalf("expected length %d, got %d", len(base), len(got))
	}
	if got[63] != ' ' {
		t.Fatalf("expected separator space at 0-based index 63, got %q", got[63])
	}
	if got[64] != '?' {
		t.Fatalf("expected non-ASCII glyph replaced with '?', got %q", got[64])
	}
}
