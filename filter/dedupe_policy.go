package filter

import "strings"

const (
	DedupePolicyFast = "FAST"
	DedupePolicyMed  = "MED"
	DedupePolicySlow = "SLOW"
)

// NormalizeDedupePolicy returns a supported policy label, defaulting to MED.
func NormalizeDedupePolicy(value string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(value))
	switch trimmed {
	case DedupePolicyFast, DedupePolicyMed, DedupePolicySlow:
		return trimmed
	default:
		return DedupePolicyMed
	}
}
