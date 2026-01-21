package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDirectoryMergesFiles(t *testing.T) {
	dir := t.TempDir()

	app := `server:
  name: "Alpha"
cty:
  enabled: false
`
	dedupe := `server:
  node_id: "NODE-1"
dedup:
  secondary_fast_prefer_stronger_snr: true
`
	modeAlloc := `bands:
  - band: "160m"
    lower_khz: 1800
    cw_end_khz: 1900
    upper_khz: 2000
    voice_mode: "LSB"
`

	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(app), 0o644); err != nil {
		t.Fatalf("write app.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dedupe.yaml"), []byte(dedupe), 0o644); err != nil {
		t.Fatalf("write dedupe.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mode_allocations.yaml"), []byte(modeAlloc), 0o644); err != nil {
		t.Fatalf("write mode_allocations.yaml: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := filepath.Clean(cfg.LoadedFrom); got != filepath.Clean(dir) {
		t.Fatalf("expected LoadedFrom=%s, got %s", dir, got)
	}
	if cfg.Server.Name != "Alpha" {
		t.Fatalf("expected server.name to merge from app.yaml, got %q", cfg.Server.Name)
	}
	if cfg.Server.NodeID != "NODE-1" {
		t.Fatalf("expected server.node_id to merge from dedupe.yaml, got %q", cfg.Server.NodeID)
	}
	if !cfg.Dedup.SecondaryFastPreferStrong {
		t.Fatalf("expected dedup.secondary_fast_prefer_stronger_snr=true from dedupe.yaml")
	}
	if cfg.CTY.Enabled {
		t.Fatalf("expected cty.enabled=false from app.yaml, got true")
	}
}

func TestLoadRejectsSingleFilePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.yaml")
	if err := os.WriteFile(path, []byte("telnet:\n  port: 9300\n"), 0o644); err != nil {
		t.Fatalf("write runtime.yaml: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatalf("expected Load() to reject non-directory config path")
	}
}
