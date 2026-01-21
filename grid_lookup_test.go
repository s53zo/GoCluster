package main

import (
	"database/sql"
	"testing"
	"time"

	"dxcluster/gridstore"
)

func TestGridLookupSyncUsesBaseIdentity(t *testing.T) {
	dir := t.TempDir()
	store, err := gridstore.Open(dir, gridstore.Options{
		CacheSizeBytes:        1 << 20,
		MemTableSizeBytes:     1 << 20,
		L0CompactionThreshold: 2,
		L0StopWritesThreshold: 4,
		WriteQueueDepth:       4,
	})
	if err != nil {
		t.Fatalf("open gridstore: %v", err)
	}
	now := time.Now().UTC()
	if err := store.Upsert(gridstore.Record{
		Call:         "KW7MM-#",
		Grid:         sql.NullString{String: "EM12", Valid: true},
		Observations: 1,
		FirstSeen:    now,
		UpdatedAt:    now,
	}); err != nil {
		store.Close()
		t.Fatalf("upsert grid record: %v", err)
	}

	cache := newCallMetaCache(64, 0)
	_, _, _, stop, gridLookup, gridLookupSync := startGridWriter(store, 50*time.Millisecond, cache, 0, true)
	defer store.Close()
	if stop != nil {
		defer stop()
	}

	if gridLookupSync == nil {
		t.Fatalf("expected sync lookup to be enabled")
	}
	if gridLookup == nil {
		t.Fatalf("expected async lookup to be available")
	}

	grid, ok := gridLookupSync("KW7MM-#")
	if !ok || grid != "EM12" {
		t.Fatalf("gridLookupSync returned %q ok=%v, want EM12", grid, ok)
	}
	grid, ok = gridLookup("KW7MM")
	if !ok || grid != "EM12" {
		t.Fatalf("gridLookup returned %q ok=%v, want EM12", grid, ok)
	}
}
