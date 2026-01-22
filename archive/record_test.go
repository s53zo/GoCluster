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

func TestArchiveRecordPreservesDerivedGridFlags(t *testing.T) {
	s := spot.NewSpot("K1ABC", "W1XYZ", 14074.0, "FT8")
	s.DXMetadata.Grid = "FN20"
	s.DXMetadata.GridDerived = true
	s.DEMetadata.Grid = "EM10"
	s.DEMetadata.GridDerived = true

	raw := encodeRecord(s)
	rec, err := decodeRecord(raw)
	if err != nil {
		t.Fatalf("decodeRecord failed: %v", err)
	}
	if !rec.dxGridDerived || !rec.deGridDerived {
		t.Fatalf("expected derived grid flags to be set")
	}
	decoded, err := decodeSpot(time.Now().UTC().UnixNano(), raw)
	if err != nil {
		t.Fatalf("decodeSpot failed: %v", err)
	}
	if !decoded.DXMetadata.GridDerived || !decoded.DEMetadata.GridDerived {
		t.Fatalf("expected decoded spots to retain derived flags")
	}
}
