//go:build windows || !cgo

package pathreliability

import (
	"fmt"
	"strings"
	"sync"

	h3light "github.com/ThingsIXFoundation/h3-light"
)

// H3Mapper is a table-backed mapper for platforms without h3-go support.
type H3Mapper struct {
	Res  int
	ToID map[uint64]CellID
	ToH3 map[CellID]uint64
}

var (
	h3InitOnce   sync.Once
	h3InitErr    error
	fineMapper   *H3Mapper
	coarseMapper *H3Mapper
)

// InitH3Mappings loads H3 mappings from the default table directory.
func InitH3Mappings() error {
	return InitH3MappingsFromDir("data/h3")
}

// InitH3MappingsFromDir loads H3 tables from dir. No generation is available.
func InitH3MappingsFromDir(dir string) error {
	h3InitOnce.Do(func() {
		trimmed := strings.TrimSpace(dir)
		if trimmed == "" {
			trimmed = "data/h3"
		}
		fineCells, err := loadH3Table(trimmed, fineResolution, h3Res2Count)
		if err != nil {
			h3InitErr = fmt.Errorf("h3map: load res-2 table: %w", err)
			return
		}
		coarseCells, err := loadH3Table(trimmed, coarseResolution, h3Res1Count)
		if err != nil {
			h3InitErr = fmt.Errorf("h3map: load res-1 table: %w", err)
			return
		}
		fineMapper, err = buildMapperFromCells(fineResolution, fineCells)
		if err != nil {
			h3InitErr = err
			return
		}
		coarseMapper, h3InitErr = buildMapperFromCells(coarseResolution, coarseCells)
	})
	return h3InitErr
}

// NewH3Mapper is unavailable without h3-go on this platform.
func NewH3Mapper(res int) (*H3Mapper, error) {
	return nil, fmt.Errorf("h3 mappings unavailable on this platform (windows or !cgo)")
}

func encodeH3Cell(grid string, mapper *H3Mapper) CellID {
	if mapper == nil {
		return InvalidCell
	}
	lat, lon, ok := gridCenterLatLon(grid)
	if !ok {
		return InvalidCell
	}
	cell := h3light.LatLonToCell(lat, lon, mapper.Res)
	if cell == 0 {
		return InvalidCell
	}
	if id := mapper.ToID[uint64(cell)]; id != 0 {
		return id
	}
	return InvalidCell
}
