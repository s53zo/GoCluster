package gridstore

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckpointVerifySuccess(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, Options{
		CacheSizeBytes:        1 << 20,
		MemTableSizeBytes:     1 << 20,
		L0CompactionThreshold: 2,
		L0StopWritesThreshold: 4,
		WriteQueueDepth:       4,
	})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	now := time.Now().UTC()
	if err := store.Upsert(Record{
		Call:         "K1ABC",
		Grid:         sql.NullString{String: "FN20", Valid: true},
		Observations: 1,
		FirstSeen:    now,
		UpdatedAt:    now,
	}); err != nil {
		_ = store.Close()
		t.Fatalf("upsert: %v", err)
	}
	checkpointDir := filepath.Join(dir, "checkpoint")
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		_ = store.Close()
		t.Fatalf("checkpoint mkdir: %v", err)
	}
	dest := filepath.Join(checkpointDir, "test")
	if err := store.Checkpoint(dest); err != nil {
		_ = store.Close()
		t.Fatalf("checkpoint: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	stats, err := VerifyCheckpoint(context.Background(), dest, 5*time.Second)
	if err != nil {
		t.Fatalf("verify checkpoint: %v", err)
	}
	if stats.Records == 0 {
		t.Fatalf("expected records > 0")
	}
}

func TestStoreVerifySuccess(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, Options{
		CacheSizeBytes:        1 << 20,
		MemTableSizeBytes:     1 << 20,
		L0CompactionThreshold: 2,
		L0StopWritesThreshold: 4,
		WriteQueueDepth:       4,
	})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	now := time.Now().UTC()
	if err := store.Upsert(Record{
		Call:         "K1DEF",
		Grid:         sql.NullString{String: "FN21", Valid: true},
		Observations: 1,
		FirstSeen:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	stats, err := store.Verify(context.Background(), 5*time.Second)
	if err != nil {
		t.Fatalf("verify store: %v", err)
	}
	if stats.Records == 0 {
		t.Fatalf("expected records > 0")
	}
}

func TestCheckpointVerifyFailsOnMissingManifest(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, Options{
		CacheSizeBytes:        1 << 20,
		MemTableSizeBytes:     1 << 20,
		L0CompactionThreshold: 2,
		L0StopWritesThreshold: 4,
		WriteQueueDepth:       4,
	})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	checkpointDir := filepath.Join(dir, "checkpoint")
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		_ = store.Close()
		t.Fatalf("checkpoint mkdir: %v", err)
	}
	dest := filepath.Join(checkpointDir, "test")
	if err := store.Checkpoint(dest); err != nil {
		_ = store.Close()
		t.Fatalf("checkpoint: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	removed := false
	currentPath := filepath.Join(dest, "CURRENT")
	if err := os.Remove(currentPath); err == nil {
		removed = true
	} else {
		entries, err := os.ReadDir(dest)
		if err != nil {
			t.Fatalf("read checkpoint: %v", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				if err := os.Remove(filepath.Join(dest, entry.Name())); err != nil {
					t.Fatalf("remove checkpoint file: %v", err)
				}
				removed = true
				break
			}
		}
	}
	if !removed {
		t.Fatalf("no checkpoint file found to corrupt")
	}
	if _, err := VerifyCheckpoint(context.Background(), dest, 5*time.Second); err == nil {
		t.Fatalf("expected verification error for missing manifest")
	}
}
