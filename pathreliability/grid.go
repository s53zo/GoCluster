package pathreliability

import "strings"

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
