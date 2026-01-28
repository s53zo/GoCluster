package pathreliability

import "testing"

func TestH3MapperDeterministic(t *testing.T) {
	m1, err := NewH3Mapper(1)
	if err != nil {
		t.Skipf("NewH3Mapper unavailable: %v", err)
	}
	m2, err := NewH3Mapper(1)
	if err != nil {
		t.Fatalf("NewH3Mapper(1) repeat: %v", err)
	}
	checkStableIDs(t, m1, m2, []CellID{1, 100, 842})

	m3, err := NewH3Mapper(2)
	if err != nil {
		t.Fatalf("NewH3Mapper(2): %v", err)
	}
	m4, err := NewH3Mapper(2)
	if err != nil {
		t.Fatalf("NewH3Mapper(2) repeat: %v", err)
	}
	checkStableIDs(t, m3, m4, []CellID{1, 1000, 5882})
}

func checkStableIDs(t *testing.T, a, b *H3Mapper, ids []CellID) {
	t.Helper()
	for _, id := range ids {
		cell := a.ToH3[id]
		if cell == 0 {
			t.Fatalf("expected cell for id %d", id)
		}
		if got := b.ToID[cell]; got != id {
			t.Fatalf("expected id %d for cell %d, got %d", id, cell, got)
		}
	}
}
