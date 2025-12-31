package gridstore

import (
	"database/sql"
	"testing"
	"time"
)

func TestUpsertBatchMerge(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	later := now.Add(2 * time.Hour)
	expires := now.Add(24 * time.Hour)

	first := Record{
		Call:         "k1abc",
		IsKnown:      false,
		Grid:         sql.NullString{String: "aa00", Valid: true},
		Observations: 1,
		FirstSeen:    now,
		UpdatedAt:    now,
	}
	if err := store.UpsertBatch([]Record{first}); err != nil {
		t.Fatalf("upsert batch first: %v", err)
	}

	got := mustGet(t, store, "K1ABC")
	assertGrid(t, got, "AA00", true)
	assertKnown(t, got, false)
	assertObservations(t, got, 1)
	assertUnixTime(t, "first_seen", got.FirstSeen, now)
	assertUnixTime(t, "updated_at", got.UpdatedAt, now)

	second := Record{
		Call:         "K1ABC",
		IsKnown:      true,
		Grid:         sql.NullString{},
		Observations: 2,
		FirstSeen:    later,
		UpdatedAt:    later,
		ExpiresAt:    &expires,
	}
	if err := store.UpsertBatch([]Record{second}); err != nil {
		t.Fatalf("upsert batch second: %v", err)
	}

	merged := mustGet(t, store, "k1abc")
	assertGrid(t, merged, "AA00", true)
	assertKnown(t, merged, true)
	assertObservations(t, merged, 3)
	assertUnixTime(t, "first_seen", merged.FirstSeen, now)
	assertUnixTime(t, "updated_at", merged.UpdatedAt, later)
	if merged.ExpiresAt == nil {
		t.Fatalf("expected expires_at to be set")
	}
	assertUnixTime(t, "expires_at", *merged.ExpiresAt, expires)

	if count, err := store.Count(); err != nil {
		t.Fatalf("count failed: %v", err)
	} else if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}
}

func TestClearKnownFlags(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	now := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	recs := []Record{
		{Call: "K1AAA", IsKnown: true, Observations: 1, FirstSeen: now, UpdatedAt: now},
		{Call: "K1BBB", IsKnown: false, Observations: 1, FirstSeen: now, UpdatedAt: now},
	}
	if err := store.UpsertBatch(recs); err != nil {
		t.Fatalf("upsert batch: %v", err)
	}
	if err := store.ClearKnownFlags(); err != nil {
		t.Fatalf("clear known flags: %v", err)
	}

	assertKnown(t, mustGet(t, store, "K1AAA"), false)
	assertKnown(t, mustGet(t, store, "K1BBB"), false)
}

func TestPurgeOlderThan(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	old := time.Date(2024, 12, 31, 23, 59, 0, 0, time.UTC)
	fresh := old.Add(2 * time.Hour)

	recs := []Record{
		{Call: "K1OLD", Observations: 1, FirstSeen: old, UpdatedAt: old},
		{Call: "K1NEW", Observations: 1, FirstSeen: fresh, UpdatedAt: fresh},
	}
	if err := store.UpsertBatch(recs); err != nil {
		t.Fatalf("upsert batch: %v", err)
	}

	removed, err := store.PurgeOlderThan(old.Add(30 * time.Minute))
	if err != nil {
		t.Fatalf("purge older than: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}

	if rec, err := store.Get("K1OLD"); err != nil {
		t.Fatalf("get old: %v", err)
	} else if rec != nil {
		t.Fatalf("expected old record to be removed")
	}
	if rec, err := store.Get("K1NEW"); err != nil {
		t.Fatalf("get new: %v", err)
	} else if rec == nil {
		t.Fatalf("expected new record to remain")
	}
	if count, err := store.Count(); err != nil {
		t.Fatalf("count failed: %v", err)
	} else if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(t.TempDir(), 0)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return store
}

func mustGet(t *testing.T, store *Store, call string) *Record {
	t.Helper()
	rec, err := store.Get(call)
	if err != nil {
		t.Fatalf("get %s: %v", call, err)
	}
	if rec == nil {
		t.Fatalf("record %s not found", call)
	}
	return rec
}

func assertGrid(t *testing.T, rec *Record, grid string, valid bool) {
	t.Helper()
	if rec.Grid.Valid != valid {
		t.Fatalf("expected grid valid=%v, got %v", valid, rec.Grid.Valid)
	}
	if valid && rec.Grid.String != grid {
		t.Fatalf("expected grid %q, got %q", grid, rec.Grid.String)
	}
}

func assertKnown(t *testing.T, rec *Record, known bool) {
	t.Helper()
	if rec.IsKnown != known {
		t.Fatalf("expected known=%v, got %v", known, rec.IsKnown)
	}
}

func assertObservations(t *testing.T, rec *Record, expected int) {
	t.Helper()
	if rec.Observations != expected {
		t.Fatalf("expected observations=%d, got %d", expected, rec.Observations)
	}
}

func assertUnixTime(t *testing.T, field string, got time.Time, want time.Time) {
	t.Helper()
	if got.Unix() != want.Unix() {
		t.Fatalf("expected %s=%d, got %d", field, want.Unix(), got.Unix())
	}
}
