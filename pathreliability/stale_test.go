package pathreliability

import (
	"testing"
	"time"
)

func TestStoreStaleAfterHalfLifeMultiplier(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DefaultHalfLifeSec = 10
	cfg.StaleAfterHalfLifeMultiplier = 5
	cfg.StaleAfterSeconds = 999
	store := NewStore(cfg, []string{"160m"})
	now := time.Now().UTC()

	store.Update(EncodeCell("FN31"), EncodeCell("FN32"), "", "", "160m", -5, 1.0, now)

	fine, _ := store.Lookup(EncodeCell("FN31"), EncodeCell("FN32"), "", "", "160m", now.Add(49*time.Second))
	if fine.Weight == 0 {
		t.Fatalf("expected sample before stale cutoff")
	}

	stale, _ := store.Lookup(EncodeCell("FN31"), EncodeCell("FN32"), "", "", "160m", now.Add(51*time.Second))
	if stale.Weight != 0 {
		t.Fatalf("expected sample to be stale")
	}
}
