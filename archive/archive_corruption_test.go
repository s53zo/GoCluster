package archive

import (
	"os"
	"path/filepath"
	"testing"

	"dxcluster/config"
)

// Purpose: Ensure corrupt archive files are deleted and recreated when enabled.
// Key aspects: Writes invalid bytes, enables auto-delete, and verifies schema exists.
// Upstream: go test.
// Downstream: NewWriter, checkArchiveIntegrity.
func TestAutoDeleteCorruptDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "archive.db")
	if err := os.WriteFile(dbPath, []byte("not a sqlite db"), 0o644); err != nil {
		t.Fatalf("write corrupt db: %v", err)
	}

	cfg := config.ArchiveConfig{
		Enabled:                 true,
		DBPath:                  dbPath,
		QueueSize:               10,
		BatchSize:               10,
		BatchIntervalMS:         1,
		CleanupIntervalSeconds:  60,
		CleanupBatchSize:        1,
		CleanupBatchYieldMS:     0,
		RetentionFTSeconds:      1,
		RetentionDefaultSeconds: 1,
		BusyTimeoutMS:           100,
		Synchronous:             "off",
		AutoDeleteCorruptDB:     true,
	}
	writer, err := NewWriter(cfg)
	if err != nil {
		t.Fatalf("NewWriter() error: %v", err)
	}
	defer writer.Stop()

	var name string
	if err := writer.db.QueryRow(`select name from sqlite_master where type='table' and name='spots'`).Scan(&name); err != nil {
		t.Fatalf("schema query failed: %v", err)
	}
	if name != "spots" {
		t.Fatalf("expected spots table, got %q", name)
	}
}
