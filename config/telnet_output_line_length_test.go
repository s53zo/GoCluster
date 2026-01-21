package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsShortTelnetOutputLineLength(t *testing.T) {
	dir := t.TempDir()
	config := `telnet:
  output_line_length: 64
`
	if err := os.WriteFile(filepath.Join(dir, "runtime.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write runtime.yaml: %v", err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatalf("expected error for short telnet output line length")
	}
	if !strings.Contains(err.Error(), "telnet.output_line_length") {
		t.Fatalf("expected telnet.output_line_length error, got %v", err)
	}
}

func TestLoadDefaultsTelnetOutputLineLength(t *testing.T) {
	dir := t.TempDir()
	config := `telnet: {}
`
	if err := os.WriteFile(filepath.Join(dir, "runtime.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write runtime.yaml: %v", err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Telnet.OutputLineLength != 76 {
		t.Fatalf("expected default telnet.output_line_length=76, got %d", cfg.Telnet.OutputLineLength)
	}
}
