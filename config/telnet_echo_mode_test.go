package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRejectsUnknownTelnetEchoMode(t *testing.T) {
	dir := t.TempDir()
	config := `telnet:
  echo_mode: "invalid"
`
	if err := os.WriteFile(filepath.Join(dir, "runtime.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write runtime.yaml: %v", err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatalf("expected error for unknown telnet echo mode")
	}
}
