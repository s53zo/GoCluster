package main

import (
	"log"
	"os"
	"path/filepath"
	"sync"
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

func TestDailyFileSinkRotateHook(t *testing.T) {
	dir := t.TempDir()
	sink, err := newDailyFileSink(dir, 1)
	if err != nil {
		t.Fatalf("newDailyFileSink: %v", err)
	}
	defer sink.Close()

	var gotPrevDate time.Time
	var gotPrevPath string
	var gotNewPath string
	hookDone := make(chan struct{})
	var hookOnce sync.Once
	sink.SetRotateHook(func(prevDate time.Time, prevPath, newPath string) {
		gotPrevDate = prevDate
		gotPrevPath = prevPath
		gotNewPath = newPath
		hookOnce.Do(func() { close(hookDone) })
	})

	day1 := time.Date(2026, time.January, 22, 12, 0, 0, 0, time.UTC)
	day2 := day1.Add(24 * time.Hour)

	sink.WriteLine("first", day1)
	sink.WriteLine("second", day2)

	select {
	case <-hookDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("rotate hook did not complete")
	}
	if gotPrevDate.IsZero() {
		t.Fatalf("expected rotate hook to capture previous date")
	}
	if gotPrevDate.Year() != day1.Year() || gotPrevDate.Month() != day1.Month() || gotPrevDate.Day() != day1.Day() {
		t.Fatalf("unexpected prev date: %s", gotPrevDate.Format(time.RFC3339))
	}
	if gotPrevPath == "" || gotNewPath == "" {
		t.Fatalf("expected prev/new log paths to be set")
	}
	if filepath.Base(gotPrevPath) != "22-Jan-2026.log" {
		t.Fatalf("unexpected prev log path: %s", gotPrevPath)
	}
	if filepath.Base(gotNewPath) != "23-Jan-2026.log" {
		t.Fatalf("unexpected new log path: %s", gotNewPath)
	}
}

func TestRotateHookLoggingDoesNotDeadlock(t *testing.T) {
	dir := t.TempDir()
	sink, err := newDailyFileSink(dir, 1)
	if err != nil {
		t.Fatalf("newDailyFileSink: %v", err)
	}
	defer sink.Close()

	fanout := newLogFanout(nil, sink)
	logger := log.New(fanout, "", 0)

	now := time.Now().UTC()
	sink.WriteLine("prime", now)

	// Force the next log write to rotate without relying on wall-clock midnight.
	sink.mu.Lock()
	sink.currentDate = now.Add(-24 * time.Hour).Format(logFileDateLayout)
	sink.mu.Unlock()

	hookDone := make(chan struct{})
	var hookOnce sync.Once
	sink.SetRotateHook(func(prevDate time.Time, prevPath, newPath string) {
		logger.Printf("rotate hook for %s", prevDate.Format(time.RFC3339))
		hookOnce.Do(func() { close(hookDone) })
	})

	done := make(chan struct{})
	go func() {
		logger.Print("trigger rotation")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("logger.Print deadlocked during rotate hook logging")
	}
	select {
	case <-hookDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("rotate hook did not complete")
	}
}
