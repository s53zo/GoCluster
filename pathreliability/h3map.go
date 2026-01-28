//go:build !windows && cgo

package pathreliability

import (
	"fmt"
	"sort"
	"sync"

	"github.com/uber/h3-go/v4"
)

// H3Mapper provides deterministic uint16 proxy IDs for a resolution's H3 cells.
type H3Mapper struct {
	Res  int
	ToID map[h3.Cell]CellID
	ToH3 map[CellID]h3.Cell
}

var (
	h3InitOnce   sync.Once
	h3InitErr    error
	fineMapper   *H3Mapper
	coarseMapper *H3Mapper
)

// InitH3Mappings builds deterministic H3 mappings for fine and coarse resolutions.
// It must be called at startup before any Encode* functions are used.
func InitH3Mappings() error {
	h3InitOnce.Do(func() {
		fineMapper, h3InitErr = NewH3Mapper(fineResolution)
		if h3InitErr != nil {
			return
		}
		coarseMapper, h3InitErr = NewH3Mapper(coarseResolution)
	})
	return h3InitErr
}

// NewH3Mapper returns a deterministic mapping for all cells at a resolution.
func NewH3Mapper(res int) (*H3Mapper, error) {
	if res < 0 || res > 15 {
		return nil, fmt.Errorf("h3map: invalid resolution %d", res)
	}
	res0, err := h3.Res0Cells()
	if err != nil {
		return nil, fmt.Errorf("h3map: res0 cells: %w", err)
	}
	var allCells []h3.Cell
	for _, base := range res0 {
		children, err := base.Children(res)
		if err != nil {
			return nil, fmt.Errorf("h3map: children for base cell %d: %w", base, err)
		}
		allCells = append(allCells, children...)
	}
	sort.Slice(allCells, func(i, j int) bool { return allCells[i] < allCells[j] })
	switch res {
	case 1:
		if len(allCells) != 842 {
			return nil, fmt.Errorf("h3map: unexpected res-1 cell count %d (want 842)", len(allCells))
		}
	case 2:
		if len(allCells) != 5882 {
			return nil, fmt.Errorf("h3map: unexpected res-2 cell count %d (want 5882)", len(allCells))
		}
	}
	m := &H3Mapper{
		Res:  res,
		ToID: make(map[h3.Cell]CellID, len(allCells)),
		ToH3: make(map[CellID]h3.Cell, len(allCells)),
	}
	for i, cell := range allCells {
		id := CellID(i + 1) // 1-based; 0 reserved for invalid
		m.ToID[cell] = id
		m.ToH3[id] = cell
	}
	return m, nil
}

func encodeH3Cell(grid string, mapper *H3Mapper) CellID {
	if mapper == nil {
		return InvalidCell
	}
	lat, lon, ok := gridCenterLatLon(grid)
	if !ok {
		return InvalidCell
	}
	cell, err := h3.LatLngToCell(h3.NewLatLng(lat, lon), mapper.Res)
	if err != nil {
		return InvalidCell
	}
	if id := mapper.ToID[cell]; id != 0 {
		return id
	}
	return InvalidCell
}
