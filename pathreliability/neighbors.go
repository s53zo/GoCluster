package pathreliability

// BuildNeighborTable returns a map grid2 -> 4-way neighbors (N/S/E/W).
func BuildNeighborTable() map[string][]string {
	m := make(map[string][]string, fieldsTotal)
	for row := 0; row < fieldsAcross; row++ {
		for col := 0; col < fieldsAcross; col++ {
			field := string('A'+byte(col)) + string('A'+byte(row))
			var list []string
			if row > 0 {
				list = append(list, string('A'+byte(col))+string('A'+byte(row-1)))
			}
			if row+1 < fieldsAcross {
				list = append(list, string('A'+byte(col))+string('A'+byte(row+1)))
			}
			if col > 0 {
				list = append(list, string('A'+byte(col-1))+string('A'+byte(row)))
			}
			if col+1 < fieldsAcross {
				list = append(list, string('A'+byte(col+1))+string('A'+byte(row)))
			}
			m[field] = list
		}
	}
	return m
}
