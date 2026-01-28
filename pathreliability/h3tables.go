package pathreliability

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

const (
	h3Res1Count = 842
	h3Res2Count = 5882
)

func h3TablePath(dir string, res int) string {
	return filepath.Join(dir, fmt.Sprintf("res%d.bin", res))
}

func loadH3Table(dir string, res int, wantCount int) ([]uint64, error) {
	path := h3TablePath(dir, res)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data)%8 != 0 {
		return nil, fmt.Errorf("h3 table %s has invalid length %d", path, len(data))
	}
	count := len(data) / 8
	if wantCount > 0 && count != wantCount {
		return nil, fmt.Errorf("h3 table %s has %d entries (want %d)", path, count, wantCount)
	}
	cells := make([]uint64, count)
	for i := 0; i < count; i++ {
		offset := i * 8
		cells[i] = binary.LittleEndian.Uint64(data[offset : offset+8])
	}
	return cells, nil
}

func buildMapperFromCells(res int, cells []uint64) (*H3Mapper, error) {
	if res < 0 || res > 15 {
		return nil, fmt.Errorf("h3map: invalid resolution %d", res)
	}
	m := &H3Mapper{
		Res:  res,
		ToID: make(map[uint64]CellID, len(cells)),
		ToH3: make(map[CellID]uint64, len(cells)),
	}
	for i, cell := range cells {
		id := CellID(i + 1) // 1-based; 0 reserved for invalid
		m.ToID[cell] = id
		m.ToH3[id] = cell
	}
	return m, nil
}
