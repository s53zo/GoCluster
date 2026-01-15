package pathreliability

import (
	"strings"
)

// packKey packs receiver cell, sender cell, and band into a uint64.
func packKey(receiver CellID, sender CellID, band uint16) uint64 {
	if receiver == InvalidCell || sender == InvalidCell {
		return 0
	}
	return (uint64(band) << 48) | (uint64(receiver&0xffff) << 32) | (uint64(sender&0xffff) << 16)
}

// packGrid2Key packs coarse grids and band into a uint64 using 5 bits per char (A-R -> 0-17).
func packGrid2Key(receiverGrid2, senderGrid2 string, band uint16) uint64 {
	r := encodeGrid2Token(receiverGrid2)
	s := encodeGrid2Token(senderGrid2)
	if r == 0 || s == 0 {
		return 0
	}
	return (uint64(band) << 32) | (uint64(r) << 16) | uint64(s)
}

func encodeGrid2Token(grid2 string) uint16 {
	g := strings.ToUpper(strings.TrimSpace(grid2))
	if len(g) < 2 {
		return 0
	}
	a, b := g[0], g[1]
	if a < 'A' || a > 'R' || b < 'A' || b > 'R' {
		return 0
	}
	fieldCol := uint16(a - 'A')
	fieldRow := uint16(b - 'A')
	return fieldRow*fieldsAcross + fieldCol + 1 // +1 so zero can mean "invalid"
}
