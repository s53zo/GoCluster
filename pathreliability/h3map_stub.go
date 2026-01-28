//go:build windows || !cgo

package pathreliability

import "fmt"

// H3Mapper is a stub for platforms without h3-go support.
type H3Mapper struct {
	Res  int
	ToID map[uint64]CellID
	ToH3 map[CellID]uint64
}

var (
	fineMapper   *H3Mapper
	coarseMapper *H3Mapper
)

// InitH3Mappings returns an error when H3 is unavailable.
func InitH3Mappings() error {
	return fmt.Errorf("h3 mappings unavailable on this platform (windows or !cgo)")
}

// NewH3Mapper returns an error when H3 is unavailable.
func NewH3Mapper(res int) (*H3Mapper, error) {
	return nil, fmt.Errorf("h3 mappings unavailable on this platform (windows or !cgo)")
}

func encodeH3Cell(grid string, mapper *H3Mapper) CellID {
	return InvalidCell
}
