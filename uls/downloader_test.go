package uls

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCleanupDownloadTemps(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	now := time.Now().UTC()
	cutoff := now.Add(-tempCleanupMinAge - time.Minute)

	for i := 0; i < tempCleanupMaxFiles+2; i++ {
		path := filepath.Join(dir, fmt.Sprintf("download-%d.tmp", i))
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
		if err := os.Chtimes(path, cutoff, cutoff); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}
	recentPath := filepath.Join(dir, "download-recent.tmp")
	if err := os.WriteFile(recentPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write recent file: %v", err)
	}

	cleanupDownloadTemps(dir, now)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	remainingOld := 0
	remainingRecent := false
	for _, entry := range entries {
		name := entry.Name()
		if name == "download-recent.tmp" {
			remainingRecent = true
			continue
		}
		if strings.HasPrefix(name, "download-") && strings.HasSuffix(name, ".tmp") {
			remainingOld++
		}
	}
	if !remainingRecent {
		t.Fatalf("expected recent temp file to remain")
	}
	if remainingOld != 2 {
		t.Fatalf("expected 2 old temp files to remain, got %d", remainingOld)
	}
}
