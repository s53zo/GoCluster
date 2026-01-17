package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGridDBCheckOnMissDefaultsTrue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grid.yaml")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.GridDBCheckOnMiss == nil {
		t.Fatalf("expected GridDBCheckOnMiss to be set")
	}
	if !*cfg.GridDBCheckOnMiss {
		t.Fatalf("expected GridDBCheckOnMiss=true by default")
	}
}

func TestGridDBCheckOnMissAllowsFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grid.yaml")
	if err := os.WriteFile(path, []byte("grid_db_check_on_miss: false\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.GridDBCheckOnMiss == nil {
		t.Fatalf("expected GridDBCheckOnMiss to be set")
	}
	if *cfg.GridDBCheckOnMiss {
		t.Fatalf("expected GridDBCheckOnMiss=false when configured")
	}
}
