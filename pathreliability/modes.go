package pathreliability

// BucketClass selects which aggregation bucket family to update.
type BucketClass uint8

const (
	BucketNone BucketClass = iota
	BucketBaseline
	BucketNarrowband
)

// BucketForIngest maps a mode to its ingest bucket class.
// Modes not explicitly listed return BucketNone (no ingest).
func BucketForIngest(mode string) BucketClass {
	switch normalizeMode(mode) {
	case "FT8", "FT4":
		return BucketBaseline
	case "CW", "RTTY", "PSK":
		return BucketNarrowband
	default:
		return BucketNone
	}
}

// IsNarrowbandMode reports whether the mode should prefer narrowband buckets for display.
func IsNarrowbandMode(mode string) bool {
	switch normalizeMode(mode) {
	case "CW", "RTTY", "PSK":
		return true
	default:
		return false
	}
}

// IsVoiceMode reports whether the mode is a human voice spot (display-only).
func IsVoiceMode(mode string) bool {
	switch normalizeMode(mode) {
	case "USB", "LSB":
		return true
	default:
		return false
	}
}
