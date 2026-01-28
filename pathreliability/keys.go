package pathreliability

// packKey packs receiver cell, sender cell, and band into a uint64.
func packKey(receiver CellID, sender CellID, band uint16) uint64 {
	if receiver == InvalidCell || sender == InvalidCell {
		return 0
	}
	return (uint64(band) << 48) | (uint64(receiver) << 32) | (uint64(sender) << 16)
}

// packCoarseKey packs coarse (res-1) cells and band into a uint64.
func packCoarseKey(receiver CellID, sender CellID, band uint16) uint64 {
	if receiver == InvalidCell || sender == InvalidCell {
		return 0
	}
	return (uint64(band) << 32) | (uint64(receiver) << 16) | uint64(sender)
}
