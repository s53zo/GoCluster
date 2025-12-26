package peer

// WWVEvent describes a parsed WWV/WWC frame.
type WWVEvent struct {
	Kind     string // PC23 or PC73
	Date     string
	Hour     string
	SFI      string
	A        string
	K        string
	Forecast string // PC23-only forecast field
	ExpK     string // PC73-only expected K
	R        string // PC73-only R value
	SA       string // PC73-only solar activity
	GMF      string // PC73-only geomagnetic field
	Aurora   string // PC73-only aurora flag
	Logger   string // Reporting callsign
	Origin   string // Originating node
	Extra    []string
	Hop      int
}
