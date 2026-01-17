package telnet

import (
	"strings"
	"testing"

	"dxcluster/spot"
)

func TestInjectGlyphsNonASCIIReplaced(t *testing.T) {
	layout := spot.CurrentDXClusterLayout()
	base := strings.Repeat("X", layout.LineLength)
	got := injectGlyphs(base, "\u2605")

	if len(got) != len(base) {
		t.Fatalf("expected length %d, got %d", len(base), len(got))
	}
	sepIdx := layout.GlyphColumn - 2
	if got[sepIdx] != ' ' {
		t.Fatalf("expected separator space at 0-based index %d, got %q", sepIdx, got[sepIdx])
	}
	glyphIdx := layout.GlyphColumn - 1
	if got[glyphIdx] != '?' {
		t.Fatalf("expected non-ASCII glyph replaced with '?', got %q", got[glyphIdx])
	}
}

func TestInjectGlyphsRespectsLayout(t *testing.T) {
	prev := spot.CurrentDXClusterLayout()
	if err := spot.SetDXClusterLineLength(70); err != nil {
		t.Fatalf("set line length: %v", err)
	}
	t.Cleanup(func() {
		_ = spot.SetDXClusterLineLength(prev.LineLength)
	})

	layout := spot.CurrentDXClusterLayout()
	base := strings.Repeat("X", layout.LineLength)
	got := injectGlyphs(base, "V")

	if got[layout.GlyphColumn-1] != 'V' {
		t.Fatalf("expected glyph at column %d, got %q", layout.GlyphColumn, got[layout.GlyphColumn-1])
	}
}
