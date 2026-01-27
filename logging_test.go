package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogFileNameForDate(t *testing.T) {
	when := time.Date(2026, time.January, 22, 12, 0, 0, 0, time.UTC)
	if got := logFileNameForDate(when); got != "22-Jan-2026.log" {
		t.Fatalf("expected log filename to be 22-Jan-2026.log, got %q", got)
	}
}

func TestParseLogFileDate(t *testing.T) {
	parsed, ok := parseLogFileDate("22-Jan-2026.log")
	if !ok {
		t.Fatalf("expected parse to succeed")
	}
	if parsed.Year() != 2026 || parsed.Month() != time.January || parsed.Day() != 22 {
		t.Fatalf("unexpected parsed date: %s", parsed.Format(time.RFC3339))
	}
	if _, ok := parseLogFileDate("notes.txt"); ok {
		t.Fatalf("expected non-log file to be rejected")
	}
}

func TestCleanupOldLogs(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"20-Jan-2026.log",
		"21-Jan-2026.log",
		"22-Jan-2026.log",
		"notes.txt",
	}
	for _, name := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	now := time.Date(2026, time.January, 22, 12, 0, 0, 0, time.UTC)
	if err := cleanupOldLogs(dir, now, 2); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	expectMissing := []string{"20-Jan-2026.log"}
	for _, name := range expectMissing {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Fatalf("expected %s to be removed", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", name, err)
		}
	}
	expectPresent := []string{"21-Jan-2026.log", "22-Jan-2026.log", "notes.txt"}
	for _, name := range expectPresent {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected %s to remain: %v", name, err)
		}
	}
}
