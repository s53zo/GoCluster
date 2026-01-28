package pathreliability

import "testing"

func TestPackKeyLayouts(t *testing.T) {
	fine := packKey(10, 20, 3)
	if fine == 0 {
		t.Fatalf("expected non-zero fine key")
	}
	if fine&0xFFFF != 0 {
		t.Fatalf("expected fine key to have low 16 bits zero")
	}
	coarse := packCoarseKey(5, 6, 7)
	if coarse == 0 {
		t.Fatalf("expected non-zero coarse key")
	}
	if coarse&0xFFFF == 0 {
		t.Fatalf("expected coarse key to have low 16 bits non-zero")
	}
}
