package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPropReportRefreshDefault(t *testing.T) {
	dir := t.TempDir()
	cfg := `prop_report:
  enabled: true
`
	if err := os.WriteFile(filepath.Join(dir, "prop_report.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.PropReport.RefreshUTC != "00:05" {
		t.Fatalf("expected default refresh_utc 00:05, got %q", loaded.PropReport.RefreshUTC)
	}
}

func TestPropReportRefreshInvalid(t *testing.T) {
	dir := t.TempDir()
	cfg := `prop_report:
  enabled: true
  refresh_utc: "25:99"
`
	if err := os.WriteFile(filepath.Join(dir, "prop_report.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatalf("expected Load() to fail for invalid prop_report.refresh_utc")
	}
}
