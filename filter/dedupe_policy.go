package filter

import "strings"

const (
	DedupePolicyFast = "FAST"
	DedupePolicySlow = "SLOW"
)

// NormalizeDedupePolicy returns a supported policy label, defaulting to FAST.
func NormalizeDedupePolicy(value string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(value))
	switch trimmed {
	case DedupePolicyFast, DedupePolicySlow:
		return trimmed
	default:
		return DedupePolicyFast
	}
}
