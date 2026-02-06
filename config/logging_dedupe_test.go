package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoggingDropDedupeWindowDefault(t *testing.T) {
	dir := t.TempDir()
	cfgText := `logging:
  enabled: true
`
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(cfgText), 0o644); err != nil {
		t.Fatalf("write app.yaml: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Logging.DropDedupeWindowSeconds != 120 {
		t.Fatalf("expected default drop_dedupe_window_seconds=120, got %d", cfg.Logging.DropDedupeWindowSeconds)
	}
}

func TestLoggingDropDedupeWindowAllowsZero(t *testing.T) {
	dir := t.TempDir()
	cfgText := `logging:
  drop_dedupe_window_seconds: 0
`
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(cfgText), 0o644); err != nil {
		t.Fatalf("write app.yaml: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Logging.DropDedupeWindowSeconds != 0 {
		t.Fatalf("expected drop_dedupe_window_seconds=0, got %d", cfg.Logging.DropDedupeWindowSeconds)
	}
}

func TestLoggingDropDedupeWindowRejectsNegative(t *testing.T) {
	dir := t.TempDir()
	cfgText := `logging:
  drop_dedupe_window_seconds: -1
`
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(cfgText), 0o644); err != nil {
		t.Fatalf("write app.yaml: %v", err)
	}

	if _, err := Load(dir); err == nil {
		t.Fatalf("expected Load() to fail for negative logging.drop_dedupe_window_seconds")
	}
}
