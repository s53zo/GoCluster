package pathreliability

// BucketClass selects which aggregation bucket family to update.
type BucketClass uint8

const (
	BucketNone BucketClass = iota
	BucketCombined
)

// BucketForIngest maps a mode to its ingest bucket class.
// Modes not explicitly listed return BucketNone (no ingest).
func BucketForIngest(mode string) BucketClass {
	switch normalizeMode(mode) {
	case "FT8", "FT4", "CW", "RTTY", "PSK", "WSPR":
		return BucketCombined
	default:
		return BucketNone
	}
}
