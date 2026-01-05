package reputation

import "testing"

func TestStoreTouchAndTrim(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, 2, 2)

	rec, created, err := store.Touch("K1ABC", "64500", "US", "NA")
	if err != nil {
		t.Fatalf("touch failed: %v", err)
	}
	if !created {
		t.Fatalf("expected created record")
	}
	if len(rec.RecentASNs) != 1 || rec.RecentASNs[0] != "64500" {
		t.Fatalf("unexpected ASN history: %+v", rec.RecentASNs)
	}

	rec, created, err = store.Touch("K1ABC", "64501", "CA", "NA")
	if err != nil {
		t.Fatalf("touch failed: %v", err)
	}
	if created {
		t.Fatalf("expected existing record")
	}
	if len(rec.RecentASNs) != 2 || rec.RecentASNs[0] != "64501" || rec.RecentASNs[1] != "64500" {
		t.Fatalf("unexpected ASN order: %+v", rec.RecentASNs)
	}

	rec, _, err = store.Touch("K1ABC", "64502", "MX", "NA")
	if err != nil {
		t.Fatalf("touch failed: %v", err)
	}
	if len(rec.RecentASNs) != 2 || rec.RecentASNs[0] != "64502" || rec.RecentASNs[1] != "64501" {
		t.Fatalf("unexpected ASN trim: %+v", rec.RecentASNs)
	}
	if len(rec.RecentCountries) != 2 || rec.RecentCountries[0] != "MX" || rec.RecentCountries[1] != "CA" {
		t.Fatalf("unexpected country trim: %+v", rec.RecentCountries)
	}
}
