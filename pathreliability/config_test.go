package pathreliability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "path_reliability.yaml")
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadFileRejectsLegacyThresholdKeys(t *testing.T) {
	path := writeTempConfig(t, `
glyph_thresholds:
  excellent: -13
  good: -17
  marginal: -21
`)
	_, err := LoadFile(path)
	if err == nil {
		t.Fatalf("expected legacy threshold keys to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unsupported glyph threshold key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFileRejectsInvalidGlyphSymbols(t *testing.T) {
	path := writeTempConfig(t, `
glyph_symbols:
  high: "++"
`)
	_, err := LoadFile(path)
	if err == nil {
		t.Fatalf("expected invalid glyph symbol to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "glyph_symbols.high") {
		t.Fatalf("unexpected error: %v", err)
	}
}
