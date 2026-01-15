package pathreliability

import "testing"

func TestEncodeCell(t *testing.T) {
	cell := EncodeCell("FN31")
	if cell == InvalidCell {
		t.Fatalf("expected valid cell for FN31")
	}
	field, col, row, ok := DecodeCell(cell)
	if !ok {
		t.Fatalf("expected to decode cell")
	}
	if field != "FN" {
		t.Fatalf("expected field FN, got %s", field)
	}
	if col < 0 || col > 1 || row < 0 || row > 1 {
		t.Fatalf("unexpected col/row: %d/%d", col, row)
	}
}

func TestPackGrid2Key(t *testing.T) {
	k := packGrid2Key("FN", "JN", 1)
	if k == 0 {
		t.Fatalf("expected non-zero key")
	}
}
