package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dxcluster/internal/propreport"
)

func TestParseLogAllowsLargeLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.log")
	payload := strings.Repeat("a", 100*1024)
	line := "2026/01/26 00:00:00 " + payload + "\n"
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	entries, err := propreport.ParseLog(path)
	if err != nil {
		t.Fatalf("parse log: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !strings.Contains(entries[0], payload) {
		t.Fatalf("expected payload to be preserved in entry")
	}
}

func TestParseLogPreservesLineBoundaries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.log")
	content := strings.Join([]string{
		"2026/01/26 00:00:00 Path buckets 160m f=1 c=1",
		"continued line",
		"2026/01/26 01:00:00 Path buckets 160m f=2 c=2",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	entries, err := propreport.ParseLog(path)
	if err != nil {
		t.Fatalf("parse log: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if !strings.Contains(entries[0], "Path buckets") || !strings.Contains(entries[0], "\ncontinued line") {
		t.Fatalf("expected continuation line to be preserved with newline")
	}
}
