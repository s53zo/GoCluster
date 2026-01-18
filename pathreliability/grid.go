package pathreliability

import (
	"math"
	"strings"
)

const (
	fieldsAcross  = 18
	fieldsTotal   = fieldsAcross * fieldsAcross // 324
	cellsPerField = 4
	totalCells    = fieldsTotal * cellsPerField // 1296
)

// CellID encodes a custom 2x2 cell inside a 2-char Maidenhead field.
type CellID uint16

// InvalidCell represents an unknown or missing grid.
const InvalidCell CellID = CellID(0xffff)

// EncodeCell returns the CellID for a 4-character Maidenhead grid.
// Returns InvalidCell when the input is malformed.
func EncodeCell(grid4 string) CellID {
	g := strings.ToUpper(strings.TrimSpace(grid4))
	if len(g) < 4 {
		return InvalidCell
	}
	a, b := g[0], g[1]
	if a < 'A' || a > 'R' || b < 'A' || b > 'R' {
		return InvalidCell
	}
	fieldCol := int(a - 'A')
	fieldRow := int(b - 'A')
	fieldIdx := fieldRow*fieldsAcross + fieldCol
	if fieldIdx < 0 || fieldIdx >= fieldsTotal {
		return InvalidCell
	}
	colDigit := g[2]
	rowDigit := g[3]
	if colDigit < '0' || colDigit > '9' || rowDigit < '0' || rowDigit > '9' {
		return InvalidCell
	}
	col := int(colDigit-'0') / 5 // 0 or 1
	row := int(rowDigit-'0') / 5 // 0 or 1
	cellIdx := fieldIdx*cellsPerField + col*2 + row
	if cellIdx < 0 || cellIdx >= totalCells {
		return InvalidCell
	}
	return CellID(cellIdx)
}

// EncodeGrid2 returns the uppercase 2-char grid (field) or empty string when invalid.
func EncodeGrid2(grid string) string {
	g := strings.ToUpper(strings.TrimSpace(grid))
	if len(g) < 2 {
		return ""
	}
	a, b := g[0], g[1]
	if a < 'A' || a > 'R' || b < 'A' || b > 'R' {
		return ""
	}
	return g[:2]
}

// Grid2FromLatLon returns the 2-char Maidenhead field for a lat/lon pair.
// It returns empty string when coordinates are out of range or NaN.
// Purpose: Provide a coarse grid2 fallback when only CTY lat/lon is known.
// Key aspects: Uses standard Maidenhead field sizing (20 deg lon, 10 deg lat).
// Upstream: Telnet glyph inference for DXSpider-style deduping.
// Downstream: path reliability coarse lookup.
func Grid2FromLatLon(lat, lon float64) string {
	if math.IsNaN(lat) || math.IsNaN(lon) {
		return ""
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return ""
	}
	fieldLon := int(math.Floor((lon + 180.0) / 20.0))
	fieldLat := int(math.Floor((lat + 90.0) / 10.0))
	if fieldLon < 0 || fieldLon >= fieldsAcross || fieldLat < 0 || fieldLat >= fieldsAcross {
		return ""
	}
	return string('A'+byte(fieldLon)) + string('A'+byte(fieldLat))
}

// DecodeCell returns the 2-char field and local col/row for diagnostics.
func DecodeCell(cell CellID) (field string, col int, row int, ok bool) {
	if cell == InvalidCell || int(cell) >= totalCells {
		return "", 0, 0, false
	}
	fieldIdx := int(cell) / cellsPerField
	fieldRow := fieldIdx / fieldsAcross
	fieldCol := fieldIdx % fieldsAcross
	col = (int(cell) % cellsPerField) / 2
	row = int(cell) % 2
	field = string('A'+byte(fieldCol)) + string('A'+byte(fieldRow))
	return field, col, row, true
}
