package archive

import (
	"testing"
	"time"

	"dxcluster/spot"
)

func TestArchiveRecordStoresStrippedDECall(t *testing.T) {
	s := spot.NewSpot("K1ABC", "W1XYZ-1", 14074.0, "FT8")
	s.DECallStripped = "W1XYZ"
	s.DECallNormStripped = "W1XYZ"

	raw := encodeRecord(s)
	rec, err := decodeRecord(raw)
	if err != nil {
		t.Fatalf("decodeRecord failed: %v", err)
	}
	if rec.deCall != "W1XYZ-1" {
		t.Fatalf("expected raw DE call, got %q", rec.deCall)
	}
	if rec.deCallStripped != "W1XYZ" {
		t.Fatalf("expected stripped DE call, got %q", rec.deCallStripped)
	}

	decoded, err := decodeSpot(time.Now().UTC().UnixNano(), raw)
	if err != nil {
		t.Fatalf("decodeSpot failed: %v", err)
	}
	if decoded.DECall != "W1XYZ-1" {
		t.Fatalf("expected decoded raw DE call, got %q", decoded.DECall)
	}
	if decoded.DECallStripped != "W1XYZ" {
		t.Fatalf("expected decoded stripped DE call, got %q", decoded.DECallStripped)
	}
}
