package pathreliability

const (
	fineResolution   = 2
	coarseResolution = 1
)

// CellID is the 16-bit proxy ID for an H3 cell at the configured resolution.
type CellID uint16

// InvalidCell represents an unknown or missing cell.
const InvalidCell CellID = 0

// EncodeCell returns the fine-resolution (res-2) CellID for a 4-6 char Maidenhead grid.
// It returns InvalidCell when the grid is malformed or mappings are unavailable.
func EncodeCell(grid string) CellID {
	return encodeH3Cell(grid, fineMapper)
}

// EncodeCoarseCell returns the coarse-resolution (res-1) CellID for a 4-6 char Maidenhead grid.
// It returns InvalidCell when the grid is malformed or mappings are unavailable.
func EncodeCoarseCell(grid string) CellID {
	return encodeH3Cell(grid, coarseMapper)
}
